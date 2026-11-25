package mutation

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
)

func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return full
}

func sampleFloors() *Floors {
	return &Floors{
		MeasuredAt:         "abc123",
		EquivalentsPath:    "quality/mutation_equivalents.json",
		TimeoutCoefficient: 100,
		Packages: []PackageConfig{
			{
				Name:      "bff/app",
				Module:    "example.com/bfftemplate",
				GoVersion: "1.27.4",
				Package:   "./bff/app",
				Sources:   []string{"bff/app", "bff/ports"},
				Floor:     80,
			},
		},
	}
}

func TestLoadFloorsValid(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := writeFile(t, dir, "floors.json", `{
		"measured_at": "rev1",
		"equivalents": "eq.json",
		"timeout_coefficient": 100,
		"packages": [{"name":"pkg/a","module":"example.com/m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]
	}`)

	// Act
	got, err := LoadFloors(path)

	// Assert
	if err != nil || len(got.Packages) != 1 {
		t.Fatalf("LoadFloors valid: got %+v, err %v; want 1 package and nil err", got, err)
	}
}

func TestLoadFloorsRejectsInvalid(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"missing measured_at", `{"equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "measured_at"},
		{"missing equivalents", `{"measured_at":"r1","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "equivalents"},
		{"zero timeout", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":0,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "timeout_coefficient"},
		{"empty packages", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[]}`, "at least one package"},
		{"duplicate package", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100},{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "listed twice"},
		{"missing name", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "package name is required"},
		{"missing module", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "requires module"},
		{"empty sources", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":[],"floor":100}]}`, "source directory"},
		{"traversal equivalents", `{"measured_at":"r1","equivalents":"../eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":100}]}`, "clean relative path"},
		{"traversal source", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["../a"],"floor":100}]}`, "clean relative path"},
		{"traversal strip_prefix", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","strip_prefix":"../q/","sources":["a"],"floor":100}]}`, "strip_prefix"},
		{"duplicate source", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a","a"],"floor":100}]}`, "duplicate source"},
		{"duplicate mirror destination", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","strip_prefix":"quality/","sources":["quality/a","a"],"floor":100}]}`, "duplicate mirror destination"},
		{"floor above 100", `{"measured_at":"r1","equivalents":"eq.json","timeout_coefficient":10,"packages":[{"name":"a","module":"m","go_version":"1.27.4","package":"./a","sources":["a"],"floor":101}]}`, "between 0 and 100"},
		{"unknown field", `{"measured_at":"r1","extra":1}`, "unknown field"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			path := writeFile(t, t.TempDir(), "floors.json", tc.content)

			// Act
			_, err := LoadFloors(path)

			// Assert
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("case %s: err=%v; want substring %q", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestLoadEquivalentsRejectsInvalid(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"empty proof", `{"equivalents":[{"id":"EQ-1","package":"bff/app","location":"notes.go:10:5","mutator":"ARITHMETIC_BASE","proof":""}]}`, "non-empty id, mutator, and proof"},
		{"unenrolled package", `{"equivalents":[{"id":"EQ-1","package":"other/pkg","location":"notes.go:10:5","mutator":"ARITHMETIC_BASE","proof":"same behavior"}]}`, "unenrolled package"},
		{"bad location", `{"equivalents":[{"id":"EQ-1","package":"bff/app","location":"notes.go:0:5","mutator":"ARITHMETIC_BASE","proof":"same behavior"}]}`, "file:line:col"},
		{"duplicate id", `{"equivalents":[{"id":"EQ-1","package":"bff/app","location":"notes.go:10:5","mutator":"ARITHMETIC_BASE","proof":"p1"},{"id":"EQ-1","package":"bff/app","location":"notes.go:11:5","mutator":"ARITHMETIC_BASE","proof":"p2"}]}`, "listed twice"},
		{"duplicate site", `{"equivalents":[{"id":"EQ-1","package":"bff/app","location":"notes.go:10:5","mutator":"ARITHMETIC_BASE","proof":"p1"},{"id":"EQ-2","package":"bff/app","location":"notes.go:10:5","mutator":"ARITHMETIC_BASE","proof":"p2"}]}`, "duplicates site"},
		{"unknown field", `{"equivalents":[],"unknown":true}`, "unknown field"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			floors := sampleFloors()
			path := writeFile(t, t.TempDir(), "eq.json", tc.content)

			// Act
			_, err := LoadEquivalents(path, floors)

			// Assert
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("case %s: err=%v; want substring %q", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestCompareDirections(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*Floors)
		wantProblem string
	}{
		{"equal floors pass", func(*Floors) {}, ""},
		{"stricter floor passes", func(f *Floors) { f.Packages[0].Floor = 95 }, ""},
		{"added package passes", func(f *Floors) {
			f.Packages = append(f.Packages, PackageConfig{Name: "pkg/b", Module: "m", GoVersion: "1.27.4", Package: "./b", Sources: []string{"b"}, Floor: 100})
		}, ""},
		{"reordered identical sources pass", func(f *Floors) {
			f.Packages[0].Sources = []string{"bff/ports", "bff/app"}
		}, ""},
		{"lowered floor fails", func(f *Floors) { f.Packages[0].Floor = 79.5 }, "floor lowered"},
		{"removed package fails", func(f *Floors) { f.Packages = nil }, "enrolled package was removed"},
		{"target changed fails", func(f *Floors) { f.Packages[0].Package = "./other" }, "package target changed"},
		{"source selection swapped at same floor fails", func(f *Floors) { f.Packages[0].Sources = []string{"bff/fake", "bff/ports"} }, "source mapping changed"},
		{"extra source added at same floor fails", func(f *Floors) { f.Packages[0].Sources = []string{"bff/app", "bff/ports", "bff/extra"} }, "source mapping changed"},
		{"destination mapping changed via strip_prefix at same floor fails", func(f *Floors) { f.Packages[0].StripPrefix = "bff/" }, "source mapping changed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			base := sampleFloors()
			proposal := sampleFloors()
			tc.mutate(proposal)

			// Act
			problems := Compare(base, proposal)

			// Assert
			joined := strings.Join(problems, "; ")
			if tc.wantProblem == "" && len(problems) != 0 {
				t.Fatalf("case %s: unexpected problems %q", tc.name, joined)
			}
			if tc.wantProblem != "" && !strings.Contains(joined, tc.wantProblem) {
				t.Fatalf("case %s: problems=%q; want substring %q", tc.name, joined, tc.wantProblem)
			}
		})
	}
}

func TestCompareMonotonicityProperty(t *testing.T) {
	// Arrange
	property := func(baseRaw, propRaw uint8) bool {
		baseFloor := float64(baseRaw % 101)
		propFloor := float64(propRaw % 101)
		base := sampleFloors()
		proposal := sampleFloors()
		base.Packages[0].Floor = baseFloor
		proposal.Packages[0].Floor = propFloor

		// Act
		problems := Compare(base, proposal)

		// Assert
		if propFloor < baseFloor {
			return len(problems) > 0
		}
		return len(problems) == 0
	}

	if err := quick.Check(property, nil); err != nil {
		t.Fatalf("monotonicity property failed: %v", err)
	}
}

func TestEvaluateReportOutcomes(t *testing.T) {
	cases := []struct {
		name        string
		floor       float64
		mutants     []Mutant
		equivalents []Equivalent
		wantErr     string
		wantProblem string
	}{
		{
			name:    "all killed passes",
			floor:   100,
			mutants: []Mutant{{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 4}},
		},
		{
			name:        "surviving mutant below floor fails",
			floor:       100,
			mutants:     []Mutant{{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 4}, {Type: "INVERT_LOGICAL", Status: "LIVED", Line: 20, Column: 8}},
			wantProblem: "LIVED INVERT_LOGICAL at notes.go:20:8",
		},
		{
			name:        "timed out is never counted as kill",
			floor:       50,
			mutants:     []Mutant{{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 4}, {Type: "INVERT_LOGICAL", Status: "TIMED OUT", Line: 20, Column: 8}},
			wantProblem: "TIMED OUT INVERT_LOGICAL at notes.go:20:8",
		},
		{
			name:        "not covered reduces score below floor",
			floor:       100,
			mutants:     []Mutant{{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 4}, {Type: "CONDITIONALS_NEGATION", Status: "NOT COVERED", Line: 30, Column: 2}},
			wantProblem: "NOT COVERED CONDITIONALS_NEGATION at notes.go:30:2",
		},
		{
			name:        "reviewed equivalent mutant accepted",
			floor:       100,
			mutants:     []Mutant{{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 4}, {Type: "CONDITIONALS_BOUNDARY", Status: "LIVED", Line: 15, Column: 9}},
			equivalents: []Equivalent{{ID: "EQ-1", Package: "bff/app", Location: "notes.go:15:9", Mutator: "CONDITIONALS_BOUNDARY", Proof: "boundary invariant holds"}},
		},
		{
			name:        "stale equivalent rejected",
			floor:       100,
			mutants:     []Mutant{{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 4}},
			equivalents: []Equivalent{{ID: "EQ-1", Package: "bff/app", Location: "notes.go:15:9", Mutator: "CONDITIONALS_BOUNDARY", Proof: "stale"}},
			wantProblem: "stale equivalent EQ-1",
		},
		{
			name:    "zero mutants refused",
			floor:   100,
			mutants: nil,
			wantErr: "reported 0 mutants",
		},
		{
			name:    "only not viable mutants refused",
			floor:   0,
			mutants: []Mutant{{Type: "ARITHMETIC_BASE", Status: "NOT VIABLE", Line: 10, Column: 4}},
			wantErr: "0 viable testable mutants",
		},
		{
			name:    "unknown status refused",
			floor:   100,
			mutants: []Mutant{{Type: "ARITHMETIC_BASE", Status: "MYSTERY", Line: 10, Column: 4}},
			wantErr: "unexpected mutant status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			pkg := PackageConfig{Name: "bff/app", Floor: tc.floor}
			report := &GremlinsReport{Files: []FileReport{{FileName: "notes.go", Mutations: tc.mutants}}}
			eqFile := &EquivalentsFile{Equivalents: tc.equivalents}

			// Act
			_, problems, err := EvaluateReport(pkg, report, eqFile)

			// Assert
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("case %s: err=%v; want %q", tc.name, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("case %s: unexpected err %v", tc.name, err)
			}
			joined := strings.Join(problems, "; ")
			if tc.wantProblem == "" && len(problems) != 0 {
				t.Fatalf("case %s: unexpected problems %q", tc.name, joined)
			}
			if tc.wantProblem != "" && !strings.Contains(joined, tc.wantProblem) {
				t.Fatalf("case %s: problems=%q; want %q", tc.name, joined, tc.wantProblem)
			}
		})
	}
}

func TestEvaluateScoreBoundsProperty(t *testing.T) {
	// Arrange
	property := func(killedN, livedN, notCovN uint8) bool {
		k := int(killedN % 10)
		l := int(livedN % 10)
		nc := int(notCovN % 10)
		if k+l+nc == 0 {
			k = 1
		}
		var muts []Mutant
		for i := range k {
			muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: i + 1, Column: 1})
		}
		for i := range l {
			muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "LIVED", Line: i + 20, Column: 1})
		}
		for i := range nc {
			muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "NOT COVERED", Line: i + 40, Column: 1})
		}
		report := &GremlinsReport{Files: []FileReport{{FileName: "a.go", Mutations: muts}}}

		// Act
		res, _, err := EvaluateReport(PackageConfig{Name: "pkg", Floor: 0}, report, nil)

		// Assert
		expected := 100.0 * float64(k) / float64(k+l+nc)
		return err == nil && res.Score >= 0 && res.Score <= 100 && math.Abs(res.Score-expected) < 1e-9
	}

	if err := quick.Check(property, nil); err != nil {
		t.Fatalf("score bounds property failed: %v", err)
	}
}

