package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// Options is one stage run.
type Options struct {
	Registry    *registrypb.Registry
	Stage       registrypb.Stage
	Requested   registrypb.Stage
	Root        string
	Adapter     string
	Bazel       string
	Tools       map[string]string
	Env         []string
	Isolation   string
	Offline     bool
	ReceiptPath string
	Rerun       string
	Jobs        int
	Deadline    time.Duration
	Slots       []string
	Inputs      func(ctx context.Context, git Git, scratch string) (*Inputs, error)
	Stdout      io.Writer
}

// StageNames maps the command line's stage names.
var StageNames = map[string]registrypb.Stage{
	"pre-commit": registrypb.Stage_STAGE_PRE_COMMIT,
	"commit-msg": registrypb.Stage_STAGE_COMMIT_MSG,
	"pre-push":   registrypb.Stage_STAGE_PRE_PUSH,
	"full":       registrypb.Stage_STAGE_FULL,
}

// StageName is the inverse of StageNames.
func StageName(stage registrypb.Stage) string {
	for name, value := range StageNames {
		if value == stage {
			return name
		}
	}
	return "unknown"
}

// DefaultJobs is the contract's pool bound.
func DefaultJobs() int { return max(1, min(4, runtime.NumCPU()/2)) }

type planned struct {
	check  *registrypb.Check
	inputs []string
}

// Run executes one stage. Anything that prevents judging returns a ConfigError.
func Run(ctx context.Context, options Options) (int, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, options.Deadline)
	defer cancel()
	scratch, err := os.MkdirTemp("", "quality-")
	if err != nil {
		return ExitConfig, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if options.ReceiptPath != "" {
		_ = os.Remove(options.ReceiptPath)
	}
	git := Git{Dir: options.Root, Env: options.Env}
	workdir := filepath.Join(scratch, "snapshot")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return ExitConfig, err
	}
	inputs, err := options.Inputs(ctx, git, workdir)
	if err != nil {
		return ExitConfig, configErrorf("inputs: %v", err)
	}
	if len(options.Slots) > 0 {
		slot, err := Acquire(ctx, options.Slots)
		if err != nil {
			_, _ = fmt.Fprintf(options.Stdout, "quality: %v (rerun: %s)\n", err, options.Rerun)
			return ExitInterrupted, nil
		}
		defer slot.Release()
	}
	execution := &execution{options: options, scratch: scratch, inputs: inputs, log: filepath.Join(scratch, "bazel.log")}
	results := execution.run(ctx)
	code := ExitCode(results)
	if ctx.Err() != nil && code == ExitOK {
		code = ExitInterrupted
	}
	if options.ReceiptPath != "" {
		receipt := execution.receipt(ctx, git, results, started, code)
		if err := WriteReceipt(options.ReceiptPath, receipt); err != nil {
			return ExitConfig, err
		}
	}
	_, _ = fmt.Fprintf(options.Stdout, "quality: %s exited %d (isolation: %s)\n", StageName(options.Stage), code, options.Isolation)
	return code, nil
}

type execution struct {
	options Options
	scratch string
	inputs  *Inputs
	log     string
	mu      sync.Mutex
	results []Result
}

func (e *execution) record(result Result, check *registrypb.Check) {
	result.Check = check.GetId()
	result.Blocking = check.GetClassification() == registrypb.Classification_CLASSIFICATION_GUARANTEE
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results = append(e.results, result)
	receipt := e.options.ReceiptPath
	if receipt == "" {
		receipt = "the output above"
	}
	PrintResult(e.options.Stdout, result, e.options.Rerun, receipt)
}

func (e *execution) run(ctx context.Context) []Result {
	var pool, exclusive, after []planned
	for _, check := range e.options.Registry.GetChecks() {
		if !slices.Contains(check.GetStages(), e.options.Stage) || check.GetKind() != registrypb.Kind_KIND_NATIVE {
			continue
		}
		plan, skip := e.plan(check)
		switch {
		case skip != nil:
			e.record(*skip, check)
		case check.GetAfterBazel():
			after = append(after, plan)
		case check.GetExclusive():
			exclusive = append(exclusive, plan)
		default:
			pool = append(pool, plan)
		}
	}
	e.runPool(ctx, pool)
	e.runBazel(ctx)
	e.runPool(ctx, after)
	for _, plan := range exclusive {
		e.record(e.native(ctx, plan), plan.check)
	}
	return e.complete()
}

