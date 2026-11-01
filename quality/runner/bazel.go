package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// clientGrace is how long an interrupted Bazel client gets before it is killed.
var clientGrace = 10 * time.Second

// serverGrace is how long a proved, owned server gets between SIGTERM and SIGKILL.
const serverGrace = 5 * time.Second

var testLine = regexp.MustCompile(`^(\S+)\s+(?:\(cached\)\s+)?(PASSED|FAILED|TIMEOUT|FLAKY|NO STATUS|SKIPPED|FAILED TO BUILD)\b`)

// TargetStates parses `bazel test` summary lines.
func TargetStates(log string) map[string]string {
	states := map[string]string{}
	for line := range strings.Lines(log) {
		if match := testLine.FindStringSubmatch(line); match != nil {
			states[match[1]] = match[2]
		}
	}
	return states
}

// matchesTarget reports whether a label falls under a pattern.
func matchesTarget(label, pattern string) bool {
	prefix, recursive := strings.CutSuffix(pattern, "...")
	if !recursive {
		return label == pattern
	}
	if strings.HasSuffix(prefix, "//") {
		return strings.HasPrefix(label, prefix)
	}
	return strings.HasPrefix(label, prefix) || strings.HasPrefix(label, strings.TrimSuffix(prefix, "/")+":")
}

// BazelPlan is the stage's one Bazel invocation.
type BazelPlan struct {
	Checks  []*registrypb.Check
	Args    []string
	Timeout time.Duration
}

// PlanBazel composes every Bazel check of a stage or leg into one invocation. Targets of checks that
// do not run here but whose scope another check would reach are subtracted, which keeps browser tests
// out of pre-push with no second list. A pattern that contains a selected target is not subtracted,
// because it would remove what was selected.
func PlanBazel(registry *registrypb.Registry, stage registrypb.Stage, shard string, flags []string) *BazelPlan {
	plan := &BazelPlan{}
	var targets, excluded []string
	verb := "test"
	for _, check := range registry.GetChecks() {
		if check.GetNeedsCoverage() && Selected(registry, check, stage, shard) {
			verb = "coverage"
		}
		if check.GetKind() != registrypb.Kind_KIND_BAZEL {
			continue
		}
		if !Selected(registry, check, stage, shard) {
			excluded = append(excluded, check.GetTargets()...)
			continue
		}
		plan.Checks = append(plan.Checks, check)
		plan.Timeout = max(plan.Timeout, time.Duration(check.GetTimeoutSeconds())*time.Second)
		for _, target := range check.GetTargets() {
			if !slices.Contains(targets, target) {
				targets = append(targets, target)
			}
		}
	}
	if len(plan.Checks) == 0 {
		return nil
	}
	plan.Args = append(append([]string{verb}, flags...), "--keep_going", "--noshow_progress", "--")
	plan.Args = append(plan.Args, targets...)
	for _, target := range excluded {
		if !slices.ContainsFunc(targets, func(selected string) bool { return matchesTarget(selected, target) }) {
			plan.Args = append(plan.Args, "-"+target)
		}
	}
	return plan
}

// TranslateBazel maps Bazel's exit code to one check's result (contracts/check-registry.md). The exit
// code and the per-target summary must agree: neither alone can pass a check.
func TranslateBazel(check *registrypb.Check, stage registrypb.Stage, code int, states map[string]string) Result {
	ran, bad, missing := 0, []string{}, []string{}
	for label, state := range states {
		for _, pattern := range check.GetTargets() {
			if matchesTarget(label, pattern) {
				ran++
				if state != "PASSED" {
					bad = append(bad, label+" "+state)
				}
				break
			}
		}
	}
	for _, pattern := range check.GetTargets() {
		if _, known := states[pattern]; !known && !strings.HasSuffix(pattern, "...") {
			missing = append(missing, pattern)
		}
	}
	slices.Sort(bad)
	slices.Sort(missing)
	switch code {
	case 0, 3:
		switch {
		case len(missing) > 0:
			return Result{State: Errored, Reason: "never ran: " + strings.Join(missing, ", ")}
		case ran == 0:
			return Result{State: Errored, Reason: "Bazel ran none of this check's targets"}
		case len(bad) > 0 && code == 0:
			return Result{State: Errored, Reason: "Bazel exited 0 beside " + strings.Join(bad, ", ")}
		case len(bad) > 0:
			return Result{State: Failed, Reason: strings.Join(bad, ", ")}
		}
		return Result{State: Passed}
	case 4:
		// An affected set may be empty. The full gate's may not.
		if stage == registrypb.Stage_STAGE_FULL {
			return Result{State: Errored, EmptyInput: true, Reason: "Bazel found no tests at full"}
		}
		return Result{State: NotApplicable, Reason: "no tests to run"}
	case 8:
		return Result{State: Cancelled, Reason: "Bazel was interrupted"}
	default:
		return Result{State: Errored, Reason: fmt.Sprintf("Bazel exited %d", code)}
	}
}

// RunBazel runs one Bazel client to its end. On ctx's end it interrupts that client once, because
// repeated interrupts ask Bazel to kill its server, and kills it after clientGrace. It signals only
// the client it started, through the process handle it holds. It never touches the server.
func RunBazel(ctx context.Context, bazel string, args []string, dir string, env []string, log *os.File) (int, error) {
	command := exec.Command(bazel, args...)
	command.Dir, command.Env, command.Stdout, command.Stderr = dir, env, log, log
	if err := command.Start(); err != nil {
		return 0, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return exitCode(err), nil
	case <-ctx.Done():
	}
	_ = command.Process.Signal(syscall.SIGINT)
	select {
	case err := <-done:
		return exitCode(err), ctx.Err()
	case <-time.After(clientGrace):
	}
	_ = command.Process.Kill()
	return exitCode(<-done), ctx.Err()
}

// StopOwnedServer stops the Bazel server of an output base this run holds exclusively. It holds a
// process handle before it reads the process's identity, so a reused PID cannot be signaled, and
// requires the exact --output_base argument. Anything else is left alone and reported. Callers must
// hold the output base's exclusive lock: no check here can establish that (spec 006 R7).
func StopOwnedServer(outputBase string) error {
	data, err := os.ReadFile(filepath.Join(outputBase, "server", "server.pid.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("server pid file: %w", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	defer func() { _ = process.Release() }()
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return fmt.Errorf("server %d: identity unavailable, left running: %w", pid, err)
	}
	if !slices.Contains(strings.Split(string(cmdline), "\x00"), "--output_base="+outputBase) {
		return fmt.Errorf("process %d is not this output base's server, left running", pid)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return nil
	}
	for end := time.Now().Add(serverGrace); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if process.Signal(syscall.Signal(0)) != nil {
			return nil
		}
	}
	return process.Kill()
}

// summaryLines keeps what a reader needs from a Bazel log.
func summaryLines(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	var kept []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ERROR") || strings.HasPrefix(line, "FAIL") ||
			testLine.MatchString(line) && !strings.Contains(line, "PASSED") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// runInner runs the pushed commit's runner, passing its output on. On the deadline it is asked to
// stop, as a Bazel client would be, and it stops its own client and native checks.
func runInner(ctx context.Context, script string, args []string, dir string, env []string, out io.Writer) (int, error) {
	command := exec.Command(script, args...)
	command.Dir, command.Env, command.Stdout, command.Stderr = dir, env, out, out
	if err := command.Start(); err != nil {
		return 0, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return exitCode(err), nil
	case <-ctx.Done():
	}
	_ = command.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(clientGrace + 5*time.Second):
		_ = command.Process.Kill()
		<-done
	}
	return ExitInterrupted, ctx.Err()
}