func summaryFromPackageResult(res PackageResult, measuredAt string) Summary {
	return Summary{
		Packages:   1,
		Killed:     res.Killed,
		Lived:      res.Lived,
		Equivalent: res.Equivalent,
		TimedOut:   res.TimedOut,
		NotCovered: res.NotCovered,
		NotViable:  res.NotViable,
		MeasuredAt: measuredAt,
	}
}

func mixedOutcomesFixture() (*GremlinsReport, *EquivalentsFile) {
	report := &GremlinsReport{
		Files: []FileReport{{
			FileName: "notes.go",
			Mutations: []Mutant{
				{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 10, Column: 2},
				{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: 11, Column: 2},
				{Type: "INVERT_LOGICAL", Status: "LIVED", Line: 12, Column: 2},
				{Type: "CONDITIONALS_BOUNDARY", Status: "LIVED", Line: 13, Column: 2},
				{Type: "INCREMENT_DECREMENT", Status: "TIMED OUT", Line: 14, Column: 2},
				{Type: "CONDITIONALS_NEGATION", Status: "NOT COVERED", Line: 15, Column: 2},
				{Type: "INVERT_NEGATIVES", Status: "NOT VIABLE", Line: 16, Column: 2},
			},
		}},
	}
	eqFile := &EquivalentsFile{
		Equivalents: []Equivalent{{
			ID:       "EQ-MIXED-1",
			Package:  "bff/app",
			Location: "notes.go:13:2",
			Mutator:  "CONDITIONALS_BOUNDARY",
			Proof:    "mixed-outcome parity fixture",
		}},
	}
	return report, eqFile
}

