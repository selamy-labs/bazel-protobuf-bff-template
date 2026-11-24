package mutation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// All 11 gremlins mutators are always enabled; disabling mutators is forbidden (#10).
var allMutatorFlags = []string{
	"--arithmetic-base",
	"--conditionals-boundary",
	"--conditionals-negation",
	"--increment-decrement",
	"--invert-assignments",
	"--invert-bitwise",
	"--invert-bwassign",
	"--invert-logical",
	"--invert-loopctrl",
	"--invert-negatives",
	"--remove-self-assignments",
}

// Runner executes offline pristine tests and gremlins mutation runs against isolated Go mirrors.
type Runner struct {
	GremlinsBin string
	GoBin       string
	Timeout     time.Duration
}

// RunAll executes mutation testing across every package declared in floors.
func (r Runner) RunAll(root string, floors *Floors, eqFile *EquivalentsFile) (Summary, []string, error) {
	summary := Summary{MeasuredAt: floors.MeasuredAt}
	var problems []string
	for _, pkg := range floors.Packages {
		res, pkgProblems, err := r.RunPackage(root, pkg, floors.TimeoutCoefficient, eqFile)
		if err != nil {
			return summary, nil, err
		}
		summary.Packages++
		summary.Killed += res.Killed
		summary.Lived += res.Lived
		summary.Equivalent += res.Equivalent
		summary.TimedOut += res.TimedOut
		summary.NotCovered += res.NotCovered
		summary.NotViable += res.NotViable
		problems = append(problems, pkgProblems...)
	}
	return summary, problems, nil
}

