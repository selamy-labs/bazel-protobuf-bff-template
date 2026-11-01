package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// State is a check's terminal result (data-model.md).
type State string

// The terminal states. Only passed and not-applicable are non-blocking.
const (
	Passed        State = "PASSED"
	Failed        State = "FAILED"
	TimedOut      State = "TIMED_OUT"
	Cancelled     State = "CANCELLED"
	Errored       State = "ERROR"
	NotApplicable State = "SKIPPED_NOT_APPLICABLE"
	Skipped       State = "SKIPPED"
	NotRun        State = "NOT_RUN"
)

// The runner's exit codes, disjoint from Bazel's (contracts/check-registry.md).
const (
	ExitOK          = 0
	ExitFailed      = 1
	ExitConfig      = 2
	ExitInterrupted = 10
	ExitNoEvidence  = 11
	ExitEmptyInput  = 12
)

// MaxDiagnostics bounds what one check prints.
const MaxDiagnostics = 50

// Result is one check's outcome.
type Result struct {
	Check      string  `json:"check"`
	State      State   `json:"result"`
	Seconds    float64 `json:"seconds"`
	Blocking   bool    `json:"blocking"`
	Inputs     int     `json:"inputs"`
	Reason     string  `json:"reason,omitempty"`
	NoEvidence bool    `json:"no_evidence,omitempty"`
	EmptyInput bool    `json:"empty_input,omitempty"`
	output     string
}

// ExitCode folds results into one code. An interruption outranks an empty input, which outranks
// missing evidence, which outranks a failure.
func ExitCode(results []Result) int {
	code := ExitOK
	rank := map[int]int{ExitOK: 0, ExitFailed: 1, ExitNoEvidence: 2, ExitEmptyInput: 3, ExitInterrupted: 4}
	raise := func(candidate int) {
		if rank[candidate] > rank[code] {
			code = candidate
		}
	}
	for _, result := range results {
		switch {
		case result.State == Passed || result.State == NotApplicable || result.State == Skipped:
		case result.State == TimedOut || result.State == Cancelled:
			raise(ExitInterrupted)
		case !result.Blocking:
		case result.EmptyInput:
			raise(ExitEmptyInput)
		case result.NoEvidence:
			raise(ExitNoEvidence)
		default:
			raise(ExitFailed)
		}
	}
	return code
}

var located = regexp.MustCompile(`^(\S+?):(\d+)(?::(\d+))?:\s*(.*)$`)

// Diagnostics renders output in the contract's line format, at most MaxDiagnostics lines. A line
// with no position keeps its text, because a repair loop can still use it.
func Diagnostics(check, output, rerun, receipt string) []string {
	var lines []string
	for line := range strings.Lines(output) {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	shown := lines
	if len(lines) > MaxDiagnostics {
		shown = lines[:MaxDiagnostics]
	}
	rendered := make([]string, 0, len(shown)+1)
	for _, line := range shown {
		if match := located.FindStringSubmatch(line); match != nil {
			column := match[3]
			if column == "" {
				column = "1"
			}
			line = fmt.Sprintf("%s:%s:%s: %s", match[1], match[2], column, match[4])
		}
		rendered = append(rendered, fmt.Sprintf("%s %s (rerun: %s)", check, line, rerun))
	}
	if len(lines) > len(shown) {
		rendered = append(rendered, fmt.Sprintf("… %d more (see %s)", len(lines)-len(shown), receipt))
	}
	if len(rendered) == 0 {
		rendered = append(rendered, fmt.Sprintf("%s the check failed and printed nothing (rerun: %s)", check, rerun))
	}
	return rendered
}

// Receipt is the evidence a stage leaves (contracts/evidence-receipt.md).
type Receipt struct {
	SchemaVersion  int               `json:"schema_version"`
	Stage          string            `json:"stage"`
	Requested      string            `json:"requested_stage"`
	Revision       string            `json:"revision"`
	Tree           string            `json:"tree"`
	TestedTree     string            `json:"tested_tree"`
	WorktreeDirty  bool              `json:"worktree_dirty"`
	PolicyDigest   string            `json:"policy_digest"`
	RegistryVer    string            `json:"registry_version"`
	Producer       string            `json:"producer"`
	Isolation      string            `json:"isolation"`
	SandboxSummary string            `json:"sandbox_summary"`
	Offline        bool              `json:"offline"`
	TestCached     bool              `json:"test_results_cached"`
	Result         string            `json:"result"`
	StartedAt      string            `json:"started_at"`
	FinishedAt     string            `json:"finished_at"`
	Host           map[string]string `json:"host"`
	BazelSHA256    string            `json:"bazel_sha256"`
	Tools          map[string]string `json:"tools"`
	Inventory      map[string]int    `json:"inventory"`
	Expected       int               `json:"expected_checks"`
	Checks         []Result          `json:"checks"`
}

// WriteReceipt writes through a temporary file, so a reader never sees a partial record.
func WriteReceipt(path string, receipt *Receipt) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// PrintResult writes one check's line and, if it blocks or failed, its diagnostics.
func PrintResult(out io.Writer, result Result, rerun, receipt string) {
	label := string(result.State)
	switch result.State {
	case Passed:
		label = "PASS"
	case Skipped, NotApplicable:
		label = "SKIP"
	}
	_, _ = fmt.Fprintf(out, "%-9s %-20s %6.1fs", label, result.Check, result.Seconds)
	if result.Reason != "" {
		_, _ = fmt.Fprintf(out, "  %s", result.Reason)
	}
	_, _ = fmt.Fprintln(out)
	if result.State == Passed || result.State == Skipped || result.State == NotApplicable {
		return
	}
	for _, line := range Diagnostics(result.Check, result.output, rerun, receipt) {
		_, _ = fmt.Fprintln(out, "    "+line)
	}
}