func equivalentOnlyFixture() (*GremlinsReport, *EquivalentsFile) {
	report := &GremlinsReport{
		Files: []FileReport{{
			FileName: "notes.go",
			Mutations: []Mutant{
				{Type: "CONDITIONALS_BOUNDARY", Status: "LIVED", Line: 15, Column: 9},
				{Type: "INVERT_NEGATIVES", Status: "NOT VIABLE", Line: 16, Column: 2},
			},
		}},
	}
	eqFile := &EquivalentsFile{
		Equivalents: []Equivalent{{
			ID:       "EQ-ONLY-1",
			Package:  "bff/app",
			Location: "notes.go:15:9",
			Mutator:  "CONDITIONALS_BOUNDARY",
			Proof:    "equivalent-only parity fixture",
		}},
	}
	return report, eqFile
}

func TestPackageScoreOnMixedOutcomes(t *testing.T) {
	// Arrange
	report, eqFile := mixedOutcomesFixture()

	// Act
	res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 40}, report, eqFile)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if math.Abs(res.Score-40.0) > 1e-9 {
		t.Fatalf("mixed package score: got %.4f; want 40.0", res.Score)
	}
}

func TestSummaryScoreMatchesPackageScoreOnMixedOutcomes(t *testing.T) {
	// Arrange
	report, eqFile := mixedOutcomesFixture()
	res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 40}, report, eqFile)
	if err != nil {
		t.Fatal(err)
	}
	summary := summaryFromPackageResult(res, "rev1")

	// Act
	got := summary.Score()

	// Assert
	if math.Abs(got-res.Score) > 1e-9 {
		t.Fatalf("mixed summary score: got %.4f; want package score %.4f", got, res.Score)
	}
}

