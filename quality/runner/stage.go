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
	Registry  *registrypb.Registry
	Stage     registrypb.Stage
	Requested registrypb.Stage
	Root      string
	Adapter   string
	Bazel     string
	// BazelStartup holds startup options, for a run on an isolated output base.
	BazelStartup []string
	Tools        map[string]string
	Env          []string
	Isolation    string
	Offline      bool
	ReceiptPath  string
	Rerun        string
	Jobs         int
	Deadline     time.Duration
	Slots        []string
	Inputs       func(ctx context.Context, git Git, scratch string) (*Inputs, error)
	Stdout       io.Writer
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

// bookkeepingBudget bounds all final bookkeeping for one run, in total. With a stopped helper's own
// grace, it adds at most this plus 9 s after the stage's work and cleanup.
var bookkeepingBudget = 30 * time.Second

// maxRetainedLog bounds one check's retained output.
var maxRetainedLog int64 = 1 << 20

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
		// One producer per receipt path, for the whole run: a second cannot remove or overwrite what
		// an active one is writing. The lock is close-on-exec, so it ends with this process.
		if err := os.MkdirAll(filepath.Dir(options.ReceiptPath), 0o755); err != nil {
			return ExitConfig, err
		}
		lock, err := Acquire(ctx, []string{options.ReceiptPath + ".lock"})
		if err != nil {
			_, _ = fmt.Fprintf(options.Stdout, "quality: another run holds %s: %v\n", options.ReceiptPath, err)
			return ExitInterrupted, nil
		}
		defer lock.Release()
		if err := os.Remove(options.ReceiptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return ExitConfig, err
		}
		if err := pruneLogDirectories(options.ReceiptPath); err != nil {
			return ExitConfig, configErrorf("pruning old logs: %v", err)
		}
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
		if code == ExitOK && len(receipt.Problems) > 0 {
			code = ExitNoEvidence
			for _, problem := range receipt.Problems {
				_, _ = fmt.Fprintf(options.Stdout, "quality: the receipt cannot name %s (rerun: %s)\n", problem, options.Rerun)
			}
		}
		if receipt.WorktreeDirty {
			_, _ = fmt.Fprintln(options.Stdout, "quality: the tested tree differs from HEAD")
		}
	}
	_, _ = fmt.Fprintf(options.Stdout, "quality: %s exited %d (isolation: %s)\n", StageName(options.Stage), code, options.Isolation)
	return code, nil
}

// readBounded reads a log only as far as a retained copy could hold, because a check's output has
// no limit of its own.
func readBounded(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	data, _ := io.ReadAll(io.LimitReader(file, maxRetainedLog+1))
	return string(data)
}

type execution struct {
	options Options
	scratch string
	inputs  *Inputs
	log     string
	logs    string
	mu      sync.Mutex
	results []Result
}

func (e *execution) record(result Result, check *registrypb.Check) {
	result.Check = check.GetId()
	result.Blocking = check.GetClassification() == registrypb.Classification_CLASSIFICATION_GUARANTEE
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.options.ReceiptPath != "" && result.output != "" && result.State != Passed {
		if e.logs == "" {
			directory, err := newLogDirectory(e.options.ReceiptPath)
			if err != nil {
				result.Reason = strings.TrimSpace(result.Reason + " (no log kept: " + err.Error() + ")")
			}
			e.logs = directory
		}
		if e.logs != "" {
			if path, err := RetainLog(e.logs, result.Check, result.output); err == nil {
				result.Log = path
			}
		}
	}
	e.results = append(e.results, result)
	PrintResult(e.options.Stdout, result, e.options.Rerun)
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
	logPath := filepath.Join(e.scratch, plan.check.GetId()+".log")
	log, err := os.Create(logPath)
	if err != nil {
		return finish(Errored, err.Error())
	}
	defer func() { _ = log.Close() }()
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(plan.check.GetTimeoutSeconds())*time.Second)
	defer cancel()
	code, runErr := supervised(checkCtx, argv, e.inputs.Root, e.options.Env, log)
	result.output = readBounded(logPath)
	switch {
	case runErr == nil:
	case checkCtx.Err() == nil:
		return finish(Errored, runErr.Error())
	case errors.Is(ctx.Err(), context.Canceled):
		return finish(Cancelled, "cancelled")
	default:
		return finish(TimedOut, fmt.Sprintf("exceeded %ds", plan.check.GetTimeoutSeconds()))
	}
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

// supervised runs argv under its own supervisor, with both output streams in one file.
func supervised(ctx context.Context, argv []string, dir string, env []string, out *os.File) (int, error) {
	return supervisedSplit(ctx, argv, dir, env, out, out)
}

