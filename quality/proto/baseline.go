// Package baseline reads the schema compatibility baseline from Git objects (specs/003-notes-contract
// FR-006, FR-007). The baseline is a declared full commit hash, so nothing a working tree or a moved
// branch does can change what a schema is compared with. Anything that stops the comparison from
// happening is an error: there is no path that passes without comparing, except a bootstrap marker
// on a schema with no history.
package baseline

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// BootstrapMarker is the marker value for a schema that has no earlier version.
const BootstrapMarker = "bootstrap"

// ErrUnavailable means no comparison is possible. Callers must fail closed.
var ErrUnavailable = errors.New("baseline unavailable")

var fullHash = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// Result says what the schema should be compared with.
type Result struct {
	// Bootstrap is true when the marker allows a first schema; nothing was written.
	Bootstrap bool
	// Revision is the commit that was read.
	Revision string
	// Files is the number of schema files written.
	Files int
}

// Reader reads one repository's baseline.
type Reader struct {
	// Repo is the repository root.
	Repo string
	// Marker is the path of the marker file, relative to Repo.
	Marker string
	// Schema is the schema directory, relative to Repo.
	Schema string
}

func unavailable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnavailable, fmt.Sprintf(format, args...))
}

func (r Reader) git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", r.Repo}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Materialize writes the baseline's schema files under dir, or reports a valid bootstrap.
func (r Reader) Materialize(dir string) (Result, error) {
	raw, err := os.ReadFile(filepath.Join(r.Repo, r.Marker))
	if err != nil {
		return Result{}, unavailable("%s cannot be read: %v", r.Marker, err)
	}
	marker := strings.TrimSuffix(string(raw), "\n")
	shallow, err := r.git("rev-parse", "--is-shallow-repository")
	if err != nil {
		return Result{}, unavailable("%v", err)
	}
	if strings.TrimSpace(string(shallow)) == "true" {
		return Result{}, unavailable("this clone is shallow; run `git fetch --unshallow`")
	}
	if marker == BootstrapMarker {
		return Result{Bootstrap: true}, r.checkBootstrap()
	}
	if !fullHash.MatchString(marker) {
		return Result{}, unavailable("%s must hold a full lowercase commit hash or %q, not %q", r.Marker, BootstrapMarker, marker)
	}
	if _, err := r.git("cat-file", "-e", marker+"^{commit}"); err != nil {
		return Result{}, unavailable("commit %s is not in this clone; fetch it", marker)
	}
	if _, err := r.git("merge-base", "--is-ancestor", marker, "HEAD"); err != nil {
		return Result{}, unavailable("commit %s is not an ancestor of HEAD", marker)
	}
	files, err := r.extract(marker, dir)
	if err != nil {
		return Result{}, err
	}
	return Result{Revision: marker, Files: files}, nil
}

// checkBootstrap allows the marker only in the commit that introduces the schema: once an earlier
// commit has a schema, the marker would turn the compatibility check off.
func (r Reader) checkBootstrap() error {
	if _, err := r.git("rev-parse", "--quiet", "--verify", "HEAD"); err != nil {
		return nil // No commit yet, so no history can exist.
	}
	line, err := r.git("rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		return unavailable("%v", err)
	}
	parents := strings.Fields(string(line))[1:]
	if len(parents) == 0 {
		return nil
	}
	args := append([]string{"log", "-1", "--format=%H"}, parents...)
	args = append(args, "--", ":(glob)"+r.Schema+"/**/*.proto")
	previous, err := r.git(args...)
	if err != nil {
		return unavailable("%v", err)
	}
	if commit := strings.TrimSpace(string(previous)); commit != "" {
		return unavailable("the bootstrap marker is refused: commit %s already has a schema; set %s to a commit hash", commit, r.Marker)
	}
	return nil
}

func (r Reader) extract(commit, dir string) (int, error) {
	listing, err := r.git("ls-tree", "-r", "-z", commit, "--", r.Schema+"/")
	if err != nil {
		return 0, unavailable("%v", err)
	}
	files := 0
	for _, entry := range bytes.Split(listing, []byte{0}) {
		meta, path, found := strings.Cut(string(entry), "\t")
		if !found || !strings.HasSuffix(path, ".proto") {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 || fields[1] != "blob" {
			return 0, unavailable("%s in %s is not a regular file", path, commit)
		}
		blob, err := r.git("cat-file", "blob", fields[2])
		if err != nil {
			return 0, unavailable("%v", err)
		}
		relative, err := filepath.Rel(r.Schema, path)
		if err != nil || !filepath.IsLocal(relative) {
			return 0, unavailable("%s is outside %s", path, r.Schema)
		}
		target := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(target, blob, 0o644); err != nil {
			return 0, err
		}
		files++
	}
	if files == 0 {
		return 0, unavailable("commit %s has no schema under %s", commit, r.Schema)
	}
	return files, nil
}