func TestReceiptParityOnMixedOutcomes(t *testing.T) {
	// Arrange
	report, eqFile := mixedOutcomesFixture()
	want := "1 packages, 2/5 killed (40.0%), 1 equivalent, 1 timed-out, 1 not-covered; floors measured at rev1"

	// Act
	res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 40}, report, eqFile)
	if err != nil {
		t.Fatal(err)
	}
	got := summaryFromPackageResult(res, "rev1").String()

	// Assert
	if got != want {
		t.Fatalf("mixed receipt: got %q; want %q", got, want)
	}
}

func TestPackageScoreOnEquivalentOnly(t *testing.T) {
	// Arrange
	report, eqFile := equivalentOnlyFixture()

	// Act
	res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 100}, report, eqFile)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if res.Score != 100 {
		t.Fatalf("equivalent-only package score: got %.4f; want 100.0", res.Score)
	}
}

func TestSummaryScoreMatchesPackageScoreOnEquivalentOnly(t *testing.T) {
	// Arrange
	report, eqFile := equivalentOnlyFixture()
	res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 100}, report, eqFile)
	if err != nil {
		t.Fatal(err)
	}
	summary := summaryFromPackageResult(res, "rev1")

	// Act
	got := summary.Score()

	// Assert
	if got != res.Score {
		t.Fatalf("equivalent-only summary score: got %.4f; want package score %.4f", got, res.Score)
	}
}