// plan decides whether a check has anything to judge. With zero inputs a check is not applicable,
// and at full that is an error unless it declared allow_empty.
func (e *execution) plan(check *registrypb.Check) (planned, *Result) {
	plan := planned{check: check}
	if check.GetNeedsNetwork() && e.options.Offline {
		return plan, &Result{State: Skipped, Reason: "--offline; not a passing result"}
	}
	if check.GetSelector() == nil {
		return plan, nil
	}
	plan.inputs = Select(check, e.inputs)
	if len(plan.inputs) > 0 {
		return plan, nil
	}
	if e.options.Stage == registrypb.Stage_STAGE_FULL && !check.GetAllowEmpty() &&
		check.GetClassification() == registrypb.Classification_CLASSIFICATION_GUARANTEE {
		return plan, &Result{State: Errored, EmptyInput: true, Reason: "zero inputs at full; discovery is broken or a selector was narrowed"}
	}
	return plan, &Result{State: NotApplicable, Reason: "no inputs"}
}

func (e *execution) runPool(ctx context.Context, plans []planned) {
	semaphore := make(chan struct{}, max(1, e.options.Jobs))
	var group sync.WaitGroup
	for _, plan := range plans {
		group.Go(func() {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			e.record(e.native(ctx, plan), plan.check)
		})
	}
	group.Wait()
}

func (e *execution) expand(plan planned) ([]string, error) {
	list := filepath.Join(e.scratch, plan.check.GetId()+".list")
	listNL := list + ".nl"
	if err := os.WriteFile(list, []byte(strings.Join(plan.inputs, "\x00")+nulIf(plan.inputs)), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(listNL, []byte(strings.Join(plan.inputs, "\n")+"\n"), 0o644); err != nil {
		return nil, err
	}
	pairs := []string{
		"{adapter}", e.options.Adapter, "{list}", list, "{list_nl}", listNL, "{root}", e.inputs.Root,
		"{bazel_log}", e.log, "{isolation}", e.options.Isolation,
	}
	for name, path := range e.options.Tools {
		pairs = append(pairs, "{"+name+"}", path)
	}
	replacer := strings.NewReplacer(pairs...)
	argv := make([]string, 0, len(plan.check.GetCommand()))
	for _, argument := range plan.check.GetCommand() {
		argv = append(argv, replacer.Replace(argument))
	}
	return argv, nil
}

func nulIf(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return "\x00"
}

// native runs one check under its own supervisor. On its deadline, or the stage's, the supervisor
// is told to stop, so the check's whole tree goes with it.
func (e *execution) native(ctx context.Context, plan planned) Result {
	started := time.Now()
	result := Result{Inputs: len(plan.inputs)}
	finish := func(state State, reason string) Result {
		result.State, result.Reason = state, reason
		result.Seconds = time.Since(started).Seconds()
		return result
	}
	if ctx.Err() != nil {
		return finish(Cancelled, "the stage deadline had passed")
	}
	argv, err := e.expand(plan)
	if err != nil {
		return finish(Errored, err.Error())
	}
	if !filepath.IsAbs(argv[0]) && strings.Contains(argv[0], "/") {
		argv[0] = filepath.Join(e.options.Root, argv[0])
	}
	self, err := os.Executable()
	if err != nil {
		return finish(Errored, err.Error())
	}
	logPath := filepath.Join(e.scratch, plan.check.GetId()+".log")
	log, err := os.Create(logPath)
	if err != nil {
		return finish(Errored, err.Error())
	}
	defer func() { _ = log.Close() }()
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(plan.check.GetTimeoutSeconds())*time.Second)
	defer cancel()
	command := exec.Command(self, append([]string{SuperviseArg}, argv...)...)
	command.Dir, command.Env, command.Stdout, command.Stderr = e.inputs.Root, e.options.Env, log, log
	if err := command.Start(); err != nil {
		return finish(Errored, err.Error())
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var code int
	select {
	case err := <-done:
		code = exitCode(err)
	case <-checkCtx.Done():
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(4*termGrace + time.Second):
			_ = command.Process.Kill()
			<-done
		}
		output, _ := os.ReadFile(logPath)
		result.output = string(output)
		if ctx.Err() != nil && !errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
			return finish(Cancelled, "cancelled")
		}
		return finish(TimedOut, fmt.Sprintf("exceeded %ds", plan.check.GetTimeoutSeconds()))
	}
	output, _ := os.ReadFile(logPath)
	result.output = string(output)
	switch code {
	case 0:
		return finish(Passed, "")
	case ExitNoEvidence:
		result.NoEvidence = true
		return finish(Failed, "baseline or evidence unavailable")
	default:
		return finish(Failed, "")
	}
}

