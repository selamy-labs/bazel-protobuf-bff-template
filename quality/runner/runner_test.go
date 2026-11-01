package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == SuperviseArg {
		os.Exit(Supervise(os.Args[2:]))
	}
	os.Exit(m.Run())
}

const baseCheck = `
checks {
  id: "good"
  stages: [STAGE_FULL]
  classification: CLASSIFICATION_GUARANTEE
  kind: KIND_NATIVE
  timeout_seconds: 5
  command: ["true"]
  witness: "//:test"
}
`

func TestRegistryValidation(t *testing.T) {
	if _, err := ParseRegistry([]byte(baseCheck)); err != nil {
		t.Fatalf("the valid control was rejected: %v", err)
	}
	cases := []struct{ name, text, want string }{
		{"duplicate id", baseCheck + baseCheck, "duplicate check id"},
		{"missing full", strings.Replace(baseCheck, "STAGE_FULL", "STAGE_PRE_COMMIT", 1), "every check must run at full"},
		{"signal with threshold", strings.Replace(strings.Replace(baseCheck, "CLASSIFICATION_GUARANTEE", "CLASSIFICATION_SIGNAL", 1), `witness: "//:test"`, "threshold { direction: DIRECTION_MIN value: 1 }", 1), "SIGNAL cannot carry"},
		{"guarantee without witness", strings.Replace(baseCheck, `witness: "//:test"`, "", 1), "needs a witness"},
		{"missing class", strings.Replace(baseCheck, "classification: CLASSIFICATION_GUARANTEE", "", 1), "missing classification"},
		{"missing kind", strings.Replace(baseCheck, "kind: KIND_NATIVE", "", 1), "missing kind"},
		{"missing timeout", strings.Replace(baseCheck, "timeout_seconds: 5", "", 1), "missing timeout"},
		{"unknown field", strings.Replace(baseCheck, "timeout_seconds: 5", "timeout_seconds: 5 skip: true", 1), "skip"},
		{"bad id", strings.Replace(baseCheck, `"good"`, `"Not_Kebab"`, 1), "kebab-case"},
		{"unknown tool", strings.Replace(baseCheck, "timeout_seconds: 5", `timeout_seconds: 5 tool: "NOPE"`, 1), "unknown tool"},
		{"bad selector", strings.Replace(baseCheck, "timeout_seconds: 5", `timeout_seconds: 5 selector { include: "(" }`, 1), "selector"},
		{"no checks", `version: "1"`, "no checks"},
		{"native with targets", strings.Replace(baseCheck, "timeout_seconds: 5", `timeout_seconds: 5 targets: "//x"`, 1), "no targets"},
		{"malformed", "checks {", "registry"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseRegistry([]byte(test.text))
			var config *ConfigError
			if !errors.As(err, &config) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want a configuration error naming %q", err, test.want)
			}
		})
	}
}

func TestRealRegistry(t *testing.T) {
	path := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("REGISTRY"))
	registry, err := LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	stages := map[registrypb.Stage]int{}
	for _, check := range registry.GetChecks() {
		for _, stage := range check.GetStages() {
			stages[stage]++
		}
	}
	if stages[registrypb.Stage_STAGE_PRE_COMMIT] == 0 || stages[registrypb.Stage_STAGE_PRE_COMMIT] >= stages[registrypb.Stage_STAGE_FULL] {
		t.Errorf("pre-commit must be a fast subset of full, got %v", stages)
	}
	for _, check := range registry.GetChecks() {
		if slices.Contains(check.GetStages(), registrypb.Stage_STAGE_PRE_COMMIT) && check.GetKind() == registrypb.Kind_KIND_BAZEL {
			t.Errorf("%s: pre-commit reads staged blobs, so it cannot build from the checkout", check.GetId())
		}
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		name    string
		results []Result
		want    int
	}{
		{"all passed", []Result{{State: Passed, Blocking: true}}, ExitOK},
		{"not applicable", []Result{{State: NotApplicable, Blocking: true}}, ExitOK},
		{"failed", []Result{{State: Failed, Blocking: true}}, ExitFailed},
		{"errored", []Result{{State: Errored, Blocking: true}}, ExitFailed},
		{"not run", []Result{{State: NotRun, Blocking: true}}, ExitFailed},
		{"signal failed", []Result{{State: Failed}}, ExitOK},
		{"timed out", []Result{{State: TimedOut, Blocking: true}}, ExitInterrupted},
		{"cancelled outranks failed", []Result{{State: Failed, Blocking: true}, {State: Cancelled, Blocking: true}}, ExitInterrupted},
		{"no evidence", []Result{{State: Failed, Blocking: true, NoEvidence: true}}, ExitNoEvidence},
		{"empty input", []Result{{State: Errored, Blocking: true, EmptyInput: true}}, ExitEmptyInput},
	}
	for _, test := range cases {
		if got := ExitCode(test.results); got != test.want {
			t.Errorf("%s: exit %d, want %d", test.name, got, test.want)
		}
	}
}

func TestBazelTranslation(t *testing.T) {
	check := &registrypb.Check{Targets: []string{"//a/...", "//b:t"}}
	passed := map[string]string{"//a/x:t": "PASSED", "//b:t": "PASSED"}
	failed := map[string]string{"//a/x:t": "FAILED", "//b:t": "PASSED", "//c:t": "PASSED"}
	other := map[string]string{"//b:t": "PASSED"}
	cases := []struct {
		name   string
		code   int
		states map[string]string
		want   State
	}{
		{"passed", 0, passed, Passed},
		{"a named target never ran", 0, map[string]string{"//a/x:t": "PASSED"}, Errored},
		{"contradictory exit 0", 0, map[string]string{"//a/x:t": "TIMEOUT", "//b:t": "PASSED"}, Errored},
		{"tests failed", 3, failed, Failed},
		{"another check's failure", 3, map[string]string{"//a/x:t": "PASSED", "//b:t": "PASSED", "//c:t": "FAILED"}, Passed},
		{"only its named target", 0, other, Passed},
		{"no tests on an affected set", 4, nil, NotApplicable},
		{"interrupted", 8, passed, Cancelled},
		{"build failed", 1, passed, Errored},
		{"crash", 37, passed, Errored},
	}
	for _, test := range cases {
		if got := TranslateBazel(check, registrypb.Stage_STAGE_PRE_PUSH, test.code, test.states).State; got != test.want {
			t.Errorf("%s: %s, want %s", test.name, got, test.want)
		}
	}
	states := TargetStates("//a:t    PASSED in 1s\n//b:t  (cached) PASSED in 0.1s\n//c:t   FAILED in 2s\n@q//d:t  TIMEOUT in 9s\nINFO: Build\n")
	if states["//a:t"] != "PASSED" || states["//b:t"] != "PASSED" || states["//c:t"] != "FAILED" || states["@q//d:t"] != "TIMEOUT" || len(states) != 4 {
		t.Errorf("states: %v", states)
	}
}

func TestDiagnostics(t *testing.T) {
	line := Diagnostics("lint", "a/b.go:3:7: bad thing [S001]\n", "tools/verify.sh", "r.json")
	if len(line) != 1 || line[0] != "lint a/b.go:3:7: bad thing [S001] (rerun: tools/verify.sh)" {
		t.Errorf("format: %q", line)
	}
	if got := Diagnostics("lint", "a.go:3: no column\n", "r", "x")[0]; got != "lint a.go:3:1: no column (rerun: r)" {
		t.Errorf("no column: %q", got)
	}
	if got := Diagnostics("lint", "", "r", "x")[0]; !strings.Contains(got, "printed nothing") || !strings.Contains(got, "(rerun: r)") {
		t.Errorf("silent failure: %q", got)
	}
	long := strings.Repeat("x\n", 51)
	got := Diagnostics("lint", long, "r", "receipt.json")
	if len(got) != MaxDiagnostics+1 || got[MaxDiagnostics] != "… 1 more (see receipt.json)" {
		t.Errorf("bound: %d lines, last %q", len(got), got[len(got)-1])
	}
	if got := Diagnostics("lint", strings.Repeat("x\n", 50), "r", "x"); len(got) != 50 {
		t.Errorf("exactly at the bound: %d", len(got))
	}
}