func TestReceiptParityOnEquivalentOnly(t *testing.T) {
	// Arrange
	report, eqFile := equivalentOnlyFixture()
	want := "1 packages, 0/0 killed (100.0%), 1 equivalent, 0 timed-out, 0 not-covered; floors measured at rev1"

	// Act
	res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 100}, report, eqFile)
	if err != nil {
		t.Fatal(err)
	}
	got := summaryFromPackageResult(res, "rev1").String()

	// Assert
	if got != want {
		t.Fatalf("equivalent-only receipt: got %q; want %q", got, want)
	}
}

func TestSummaryScoreIsZeroWhenEmpty(t *testing.T) {
	// Arrange
	summary := Summary{MeasuredAt: "rev1"}

	// Act
	got := summary.Score()

	// Assert
	if got != 0 {
		t.Fatalf("empty summary score: got %.4f; want 0", got)
	}
}

func buildParityFixture(kRaw, lRaw, tRaw, ncRaw, eqRaw uint8) (*GremlinsReport, *EquivalentsFile) {
	k, l, to, nc, eq := int(kRaw%6), int(lRaw%6), int(tRaw%6), int(ncRaw%6), int(eqRaw%4)
	if k+l+to+nc+eq == 0 {
		eq = 1
	}
	var muts []Mutant
	var eqs []Equivalent
	line := 1
	for range k {
		muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "KILLED", Line: line, Column: 1})
		line++
	}
	for range l {
		muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "LIVED", Line: line, Column: 1})
		line++
	}
	for range to {
		muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "TIMED OUT", Line: line, Column: 1})
		line++
	}
	for range nc {
		muts = append(muts, Mutant{Type: "ARITHMETIC_BASE", Status: "NOT COVERED", Line: line, Column: 1})
		line++
	}
	for i := range eq {
		muts = append(muts, Mutant{Type: "CONDITIONALS_BOUNDARY", Status: "LIVED", Line: line, Column: 1})
		eqs = append(eqs, Equivalent{
			ID:       fmt.Sprintf("EQ-%d", i+1),
			Package:  "bff/app",
			Location: fmt.Sprintf("notes.go:%d:1", line),
			Mutator:  "CONDITIONALS_BOUNDARY",
			Proof:    "parity property fixture",
		})
		line++
	}
	return &GremlinsReport{Files: []FileReport{{FileName: "notes.go", Mutations: muts}}}, &EquivalentsFile{Equivalents: eqs}
}

func TestEvaluateAndSummaryScoreParityProperty(t *testing.T) {
	// Arrange
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(kRaw, lRaw, tRaw, ncRaw, eqRaw uint8) bool {
		report, eqFile := buildParityFixture(kRaw, lRaw, tRaw, ncRaw, eqRaw)
		res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 0}, report, eqFile)
		if err != nil {
			return false
		}
		summary := summaryFromPackageResult(res, "rev1")
		return math.Abs(res.Score-summary.Score()) < 1e-9
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("evaluate/summary score parity property failed: %v", err)
	}
}

