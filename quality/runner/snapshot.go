package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

type indexEntry struct{ mode, object string }

// readIndex lists the index Git named. Conflicted entries cannot be committed, so they are refused.
func readIndex(ctx context.Context, git Git) (map[string]indexEntry, error) {
	listed, err := git.Run(ctx, nil, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	entries := map[string]indexEntry{}
	for _, line := range Split(listed) {
		meta, path, found := strings.Cut(line, "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) != 3 {
			return nil, fmt.Errorf("unexpected index line %q", line)
		}
		if fields[2] != "0" {
			return nil, fmt.Errorf("%s is unmerged", path)
		}
		entries[path] = indexEntry{mode: fields[0], object: fields[1]}
	}
	return entries, nil
}

// StagedPaths lists what the commit changes. Rename detection is off, so a rename is a deletion and
// an addition, and the new path is judged.
func StagedPaths(ctx context.Context, git Git) ([]string, error) {
	listed, err := git.Run(ctx, nil, "diff", "--cached", "--name-only", "-z", "--no-renames", "--diff-filter=ACMRD")
	if err != nil {
		return nil, err
	}
	return Split(listed), nil
}

// Snapshot writes staged blobs into scratch as raw bytes. Nothing touches the index or the working
// tree, and no filter runs, so what is judged is what the commit will hold. A symlink becomes a file
// holding its target; a submodule has no content here.
func Snapshot(ctx context.Context, git Git, registry *registrypb.Registry, scratch string) (*Inputs, error) {
	changed, err := StagedPaths(ctx, git)
	if err != nil {
		return nil, err
	}
	index, err := readIndex(ctx, git)
	if err != nil {
		return nil, err
	}
	inputs := &Inputs{Root: scratch}
	wanted := map[string]indexEntry{}
	for _, path := range changed {
		entry, present := index[path]
		switch {
		case !present:
			inputs.Deleted = append(inputs.Deleted, path)
		case entry.mode == "160000":
		default:
			wanted[path] = entry
			inputs.All = append(inputs.All, path)
		}
	}
	if len(inputs.All) == 0 {
		return inputs, nil
	}
	patterns := contextPatterns(registry)
	for path, entry := range index {
		if entry.mode == "160000" {
			continue
		}
		if path == registry.GetVendoredManifest() || path == registry.GetFixtureManifest() || matchesAny(path, patterns) {
			wanted[path] = entry
		}
	}
	if err := writeBlobs(ctx, git, wanted, scratch); err != nil {
		return nil, err
	}
	if err := inputs.loadManifests(registry); err != nil {
		return nil, err
	}
	return inputs, nil
}

func contextPatterns(registry *registrypb.Registry) []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, check := range registry.GetChecks() {
		for _, pattern := range check.GetSelector().GetContext() {
			patterns = append(patterns, regexp.MustCompile(pattern))
		}
	}
	return patterns
}

func matchesAny(path string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

func writeBlobs(ctx context.Context, git Git, wanted map[string]indexEntry, scratch string) error {
	paths := make([]string, 0, len(wanted))
	var requests bytes.Buffer
	for path, entry := range wanted {
		paths = append(paths, path)
		requests.WriteString(entry.object + "\n")
	}
	command := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	command.Dir, command.Env = git.Dir, git.Env
	command.Stdin = &requests
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	reader := bufio.NewReader(stdout)
	for _, path := range paths {
		if err := writeBlob(reader, filepath.Join(scratch, path), wanted[path].mode); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return command.Wait()
}

func writeBlob(reader *bufio.Reader, target, mode string) error {
	header, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	fields := strings.Fields(header)
	if len(fields) != 3 || fields[1] != "blob" {
		return fmt.Errorf("unexpected object %q", strings.TrimSpace(header))
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	permissions := os.FileMode(0o644)
	if mode == "100755" {
		permissions = 0o755
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, permissions)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(file, reader, size); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = reader.Discard(1)
	return err
}
