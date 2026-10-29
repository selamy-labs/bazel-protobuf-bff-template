package suppression

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scanFixture(t *testing.T, name string) []Finding {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return Scan("pkg/"+name, data)
}

func TestEachCodeHasDefectAndCorrectedFixture(t *testing.T) {
	for _, rule := range Rules {
		t.Run(rule.Code, func(t *testing.T) {
			bad := scanFixture(t, rule.Code+"_bad"+rule.fixtureExt)
			if len(bad) != 1 || bad[0].Code != rule.Code {
				t.Fatalf("defect fixture: got %v, want exactly one %s finding", bad, rule.Code)
			}
			if bad[0].Line < 1 {
				t.Fatalf("finding has no line number: %+v", bad[0])
			}
			if good := scanFixture(t, rule.Code+"_good"+rule.fixtureExt); len(good) != 0 {
				t.Fatalf("corrected fixture: got %v, want no findings", good)
			}
		})
	}
}

func TestMarkdownMentionsAreNotSuppressions(t *testing.T) {
	if got := scanFixture(t, "doc_mention.md"); len(got) != 0 {
		t.Fatalf("got %v, want no findings in documentation", got)
	}
}

func TestExemptPaths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "S001_bad.go"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path   string
		exempt bool
	}{
		{".specify/scripts/bash/common.sh", true},
		{".claude/skills/speckit-plan/SKILL.md", true},
		{"quality/checks/suppression/testdata/S001_bad.go", true},
		{"quality/checks/suppression/rules.go", true},
		{"quality/checks/suppression/scan.go", false},
		{"bff/app/notes.go", false},
		{".claude/settings.json", false},
	}
	for _, c := range cases {
		got := len(Scan(c.path, data)) == 0
		if got != c.exempt {
			t.Errorf("Scan(%q): exempt=%v, want %v", c.path, got, c.exempt)
		}
	}
}

func TestBinaryContentIsSkipped(t *testing.T) {
	// The directive is assembled so this test file contains no banned literal itself.
	data := append([]byte{0, 1, 2}, []byte("//"+"nolint")...)
	if got := Scan("blob.bin", data); len(got) != 0 {
		t.Fatalf("got %v, want binary content skipped", got)
	}
}

func TestReportIsBoundedAndActionable(t *testing.T) {
	var findings []Finding
	for i := 1; i <= 60; i++ {
		findings = append(findings, Finding{Path: "a.go", Line: i, Code: "S001", Message: "m"})
	}
	var b strings.Builder
	Report(&b, findings, 50)
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 51 {
		t.Fatalf("got %d lines, want 50 findings plus one summary line", len(lines))
	}
	if !strings.HasPrefix(lines[0], "suppression-ban a.go:1: [S001]") || !strings.Contains(lines[0], "(rerun: ") {
		t.Fatalf("diagnostic line format: %q", lines[0])
	}
	if !strings.Contains(lines[50], "10 more") {
		t.Fatalf("summary line: %q", lines[50])
	}
}

func TestParsePathsIsNULDelimited(t *testing.T) {
	got := ParsePaths([]byte("a b.go\x00dir/c.ts\x00\x00"))
	want := []string{"a b.go", "dir/c.ts"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}
