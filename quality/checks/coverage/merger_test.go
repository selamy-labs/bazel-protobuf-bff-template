package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMergeKeepsLcovAndTheRawProfile(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "coverage.dat")
	write(t, filepath.Join(dir, "go_coverage.1.dat"), "SF:a.go\nDA:1,1\nend_of_record\n")
	write(t, filepath.Join(dir, "_rules_js_report.lcov"), "SF:b.js\nDA:1,0\nend_of_record")
	write(t, output+".cover", "mode: set\nexample.com/a.go:1.1,2.2 3 1\n")
	if err := Merge(dir, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"SF:a.go\n", "SF:b.js\n", ProfilePrefix + "example.com/a.go:1.1,2.2 3 1\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "mode:") {
		t.Errorf("the profile's header leaked into the report:\n%s", text)
	}
}

func TestMergeLeavesATestThatWroteNothingEmpty(t *testing.T) {
	output := filepath.Join(t.TempDir(), "coverage.dat")
	if err := Merge(t.TempDir(), output); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(output); err != nil || len(data) != 0 {
		t.Fatalf("got %q, %v", data, err)
	}
}

func TestMergeDoesNotReadItsOwnOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "_coverage.dat")
	write(t, output, "stale\n")
	if err := Merge(dir, output); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(output); strings.Contains(string(data), "stale") {
		t.Fatal("the output merged itself")
	}
}

func TestCombineJoinsTheListedReports(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	write(t, first, "SF:a\n")
	write(t, second, "SF:b\n")
	list := filepath.Join(dir, "reports")
	write(t, list, first+"\n"+second+"\n")
	output := filepath.Join(dir, "combined")
	if err := Combine(list, output); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(output); string(data) != "SF:a\nSF:b\n" {
		t.Fatalf("got %q", data)
	}
	write(t, list, filepath.Join(dir, "absent")+"\n")
	if err := Combine(list, output); err == nil {
		t.Fatal("a missing report was accepted")
	}
}