func TestEvaluateAndSummaryReceiptPrefixProperty(t *testing.T) {
	// Arrange
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(kRaw, lRaw, tRaw, ncRaw, eqRaw uint8) bool {
		report, eqFile := buildParityFixture(kRaw, lRaw, tRaw, ncRaw, eqRaw)
		res, _, err := EvaluateReport(PackageConfig{Name: "bff/app", Floor: 0}, report, eqFile)
		if err != nil {
			return false
		}
		summary := summaryFromPackageResult(res, "rev1")
		expectedPrefix := fmt.Sprintf("1 packages, %d/%d killed (%.1f%%), %d equivalent, %d timed-out, %d not-covered;",
			res.Killed, res.Denominator(), res.Score, res.Equivalent, res.TimedOut, res.NotCovered)
		return strings.HasPrefix(summary.String(), expectedPrefix)
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("evaluate/summary receipt prefix property failed: %v", err)
	}
}

func TestParseReportRejectsEmpty(t *testing.T) {
	// Arrange
	raw := []byte("   \n")

	// Act
	_, err := ParseReport(raw)

	// Assert
	if err == nil || !strings.Contains(err.Error(), "empty gremlins report") {
		t.Fatalf("ParseReport empty: err=%v; want empty gremlins report", err)
	}
}

func TestRunPackageRefusesPristineFailure(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, root, "bff/app/notes.go", "package app\n")
	writeFile(t, root, "bff/ports/store.go", "package ports\n")
	binDir := t.TempDir()
	fakeGo := writeFile(t, binDir, "bin/go", "#!/bin/sh\necho 'FAIL test' >&2\nexit 1\n")
	fakeGremlins := writeFile(t, binDir, "bin/gremlins", "#!/bin/sh\necho 'should not run' >&2\nexit 99\n")
	if err := os.Chmod(fakeGo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fakeGremlins, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{GremlinsBin: fakeGremlins, GoBin: fakeGo}

	// Act
	_, _, err := runner.RunPackage(root, sampleFloors().Packages[0], 100, &EquivalentsFile{})

	// Assert
	if err == nil || !strings.Contains(err.Error(), "pristine go test failed") {
		t.Fatalf("RunPackage pristine failure: err=%v; want pristine go test failed", err)
	}
}

func TestRunAllAggregatesStubReport(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, root, "bff/app/notes.go", "package app\n")
	writeFile(t, root, "bff/ports/store.go", "package ports\n")
	binDir := t.TempDir()
	fakeGo := writeFile(t, binDir, "bin/go", "#!/bin/sh\nexit 0\n")
	script := fmt.Sprintf(`#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then out="$2"; shift 2; continue; fi
  shift
done
printf '{"go_module":"m","files":[{"file_name":"notes.go","mutations":[{"type":"ARITHMETIC_BASE","status":"KILLED","line":1,"column":1}]}]}' > "$out"
`)
	fakeGremlins := writeFile(t, binDir, "bin/gremlins", script)
	if err := os.Chmod(fakeGo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fakeGremlins, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{GremlinsBin: fakeGremlins, GoBin: fakeGo}

	// Act
	summary, problems, err := runner.RunAll(root, sampleFloors(), &EquivalentsFile{})

	// Assert
	if err != nil || len(problems) != 0 || summary.Killed != 1 || !strings.Contains(summary.String(), "1/1 killed (100.0%)") {
		t.Fatalf("RunAll: summary=%q problems=%v err=%v", summary.String(), problems, err)
	}
}

func TestMaterializeMirrorCopiesRegularGoFiles(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, root, "bff/app/notes.go", "package app\n")
	writeFile(t, root, "bff/app/README.md", "ignored\n")
	writeFile(t, root, "bff/ports/store.go", "package ports\n")
	mirror := filepath.Join(t.TempDir(), "mod")

	// Act
	if err := materializeMirror(root, mirror, sampleFloors().Packages[0]); err != nil {
		t.Fatalf("materializeMirror regular files: %v", err)
	}

	// Assert
	got, err := os.ReadFile(filepath.Join(mirror, "bff", "app", "notes.go"))
	if err != nil || string(got) != "package app\n" {
		t.Fatalf("copied notes.go: content=%q err=%v", string(got), err)
	}
}

