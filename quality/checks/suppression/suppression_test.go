package suppression

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const manifestName = "fixtures.sha256"

func scanFixture(t *testing.T, name string) []Finding {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return Scan("pkg/"+name, data)
}

func TestSupportedSpellingsAreCaught(t *testing.T) {
	cases := []struct{ fixture, code string }{
		{"S001_bad_upper.go", "S001"},
		{"S002_bad_config.ts", "S002"},
		{"S006_bad_nul.txt", "S006"},
		{"S006_bad_utf16.txt", "S006"},
		{"S006_bad_combined.txt", "S006"},
		{"S006_bad_shell.txt", "S006"},
		{"S006_bad_enable.txt", "S006"},
		{"S007_bad_flake8.py", "S007"},
		{"S007_bad_mypy.py", "S007"},
		{"S007_bad_pylint.py", "S007"},
		{"S007_bad_ruff.py", "S007"},
		{"S009_bad_buildozer.txt", "S009"},
		{"S009_bad_leave.txt", "S009"},
		{"S009_bad_unused.txt", "S009"},
		{"S011_bad_lint.go", "S011"},
		{"S012_bad.ts", "S012"},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			found := false
			for _, f := range scanFixture(t, c.fixture) {
				found = found || f.Code == c.code
			}
			if !found {
				t.Fatalf("want a %s finding", c.code)
			}
		})
	}
}

func TestDirectivesSpanningLinesAreReportedAtTheirStart(t *testing.T) {
	cases := []struct {
		fixture string
		code    string
		line    int
	}{
		{"S002_bad_multiline.ts", "S002", 1},
		{"S002_bad_leading.ts", "S002", 2},
		{"S002_bad_quoted.ts", "S002", 2},
		{"S002_bad_squoted.ts", "S002", 1},
		{"S002_bad_global.ts", "S002", 1},
		{"S002_bad_globals.ts", "S002", 3},
		{"S002_bad_env.ts", "S002", 1},
		{"S005_bad_multiline.ts", "S005", 1},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			got := scanFixture(t, c.fixture)
			if len(got) != 1 || got[0].Code != c.code || got[0].Line != c.line {
				t.Fatalf("got %+v, want one %s finding at line %d", got, c.code, c.line)
			}
		})
	}
}

func TestLegitimateTextIsNotADirective(t *testing.T) {
	for _, fixture := range []string{"S002_good_prose.ts", "S006_good_source.txt", "S007_good_field.yml"} {
		if got := scanFixture(t, fixture); len(got) != 0 {
			t.Errorf("%s: got %+v, want no findings", fixture, got)
		}
	}
}

func TestUnknownFixtureLookalikeIsScanned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "S006_bad.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(Scan("quality/checks/suppression/testdata/S999_good.sh", data)) == 0 {
		t.Fatal("an unknown fixture-looking path was exempted")
	}
}

func TestExemptionLookalikesAreScanned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "S001_bad.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"quality/checks/suppression/rules.go.orig",
		"quality/checks/suppression/rules.go/x.go",
		"quality/checks/suppression/testdata/evil.go",
		"quality/checks/suppression/testdata/sub/S001_bad.go",
		".specify/x/evil.go",
		".claude/skills/speckit-evil/x.go",
	} {
		if len(Scan(path, data)) == 0 {
			t.Errorf("Scan(%q): lookalike was exempted", path)
		}
	}
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

func sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func TestExemptionNeedsPathAndContent(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "S001_bad.go"))
	if err != nil {
		t.Fatal(err)
	}
	enrolled := ".specify/scripts/bash/common.sh"
	scanner := Scanner{Exempt: map[string]string{enrolled: sum(data)}}
	cases := []struct {
		name   string
		path   string
		data   []byte
		exempt bool
	}{
		{"enrolled path and content", enrolled, data, true},
		{"enrolled path, changed content", enrolled, append(data, '\n'), false},
		{"unknown path, same content", ".specify/scripts/bash/other.sh", data, false},
		{"pattern table", "quality/checks/suppression/rules.go", data, true},
		{"ordinary source", "bff/app/notes.go", data, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(scanner.Scan(c.path, c.data)) == 0; got != c.exempt {
				t.Errorf("exempt=%v, want %v", got, c.exempt)
			}
		})
	}
}

// TestFixtureManifestIsExact keeps the enrolled inventory equal to the files on disk. Regenerate
// with: sha256sum quality/checks/suppression/testdata/[Sd]* > .../testdata/fixtures.sha256
func TestFixtureManifestIsExact(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", manifestName))
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "quality/checks/suppression/testdata/"
	present := 0
	for _, entry := range entries {
		if entry.Name() == manifestName {
			continue
		}
		present++
		content, err := os.ReadFile(filepath.Join("testdata", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if enrolled[prefix+entry.Name()] != sum(content) {
			t.Errorf("%s is not enrolled with its current content", entry.Name())
		}
	}
	if present == 0 || present != len(enrolled) {
		t.Errorf("%d fixtures on disk, %d enrolled", present, len(enrolled))
	}
}

func TestManifestParsing(t *testing.T) {
	hash := strings.Repeat("a", 64)
	got, err := ParseManifest([]byte(hash + "  a b.sh\n" + hash + "  c.sh\n"))
	if err != nil || got["a b.sh"] != hash || got["c.sh"] != hash || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"", "not a manifest", "abc  short.sh"} {
		if _, err := ParseManifest([]byte(bad)); err == nil {
			t.Errorf("ParseManifest(%q) accepted malformed input", bad)
		}
	}
}

func TestNULBytesDoNotHideDirectives(t *testing.T) {
	// The directive is assembled so this test file contains no banned literal itself.
	data := append([]byte{0, 1, 2}, []byte("//"+"nolint")...)
	if got := Scan("blob.go", data); len(got) != 1 {
		t.Fatalf("got %v, want the directive found despite NUL bytes", got)
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