func (e *execution) bazelEnv() []string {
	env := make([]string, 0, len(e.options.Env))
	for _, entry := range e.options.Env {
		name, _, _ := strings.Cut(entry, "=")
		if name == "BUILD_WORKSPACE_DIRECTORY" || name == "BUILD_WORKING_DIRECTORY" || strings.HasPrefix(name, "RUNFILES") ||
			strings.HasSuffix(name, "_RUNFILES") {
			continue
		}
		env = append(env, entry)
	}
	return env
}

func (e *execution) bazelFlags() []string {
	var flags []string
	if e.options.Isolation == "weakened" {
		flags = append(flags, "--config=weak-isolation")
	}
	if e.options.Offline {
		flags = append(flags, "--repository_disable_download")
	}
	if e.options.Stage == registrypb.Stage_STAGE_FULL {
		flags = append(flags, "--nocache_test_results")
	}
	return flags
}

func (e *execution) runBazel(ctx context.Context) {
	plan := PlanBazel(e.options.Registry, e.options.Stage, e.bazelFlags())
	log, err := os.Create(e.log)
	if plan == nil {
		if err == nil {
			_ = log.Close()
		}
		return
	}
	started := time.Now()
	fail := func(state State, reason, output string) {
		for _, check := range plan.Checks {
			e.record(Result{State: state, Reason: reason, Seconds: time.Since(started).Seconds(), output: output}, check)
		}
	}
	if err != nil {
		fail(Errored, err.Error(), "")
		return
	}
	defer func() { _ = log.Close() }()
	if ctx.Err() != nil {
		fail(Cancelled, "the stage deadline had passed", "")
		return
	}
	bazelCtx, cancel := context.WithTimeout(ctx, plan.Timeout)
	defer cancel()
	code, runErr := RunBazel(bazelCtx, e.options.Bazel, plan.Args, e.options.Root, e.bazelEnv(), log)
	seconds := time.Since(started).Seconds()
	if runErr != nil && !errors.Is(runErr, context.DeadlineExceeded) && !errors.Is(runErr, context.Canceled) {
		fail(Errored, runErr.Error(), "")
		return
	}
	data, _ := os.ReadFile(e.log)
	states := TargetStates(string(data))
	output := summaryLines(e.log)
	if runErr != nil {
		state := TimedOut
		if errors.Is(ctx.Err(), context.Canceled) {
			state = Cancelled
		}
		fail(state, "Bazel client stopped; a shared server is left running", output)
		return
	}
	for _, check := range plan.Checks {
		state, reason := TranslateBazel(check, code, states)
		result := Result{State: state, Reason: reason, Seconds: seconds}
		if state != Passed {
			result.output = output
		}
		e.record(result, check)
	}
}

// complete adds a NOT_RUN result for any entry that left none, so a check that never ran cannot be
// mistaken for a pass.
func (e *execution) complete() []Result {
	seen := map[string]bool{}
	for _, result := range e.results {
		seen[result.Check] = true
	}
	for _, check := range e.options.Registry.GetChecks() {
		if slices.Contains(check.GetStages(), e.options.Stage) && !seen[check.GetId()] {
			e.record(Result{State: NotRun}, check)
		}
	}
	slices.SortFunc(e.results, func(a, b Result) int { return strings.Compare(a.Check, b.Check) })
	return e.results
}

var sandboxSummary = regexp.MustCompile(`(?m)^INFO: [0-9]+ process.*$`)

