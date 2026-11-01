package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// PushOptions is the orchestrating half of pre-push. It runs in the checkout, moves a private
// worktree to the pushed commit, and leaves the verdict to that commit's own runner.
type PushOptions struct {
	Root     string
	Bazel    string
	Env      []string
	From, To string
	// Base and BaseSource pass through to the pushed commit's runner. From selects inputs, and is
	// never a base.
	Base, BaseSource string
	Isolation        string
	Offline          bool
	CacheDir         string
	// Receipt is where the pushed commit's runner writes its evidence.
	Receipt  string
	Deadline time.Duration
	Stdout   io.Writer
}

func pushPaths(ctx context.Context, git Git, cache string) (worktree, outputBase, lock string, err error) {
	common, err := git.Run(ctx, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", "", err
	}
	dir := strings.TrimSpace(string(common))
	state := filepath.Join(dir, "bff-template")
	if err := os.MkdirAll(state, 0o755); err != nil {
		return "", "", "", err
	}
	// The output base stays outside the repository: its path holds no spaces of the checkout's.
	sum := sha256.Sum256([]byte(dir))
	outputBase = filepath.Join(cache, "push", hex.EncodeToString(sum[:8]))
	return filepath.Join(state, "push-worktree"), outputBase, filepath.Join(state, "push.lock"), nil
}

// prepareWorktree puts the detached worktree at the pushed commit, exactly: nothing left by an
// earlier push can reach this verdict.
func prepareWorktree(ctx context.Context, git Git, worktree, to string) error {
	if _, err := git.Run(ctx, nil, "worktree", "prune"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(worktree, ".git")); errors.Is(err, os.ErrNotExist) {
		_, err := git.Run(ctx, nil, "worktree", "add", "--quiet", "--detach", worktree, to)
		return err
	}
	inside := Git{Dir: worktree, Env: git.Env}
	if _, err := inside.Run(ctx, nil, "checkout", "--quiet", "--detach", "--force", to); err != nil {
		return err
	}
	_, err := inside.Run(ctx, nil, "clean", "-ffdxq")
	return err
}

// RunPush judges a pushed commit. Exclusive use of the output base comes from the worktree lock it
// holds, so this is the one place a server may be stopped, and only after a deadline.
func RunPush(ctx context.Context, options PushOptions) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, options.Deadline)
	defer cancel()
	git := Git{Dir: options.Root, Env: options.Env}
	worktree, outputBase, lockPath, err := pushPaths(ctx, git, options.CacheDir)
	if err != nil {
		return ExitConfig, configErrorf("push: %v", err)
	}
	lock, err := Acquire(ctx, []string{lockPath})
	if err != nil {
		_, _ = fmt.Fprintf(options.Stdout, "quality: another push holds the worktree: %v\n", err)
		return ExitInterrupted, nil
	}
	defer lock.Release()
	if err := prepareWorktree(ctx, git, worktree, options.To); err != nil {
		return ExitConfig, configErrorf("push: %v", err)
	}
	script := filepath.Join(filepath.Dir(lockPath), "push-runner.sh")
	startup := []string{"--output_base=" + outputBase, "--max_idle_secs=300"}
	build := append(append([]string{}, startup...), "run", "--script_path="+script)
	if options.Isolation == "weakened" {
		build = append(build, "--config=weak-isolation")
	}
	if options.Offline {
		build = append(build, "--repository_disable_download")
	}
	build = append(build, "--noshow_progress", "//:quality")
	pushed := filepath.Join(worktree, "tools", "bootstrap", "bazel.sh")
	if _, err := os.Stat(pushed); err != nil {
		return ExitConfig, configErrorf("bootstrap ERROR: the pushed commit %s has no pinned Bazel wrapper; no check ran", options.To)
	}
	log, err := os.Create(filepath.Join(filepath.Dir(lockPath), "push-bootstrap.log"))
	if err != nil {
		return ExitConfig, err
	}
	defer func() { _ = log.Close() }()
	code, runErr := RunBazel(ctx, pushed, build, worktree, options.Env, log)
	timedOut := func() (int, error) {
		if err := StopOwnedServer(outputBase); err != nil {
			_, _ = fmt.Fprintf(options.Stdout, "quality: %v\n", err)
		}
		_, _ = fmt.Fprintln(options.Stdout, "quality: pre-push deadline passed; the push is refused")
		return ExitInterrupted, nil
	}
	if runErr != nil {
		return timedOut()
	}
	if code != 0 {
		data, _ := os.ReadFile(log.Name())
		return ExitConfig, configErrorf("bootstrap ERROR: the pushed commit's runner did not build; no check ran\n%s", tail(string(data), 20))
	}
	args := []string{"--stage=pre-push", "--inner", "--to=" + options.To, "--isolation=" + options.Isolation}
	if options.From != "" {
		args = append(args, "--from="+options.From)
	}
	if options.Offline {
		args = append(args, "--offline")
	}
	if options.Base != "" {
		args = append(args, "--base="+options.Base, "--base-source="+options.BaseSource)
	}
	if options.Receipt != "" {
		args = append(args, "--receipt="+options.Receipt)
	}
	for _, option := range startup {
		args = append(args, "--bazel-startup="+option)
	}
	code, runErr = runInner(ctx, script, args, worktree, options.Env, options.Stdout)
	if runErr != nil {
		return timedOut()
	}
	return code, nil
}

func tail(text string, count int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

// PushInputs selects what a pushed commit changed. An unknown base cannot be narrowed.
func PushInputs(ctx context.Context, git Git, inputs *Inputs, from, to string) error {
	changed, err := ChangedPaths(ctx, git, from, to)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, path := range changed {
		keep[path] = true
	}
	var kept []string
	for _, path := range inputs.All {
		if keep[path] {
			kept = append(kept, path)
		}
	}
	inputs.All = kept
	return nil
}

// ChangedPaths lists what a push changes.
func ChangedPaths(ctx context.Context, git Git, from, to string) ([]string, error) {
	listed, err := git.Run(ctx, nil, "diff", "--name-only", "-z", "--no-renames", from, to)
	if err != nil {
		return nil, err
	}
	return Split(listed), nil
}

// Escalates reports whether a push must run the full policy: an unknown base, or a change to policy
// inputs. Pre-commit never asks this.
func Escalates(ctx context.Context, git Git, patterns []string, from, to string) (bool, error) {
	if from == "" || strings.Trim(from, "0") == "" {
		return true, nil
	}
	changed, err := ChangedPaths(ctx, git, from, to)
	if err != nil {
		return true, nil
	}
	for _, pattern := range patterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		if matchesAny2(changed, compiled) {
			return true, nil
		}
	}
	return false, nil
}

func matchesAny2(paths []string, pattern *regexp.Regexp) bool {
	for _, path := range paths {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}