// supervisedSplit is supervised with its streams apart. Its whole process tree ends when it does or
// when ctx does. Output goes to files, which a surviving descendant cannot hold open against this wait.
func supervisedSplit(ctx context.Context, argv []string, dir string, env []string, stdout, stderr *os.File) (int, error) {
	// A spent context starts nothing: a late child would only add its own stop to a budget already used.
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	command := exec.Command(self, append([]string{SuperviseArg}, argv...)...)
	command.Dir, command.Env, command.Stdout, command.Stderr = dir, env, stdout, stderr
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
	case <-time.After(4*termGrace + time.Second):
		_ = command.Process.Kill()
		<-done
	}
	return 0, ctx.Err()
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
	args := append(slices.Clone(e.options.BazelStartup), plan.Args...)
	code, runErr := RunBazel(bazelCtx, e.options.Bazel, args, e.options.Root, e.options.Env, log)
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
		result := TranslateBazel(check, e.options.Stage, code, states)
		result.Seconds = seconds
		if result.State != Passed {
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

// receipt records what was judged. Final bookkeeping shares one bound, whatever the stage's own
// deadline did, so it cannot become a second unbounded phase. It runs helpers under supervisors.
// Identity it cannot establish goes in Problems and denies PASSED: full and pre-push evidence names
// exactly what was tested.
func (e *execution) receipt(ctx context.Context, git Git, results []Result, started time.Time, code int) *Receipt {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingBudget)
	defer cancel()
	options := e.options
	receipt := &Receipt{
		SchemaVersion: 4, Stage: StageName(options.Stage), Requested: StageName(options.Requested),
		Isolation: options.Isolation, Offline: options.Offline, TestCached: options.Stage != registrypb.Stage_STAGE_FULL,
		RegistryVer: options.Registry.GetVersion(), Checks: results, Expected: len(results),
		StartedAt:  started.UTC().Format(time.RFC3339),
		FinishedAt: time.Now().UTC().Format(time.RFC3339),
		Host:       map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH},
		Inventory:  map[string]int{"all": len(e.inputs.All), "deleted": len(e.inputs.Deleted)},
		Producer:   "local", Tools: map[string]string{},
	}
	if run := os.Getenv("GITHUB_RUN_ID"); run != "" {
		receipt.Producer = fmt.Sprintf("github-actions/%s/%s/%s/%s", os.Getenv("GITHUB_WORKFLOW"), os.Getenv("GITHUB_JOB"), run, os.Getenv("GITHUB_RUN_ATTEMPT"))
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
	if options.Stage != registrypb.Stage_STAGE_PRE_COMMIT && options.Stage != registrypb.Stage_STAGE_COMMIT_MSG {
		e.identify(ctx, git, receipt)
	}
	receipt.Result = "FAILED"
	if code == ExitOK && len(receipt.Problems) == 0 {
		receipt.Result = "PASSED"
		if options.Offline {
			receipt.Result = "INCOMPLETE"
		}
	}
	return receipt
}

// identify fills in what a commit-level stage must name.
func (e *execution) identify(ctx context.Context, git Git, receipt *Receipt) {
	missing := func(what string, err error) {
		receipt.Problems = append(receipt.Problems, fmt.Sprintf("%s: %v", what, err))
	}
	if head, err := git.Run(ctx, nil, "rev-parse", "--verify", "HEAD"); err != nil {
		missing("revision", err)
	} else {
		receipt.Revision = strings.TrimSpace(string(head))
	}
	if tree, err := git.Run(ctx, nil, "rev-parse", "--verify", "HEAD^{tree}"); err != nil {
		missing("tree", err)
	} else {
		receipt.Tree = strings.TrimSpace(string(tree))
	}
	inputs, err := Discover(ctx, git, e.options.Registry)
	if err != nil {
		missing("tested tree", err)
	} else if tree, digest, err := TestedTree(ctx, git, inputs, e.options.Registry.GetEscalate(), filepath.Join(e.scratch, "tree")); err != nil {
		missing("tested tree", err)
	} else {
		receipt.TestedTree, receipt.PolicyDigest = tree, digest
		receipt.WorktreeDirty = tree != receipt.Tree
	}
	if digest, err := e.bazelDigest(ctx); err != nil {
		missing("bazel binary", err)
	} else {
		receipt.BazelSHA256 = digest
	}
	if e.options.Stage == registrypb.Stage_STAGE_FULL {
		e.toolVersions(ctx, receipt, missing)
	}
}

// MaxCapturedStderr bounds what a failed helper adds to a diagnostic.
const MaxCapturedStderr = 2048

func (e *execution) capture(ctx context.Context, name string, argv []string) (string, error) {
	stdout, err := os.CreateTemp(e.scratch, name+"-stdout-")
	if err != nil {
		return "", err
	}
	defer func() { _ = stdout.Close() }()
	stderr, err := os.CreateTemp(e.scratch, name+"-stderr-")
	if err != nil {
		return "", err
	}
	defer func() { _ = stderr.Close() }()
	code, err := supervisedSplit(ctx, argv, e.options.Root, e.options.Env, stdout, stderr)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("exited %d: %s", code, stderrTail(stderr.Name(), MaxCapturedStderr))
	}
	data, err := os.ReadFile(stdout.Name())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// stderrTail returns the end of a file, where a tool usually states its cause.
func stderrTail(path string, limit int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(no stderr)"
	}
	return strings.TrimSpace(string(data[max(0, len(data)-limit):]))
}

func (e *execution) bazelDigest(ctx context.Context) (string, error) {
	path, err := e.capture(ctx, "bazel-path", []string{e.options.Bazel, "--path"})
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (e *execution) toolVersions(ctx context.Context, receipt *Receipt, missing func(string, error)) {
	for _, tool := range e.options.Registry.GetTools() {
		if len(tool.GetVersionArgs()) == 0 {
			continue
		}
		name := strings.ToLower(tool.GetName())
		path, known := e.options.Tools[tool.GetName()]
		if !known {
			missing(name+" version", errors.New("the tool was not passed in"))
			continue
		}
		output, err := e.capture(ctx, "version", append([]string{path}, tool.GetVersionArgs()...))
		if err != nil {
			missing(name+" version", err)
			continue
		}
		if strings.TrimSpace(output) == "" {
			missing(name+" version", errors.New("it printed nothing on stdout"))
			continue
		}
		line, _, _ := strings.Cut(output, "\n")
		if strings.HasPrefix(line, "ShellCheck") {
			for item := range strings.Lines(output) {
				if version, ok := strings.CutPrefix(item, "version: "); ok {
					line = version
				}
			}
		}
		receipt.Tools[name] = strings.TrimSpace(line)
	}
}