func (e *execution) receipt(ctx context.Context, git Git, results []Result, started time.Time, code int) *Receipt {
	// The stage's deadline may have passed; a receipt is still owed.
	ctx = context.WithoutCancel(ctx)
	options := e.options
	receipt := &Receipt{
		SchemaVersion: 4, Stage: StageName(options.Stage), Requested: StageName(options.Requested),
		Isolation: options.Isolation, Offline: options.Offline, TestCached: options.Stage != registrypb.Stage_STAGE_FULL,
		RegistryVer: options.Registry.GetVersion(), Checks: results, Expected: len(results),
		StartedAt:  started.UTC().Format(time.RFC3339),
		FinishedAt: time.Now().UTC().Format(time.RFC3339),
		Host:       map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH},
		Inventory:  map[string]int{"all": len(e.inputs.All), "deleted": len(e.inputs.Deleted)},
		Tools:      e.toolVersions(ctx),
		Result:     "FAILED", Producer: "local",
	}
	if run := os.Getenv("GITHUB_RUN_ID"); run != "" {
		receipt.Producer = fmt.Sprintf("github-actions/%s/%s/%s/%s", os.Getenv("GITHUB_WORKFLOW"), os.Getenv("GITHUB_JOB"), run, os.Getenv("GITHUB_RUN_ATTEMPT"))
	}
	if code == ExitOK {
		receipt.Result = "PASSED"
		if options.Offline {
			receipt.Result = "INCOMPLETE"
		}
	}
	data, _ := os.ReadFile(e.log)
	targets := 0
	for _, state := range TargetStates(string(data)) {
		if state == "PASSED" {
			targets++
		}
	}
	receipt.Inventory["test_targets"] = targets
	if summaries := sandboxSummary.FindAllString(string(data), -1); len(summaries) > 0 {
		receipt.SandboxSummary = summaries[len(summaries)-1]
	}
	if head, err := git.Run(ctx, nil, "rev-parse", "HEAD"); err == nil {
		receipt.Revision = strings.TrimSpace(string(head))
	}
	if tree, err := git.Run(ctx, nil, "rev-parse", "HEAD^{tree}"); err == nil {
		receipt.Tree = strings.TrimSpace(string(tree))
	}
	receipt.TestedTree, receipt.PolicyDigest = "unknown", "unknown"
	if options.Stage == registrypb.Stage_STAGE_FULL || options.Stage == registrypb.Stage_STAGE_PRE_PUSH {
		if inputs, err := Discover(ctx, git, options.Registry); err == nil {
			if tree, digest, err := TestedTree(ctx, git, inputs, filepath.Join(e.scratch, "tree")); err == nil {
				receipt.TestedTree, receipt.PolicyDigest = tree, digest
			}
		}
	}
	receipt.WorktreeDirty = receipt.TestedTree != receipt.Tree
	if binary, err := exec.Command(options.Bazel, "--path").Output(); err == nil {
		if data, err := os.ReadFile(strings.TrimSpace(string(binary))); err == nil {
			sum := sha256.Sum256(data)
			receipt.BazelSHA256 = hex.EncodeToString(sum[:])
		}
	}
	return receipt
}

func (e *execution) toolVersions(ctx context.Context) map[string]string {
	versions := map[string]string{}
	if e.options.Stage != registrypb.Stage_STAGE_FULL {
		return versions
	}
	for _, tool := range e.options.Registry.GetTools() {
		path := e.options.Tools[tool.GetName()]
		if len(tool.GetVersionArgs()) == 0 || path == "" {
			continue
		}
		versionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		command := exec.CommandContext(versionCtx, path, tool.GetVersionArgs()...)
		command.Env = e.options.Env
		output, err := command.CombinedOutput()
		cancel()
		if err == nil {
			line, _, _ := strings.Cut(string(output), "\n")
			if strings.HasPrefix(line, "ShellCheck") {
				for item := range strings.Lines(string(output)) {
					if version, ok := strings.CutPrefix(item, "version: "); ok {
						line = version
					}
				}
			}
			versions[strings.ToLower(tool.GetName())] = strings.TrimSpace(line)
		}
	}
	return versions
}
