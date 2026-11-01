package coverage

import (
	"path/filepath"
	"strings"
	"testing"
)

func contract(t *testing.T, mutate func(*Config)) *Config {
	t.Helper()
	config := &Config{
		Languages: map[string][]string{"go": {".go"}, "typescript": {".ts"}},
		GoModules: map[string]string{"example.com/m/": ""},
		Excludes:  []Exclude{{Pattern: `_test\.go$`, Reason: "test-code"}, {Pattern: `^kept/excluded/`, Reason: "test-data"}},
		Profiles: []Profile{
			{Name: "go", Language: "go", Metric: Statements, Include: `^src/`, Floor: 50},
			{Name: "ts", Language: "typescript", Metric: CompiledJSLines, Include: `^client/`},
			{Name: "browser", Language: "typescript", Metric: Unmeasured, Include: `^app/`},
		},
	}
	if mutate != nil {
		mutate(config)
	}
	if err := config.prepare(); err != nil {
		t.Fatal(err)
	}
	return config
}

const goodReport = "SF:client/a.js\nDA:1,1\nDA:2,0\nBRDA:1,0,0,1\nBRDA:1,0,1,0\nend_of_record\n" +
	ProfilePrefix + "example.com/m/src/a.go:1.1,2.2 3 1\n" + ProfilePrefix + "example.com/m/src/a.go:3.1,4.2 1 0\n"

func measured(t *testing.T, text string) *Report {
	t.Helper()
	report := NewReport()
	if err := report.Add("t", text); err != nil {
		t.Fatal(err)
	}
	return report
}

func problems(t *testing.T, config *Config, files []string, text string) string {
	t.Helper()
	_, found := config.Check(files, measured(t, text))
	return strings.Join(found, "\n")
}

