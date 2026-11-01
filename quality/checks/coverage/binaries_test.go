package coverage

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func binary(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv(name))
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMergerCommand(t *testing.T) {
	merger := binary(t, "MERGER")
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.dat"), "SF:a\nend_of_record\n")
	output := filepath.Join(t.TempDir(), "out")
	if out, err := exec.Command(merger, "--coverage_dir="+dir, "--output_file="+output, "--filter_sources=x").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	list := filepath.Join(dir, "reports")
	write(t, list, output+"\n")
	combined := filepath.Join(t.TempDir(), "combined")
	if out, err := exec.Command(merger, "--reports_file="+list, "--output_file="+combined).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if data, _ := os.ReadFile(combined); string(data) != "SF:a\nend_of_record\n" {
		t.Fatalf("got %q", data)
	}
	if err := exec.Command(merger).Run(); err == nil {
		t.Fatal("no arguments were accepted")
	}
}

// repository makes a real Git repository holding a contract and the given files.
func repository(t *testing.T, config map[string]any, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "quality"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "quality/coverage_floors.json"), string(data))
	for name, text := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, name), text)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func simple(floor float64) map[string]any {
	return map[string]any{
		"languages": map[string]any{"go": []string{".go"}},
		"excludes":  []any{map[string]any{"pattern": "^fixtures/", "reason": "test-data"}},
		"profiles":  []any{map[string]any{"name": "go", "language": "go", "metric": Statements, "include": "^src/", "floor": floor}},
	}
}

func floors(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	command := exec.Command(binary(t, "FLOORS"), args...)
	command.Stdout, command.Stderr = &out, &out
	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), code
}

func checkRun(t *testing.T, root, exit, log string, reports map[string]string) (string, int) {
	t.Helper()
	logs := t.TempDir()
	for target, text := range reports {
		dir := filepath.Join(logs, target)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, "coverage.dat"), text)
	}
	path := filepath.Join(t.TempDir(), "log")
	write(t, path, log)
	return floors(t, "check", "--root="+root, "--log="+path, "--testlogs="+logs, "--bazel-exit="+exit)
}

func TestFloorsCheckCommand(t *testing.T) {
	root := repository(t, simple(50), map[string]string{"src/a.go": "package a\n"})
	good := map[string]string{"a/t": ProfilePrefix + "src/a.go:1.1,2.2 1 1\n"}
	if out, code := checkRun(t, root, "0", "//a:t   PASSED in 1s\n", good); code != 0 || !strings.Contains(out, "measurement: go: 100.0% statements") {
		t.Fatalf("the control: exit %d\n%s", code, out)
	}
	passing := "//a:t   PASSED in 1s\n"
	cases := map[string]struct {
		exit, log string
		reports   map[string]string
		want      string
	}{
		// Every summary line passes and the report is good. Only the invocation's exit says otherwise.
		"exit 3, only passing summaries": {"3", passing, good, "the Bazel invocation exited 3"},
		"interrupted":                    {"interrupted", passing, good, "the Bazel invocation exited interrupted"},
		"not run":                        {"not-run", passing, good, "the Bazel invocation exited not-run"},
		"exit 3 with a failed summary":   {"3", passing + "//b:t   FAILED in 1s\n", good, "the Bazel invocation exited 3"},
		"exit 0 with a failed summary":   {"0", passing + "//b:t   FAILED in 1s\n", good, "//b:t is FAILED"},
		"no test":                        {"0", "", good, "the log shows no test"},
		"report of no target":            {"0", "//z:t   PASSED in 1s\n", good, "src/a.go is absent from the report"},
		"truncated":                      {"0", passing, map[string]string{"a/t": "SF:x.js\nDA:1,1\n"}, "truncated"},
		"no countable data":              {"0", passing, map[string]string{"a/t": ProfilePrefix + "src/a.go:1.1,2.2 0 0\n"}, "no countable data"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			out, code := checkRun(t, root, test.exit, test.log, test.reports)
			if code != 1 || !strings.Contains(out, test.want) {
				t.Fatalf("exit %d, want 1 with %q\n%s", code, test.want, out)
			}
		})
	}
	if out, code := floors(t, "check", "--root="+root, "--log=/dev/null", "--testlogs="+root); code != 2 {
		t.Fatalf("no invocation outcome was given: exit %d\n%s", code, out)
	}
}

// A delete and an add, as real Git commits, judged by the real command. There is no move detection:
// the new path is enrolled like any other.
func TestFloorsCompareJudgesARelocationByEnrollment(t *testing.T) {
	base := repository(t, simple(50), map[string]string{"src/a.go": "package a\n", "src/b.go": "package a\n", "fixtures/keep.txt": "x\n"})
	for name, test := range map[string]struct {
		target string
		code   int
		want   string
	}{
		"inside a profile":         {"src/moved/b.go", 0, ""},
		"into an existing exclude": {"fixtures/b.go", 0, ""},
		"outside every profile":    {"elsewhere/b.go", 1, "unenrolled source: elsewhere/b.go"},
	} {
		t.Run(name, func(t *testing.T) {
			proposal := t.TempDir()
			git(t, proposal, "clone", "-q", base, ".")
			git(t, proposal, "rm", "-q", "src/b.go")
			if err := os.MkdirAll(filepath.Dir(filepath.Join(proposal, test.target)), 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(proposal, test.target), "package a\n")
			git(t, proposal, "add", "-A")
			git(t, proposal, "commit", "-q", "-m", "relocate")
			out, code := floors(t, "compare", "--base-root="+base, "--root="+proposal)
			if code != test.code || !strings.Contains(out, test.want) {
				t.Fatalf("exit %d, want %d with %q\n%s", code, test.code, test.want, out)
			}
		})
	}
}

func TestFloorsCompareCommand(t *testing.T) {
	// Split, so this file does not itself carry the directives it plants.
	tsDirective, goDirective := "// @ts-"+"ignore\n", "//no"+"lint\n"
	base := repository(t, simple(50), map[string]string{"src/a.go": "package a\n", "fixtures/x.ts": tsDirective})
	if out, code := floors(t, "compare", "--base-root="+base, "--root="+base); code != 0 {
		t.Fatalf("the control: exit %d\n%s", code, out)
	}
	proposal := repository(t, simple(40), map[string]string{"src/a.go": "package a\n" + goDirective, "fixtures/x.ts": tsDirective})
	out, code := floors(t, "compare", "--base-root="+base, "--root="+proposal)
	if code != 1 || !strings.Contains(out, "floor lowered") || !strings.Contains(out, "src/a.go:2: S001") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if strings.Contains(out, "fixtures/x.ts") {
		t.Fatalf("the base's test data was scanned:\n%s", out)
	}
	if out, code := floors(t, "compare", "--base-root="+t.TempDir(), "--root="+proposal); code != 2 {
		t.Fatalf("a base with no contract: exit %d\n%s", code, out)
	}
	if _, code := floors(t, "nonsense"); code != 2 {
		t.Fatalf("an unknown command: exit %d", code)
	}
}
