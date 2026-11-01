package runner

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// A measurement summary stays small: it says what was measured, and it is not a tool log.
const (
	measurementPrefix = "measurement: "
	MaxMeasurements   = 32
	MaxMeasurementLen = 200
)

// Result is one check's outcome.
type Result struct {
	Check    string  `json:"check"`
	State    State   `json:"result"`
	Seconds  float64 `json:"seconds"`
	Blocking bool    `json:"blocking"`
	Inputs   int     `json:"inputs"`
	// Log is the retained output of a check that did not pass, beside the receipt.
	Log        string `json:"log,omitempty"`
	Reason     string `json:"reason,omitempty"`
	NoEvidence bool   `json:"no_evidence,omitempty"`
	// Measurements is the bounded summary of a check that declares one.
	Measurements []string `json:"measurements,omitempty"`
	EmptyInput   bool     `json:"empty_input,omitempty"`
	output       string
}

// Measurements picks a check's summary out of its output, within the bound. Overflow is named, so a
// cut summary cannot read as a complete one.
func Measurements(output string) []string {
	var kept []string
	extra := 0
	for line := range strings.Lines(output) {
		text, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), measurementPrefix)
		switch {
		case !ok:
		case len(kept) >= MaxMeasurements:
			extra++
		default:
			kept = append(kept, text[:min(len(text), MaxMeasurementLen)])
		}
	}
	if extra > 0 {
		kept = append(kept, fmt.Sprintf("… %d more", extra))
	}
	return kept
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
// with no position keeps its text, because a repair loop can still use it. Lines beyond the bound are
// pointed to only where they exist: the retained log if there is one, or else the rerun command.
func Diagnostics(check, output, rerun, retained string) []string {
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
		pointer := "rerun: " + rerun
		if retained != "" {
			pointer = "see " + retained
		}
		rendered = append(rendered, fmt.Sprintf("… %d more (%s)", len(lines)-len(shown), pointer))
	}
	if len(rendered) == 0 {
		rendered = append(rendered, fmt.Sprintf("%s the check failed and printed nothing (rerun: %s)", check, rerun))
	}
	return rendered
}

// Receipt is the evidence a stage leaves (contracts/evidence-receipt.md).
type Receipt struct {
	SchemaVersion int    `json:"schema_version"`
	Stage         string `json:"stage"`
	Requested     string `json:"requested_stage"`
	Shard         string `json:"shard,omitempty"`
	Revision      string `json:"revision"`
	Tree          string `json:"tree"`
	TestedTree    string `json:"tested_tree"`
	WorktreeDirty bool   `json:"worktree_dirty"`
	PolicyDigest  string `json:"policy_digest"`
	// The policy base, how it was chosen, and why it could not be used. None of these claims review.
	BaseRevision     string `json:"base_revision,omitempty"`
	BaseSource       string `json:"base_source,omitempty"`
	BaseRefusal      string `json:"base_refusal,omitempty"`
	BasePolicyDigest string `json:"base_policy_digest,omitempty"`
	RegistryVer      string `json:"registry_version"`
	Producer         string `json:"producer"`
	// Attempt is the hosted run attempt, and ClaimLayer says which kind of evidence this is.
	Attempt        string            `json:"attempt,omitempty"`
	ClaimLayer     string            `json:"claim_layer"`
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
	// Problems names each identity this receipt could not establish. Any entry denies PASSED.
	Problems []string `json:"problems,omitempty"`
}

// WriteReceipt writes through a temporary file of its own, so a reader never sees a partial record
// and no name is shared with another writer.
func WriteReceipt(path string, receipt *Receipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// logOwnerMark marks a directory this runner created. A receipt's path proves nothing about what
// sits beside it, so only a directory holding this mark may ever be removed.
const logOwnerMark = ".quality-run-logs"

// keepLogRuns bounds how many runs of one receipt path keep logs: at most this many directories of
// at most maxRetainedLog per check. The next run on that path prunes, under the receipt's lock.
const keepLogRuns = 5

func logPrefix(receipt string) string {
	return strings.TrimSuffix(filepath.Base(receipt), ".json") + ".logs-"
}

// newLogDirectory makes one run's private directory, with a name no other run can hold.
func newLogDirectory(receipt string) (string, error) {
	directory, err := os.MkdirTemp(filepath.Dir(receipt), logPrefix(receipt))
	if err != nil {
		return "", err
	}
	return directory, os.WriteFile(filepath.Join(directory, logOwnerMark), nil, 0o600)
}

// pruneLogDirectories removes the oldest marked directories of this receipt path so that a new one
// stays within keepLogRuns. The caller holds the receipt's lock, so none belongs to an active run.
func pruneLogDirectories(receipt string) error {
	entries, err := os.ReadDir(filepath.Dir(receipt))
	if err != nil {
		return err
	}
	type owned struct {
		path    string
		modTime int64
	}
	var runs []owned
	for _, entry := range entries {
		path := filepath.Join(filepath.Dir(receipt), entry.Name())
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), logPrefix(receipt)) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(path, logOwnerMark)); err != nil {
			continue
		}
		if info, err := entry.Info(); err == nil {
			runs = append(runs, owned{path, info.ModTime().UnixNano()})
		}
	}
	slices.SortFunc(runs, func(a, b owned) int { return cmp.Compare(a.modTime, b.modTime) })
	for len(runs) >= keepLogRuns {
		if err := os.RemoveAll(runs[0].path); err != nil {
			return err
		}
		runs = runs[1:]
	}
	return nil
}

// RetainLog keeps a failed check's output in its run's directory, so the terminal's bound hides
// nothing for good. It holds what the terminal would have shown in full, so it carries the same
// redaction, and it is private to this user. Size is bounded, and a cut says so.
func RetainLog(directory, check, output string) (string, error) {
	path := filepath.Join(directory, check+".log")
	if int64(len(output)) > maxRetainedLog {
		output = output[:maxRetainedLog] + fmt.Sprintf("\n[truncated at %d bytes; rerun the check for the rest]\n", maxRetainedLog)
	}
	return path, os.WriteFile(path, []byte(output), 0o600)
}

// PrintResult writes one check's line and, if it blocks or failed, its diagnostics.
func PrintResult(out io.Writer, result Result, rerun string) {
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
	for _, line := range Diagnostics(result.Check, result.output, rerun, result.Log) {
		_, _ = fmt.Fprintln(out, "    "+line)
	}
}