func TestCheckPassesAndLabelsEachMetric(t *testing.T) {
	results, found := contract(t, nil).Check([]string{"src/a.go", "src/a_test.go", "client/a.ts", "app/b.ts"}, measured(t, goodReport))
	if len(found) != 0 {
		t.Fatalf("the control failed: %v", found)
	}
	text := ""
	for _, result := range results {
		text += result.String() + "\n"
	}
	for _, want := range []string{"go: 75.0% statements (3 of 4), floor 50.0%, branches unmeasured", "ts: 50.0% compiled-js-lines (1 of 2), floor 0.0%, branches 1 of 2", "browser: unmeasured, 1 files, no data"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestCheckFindings(t *testing.T) {
	files := []string{"src/a.go", "client/a.ts"}
	cases := map[string]struct {
		files  []string
		report string
		want   string
	}{
		"unenrolled":     {append(files, "elsewhere/b.go"), goodReport, "unenrolled source: elsewhere/b.go"},
		"absent":         {append(files, "src/b.go"), goodReport, "go: src/b.go is absent from the report"},
		"empty report":   {files, "", "is absent from the report"},
		"placeholder":    {files, ProfilePrefix + "example.com/m/src/a.go:1.1,2.2 1 1\nSF:client/a.ts\nDA:1,0\nend_of_record\n", "ts: client/a.ts is absent from the report"},
		"below floor":    {files, "SF:client/a.js\nDA:1,1\nend_of_record\n" + ProfilePrefix + "example.com/m/src/a.go:1.1,2.2 1 0\n", "go: 0.0% is below its floor of 50.0%"},
		"no source":      {[]string{"client/a.ts"}, goodReport, "go: measures no files"},
		"lcov not stmts": {files, "SF:src/a.go\nDA:1,1\nend_of_record\nSF:client/a.js\nDA:1,1\nend_of_record\n", "go: src/a.go is absent from the report"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got := problems(t, contract(t, nil), test.files, test.report); !strings.Contains(got, test.want) {
				t.Fatalf("want %q in:\n%s", test.want, got)
			}
		})
	}
}

func TestAbsentDataIsNeverCovered(t *testing.T) {
	results, _ := contract(t, nil).Check([]string{"src/a.go", "src/b.go"}, measured(t, ProfilePrefix+"example.com/m/src/a.go:1.1,2.2 1 1\n"))
	if results[0].Total != 1 {
		t.Fatalf("an absent file was counted: %+v", results[0])
	}
}

func TestTruncatedReportsDoNotParse(t *testing.T) {
	for name, text := range map[string]string{
		"open record": "SF:a.js\nDA:1,1\n",
		"two opens":   "SF:a.js\nSF:b.js\nend_of_record\n",
		"bad line":    "SF:a.js\nDA:x,1\nend_of_record\n",
		"orphan line": "DA:1,1\n",
		"cut profile": ProfilePrefix + "example.com/m/a.go:1.1,2\n",
		"orphan end":  "end_of_record\n",
		"bad branch":  "SF:a.js\nBRDA:1,0\nend_of_record\n",
	} {
		if err := NewReport().Add("t", text); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestReportsMergeAcrossTests(t *testing.T) {
	report := NewReport()
	for _, text := range []string{ProfilePrefix + "m/a.go:1.1,2.2 2 0\nSF:a.js\nDA:1,0\nend_of_record\n", ProfilePrefix + "m/a.go:1.1,2.2 2 1\nSF:a.js\nDA:1,3\nend_of_record\n"} {
		if err := report.Add("t", text); err != nil {
			t.Fatal(err)
		}
	}
	if !report.Statements["m/a.go"]["1.1,2.2"].covered || !report.Lines["a.js"][1] {
		t.Fatalf("a later test's coverage was lost: %+v", report)
	}
}

func TestConfigRejects(t *testing.T) {
	cases := map[string]func(*Config){
		"unlisted reason":    func(c *Config) { c.Excludes[0].Reason = "no test yet" },
		"empty reason":       func(c *Config) { c.Excludes[0].Reason = "" },
		"unknown metric":     func(c *Config) { c.Profiles[0].Metric = "lines" },
		"unknown language":   func(c *Config) { c.Profiles[0].Language = "python" },
		"repeated name":      func(c *Config) { c.Profiles[1].Name = "go" },
		"floored unmeasured": func(c *Config) { c.Profiles[2].Floor = 10 },
	}
	for name, mutate := range cases {
		config := &Config{
			Languages: map[string][]string{"go": {".go"}, "typescript": {".ts"}},
			Excludes:  []Exclude{{Pattern: "x", Reason: "test-code"}},
			Profiles: []Profile{
				{Name: "go", Language: "go", Metric: Statements, Include: "a"},
				{Name: "ts", Language: "typescript", Metric: CompiledJSLines, Include: "b"},
				{Name: "browser", Language: "typescript", Metric: Unmeasured, Include: "c"},
			},
		}
		mutate(config)
		if config.prepare() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.json")
	write(t, path, `{"floors":{}}`)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("got %v", err)
	}
}

func TestCompareAllows(t *testing.T) {
	base := contract(t, nil)
	files := []string{"src/a.go", "src/b.go"}
	cases := map[string]struct {
		mutate func(*Config)
		files  []string
	}{
		"same":           {nil, files},
		"floor raised":   {func(c *Config) { c.Profiles[0].Floor = 60 }, files},
		"new file":       {nil, append(files, "src/c.go")},
		"deleted file":   {nil, files[:1]},
		"moved inside":   {nil, []string{"src/a.go", "src/moved/b.go"}},
		"include gained": {func(c *Config) { c.Profiles[0].Include = `^(src|lib)/` }, append(files, "lib/c.go")},
		"new profile": {func(c *Config) {
			c.Profiles = append(c.Profiles, Profile{Name: "new", Language: "go", Metric: Statements, Include: `^new/`})
		}, files},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Compare(base, files, contract(t, test.mutate), test.files); len(got) != 0 {
				t.Fatalf("a non-weakening change failed: %v", got)
			}
		})
	}
}

func TestCompareRejects(t *testing.T) {
	base := contract(t, nil)
	files := []string{"src/a.go", "src/b.go"}
	cases := map[string]struct {
		mutate func(*Config)
		want   string
	}{
		"floor lowered":    {func(c *Config) { c.Profiles[0].Floor = 40 }, "go: floor lowered from 50.0 to 40.0"},
		"profile removed":  {func(c *Config) { c.Profiles = c.Profiles[1:] }, "go: the profile was removed"},
		"metric changed":   {func(c *Config) { c.Profiles[0].Metric = BlockSpanLines }, "go: metric changed"},
		"made unmeasured":  {func(c *Config) { c.Profiles[0].Metric = Unmeasured; c.Profiles[0].Floor = 0 }, "go: metric changed"},
		"language changed": {func(c *Config) { c.Profiles[0].Language = "typescript" }, "go: language changed"},
		"exclude gained": {func(c *Config) { c.Excludes = append(c.Excludes, Exclude{`^src/b`, "test-data"}) },
			"go: narrowed: src/b.go left the measured source set"},
		"include lost": {func(c *Config) { c.Profiles[0].Include = `^src/a` }, "go: narrowed: src/b.go"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(Compare(base, files, contract(t, test.mutate), files), "\n")
			if !strings.Contains(got, test.want) {
				t.Fatalf("want %q in: %s", test.want, got)
			}
		})
	}
}

// A relocation has no surviving path, so Compare has nothing to say about it: enrollment judges the
// new path. Moving into an existing exclude is the stated limit (spec 007).
func TestRelocationIsJudgedByEnrollment(t *testing.T) {
	base := contract(t, nil)
	config := contract(t, nil)
	moved := []string{"src/a.go", "elsewhere/b.go"}
	if got := Compare(base, []string{"src/a.go", "src/b.go"}, config, moved); len(got) != 0 {
		t.Fatalf("Compare judged a move: %v", got)
	}
	if got := problems(t, config, moved, goodReport); !strings.Contains(got, "unenrolled source: elsewhere/b.go") {
		t.Fatalf("the new path was not named: %s", got)
	}
	limit := []string{"src/a.go", "kept/excluded/b.go"}
	if got := problems(t, config, limit, goodReport); strings.Contains(got, "kept/excluded") {
		t.Fatalf("the stated limit changed, so update the spec: %s", got)
	}
}