// RunPackage materializes an offline module mirror, verifies pristine tests pass first,
// runs gremlins with all 11 mutators, and evaluates the resulting JSON report.
func (r Runner) RunPackage(root string, pkg PackageConfig, timeoutCoeff int, eqFile *EquivalentsFile) (PackageResult, []string, error) {
	work, err := os.MkdirTemp("", "mutation-*")
	if err != nil {
		return PackageResult{}, nil, err
	}
	defer os.RemoveAll(work)

	mirror := filepath.Join(work, "mod")
	if err := materializeMirror(root, mirror, pkg); err != nil {
		return PackageResult{}, nil, fmt.Errorf("%s: mirror setup: %w", pkg.Name, err)
	}
	env, err := buildOfflineEnv(work, r.GoBin)
	if err != nil {
		return PackageResult{}, nil, fmt.Errorf("%s: go sdk env: %w", pkg.Name, err)
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if out, err := runCommand(timeout, mirror, env, r.GoBin, "test", "-count=1", pkg.Package); err != nil {
		return PackageResult{}, nil, fmt.Errorf("%s: pristine go test failed (zero kill credit): %w\n%s", pkg.Name, err, trimTail(out))
	}

	reportPath := filepath.Join(work, "report.json")
	args := append([]string{"unleash"}, allMutatorFlags...)
	args = append(args, "--timeout-coefficient", strconv.Itoa(timeoutCoeff), "--output", reportPath, pkg.Package)
	if out, err := runCommand(timeout, mirror, env, r.GremlinsBin, args...); err != nil {
		return PackageResult{}, nil, fmt.Errorf("%s: gremlins failed: %w\n%s", pkg.Name, err, trimTail(out))
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return PackageResult{}, nil, fmt.Errorf("%s: read gremlins report: %w", pkg.Name, err)
	}
	report, err := ParseReport(data)
	if err != nil {
		return PackageResult{}, nil, fmt.Errorf("%s: %w", pkg.Name, err)
	}
	return EvaluateReport(pkg, report, eqFile)
}

func materializeMirror(root, mirror string, pkg PackageConfig) error {
	mappings, err := pkg.CanonicalMappings()
	if err != nil {
		return err
	}
	repoRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer repoRoot.Close()
	if err := os.MkdirAll(mirror, 0o700); err != nil {
		return err
	}
	goMod := fmt.Sprintf("module %s\n\ngo %s\n", pkg.Module, pkg.GoVersion)
	if err := os.WriteFile(filepath.Join(mirror, "go.mod"), []byte(goMod), 0o600); err != nil {
		return err
	}
	for _, mapping := range mappings {
		dstPath := filepath.Join(mirror, filepath.FromSlash(mapping.Dest))
		if err := copyContainedGoFiles(repoRoot, mapping.Source, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func verifyContainedDir(repoRoot *os.Root, relDir string) (os.FileInfo, error) {
	var lastInfo os.FileInfo
	prefix := ""
	for _, part := range strings.Split(relDir, "/") {
		if prefix == "" {
			prefix = part
		} else {
			prefix += "/" + part
		}
		info, err := repoRoot.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("source directory component %q must not be a symlink", prefix)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("source directory component %q is not a directory", prefix)
		}
		lastInfo = info
	}
	return lastInfo, nil
}

func copyContainedGoFiles(repoRoot *os.Root, srcRel, dstDir string) error {
	dirInfo, err := verifyContainedDir(repoRoot, srcRel)
	if err != nil {
		return err
	}
	dir, err := repoRoot.Open(srcRel)
	if err != nil {
		return err
	}
	openedInfo, statErr := dir.Stat()
	if statErr != nil {
		_ = dir.Close()
		return statErr
	}
	if !openedInfo.IsDir() || !os.SameFile(dirInfo, openedInfo) {
		_ = dir.Close()
		return fmt.Errorf("source directory %q changed during copy or is not a directory", srcRel)
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source file %q must not be a symlink", srcRel+"/"+entry.Name())
		}
		if entry.IsDir() {
			continue
		}
		fileRel := srcRel + "/" + entry.Name()
		if err := copyContainedFile(repoRoot, fileRel, filepath.Join(dstDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyContainedFile(repoRoot *os.Root, srcRel, dst string) error {
	info, err := repoRoot.Lstat(srcRel)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source file %q must not be a symlink", srcRel)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source file %q must be a regular file", srcRel)
	}
	in, err := repoRoot.Open(srcRel)
	if err != nil {
		return err
	}
	defer in.Close()
	openedInfo, err := in.Stat()
	if err != nil {
		return err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return fmt.Errorf("source file %q changed during copy or is not a regular file", srcRel)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func resolveGoRoot(goBin string) (string, string) {
	directDir := filepath.Dir(goBin)
	directRoot := filepath.Dir(directDir)
	if _, err := os.Stat(filepath.Join(directRoot, "pkg", "tool")); err == nil {
		return directDir, directRoot
	}
	resolvedGo, err := filepath.EvalSymlinks(goBin)
	if err != nil {
		return directDir, directRoot
	}
	resolvedDir := filepath.Dir(resolvedGo)
	return resolvedDir, filepath.Dir(resolvedDir)
}

func buildOfflineEnv(work, goBin string) ([]string, error) {
	goBinDir, goRoot := resolveGoRoot(goBin)
	homeDir := filepath.Join(work, "home")
	cacheDir := filepath.Join(work, "gocache")
	telemetryDir := filepath.Join(homeDir, ".config", "go", "telemetry")
	if err := os.MkdirAll(telemetryDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(telemetryDir, "mode"), []byte("off\n"), 0o600); err != nil {
		return nil, err
	}
	return []string{
		"PATH=" + goBinDir + ":/usr/bin:/bin",
		"GOROOT=" + goRoot,
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + filepath.Join(homeDir, ".config"),
		"GOCACHE=" + cacheDir,
		"GOPATH=" + filepath.Join(work, "gopath"),
		"GOPROXY=off",
		"GOTOOLCHAIN=local",
		"GOTELEMETRY=off",
		"TMPDIR=" + work,
	}, nil
}

func runCommand(timeout time.Duration, dir string, env []string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func trimTail(s string) string {
	const limit = 2000
	if len(s) <= limit {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(s[len(s)-limit:])
}