// harness runs a stage against a registry in a scratch repository.
type harness struct {
	t     *testing.T
	dir   string
	out   bytes.Buffer
	bazel string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir()}
	h.git("init", "-q", "-b", "main")
	h.write("tools/speckit/scaffold.sha256", strings.Repeat("0", 64)+"  vendored.sh\n")
	h.write("vendored.sh", "#!/bin/sh\n")
	h.write("quality/fixtures.sha256", strings.Repeat("0", 64)+"  fixture.go\n")
	h.write("fixture.go", "bad\n")
	h.write("real.go", "package a\n")
	h.bazel = filepath.Join(t.TempDir(), "bazel")
	h.writeAbs(h.bazel, "#!/bin/sh\n[ \"$1\" = --path ] && echo \"$0\"\nexit 0\n", 0o755)
	return h
}

func (h *harness) writeAbs(path, text string, mode os.FileMode) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) write(path, text string) {
	h.t.Helper()
	h.writeAbs(filepath.Join(h.dir, path), text, 0o644)
}

func gitEnv() []string {
	return append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com")
}

func (h *harness) git(args ...string) string {
	h.t.Helper()
	out, err := Git{Dir: h.dir, Env: gitEnv()}.Run(context.Background(), nil, args...)
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (h *harness) registry(body string) *registrypb.Registry {
	h.t.Helper()
	text := `vendored_manifest: "tools/speckit/scaffold.sha256"
fixture_manifest: "quality/fixtures.sha256"
` + body
	registry, err := ParseRegistry([]byte(text))
	if err != nil {
		h.t.Fatal(err)
	}
	return registry
}

func (h *harness) options(body string, stage registrypb.Stage) Options {
	h.t.Helper()
	registry := h.registry(body)
	return Options{
		Registry: registry, Stage: stage, Requested: stage, Root: h.dir, Adapter: "/bin/true", Bazel: h.bazel,
		Env: gitEnv(), Isolation: "full", Rerun: "rerun-me", Jobs: 2, Deadline: 30 * time.Second, Stdout: &h.out,
		Inputs: func(ctx context.Context, git Git, _ string) (*Inputs, error) { return Discover(ctx, git, registry) },
	}
}

func (h *harness) run(options Options) int {
	h.t.Helper()
	code, err := Run(context.Background(), options)
	if err != nil {
		h.t.Fatal(err)
	}
	return code
}

func native(id, command string, extra ...string) string {
	return fmt.Sprintf(`checks { id: %q stages: [STAGE_FULL] classification: CLASSIFICATION_GUARANTEE kind: KIND_NATIVE
 timeout_seconds: 30 witness: "w" command: ["sh", "-c", %q] %s }
`, id, command, strings.Join(extra, " "))
}

func TestStageRunsExactlyItsEntries(t *testing.T) {
	h := committed(newHarness(t))
	options := h.options(native("one", "exit 0")+native("two", "exit 0"), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	if code := h.run(options); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	var receipt Receipt
	data, _ := os.ReadFile(options.ReceiptPath)
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, check := range receipt.Checks {
		ids = append(ids, check.Check+":"+string(check.State))
	}
	if strings.Join(ids, " ") != "one:PASSED two:PASSED" || receipt.Result != "PASSED" {
		t.Fatalf("receipt: %v %s", ids, receipt.Result)
	}
	h2 := newHarness(t)
	options = h2.options(native("one", "exit 0"), registrypb.Stage_STAGE_PRE_PUSH)
	h2.run(options)
	if strings.Contains(h2.out.String(), "one") && !strings.Contains(h2.out.String(), "pre-push exited 0") {
		t.Fatalf("an entry for another stage ran: %s", h2.out.String())
	}
}

func TestFailureNamesCheckAndRerun(t *testing.T) {
	h := newHarness(t)
	code := h.run(h.options(native("lint", "echo real.go:1:2: broken; exit 1"), registrypb.Stage_STAGE_FULL))
	if code != ExitFailed || !strings.Contains(h.out.String(), "lint real.go:1:2: broken (rerun: rerun-me)") {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
}

func TestSignalNeverBlocks(t *testing.T) {
	h := newHarness(t)
	body := strings.Replace(native("advice", "exit 1"), "CLASSIFICATION_GUARANTEE", "CLASSIFICATION_SIGNAL", 1)
	if code := h.run(h.options(body, registrypb.Stage_STAGE_FULL)); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	if !strings.Contains(h.out.String(), "FAILED") {
		t.Fatalf("a failing signal was hidden:\n%s", h.out.String())
	}
}

func TestEvidenceUnavailableIsDistinct(t *testing.T) {
	h := newHarness(t)
	if code := h.run(h.options(native("baseline", "exit 11"), registrypb.Stage_STAGE_FULL)); code != ExitNoEvidence {
		t.Fatalf("exit %d", code)
	}
}

func TestZeroInputs(t *testing.T) {
	selector := `selector { include: "\\.nothing$" }`
	h := newHarness(t)
	if code := h.run(h.options(native("empty", "exit 0", selector), registrypb.Stage_STAGE_FULL)); code != ExitEmptyInput {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	h = newHarness(t)
	if code := h.run(h.options(native("empty", "exit 0", selector, "allow_empty: true"), registrypb.Stage_STAGE_FULL)); code != ExitOK {
		t.Fatalf("allow_empty: exit %d", code)
	}
	h = newHarness(t)
	body := strings.Replace(native("empty", "exit 1", selector), "[STAGE_FULL]", "[STAGE_FULL, STAGE_PRE_PUSH]", 1)
	if code := h.run(h.options(body, registrypb.Stage_STAGE_PRE_PUSH)); code != ExitOK {
		t.Fatalf("below full, no inputs is not applicable: exit %d", code)
	}
}

func TestSelectorsSeeOnlyTheirFiles(t *testing.T) {
	h := newHarness(t)
	list := filepath.Join(t.TempDir(), "seen")
	body := native("list", `tr "\0" "\n" <"$0" >`+list, `selector { include: "\\.(go|sh)$" }`)
	options := h.options(strings.Replace(body, `"-c", `, `"-c", `, 1), registrypb.Stage_STAGE_FULL)
	check := options.Registry.GetChecks()[0]
	check.Command = append(check.GetCommand(), "{list}")
	if code := h.run(options); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	seen, _ := os.ReadFile(list)
	if string(seen) != "real.go\n" {
		t.Fatalf("selected %q; vendored and fixture files must stay out, and real.go in", seen)
	}
}

func TestOfflineSkipIsNotAPass(t *testing.T) {
	h := committed(newHarness(t))
	options := h.options(native("network", "exit 1", "needs_network: true"), registrypb.Stage_STAGE_FULL)
	options.Offline = true
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	if code := h.run(options); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	data, _ := os.ReadFile(options.ReceiptPath)
	if !bytes.Contains(data, []byte(`"result": "INCOMPLETE"`)) || !bytes.Contains(data, []byte(`"SKIPPED"`)) {
		t.Fatalf("receipt: %s", data)
	}
}

func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	return !strings.Contains(string(stat[bytes.LastIndexByte(stat, ')'):]), ") Z")
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the check never reported its child: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestStdinIsClosed(t *testing.T) {
	// The caller's stdin stays open and silent, as a terminal's does, so a reader that inherited it
	// would wait until its deadline.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	saved := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = saved }()
	h := newHarness(t)
	body := strings.Replace(native("reader", "cat"), "timeout_seconds: 30", "timeout_seconds: 2", 1)
	if code := h.run(h.options(body, registrypb.Stage_STAGE_FULL)); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
}

func TestHungTreeIsRemovedAtItsDeadline(t *testing.T) {
	h := newHarness(t)
	pidfile := filepath.Join(t.TempDir(), "pid")
	script := `trap "" TERM INT
setsid sh -c 'trap "" TERM INT; echo $$ >` + pidfile + `; sleep 300; sleep 300' &
while :; do sleep 1; done`
	body := strings.Replace(native("hung", script), "timeout_seconds: 30", "timeout_seconds: 1", 1)
	started := time.Now()
	code := h.run(h.options(body, registrypb.Stage_STAGE_FULL))
	if code != ExitInterrupted || !strings.Contains(h.out.String(), "TIMED_OUT") {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	if elapsed := time.Since(started); elapsed > 16*time.Second {
		t.Fatalf("took %s, past the deadline plus 15 s", elapsed)
	}
	if pid := readPid(t, pidfile); alive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("the setsid child %d survived", pid)
	}
}

func TestDaemonLeftByAPassingCheckIsRemoved(t *testing.T) {
	h := newHarness(t)
	pidfile := filepath.Join(t.TempDir(), "pid")
	script := `setsid sh -c 'echo $$ >` + pidfile + `; exec sleep 300' &
sleep 0.3`
	if code := h.run(h.options(native("leaky", script), registrypb.Stage_STAGE_FULL)); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	if pid := readPid(t, pidfile); alive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("the daemon %d survived its check", pid)
	}
}

func fakeBazel(h *harness, body string) string {
	calls := filepath.Join(h.t.TempDir(), "calls")
	h.writeAbs(h.bazel, fmt.Sprintf("#!/bin/bash\n[ \"$1\" = --path ] && { echo \"$0\"; exit 0; }\necho \"$*\" >>%q\n%s\n", calls, body), 0o755)
	return calls
}

func bazelCheck(id, target string, stages string) string {
	return fmt.Sprintf(`checks { id: %q stages: [%s] classification: CLASSIFICATION_GUARANTEE kind: KIND_BAZEL
 timeout_seconds: 30 witness: "w" targets: [%q] }
`, id, stages, target)
}

func TestOneBazelInvocationPerStage(t *testing.T) {
	h := newHarness(t)
	calls := fakeBazel(h, `echo "//a:t   PASSED in 1s"; echo "//web:t   PASSED in 1s"`)
	body := bazelCheck("tests", "//a/...", "STAGE_FULL") + bazelCheck("e2e", "//web/...", "STAGE_FULL") + bazelCheck("rules", "//a:t", "STAGE_FULL")
	if code := h.run(h.options(body, registrypb.Stage_STAGE_FULL)); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	data, _ := os.ReadFile(calls)
	if lines := strings.Count(string(data), "\n"); lines != 1 {
		t.Fatalf("%d Bazel invocations: %s", lines, data)
	}
	if !strings.Contains(string(data), "--nocache_test_results") || !strings.Contains(string(data), "//web/...") {
		t.Fatalf("args: %s", data)
	}
}

func TestPrePushSubtractsFullOnlyTargets(t *testing.T) {
	h := newHarness(t)
	calls := fakeBazel(h, `echo "//a:t   (cached) PASSED in 1s"`)
	body := bazelCheck("tests", "//...", "STAGE_PRE_PUSH, STAGE_FULL") + bazelCheck("e2e", "//web/...", "STAGE_FULL")
	if code := h.run(h.options(body, registrypb.Stage_STAGE_PRE_PUSH)); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	data, _ := os.ReadFile(calls)
	if !strings.Contains(string(data), "-- //... -//web/...") || strings.Contains(string(data), "--nocache_test_results") {
		t.Fatalf("args: %s", data)
	}
}

func TestDeadlineInterruptsOnlyOurClientOnce(t *testing.T) {
	saved := clientGrace
	clientGrace = 300 * time.Millisecond
	defer func() { clientGrace = saved }()
	h := newHarness(t)
	signals := filepath.Join(t.TempDir(), "signals")
	fakeBazel(h, `trap 'echo INT >>`+signals+`' INT; while :; do sleep 0.05; done`)
	options := h.options(bazelCheck("tests", "//...", "STAGE_FULL"), registrypb.Stage_STAGE_FULL)
	options.Deadline = 500 * time.Millisecond
	code := h.run(options)
	if code != ExitInterrupted {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	data, _ := os.ReadFile(signals)
	if string(data) != "INT\n" {
		t.Fatalf("want exactly one interrupt, because repeated ones ask Bazel to kill its server: %q", data)
	}
	if !strings.Contains(h.out.String(), "a shared server is left running") {
		t.Fatalf("the run must say what it left:\n%s", h.out.String())
	}
}

func startProcess(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	command := exec.Command(args[0], args[1:]...)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	return command
}

func TestServerStopRequiresExactIdentity(t *testing.T) {
	base := t.TempDir()
	pidfile := filepath.Join(base, "server", "server.pid.txt")
	if err := os.MkdirAll(filepath.Dir(pidfile), 0o755); err != nil {
		t.Fatal(err)
	}
	loop := "while :; do sleep 0.05; done"
	bystander := startProcess(t, "sh", "-c", loop, "--output_base="+base+"-other")
	if err := os.WriteFile(pidfile, []byte(strconv.Itoa(bystander.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StopOwnedServer(base); err == nil || !strings.Contains(err.Error(), "left running") {
		t.Fatalf("a process that is not this base's server: %v", err)
	}
	if !alive(bystander.Process.Pid) {
		t.Fatal("the bystander was signaled on the word of a PID file")
	}

	owned := startProcess(t, "sh", "-c", loop, "--output_base="+base)
	done := make(chan error, 1)
	go func() { done <- owned.Wait() }()
	if err := os.WriteFile(pidfile, []byte(strconv.Itoa(owned.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StopOwnedServer(base); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the proved owned server is still running")
	}
	if err := os.Remove(pidfile); err != nil {
		t.Fatal(err)
	}
	if err := StopOwnedServer(base); err != nil {
		t.Fatalf("no server to stop: %v", err)
	}
}

func TestSlotWaitsWhileForegroundWorkHoldsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot")
	holder, err := Acquire(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := Acquire(short, []string{path}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a held slot was granted: %v", err)
	}
	holder.Release()
	next, err := Acquire(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("a released slot was not granted: %v", err)
	}
	next.Release()
}

func TestSlotDoesNotOutliveTheParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot")
	holder, err := Acquire(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	daemon := startProcess(t, "sleep", "30")
	holder.Release()
	short, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	next, err := Acquire(short, []string{path})
	if err != nil {
		t.Fatalf("a child kept the slot after the parent let go: %v", err)
	}
	next.Release()
	if !alive(daemon.Process.Pid) {
		t.Fatal("the control needs the child alive")
	}

	// The control: an inherited descriptor keeps the flock, so the check above can see a leak.
	holder, err = Acquire(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	leaky := exec.Command("sleep", "30")
	leaky.ExtraFiles = []*os.File{holder.file}
	if err := leaky.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = leaky.Process.Kill(); _ = leaky.Wait() })
	holder.Release()
	short2, cancel2 := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel2()
	if _, err := Acquire(short2, []string{path}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the inherited-lock control did not hold the slot: %v", err)
	}
}

func TestDiscover(t *testing.T) {
	h := newHarness(t)
	h.write("sp ace/authored file.sh", "#!/bin/sh\n")
	h.write("noextension", "#!/usr/bin/env bash\n")
	h.write("notes.txt", "hello\n")
	h.write("BUILD", "\n")
	h.write("gone.sh", "#!/bin/sh\n")
	h.write("excluded.sh", "#!/bin/sh\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	h.write("untracked.sh", "#!/bin/sh\n")
	if err := os.Remove(filepath.Join(h.dir, "gone.sh")); err != nil {
		t.Fatal(err)
	}
	h.write(".git/info/exclude", "excluded.sh\nhidden.sh\n")
	h.write("hidden.sh", "#!/bin/sh\n")
	if err := os.Symlink("nowhere", filepath.Join(h.dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	registry := h.registry(baseCheck)
	inputs, err := Discover(context.Background(), Git{Dir: h.dir, Env: gitEnv()}, registry)
	if err != nil {
		t.Fatal(err)
	}
	check := &registrypb.Check{Selector: &registrypb.Selector{Include: []string{`\.(sh|bash)$`}, ShellScripts: true}}
	shell := Select(check, inputs)
	for _, want := range []string{"sp ace/authored file.sh", "untracked.sh", "noextension", "hidden.sh", "excluded.sh"} {
		if !slices.Contains(shell, want) {
			t.Errorf("%q was lost: %v", want, shell)
		}
	}
	for _, reject := range []string{"vendored.sh", "notes.txt", "dangling", "gone.sh"} {
		if slices.Contains(shell, reject) {
			t.Errorf("%q was selected", reject)
		}
	}
	if !slices.Contains(inputs.All, "vendored.sh") || !slices.Contains(inputs.Deleted, "gone.sh") || slices.Contains(inputs.All, "gone.sh") {
		t.Errorf("all %v deleted %v", inputs.All, inputs.Deleted)
	}
	h.write("tools/speckit/scaffold.sha256", strings.Repeat("0", 64)+"  missing.sh\n")
	if _, err := Discover(context.Background(), Git{Dir: h.dir, Env: gitEnv()}, registry); err == nil {
		t.Fatal("a missing vendored entry passed")
	}
	h.write("tools/speckit/scaffold.sha256", "")
	if _, err := Discover(context.Background(), Git{Dir: h.dir, Env: gitEnv()}, registry); err == nil {
		t.Fatal("an empty manifest passed")
	}
}

func TestTestedTree(t *testing.T) {
	h := newHarness(t)
	h.write("MODULE.bazel", "module(name = \"a\")\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	git := Git{Dir: h.dir, Env: gitEnv()}
	registry := h.registry(baseCheck)
	measure := func() (string, string) {
		inputs, err := Discover(context.Background(), git, registry)
		if err != nil {
			t.Fatal(err)
		}
		tree, digest, err := TestedTree(context.Background(), git, inputs, []string{"^MODULE"}, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return tree, digest
	}
	tree, digest := measure()
	if tree != h.git("rev-parse", "HEAD^{tree}") {
		t.Fatal("a clean checkout must equal HEAD's tree")
	}
	status := h.git("status", "--porcelain")
	h.write("MODULE.bazel", "module(name = \"b\")\n")
	h.git("update-index", "--skip-worktree", "MODULE.bazel")
	tree2, digest2 := measure()
	if tree2 == tree || digest2 == digest {
		t.Fatal("a skip-worktree edit looked clean")
	}
	if status != "" {
		t.Fatalf("measuring changed the repository: %q", status)
	}
}

func snapshot(t *testing.T, h *harness, env []string) (*Inputs, string) {
	t.Helper()
	scratch := t.TempDir()
	inputs, err := Snapshot(context.Background(), Git{Dir: h.dir, Env: env}, h.registry(baseCheck), scratch)
	if err != nil {
		t.Fatal(err)
	}
	return inputs, scratch
}

func TestSnapshotReadsTheIndex(t *testing.T) {
	h := newHarness(t)
	h.write("keep.txt", "committed\n")
	h.write("old name.txt", "renamed\n")
	h.write("delete.txt", "gone\n")
	h.write("tool.sh", "#!/bin/sh\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")

	h.write("keep.txt", "staged\n")
	h.git("add", "keep.txt")
	h.write("keep.txt", "unstaged\n")
	h.git("mv", "old name.txt", "new name.txt")
	h.git("rm", "-q", "delete.txt")
	h.write("line\nbreak.txt", "odd\n")
	h.git("add", "line\nbreak.txt")
	h.git("update-index", "--chmod=+x", "real.go")
	if err := os.Symlink("real.go", filepath.Join(h.dir, "link")); err != nil {
		t.Fatal(err)
	}
	h.git("add", "link")
	h.write("untracked.txt", "bad\n")
	indexBefore, _ := os.ReadFile(filepath.Join(h.dir, ".git", "index"))
	status := h.git("status", "--porcelain")

	inputs, scratch := snapshot(t, h, gitEnv())

	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(scratch, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := read("keep.txt"); got != "staged\n" {
		t.Errorf("read the working tree, not the index: %q", got)
	}
	if got := read("line\nbreak.txt"); got != "odd\n" {
		t.Errorf("newline path: %q", got)
	}
	if got := read("new name.txt"); got != "renamed\n" {
		t.Errorf("rename: %q", got)
	}
	if got := read("link"); got != "real.go" {
		t.Errorf("symlink: %q", got)
	}
	if info, err := os.Stat(filepath.Join(scratch, "real.go")); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("mode lost: %v %v", info, err)
	}
	for _, absent := range []string{"untracked.txt", "tool.sh", "old name.txt"} {
		if _, err := os.Stat(filepath.Join(scratch, absent)); err == nil {
			t.Errorf("%s reached the snapshot", absent)
		}
	}
	if !slices.Contains(inputs.Deleted, "delete.txt") || !slices.Contains(inputs.Deleted, "old name.txt") {
		t.Errorf("deleted: %v", inputs.Deleted)
	}
	indexAfter, _ := os.ReadFile(filepath.Join(h.dir, ".git", "index"))
	if !bytes.Equal(indexBefore, indexAfter) || h.git("status", "--porcelain") != status {
		t.Error("the snapshot changed the index or the working tree")
	}
}

func TestSnapshotFollowsTheIndexGitNamed(t *testing.T) {
	h := newHarness(t)
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	h.write("real.go", "package staged\n")
	h.git("add", "real.go")
	h.write("real.go", "package other\n")
	alternate := filepath.Join(t.TempDir(), "next-index")
	if data, err := os.ReadFile(filepath.Join(h.dir, ".git", "index")); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(alternate, data, 0o644); err != nil {
		t.Fatal(err)
	}
	env := append(gitEnv(), "GIT_INDEX_FILE="+alternate)
	if _, err := (Git{Dir: h.dir, Env: env}).Run(context.Background(), nil, "add", "real.go"); err != nil {
		t.Fatal(err)
	}
	_, scratch := snapshot(t, h, env)
	if data, _ := os.ReadFile(filepath.Join(scratch, "real.go")); string(data) != "package other\n" {
		t.Fatalf("read the main index: %q", data)
	}
	_, scratch = snapshot(t, h, gitEnv())
	if data, _ := os.ReadFile(filepath.Join(scratch, "real.go")); string(data) != "package staged\n" {
		t.Fatalf("control: %q", data)
	}
}

func TestDeletionOnlyCommitHasNoInputs(t *testing.T) {
	h := newHarness(t)
	h.write("delete.txt", "gone\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	h.git("rm", "-q", "delete.txt")
	inputs, _ := snapshot(t, h, gitEnv())
	if len(inputs.All) != 0 || len(inputs.Deleted) != 1 {
		t.Fatalf("%+v", inputs)
	}
}

func TestEscalation(t *testing.T) {
	h := newHarness(t)
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	base := h.git("rev-parse", "HEAD")
	h.write("src.go", "package a\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "second")
	ordinary := h.git("rev-parse", "HEAD")
	h.write("quality/x", "y\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "third")
	policy := h.git("rev-parse", "HEAD")
	git := Git{Dir: h.dir, Env: gitEnv()}
	patterns := []string{"^quality/"}
	cases := []struct {
		name, from, to string
		want           bool
	}{
		{"ordinary edit", base, ordinary, false},
		{"policy edit", ordinary, policy, true},
		{"unknown base", "", policy, true},
		{"zero base", strings.Repeat("0", 40), ordinary, true},
		{"unreadable base", strings.Repeat("a", 40), ordinary, true},
	}
	for _, test := range cases {
		got, err := Escalates(context.Background(), git, patterns, test.from, test.to)
		if err != nil || got != test.want {
			t.Errorf("%s: %v, %v", test.name, got, err)
		}
	}
}

func realRegistry(t *testing.T) *registrypb.Registry {
	t.Helper()
	registry, err := LoadRegistry(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("REGISTRY")))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// The real registry over real commits with a known base: each file that shapes a verdict widens the
// push, and ordinary code and documents do not.
func TestRealRegistryEscalatesPolicyInputs(t *testing.T) {
	registry := realRegistry(t)
	h := committed(newHarness(t))
	git := Git{Dir: h.dir, Env: gitEnv()}
	policy := []string{
		".bazelrc", ".bazelversion", ".github/workflows/ci.yml", ".gitignore", ".gitleaks.toml",
		"BUILD.bazel", "MODULE.bazel", "MODULE.bazel.lock", "REPO.bazel", "_typos.toml", "api/BASELINE",
		"go.mod", "go.sum", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "prek.toml",
		"quality/registry.textproto", "tools/bootstrap/bazel.sh", "tools/browser/debs.bzl",
		"tools/checks/verify_body.sh", "tools/ci/policy.json", "tools/hooks/stage.sh",
		"tools/pins/generate.sh", "tools/policy/bazelrc.allow", "tools/proto/defs.bzl",
		"tools/speckit/bootstrap.sh", "tools/verify.sh", "tools/verify_deadline_test.sh",
		"tsconfig.json", "web/app/vite.config.mjs", "web/e2e/playwright.config.mjs",
		"web/lint/BUILD.bazel", "web/lint/eslint.config.mjs",
	}
	ordinary := []string{
		"README.md", "api/template/notes/v1/notes.proto", "bff/BUILD.bazel", "bff/app/notes.go",
		"docs/quality/hooks.md", "specs/006-quality-hooks/spec.md", "tools/README.md",
		"tools/bench/measure.sh", "tools/dev/dev.sh", "web/app/src/App.tsx", "web/e2e/notes.spec.ts",
	}
	judge := func(path string, want bool) {
		from := h.git("rev-parse", "HEAD")
		h.write(path, "changed "+path+"\n")
		h.git("add", "-A")
		h.git("commit", "-q", "-m", path)
		got, err := Escalates(context.Background(), git, registry.GetEscalate(), from, h.git("rev-parse", "HEAD"))
		if err != nil || got != want {
			t.Errorf("%s: escalates %v, want %v (%v)", path, got, want, err)
		}
	}
	for _, path := range policy {
		judge(path, true)
	}
	for _, path := range ordinary {
		judge(path, false)
	}
}

// The digest in a receipt names the same inputs that escalate: a change to one moves it, and an
// ordinary change does not.
func TestPolicyDigestCoversTheEscalationInputs(t *testing.T) {
	h := committed(newHarness(t))
	git := Git{Dir: h.dir, Env: gitEnv()}
	paths := []string{"README.md", "go.sum", "package.json", "pnpm-lock.yaml", "tsconfig.json", "tools/verify.sh", "web/lint/eslint.config.mjs"}
	digest := func() string {
		_, value, err := TestedTree(context.Background(), git, &Inputs{Root: h.dir, All: paths}, realRegistry(t).GetEscalate(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	for _, path := range paths {
		h.write(path, "first\n")
	}
	before := digest()
	h.write("README.md", "second\n")
	if digest() != before {
		t.Error("an ordinary file moved the policy digest")
	}
	for _, path := range paths[1:] {
		h.write(path, "second\n")
		after := digest()
		if after == before {
			t.Errorf("%s did not move the policy digest", path)
		}
		before = after
	}
}

func TestPushWorktreeIsExactlyThePushedCommit(t *testing.T) {
	h := newHarness(t)
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	first := h.git("rev-parse", "HEAD")
	h.write("src.go", "package a\n")
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "second")
	second := h.git("rev-parse", "HEAD")
	git := Git{Dir: h.dir, Env: gitEnv()}
	worktree := filepath.Join(t.TempDir(), "push worktree")
	if err := prepareWorktree(context.Background(), git, worktree, second); err != nil {
		t.Fatal(err)
	}
	h.writeAbs(filepath.Join(worktree, "src.go"), "changed\n", 0o644)
	h.writeAbs(filepath.Join(worktree, "left-behind.go"), "stale\n", 0o644)
	if err := prepareWorktree(context.Background(), git, worktree, first); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"src.go", "left-behind.go"} {
		if _, err := os.Stat(filepath.Join(worktree, absent)); err == nil {
			t.Errorf("%s survived into the next push", absent)
		}
	}
	out, _ := (Git{Dir: worktree, Env: gitEnv()}).Run(context.Background(), nil, "rev-parse", "HEAD")
	if strings.TrimSpace(string(out)) != first {
		t.Errorf("at %s", out)
	}
	if h.git("rev-parse", "--abbrev-ref", "HEAD") != "main" {
		t.Error("the push moved the checkout")
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	if err := prepareWorktree(context.Background(), git, worktree, second); err != nil {
		t.Fatalf("a removed worktree must come back: %v", err)
	}
}

func TestRegistryTextRoundTrips(t *testing.T) {
	registry := &registrypb.Registry{}
	if err := prototext.Unmarshal([]byte(baseCheck), registry); err != nil {
		t.Fatal(err)
	}
	if registry.GetChecks()[0].GetId() != "good" {
		t.Fatal("generated schema mismatch")
	}
}

// The counterexamples below came from independent review of the first draft (spec 006 tasks,
// execution record). They were seen failing against that draft before the repair.

func TestContradictoryBazelResultFailsClosed(t *testing.T) {
	h := newHarness(t)
	fakeBazel(h, `echo "//a:good   PASSED in 1s"; echo "//a:bad   FAILED in 1s"; exit 0`)
	if code := h.run(h.options(bazelCheck("tests", "//a/...", "STAGE_FULL"), registrypb.Stage_STAGE_FULL)); code == ExitOK {
		t.Fatalf("Bazel exit 0 beside a FAILED target passed\n%s", h.out.String())
	}
}

func TestSkippedTargetIsNotAPass(t *testing.T) {
	h := newHarness(t)
	fakeBazel(h, `echo "//a:good   PASSED in 1s"; echo "//a:skip   SKIPPED"; exit 0`)
	if code := h.run(h.options(bazelCheck("tests", "//a/...", "STAGE_FULL"), registrypb.Stage_STAGE_FULL)); code == ExitOK {
		t.Fatalf("a SKIPPED target passed\n%s", h.out.String())
	}
}

func TestEmptyTargetSetAtFullFailsClosed(t *testing.T) {
	h := newHarness(t)
	fakeBazel(h, `exit 4`)
	if code := h.run(h.options(bazelCheck("tests", "//a/...", "STAGE_FULL"), registrypb.Stage_STAGE_FULL)); code != ExitEmptyInput {
		t.Fatalf("exit %d: no tests at full must not pass\n%s", code, h.out.String())
	}
	h = newHarness(t)
	fakeBazel(h, `exit 4`)
	body := bazelCheck("tests", "//a/...", "STAGE_PRE_PUSH, STAGE_FULL")
	if code := h.run(h.options(body, registrypb.Stage_STAGE_PRE_PUSH)); code != ExitOK {
		t.Fatalf("exit %d: an affected set may be empty\n%s", code, h.out.String())
	}
}

func TestMixedBazelFailureKeepsThePassingCheck(t *testing.T) {
	h := newHarness(t)
	fakeBazel(h, `echo "//a:t   PASSED in 1s"; echo "//b:t   FAILED in 1s"; exit 3`)
	body := bazelCheck("first", "//a/...", "STAGE_FULL") + bazelCheck("second", "//b/...", "STAGE_FULL")
	options := h.options(body, registrypb.Stage_STAGE_FULL)
	if code := h.run(options); code != ExitFailed {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	out := h.out.String()
	if !strings.Contains(out, "PASS") || !strings.Contains(out, "FAILED") {
		t.Fatalf("each check must keep its own result:\n%s", out)
	}
}

func TestNamedTargetThatNeverRanIsNotAPass(t *testing.T) {
	h := newHarness(t)
	fakeBazel(h, `echo "//a:other   FAILED in 1s"; exit 3`)
	body := bazelCheck("first", "//a/...", "STAGE_FULL") + bazelCheck("named", "//a:missing", "STAGE_FULL")
	if code := h.run(h.options(body, registrypb.Stage_STAGE_FULL)); code == ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	if out := h.out.String(); strings.Contains(out, "PASS      named") {
		t.Fatalf("a target that never ran passed:\n%s", out)
	}
}

// committed gives a harness a commit, so a receipt has an identity to record.
func committed(h *harness) *harness {
	h.t.Helper()
	h.git("add", "-A")
	h.git("commit", "-q", "-m", "first")
	return h
}

func receiptOf(t *testing.T, path string) Receipt {
	t.Helper()
	var receipt Receipt
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestHungBazelPathCannotHoldTheRun(t *testing.T) {
	saved := bookkeepingBudget
	bookkeepingBudget = 500 * time.Millisecond
	defer func() { bookkeepingBudget = saved }()
	h := committed(newHarness(t))
	pidfile := filepath.Join(t.TempDir(), "pid")
	h.writeAbs(h.bazel, `#!/bin/bash
if [ "$1" = --path ]; then
  setsid sh -c 'trap "" TERM; echo $$ >`+pidfile+`; sleep 300; sleep 300' &
  trap "" TERM
  while :; do sleep 1; done
fi
echo "//a:t   PASSED in 1s"
`, 0o755)
	options := h.options(bazelCheck("tests", "//a/...", "STAGE_FULL"), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	started := time.Now()
	code := h.run(options)
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("final bookkeeping took %s", elapsed)
	}
	if code == ExitOK {
		t.Fatalf("a receipt with no Bazel identity passed\n%s", h.out.String())
	}
	if receipt := receiptOf(t, options.ReceiptPath); receipt.Result == "PASSED" {
		t.Fatal("the receipt says PASSED")
	}
	if pid := readPid(t, pidfile); alive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("the helper's descendant %d survived", pid)
	}
}

func TestHungVersionHelperCannotHoldTheRun(t *testing.T) {
	saved := bookkeepingBudget
	bookkeepingBudget = 500 * time.Millisecond
	defer func() { bookkeepingBudget = saved }()
	h := committed(newHarness(t))
	fakeBazel(h, `echo "//a:t   PASSED in 1s"`)
	tool := filepath.Join(t.TempDir(), "tool")
	h.writeAbs(tool, "#!/bin/sh\ntrap '' TERM\nwhile :; do sleep 1; done\n", 0o755)
	options := h.options(bazelCheck("tests", "//a/...", "STAGE_FULL")+`tools { name: "HUNG" version_args: "--version" }`, registrypb.Stage_STAGE_FULL)
	options.Tools = map[string]string{"HUNG": tool}
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	started := time.Now()
	code := h.run(options)
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("version capture took %s", elapsed)
	}
	if code == ExitOK {
		t.Fatalf("a missing tool version passed\n%s", h.out.String())
	}
}

// A spent context starts nothing. The directory does not exist, so starting anything reports that
// instead: a child's own output could not show this, because a stop can beat its first line.
func TestCancelledContextStartsNoChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	_, err = supervised(ctx, []string{"/bin/true"}, filepath.Join(t.TempDir(), "absent"), nil, output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a spent context tried to start a child: %v", err)
	}
}

func legRegistry(extra string) string {
	return `shards: "alpha" shards: "beta"
checks { id: "a" stages: [STAGE_FULL] classification: CLASSIFICATION_GUARANTEE kind: KIND_BAZEL
 timeout_seconds: 5 witness: "w" targets: ["//..."] shard: "alpha" }
checks { id: "b" stages: [STAGE_FULL] classification: CLASSIFICATION_GUARANTEE kind: KIND_BAZEL
 timeout_seconds: 5 witness: "w" targets: ["//web/e2e/..."] shard: "beta" }
checks { id: "c" stages: [STAGE_FULL] classification: CLASSIFICATION_GUARANTEE kind: KIND_NATIVE
 timeout_seconds: 5 witness: "w" command: ["true"] after_bazel: true postcondition: true }
` + extra
}

func TestLegsValidate(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"neither": {native("n", "true"), "n: is neither owned by a shard nor a postcondition"},
		"both": {native("n", "true", `after_bazel: true postcondition: true shard: "alpha"`),
			"n: is both owned by alpha and a postcondition"},
		"unknown":        {native("n", "true", `shard: "gamma"`), "n: unknown shard gamma"},
		"not after":      {native("n", "true", `postcondition: true`), "n: a postcondition must set after_bazel"},
		"empty leg":      {`shards: "gamma"`, "shard gamma owns no check"},
		"duplicate leg":  {`shards: "alpha"`, "duplicate shard alpha"},
		"not kebab case": {`shards: "Bad_Leg"` + "\n" + native("n", "true", `shard: "Bad_Leg"`), "shard Bad_Leg is not kebab-case"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRegistry([]byte(legRegistry(test.body)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
	if _, err := ParseRegistry([]byte(legRegistry(""))); err != nil {
		t.Fatalf("the control is rejected: %v", err)
	}
}

func TestObligationsCoverEveryLegAndPostcondition(t *testing.T) {
	registry, err := ParseRegistry([]byte(legRegistry(`shards: "gamma"` + "\n" + native("n", "true", `shard: "gamma"`))))
	if err != nil {
		t.Fatal(err)
	}
	got := Obligations(registry)
	want := map[string][]string{"alpha": {"a", "c"}, "beta": {"b", "c"}, "gamma": {"n"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLegPlanSubtractsOnlyWhatItDoesNotContain(t *testing.T) {
	registry, err := ParseRegistry([]byte(legRegistry("")))
	if err != nil {
		t.Fatal(err)
	}
	args := func(shard string) string {
		plan := PlanBazel(registry, registrypb.Stage_STAGE_FULL, shard, nil)
		return strings.Join(plan.Args[slices.Index(plan.Args, "--")+1:], " ")
	}
	if got := args("alpha"); got != "//... -//web/e2e/..." {
		t.Errorf("alpha: %q", got)
	}
	// Subtracting //... here would remove the leg's own targets.
	if got := args("beta"); got != "//web/e2e/..." {
		t.Errorf("beta: %q", got)
	}
	if got := args(""); got != "//... //web/e2e/..." {
		t.Errorf("the whole stage: %q", got)
	}
}

// The real policy is split, every check is accounted for, and only the legs with Bazel work owe the
// postconditions.
func TestRealRegistryPartitionsFull(t *testing.T) {
	registry := realRegistry(t)
	legs := Obligations(registry)
	if len(legs) != 3 {
		t.Fatalf("legs: %v", legs)
	}
	counts := map[string]int{}
	for _, checks := range legs {
		for _, id := range checks {
			counts[id]++
		}
	}
	for _, check := range registry.GetChecks() {
		want := 1
		if check.GetPostcondition() {
			want = 2
		}
		if counts[check.GetId()] != want {
			t.Errorf("%s appears in %d legs, want %d", check.GetId(), counts[check.GetId()], want)
		}
	}
	for _, leg := range []string{"build-test", "e2e"} {
		if !slices.Contains(legs[leg], "test-inventory") || !slices.Contains(legs[leg], "sandbox-evidence") {
			t.Errorf("%s lacks a postcondition: %v", leg, legs[leg])
		}
	}
	if slices.Contains(legs["policy"], "test-inventory") {
		t.Errorf("a leg with no Bazel work has nothing for a postcondition to judge")
	}
}

// A leg with no Bazel work owes no postcondition, and an unrun obligation denies a pass.
func TestLegRunsItsOwnChecksAndDeniesAMissingOne(t *testing.T) {
	h := committed(newHarness(t))
	fakeBazel(h, `echo "//a:t   PASSED in 1s"`)
	body := legRegistry(`shards: "gamma"` + "\n" + native("n", "true", `shard: "gamma"`))
	options := h.options(body, registrypb.Stage_STAGE_FULL)
	options.Shard = "gamma"
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	if code := h.run(options); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
	receipt := receiptOf(t, options.ReceiptPath)
	if receipt.Shard != "gamma" || receipt.Expected != 1 || receipt.Checks[0].Check != "n" {
		t.Fatalf("the receipt is not the leg's: %+v", receipt)
	}
	options.Shard = "alpha"
	if h.run(options) != ExitOK {
		t.Fatalf("the control leg failed\n%s", h.out.String())
	}
	receipt = receiptOf(t, options.ReceiptPath)
	if receipt.Expected != 2 {
		t.Fatalf("leg alpha owes its check and the postcondition: %+v", receipt.Checks)
	}
}

// An instrumented tool warns on stderr, and that warning must not become part of what it printed.
func TestCaptureReadsStdoutOnly(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	script := "#!/bin/sh\necho 'warning: GOCOVERDIR not set' >&2\necho /the/path\necho 'late warning' >&2\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &execution{options: Options{Root: dir, Env: os.Environ()}, scratch: t.TempDir()}
	got, err := e.capture(context.Background(), "helper", []string{helper})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/the/path\n" {
		t.Fatalf("captured %q, want stdout alone", got)
	}
}

// A helper that fails says why, within a bound, and its stdout is not offered as the cause.
func TestCaptureFailureCarriesBoundedStderr(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	long := strings.Repeat("x", 10*MaxCapturedStderr)
	script := "#!/bin/sh\necho stdout-noise\necho '" + long + "' >&2\necho 'the real cause' >&2\nexit 7\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &execution{options: Options{Root: dir, Env: os.Environ()}, scratch: t.TempDir()}
	_, err := e.capture(context.Background(), "helper", []string{helper})
	if err == nil {
		t.Fatal("a failed helper was accepted")
	}
	message := err.Error()
	if !strings.Contains(message, "exited 7") || !strings.Contains(message, "the real cause") {
		t.Fatalf("no cause in %q", message)
	}
	if strings.Contains(message, "stdout-noise") || len(message) > 2*MaxCapturedStderr {
		t.Fatalf("unbounded or mixed diagnostic: %d bytes", len(message))
	}
}

// A tool whose version cannot be read is not recorded as an empty version.
func TestEmptyToolVersionIsAProblem(t *testing.T) {
	h := committed(newHarness(t))
	fakeBazel(h, `echo "//a:t   PASSED in 1s"`)
	tool := filepath.Join(t.TempDir(), "tool")
	h.writeAbs(tool, "#!/bin/sh\necho 'only on stderr' >&2\n", 0o755)
	body := bazelCheck("tests", "//a/...", "STAGE_FULL") + `tools { name: "SILENT" version_args: "--version" }` + "\n"
	options := h.options(body, registrypb.Stage_STAGE_FULL)
	options.Tools = map[string]string{"SILENT": tool}
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	if code := h.run(options); code != ExitNoEvidence {
		t.Fatalf("exit %d, want %d\n%s", code, ExitNoEvidence, h.out.String())
	}
	if problems := strings.Join(receiptOf(t, options.ReceiptPath).Problems, "\n"); !strings.Contains(problems, "silent version") {
		t.Fatalf("the receipt does not name the tool: %q", problems)
	}
}

// One budget covers every helper: once it has gone, no further helper starts, and each one that did
// not run is still named, so a skipped version cannot become a PASSED receipt.
func TestExpiredBudgetStartsNoFurtherHelper(t *testing.T) {
	saved := bookkeepingBudget
	bookkeepingBudget = 500 * time.Millisecond
	defer func() { bookkeepingBudget = saved }()
	h := committed(newHarness(t))
	fakeBazel(h, `echo "//a:t   PASSED in 1s"`)
	starts := filepath.Join(t.TempDir(), "starts")
	tool := filepath.Join(t.TempDir(), "tool")
	h.writeAbs(tool, "#!/bin/sh\necho started >>"+starts+"\ntrap '' TERM\nwhile :; do sleep 1; done\n", 0o755)
	body := bazelCheck("tests", "//a/...", "STAGE_FULL")
	tools := map[string]string{}
	names := []string{"HUNGA", "HUNGB", "HUNGC", "HUNGD"}
	for _, name := range names {
		body += `tools { name: "` + name + `" version_args: "--version" }` + "\n"
		tools[name] = tool
	}
	options := h.options(body, registrypb.Stage_STAGE_FULL)
	options.Tools = tools
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	if code := h.run(options); code == ExitOK {
		t.Fatalf("a receipt with no tool versions passed\n%s", h.out.String())
	}
	data, err := os.ReadFile(starts)
	if err != nil {
		t.Fatal(err)
	}
	if started := strings.Count(string(data), "started"); started != 1 {
		t.Fatalf("%d helpers started; the budget ended during the first", started)
	}
	receipt := receiptOf(t, options.ReceiptPath)
	if receipt.Result == "PASSED" {
		t.Fatal("the receipt says PASSED")
	}
	problems := strings.Join(receipt.Problems, "\n")
	for _, name := range names {
		if !strings.Contains(problems, strings.ToLower(name)+" version") {
			t.Fatalf("the receipt does not name %s:\n%s", name, problems)
		}
	}
}

func TestReceiptWithoutIdentityIsNotPassed(t *testing.T) {
	h := newHarness(t) // No commit: HEAD, and so the source identity, does not exist.
	options := h.options(native("one", "exit 0"), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	code := h.run(options)
	receipt := receiptOf(t, options.ReceiptPath)
	if code == ExitOK || receipt.Result == "PASSED" {
		t.Fatalf("exit %d, result %s, with no revision", code, receipt.Result)
	}
	if !strings.Contains(strings.Join(receipt.Problems, " "), "revision") {
		t.Fatalf("problems: %v", receipt.Problems)
	}
}

func TestReceiptKeepsCleanAndDirtyApart(t *testing.T) {
	h := committed(newHarness(t))
	options := h.options(native("one", "exit 0"), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	if code := h.run(options); code != ExitOK {
		t.Fatalf("clean: exit %d\n%s", code, h.out.String())
	}
	clean := receiptOf(t, options.ReceiptPath)
	if clean.WorktreeDirty || clean.TestedTree != clean.Tree || clean.Result != "PASSED" || clean.PolicyDigest == "" {
		t.Fatalf("clean: %+v", clean)
	}
	h.write("real.go", "package changed\n")
	if code := h.run(options); code != ExitOK {
		t.Fatalf("dirty: exit %d", code)
	}
	dirty := receiptOf(t, options.ReceiptPath)
	if !dirty.WorktreeDirty || dirty.TestedTree == dirty.Tree || dirty.Result != "PASSED" {
		t.Fatalf("dirty: %+v", dirty)
	}
}

func TestRegistryClosedValues(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"unknown numeric stage", strings.Replace(baseCheck, "[STAGE_FULL]", "[STAGE_FULL, 99]", 1), "unknown stage"},
		{"unspecified stage", strings.Replace(baseCheck, "[STAGE_FULL]", "[STAGE_FULL, STAGE_UNSPECIFIED]", 1), "unknown stage"},
		{"repeated stage", strings.Replace(baseCheck, "[STAGE_FULL]", "[STAGE_FULL, STAGE_FULL]", 1), "repeated stage"},
		{"unknown numeric class", strings.Replace(baseCheck, "CLASSIFICATION_GUARANTEE", "7", 1), "missing classification"},
		{"unknown numeric kind", strings.Replace(baseCheck, "KIND_NATIVE", "7", 1), "missing kind"},
		{"empty tool name", baseCheck + `tools { version_args: "x" }`, "tool with no name"},
		{"duplicate tool", baseCheck + `tools { name: "A" } tools { name: "A" }`, "duplicate tool"},
		{"threshold without direction", strings.Replace(baseCheck, `witness: "//:test"`, `witness: "//:test" threshold { value: 1 }`, 1), "threshold"},
		{"threshold not finite", strings.Replace(baseCheck, `witness: "//:test"`, `witness: "//:test" threshold { direction: DIRECTION_MIN value: nan }`, 1), "threshold"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseRegistry([]byte(test.text))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
	if _, err := ParseRegistry([]byte(strings.Replace(baseCheck, `witness: "//:test"`, `witness: "//:test" threshold { direction: DIRECTION_MIN value: 80 }`, 1))); err != nil {
		t.Fatalf("a valid threshold was rejected: %v", err)
	}
}

func manyLines(id string, count int) string {
	return native(id, fmt.Sprintf(`i=1; while [ $i -le %d ]; do echo "distinctive-line-$i"; i=$((i+1)); done; exit 1`, count))
}

func TestOmittedDiagnosticsCanBeRetrieved(t *testing.T) {
	h := committed(newHarness(t))
	options := h.options(manyLines("noisy", 51), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(t.TempDir(), "evidence", "r.json")
	if code := h.run(options); code != ExitFailed {
		t.Fatalf("exit %d", code)
	}
	out := h.out.String()
	if strings.Contains(out, "distinctive-line-51\n") || strings.Contains(out, "distinctive-line-51 ") {
		t.Fatalf("the terminal bound broke:\n%s", out)
	}
	if !strings.Contains(out, "distinctive-line-50 ") {
		t.Fatalf("line 50 is within the bound:\n%s", out)
	}
	receipt := receiptOf(t, options.ReceiptPath)
	retained := receipt.Checks[0].Log
	if retained == "" || !strings.Contains(out, "… 1 more (see "+retained+")") {
		t.Fatalf("the terminal must name the retained file %q:\n%s", retained, out)
	}
	data, err := os.ReadFile(retained)
	if err != nil {
		t.Fatalf("the advertised location is dead: %v", err)
	}
	if !bytes.Contains(data, []byte("distinctive-line-51\n")) {
		t.Fatalf("line 51 is not at the advertised location:\n%s", data)
	}
	info, err := os.Stat(retained)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("a retained log stays private: %v %v", info, err)
	}
}

func TestRetainedLogIsBounded(t *testing.T) {
	saved := maxRetainedLog
	maxRetainedLog = 200
	defer func() { maxRetainedLog = saved }()
	h := committed(newHarness(t))
	options := h.options(manyLines("noisy", 500), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(t.TempDir(), "r.json")
	h.run(options)
	retained := receiptOf(t, options.ReceiptPath).Checks[0].Log
	data, err := os.ReadFile(retained)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) > maxRetainedLog+200 || !bytes.Contains(data, []byte("truncated")) {
		t.Fatalf("%d bytes:\n%s", len(data), data)
	}
}

func TestWithoutAReceiptTheOnlyPointerIsTheRerun(t *testing.T) {
	h := committed(newHarness(t))
	options := h.options(manyLines("noisy", 51), registrypb.Stage_STAGE_FULL)
	h.run(options)
	if out := h.out.String(); !strings.Contains(out, "… 1 more (rerun: rerun-me)") || strings.Contains(out, "(see ") {
		t.Fatalf("a dead pointer:\n%s", out)
	}
}

func TestReceiptPathOwnsNoSiblingDirectory(t *testing.T) {
	dir := t.TempDir()
	h := committed(newHarness(t))
	h.writeAbs(filepath.Join(dir, "report.logs", "user-file"), "keep me\n", 0o644)
	h.writeAbs(filepath.Join(dir, "report.logs-mine", "user-file"), "keep me\n", 0o644)
	options := h.options(manyLines("noisy", 51), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(dir, "report.json")
	h.run(options)
	for _, name := range []string{"report.logs", "report.logs-mine"} {
		if _, err := os.Stat(filepath.Join(dir, name, "user-file")); err != nil {
			t.Fatalf("a path proves no ownership, but %s lost a file: %v", name, err)
		}
	}
}

func TestSameReceiptIsSerialized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.json")
	first := committed(newHarness(t))
	release := filepath.Join(dir, "release")
	body := native("slow", fmt.Sprintf(`touch %q.started; while [ ! -e %q ]; do sleep 0.02; done; echo first-output; exit 1`, release, release))
	options := first.options(body, registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = path
	done := make(chan int, 1)
	go func() {
		code, _ := Run(context.Background(), options)
		done <- code
	}()
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(release + ".started"); err == nil {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the first run never started")
		}
	}
	second := committed(newHarness(t))
	rival := second.options(manyLines("rival", 3), registrypb.Stage_STAGE_FULL)
	rival.ReceiptPath = path
	rival.Deadline = 300 * time.Millisecond
	if code := second.run(rival); code != ExitInterrupted {
		t.Fatalf("a second producer on the same receipt ran alongside the first: exit %d\n%s", code, second.out.String())
	}
	h := first
	h.writeAbs(release, "", 0o644)
	if code := <-done; code != ExitFailed {
		t.Fatalf("the first run: exit %d", code)
	}
	receipt := receiptOf(t, path)
	data, err := os.ReadFile(receipt.Checks[0].Log)
	if err != nil || !bytes.Contains(data, []byte("first-output")) {
		t.Fatalf("the first run's log was disturbed: %v %q", err, data)
	}
}

func TestLogRunsAreBoundedAndOnlyOwnedOnesGo(t *testing.T) {
	dir := t.TempDir()
	h := committed(newHarness(t))
	h.writeAbs(filepath.Join(dir, "r.logs-stranger", "note"), "not ours\n", 0o644)
	var logs []string
	for range keepLogRuns + 3 {
		options := h.options(manyLines("noisy", 2), registrypb.Stage_STAGE_FULL)
		options.ReceiptPath = filepath.Join(dir, "r.json")
		h.run(options)
		logs = append(logs, receiptOf(t, options.ReceiptPath).Checks[0].Log)
		time.Sleep(10 * time.Millisecond)
	}
	entries, err := filepath.Glob(filepath.Join(dir, "r.logs-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != keepLogRuns+1 {
		t.Fatalf("want %d owned runs and the stranger, got %v", keepLogRuns, entries)
	}
	if _, err := os.Stat(logs[len(logs)-1]); err != nil {
		t.Fatalf("the newest log went: %v", err)
	}
	if _, err := os.Stat(logs[0]); err == nil {
		t.Fatal("the oldest log was kept past the bound")
	}
	if _, err := os.Stat(filepath.Join(dir, "r.logs-stranger", "note")); err != nil {
		t.Fatalf("a directory with no owner mark was pruned: %v", err)
	}
}

func TestReceiptWriteUsesAUniqueTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.json")
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(path, &Receipt{Stage: "full"}); err != nil {
		t.Fatalf("a fixed temporary name can be taken: %v", err)
	}
}

func TestAdoptedOrphanIsReapedWhileTheCheckRuns(t *testing.T) {
	// A Bazel client waits for its server to go after a shutdown. Left as a zombie of the supervisor,
	// the server still answers `kill -0`, and the client fails.
	h := newHarness(t)
	pidfile := filepath.Join(t.TempDir(), "pid")
	script := `(setsid sh -c 'echo $$ >` + pidfile + `; sleep 0.1' &)
while [ ! -s ` + pidfile + ` ]; do sleep 0.01; done
pid=$(cat ` + pidfile + `)
i=0
while kill -0 "$pid" 2>/dev/null; do
  i=$((i+1)); [ $i -gt 200 ] && { echo "the orphan is a zombie"; exit 1; }
  sleep 0.01
done`
	if code := h.run(h.options(native("waiter", script), registrypb.Stage_STAGE_FULL)); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, h.out.String())
	}
}
