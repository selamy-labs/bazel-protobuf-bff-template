package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// Git runs git in one directory with one environment.
type Git struct {
	Dir string
	Env []string
}

// Run returns stdout, and git's stderr in the error.
func (g Git) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = g.Dir
	command.Env = g.Env
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Split parses a NUL-delimited list, so a path may hold any byte but NUL.
func Split(data []byte) []string {
	var paths []string
	for item := range bytes.SplitSeq(data, []byte{0}) {
		if len(item) > 0 {
			paths = append(paths, string(item))
		}
	}
	return paths
}

// Inputs is what one stage may judge.
type Inputs struct {
	Root     string
	All      []string
	Deleted  []string
	Vendored map[string]bool
	Fixtures map[string]bool
	Counts map[string]int
}

func manifest(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries := map[string]bool{}
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\n")
		if len(line) < 67 {
			return nil, fmt.Errorf("%s: malformed line %q", path, line)
		}
		entries[line[66:]] = true
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	return entries, nil
}

// Discover lists the whole checkout. Only committed ignore rules apply, so local Git state such as
// .git/info/exclude cannot narrow what is judged.
func Discover(ctx context.Context, git Git, registry *registrypb.Registry) (*Inputs, error) {
	listed, err := git.Run(ctx, nil, "ls-files", "-z", "--cached", "--others", "--exclude-per-directory=.gitignore")
	if err != nil {
		return nil, err
	}
	paths := Split(listed)
	sort.Strings(paths)
	paths = compact(paths)
	inputs := &Inputs{Root: git.Dir}
	for _, path := range paths {
		if _, err := os.Lstat(filepath.Join(git.Dir, path)); err == nil {
			inputs.All = append(inputs.All, path)
		} else {
			inputs.Deleted = append(inputs.Deleted, path)
		}
	}
	if len(inputs.All) == 0 {
		return nil, fmt.Errorf("inventory: discovery found no files")
	}
	if err := inputs.loadManifests(registry); err != nil {
		return nil, err
	}
	for path := range inputs.Vendored {
		if info, err := os.Stat(filepath.Join(git.Dir, path)); err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("inventory: manifest entry is missing from the tree: %s", path)
		}
	}
	return inputs, nil
}

func compact(sorted []string) []string {
	unique := sorted[:0]
	for index, path := range sorted {
		if index == 0 || path != sorted[index-1] {
			unique = append(unique, path)
		}
	}
	return unique
}

func (i *Inputs) loadManifests(registry *registrypb.Registry) error {
	vendored, err := manifest(filepath.Join(i.Root, registry.GetVendoredManifest()))
	if err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	i.Vendored = vendored
	i.Fixtures = map[string]bool{}
	if fixtures, err := manifest(filepath.Join(i.Root, registry.GetFixtureManifest())); err == nil {
		i.Fixtures = fixtures
	} else if !os.IsNotExist(unwrapPath(err)) {
		return fmt.Errorf("inventory: %w", err)
	}
	return nil
}

func unwrapPath(err error) error {
	for current := err; current != nil; {
		if _, ok := current.(*os.PathError); ok {
			return current
		}
		next, ok := current.(interface{ Unwrap() error })
		if !ok {
			break
		}
		current = next.Unwrap()
	}
	return err
}

var shebang = regexp.MustCompile(`^#!.*[/ ](bash|dash|ksh|zsh|sh)(\s|$)`)

// firstLine reads at most one line, so a large file costs nothing.
func firstLine(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	buffer := make([]byte, 256)
	count, _ := file.Read(buffer)
	line, _, _ := bytes.Cut(buffer[:count], []byte{'\n'})
	return string(line)
}

// Select returns the paths one check judges.
func Select(check *registrypb.Check, inputs *Inputs) []string {
	selector := check.GetSelector()
	patterns := make([]*regexp.Regexp, 0, len(selector.GetInclude()))
	for _, pattern := range selector.GetInclude() {
		patterns = append(patterns, regexp.MustCompile(pattern))
	}
	var selected []string
	for _, path := range inputs.All {
		if inputs.Vendored[path] && !selector.GetIncludeVendored() {
			continue
		}
		if inputs.Fixtures[path] && !selector.GetIncludeFixtures() {
			continue
		}
		if matches(path, patterns, selector.GetShellScripts(), inputs.Root) {
			selected = append(selected, path)
		}
	}
	return selected
}

func matches(path string, patterns []*regexp.Regexp, shell bool, root string) bool {
	if len(patterns) == 0 && !shell {
		return true
	}
	for _, pattern := range patterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	if !shell {
		return false
	}
	full := filepath.Join(root, path)
	info, err := os.Stat(full)
	return err == nil && info.Mode().IsRegular() && shebang.MatchString(firstLine(full))
}

// policyPaths is what the receipt's policy digest covers.
var policyPaths = []string{
	".bazelrc", ".bazelversion", "BUILD.bazel", "MODULE.bazel", "MODULE.bazel.lock", "REPO.bazel",
	".gitleaks.toml", "_typos.toml", "prek.toml", "quality", "tools", ".github",
}

// TestedTree returns the Git tree of exactly what was found on disk, and the policy digest, written
// to a scratch object store so measuring writes nothing into the repository.
func TestedTree(ctx context.Context, git Git, inputs *Inputs, scratch string) (tree, digest string, err error) {
	objects, err := git.Run(ctx, nil, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return "", "", err
	}
	store := filepath.Join(scratch, "objects")
	if err := os.MkdirAll(store, 0o755); err != nil {
		return "", "", err
	}
	private := git
	private.Env = append(append([]string{}, git.Env...),
		"GIT_INDEX_FILE="+filepath.Join(scratch, "index"),
		"GIT_OBJECT_DIRECTORY="+store,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES="+strings.TrimSpace(string(objects)))
	list := []byte(strings.Join(inputs.All, "\x00") + "\x00")
	if _, err := private.Run(ctx, list, "-c", "core.fsmonitor=false", "update-index", "--add", "-z", "--stdin"); err != nil {
		return "", "", err
	}
	written, err := private.Run(ctx, nil, "write-tree")
	if err != nil {
		return "", "", err
	}
	tree = strings.TrimSpace(string(written))
	listing, err := private.Run(ctx, nil, append([]string{"ls-tree", "-r", tree, "--"}, policyPaths...)...)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(listing)
	return tree, hex.EncodeToString(sum[:]), nil
}
