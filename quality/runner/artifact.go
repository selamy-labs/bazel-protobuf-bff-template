package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// ArtifactOptions selects content for transport, not evidence validation or a quality verdict.
type ArtifactOptions struct {
	Registry    *registrypb.Registry
	Root        string
	ReceiptPath string
}

type artifactFile struct {
	name string
	data []byte
}

// ExportReceipt creates a fresh private directory. Only this receipt and its explicitly named,
// bounded, marked regular logs are copied. It never traverses a directory glob or edits the source.
func ExportReceipt(ctx context.Context, options ArtifactOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	workspace, err := filepath.Abs(options.Root)
	if err != nil {
		return "", err
	}
	receipt, err := artifactAbsolute(workspace, options.ReceiptPath)
	if err != nil {
		return "", err
	}
	files, err := artifactInputs(options.Registry, workspace, receipt)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return writeArtifact(filepath.Dir(receipt), files)
}

func artifactAbsolute(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Abs(path)
}

func artifactInputs(registry *registrypb.Registry, workspace, receipt string) ([]artifactFile, error) {
	root, err := artifactReceiptRoot(workspace, receipt)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := artifactRead(root, filepath.Base(receipt), maxRetainedLog)
	if err != nil {
		return nil, err
	}
	var record Receipt
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	logs, err := artifactLogs(root, registry, workspace, receipt, record.Checks)
	if err != nil {
		return nil, err
	}
	return append([]artifactFile{{name: filepath.Base(receipt), data: data}}, logs...), nil
}

func artifactReceiptRoot(workspace, receipt string) (*os.Root, error) {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent, err := filepath.Rel(workspace, filepath.Dir(receipt))
	if err != nil {
		return nil, err
	}
	info, err := root.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact receipt parent is not a regular directory")
	}
	directory, err := root.OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	actual, err := directory.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		_ = directory.Close()
		return nil, fmt.Errorf("artifact receipt parent changed while opening")
	}
	return directory, nil
}

func artifactLogs(root *os.Root, registry *registrypb.Registry, workspace, receipt string, checks []Result) ([]artifactFile, error) {
	known := make(map[string]bool)
	for _, check := range registry.GetChecks() {
		known[check.GetId()] = true
	}
	if len(checks) > len(known) {
		return nil, fmt.Errorf("artifact checks exceed the registry bound")
	}
	seen := make(map[string]bool)
	var files []artifactFile
	var directory string
	var run *os.Root
	for _, check := range checks {
		if check.Log == "" {
			continue
		}
		if !known[check.Check] || seen[check.Check] {
			return nil, fmt.Errorf("artifact log has an unknown or duplicate check")
		}
		seen[check.Check] = true
		name, err := artifactLogName(workspace, receipt, check)
		if err != nil {
			return nil, err
		}
		parent := filepath.Dir(name)
		if directory == "" {
			directory = parent
			run, err = artifactRun(root, parent)
			if err != nil {
				return nil, err
			}
			defer run.Close()
		}
		if directory != parent {
			return nil, fmt.Errorf("artifact references multiple log runs")
		}
		limit := maxRetainedLog + int64(len(fmt.Sprintf("\n[truncated at %d bytes; rerun the check for the rest]\n", maxRetainedLog)))
		data, err := artifactRead(run, filepath.Base(name), limit)
		if err != nil {
			return nil, err
		}
		files = append(files, artifactFile{name: name, data: data})
	}
	return files, nil
}

func artifactLogName(workspace, receipt string, check Result) (string, error) {
	absolute, err := artifactAbsolute(workspace, check.Log)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(filepath.Dir(receipt), absolute)
	if err != nil {
		return "", err
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 2 || !strings.HasPrefix(parts[0], logPrefix(receipt)) || parts[1] != check.Check+".log" {
		return "", fmt.Errorf("artifact log is not this receipt's direct check filename")
	}
	return relative, nil
}

func artifactRun(root *os.Root, name string) (*os.Root, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("artifact log run is not a regular directory")
	}
	run, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := run.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = run.Close()
		return nil, fmt.Errorf("artifact log run changed while opening")
	}
	if _, err := artifactRead(run, logOwnerMark, 0); err != nil {
		_ = run.Close()
		return nil, fmt.Errorf("artifact log run is unmarked: %w", err)
	}
	return run, nil
}

func artifactRead(root *os.Root, name string, limit int64) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, fmt.Errorf("artifact file is not regular or exceeds its bound: %s", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, fmt.Errorf("artifact file changed while opening: %s", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("artifact file exceeds its bound: %s", name)
	}
	return data, nil
}

func writeArtifact(parent string, files []artifactFile) (directory string, err error) {
	directory, err = os.MkdirTemp(parent, ".quality-artifact-")
	if err != nil {
		return "", err
	}
	created := directory
	defer func() {
		if err != nil {
			_ = os.RemoveAll(created)
		}
	}()
	for _, file := range files {
		path := filepath.Join(directory, file.name)
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		if err = os.WriteFile(path, file.data, 0o600); err != nil {
			return "", err
		}
	}
	return directory, nil
}