func TestMaterializeMirrorSkipsNonGoFiles(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, root, "bff/app/notes.go", "package app\n")
	writeFile(t, root, "bff/app/README.md", "ignored\n")
	writeFile(t, root, "bff/ports/store.go", "package ports\n")
	mirror := filepath.Join(t.TempDir(), "mod")

	// Act
	if err := materializeMirror(root, mirror, sampleFloors().Packages[0]); err != nil {
		t.Fatalf("materializeMirror regular files: %v", err)
	}

	// Assert
	if _, err := os.Stat(filepath.Join(mirror, "bff", "app", "README.md")); !os.IsNotExist(err) {
		t.Fatalf("non-Go file should not be copied: err=%v", err)
	}
}

func TestMaterializeMirrorRefusesSymlinks(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, root, outside string)
		wantErr string
	}{
		{
			name: "symlinked parent directory component outside repo",
			setup: func(t *testing.T, root, outside string) {
				writeFile(t, outside, "app/notes.go", "package app\n")
				writeFile(t, outside, "ports/store.go", "package ports\n")
				if err := os.Symlink(outside, filepath.Join(root, "bff")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "source directory component \"bff\" must not be a symlink",
		},
		{
			name: "symlinked parent directory component inside repo",
			setup: func(t *testing.T, root, _ string) {
				writeFile(t, root, "alt/app/notes.go", "package app\n")
				writeFile(t, root, "alt/ports/store.go", "package ports\n")
				if err := os.Symlink("alt", filepath.Join(root, "bff")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "source directory component \"bff\" must not be a symlink",
		},
		{
			name: "symlinked leaf source directory outside repo",
			setup: func(t *testing.T, root, outside string) {
				writeFile(t, root, "bff/ports/store.go", "package ports\n")
				writeFile(t, outside, "notes.go", "package app\n")
				if err := os.Symlink(outside, filepath.Join(root, "bff", "app")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "source directory component \"bff/app\" must not be a symlink",
		},
		{
			name: "symlinked leaf source directory inside repo",
			setup: func(t *testing.T, root, _ string) {
				writeFile(t, root, "bff/ports/store.go", "package ports\n")
				writeFile(t, root, "bff/fake/notes.go", "package app\n")
				if err := os.Symlink("fake", filepath.Join(root, "bff", "app")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "source directory component \"bff/app\" must not be a symlink",
		},
		{
			name: "symlinked Go file outside repo",
			setup: func(t *testing.T, root, outside string) {
				writeFile(t, root, "bff/ports/store.go", "package ports\n")
				extFile := writeFile(t, outside, "notes.go", "package app\n")
				if err := os.MkdirAll(filepath.Join(root, "bff", "app"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(extFile, filepath.Join(root, "bff", "app", "notes.go")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "source file \"bff/app/notes.go\" must not be a symlink",
		},
		{
			name: "symlinked Go file inside repo",
			setup: func(t *testing.T, root, _ string) {
				writeFile(t, root, "bff/ports/store.go", "package ports\n")
				writeFile(t, root, "bff/fake/notes.go", "package app\n")
				if err := os.MkdirAll(filepath.Join(root, "bff", "app"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../fake/notes.go", filepath.Join(root, "bff", "app", "notes.go")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "source file \"bff/app/notes.go\" must not be a symlink",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			outside := t.TempDir()
			tc.setup(t, root, outside)
			mirror := filepath.Join(t.TempDir(), "mod")

			// Act
			err := materializeMirror(root, mirror, sampleFloors().Packages[0])

			// Assert
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("case %s: err=%v; want substring %q", tc.name, err, tc.wantErr)
			}
		})
	}
}
