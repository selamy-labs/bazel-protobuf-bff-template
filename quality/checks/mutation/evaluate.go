package mutation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GremlinsReport mirrors gremlins unleash --output JSON.
type GremlinsReport struct {
	GoModule string       `json:"go_module"`
	Files    []FileReport `json:"files"`
}

// FileReport groups mutations for one source file in a gremlins report.
type FileReport struct {
	FileName  string   `json:"file_name"`
	Mutations []Mutant `json:"mutations"`
}

// Mutant is one mutation outcome emitted by gremlins.
type Mutant struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Location returns the canonical file:line:col locator.
func (m Mutant) Location(fileName string) string {
	return fmt.Sprintf("%s:%d:%d", fileName, m.Line, m.Column)
}

// PackageResult holds exact outcome counts for one evaluated package.
type PackageResult struct {
	Name       string
	Floor      float64
	Killed     int
	Lived      int
	Equivalent int
	TimedOut   int
	NotCovered int
	NotViable  int
	Score      float64
}

type scoreCounts struct {
	killed     int
	lived      int
	equivalent int
	timedOut   int
	notCovered int
}

func (c scoreCounts) denominator() int {
	return c.killed + c.lived + c.timedOut + c.notCovered
}

func (c scoreCounts) score() float64 {
	denom := c.denominator()
	if denom == 0 {
		if c.equivalent > 0 {
			return 100
		}
		return 0
	}
	return 100.0 * float64(c.killed) / float64(denom)
}

func (r PackageResult) counts() scoreCounts {
	return scoreCounts{
		killed:     r.Killed,
		lived:      r.Lived,
		equivalent: r.Equivalent,
		timedOut:   r.TimedOut,
		notCovered: r.NotCovered,
	}
}

// Denominator returns the count of testable non-equivalent mutants.
func (r PackageResult) Denominator() int {
	return r.counts().denominator()
}

// Summary aggregates mutation outcomes across all enrolled packages.
type Summary struct {
	Packages   int
	Killed     int
	Lived      int
	Equivalent int
	TimedOut   int
	NotCovered int
	NotViable  int
	MeasuredAt string
}

func (s Summary) counts() scoreCounts {
	return scoreCounts{
		killed:     s.Killed,
		lived:      s.Lived,
		equivalent: s.Equivalent,
		timedOut:   s.TimedOut,
		notCovered: s.NotCovered,
	}
}

// Score returns the aggregate kill percentage across testable non-equivalent mutants.
func (s Summary) Score() float64 {
	return s.counts().score()
}

// String renders the receipt measurement line.
func (s Summary) String() string {
	c := s.counts()
	return fmt.Sprintf("%d packages, %d/%d killed (%.1f%%), %d equivalent, %d timed-out, %d not-covered; floors measured at %s",
		s.Packages, s.Killed, c.denominator(), c.score(), s.Equivalent, s.TimedOut, s.NotCovered, s.MeasuredAt)
}

// ParseReport decodes a gremlins JSON report and rejects empty or malformed payloads.
func ParseReport(data []byte) (*GremlinsReport, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("empty gremlins report")
	}
	report := &GremlinsReport{}
	if err := json.Unmarshal(data, report); err != nil {
		return nil, fmt.Errorf("parse gremlins report: %w", err)
	}
	return report, nil
}

// EvaluateReport scores one package from its gremlins JSON report and reviewed equivalents.
// Timeouts and uncovered mutants are never reclassified as kills; zero observed mutants is a refusal.
func EvaluateReport(pkg PackageConfig, report *GremlinsReport, eqFile *EquivalentsFile) (PackageResult, []string, error) {
	result := PackageResult{Name: pkg.Name, Floor: pkg.Floor}
	eqMap := packageEquivalents(pkg.Name, eqFile)
	usedEq := map[string]bool{}
	var survivors []string
	var timeouts []string
	total := 0

	for _, file := range report.Files {
		for _, m := range file.Mutations {
			total++
			if err := tallyMutant(pkg.Name, file.FileName, m, eqMap, usedEq, &result, &survivors, &timeouts); err != nil {
				return result, nil, err
			}
		}
	}
	if total == 0 {
		return result, nil, fmt.Errorf("%s: gremlins reported 0 mutants; refusing empty measurement", pkg.Name)
	}
	if result.Denominator() == 0 && result.Equivalent == 0 {
		return result, nil, fmt.Errorf("%s: gremlins produced 0 viable testable mutants; refusing measurement", pkg.Name)
	}
	result.Score = result.counts().score()
	problems := judgePackage(result, survivors, timeouts, eqMap, usedEq)
	return result, problems, nil
}

func packageEquivalents(pkgName string, eqFile *EquivalentsFile) map[string]Equivalent {
	out := map[string]Equivalent{}
	if eqFile == nil {
		return out
	}
	for _, e := range eqFile.Equivalents {
		if e.Package == pkgName {
			out[e.Key()] = e
		}
	}
	return out
}

func tallyMutant(
	pkgName, fileName string,
	m Mutant,
	eqMap map[string]Equivalent,
	usedEq map[string]bool,
	result *PackageResult,
	survivors, timeouts *[]string,
) error {
	loc := m.Location(fileName)
	switch m.Status {
	case "KILLED":
		result.Killed++
	case "LIVED":
		key := pkgName + "|" + loc + "|" + m.Type
		if _, ok := eqMap[key]; ok {
			result.Equivalent++
			usedEq[key] = true
			return nil
		}
		result.Lived++
		*survivors = append(*survivors, fmt.Sprintf("%s: LIVED %s at %s", pkgName, m.Type, loc))
	case "TIMED OUT":
		result.TimedOut++
		*timeouts = append(*timeouts, fmt.Sprintf("%s: TIMED OUT %s at %s (timeouts are never counted as kills)", pkgName, m.Type, loc))
	case "NOT COVERED":
		result.NotCovered++
		*survivors = append(*survivors, fmt.Sprintf("%s: NOT COVERED %s at %s", pkgName, m.Type, loc))
	case "NOT VIABLE":
		result.NotViable++
	default:
		return fmt.Errorf("%s: unexpected mutant status %q at %s", pkgName, m.Status, loc)
	}
	return nil
}

func judgePackage(
	result PackageResult,
	survivors, timeouts []string,
	eqMap map[string]Equivalent,
	usedEq map[string]bool,
) []string {
	var problems []string
	problems = append(problems, timeouts...)
	if result.Score < result.Floor {
		problems = append(problems, fmt.Sprintf("%s: mutation score %.1f%% (%d/%d killed) is below floor %.1f%%",
			result.Name, result.Score, result.Killed, result.Denominator(), result.Floor))
		problems = append(problems, survivors...)
	}
	for key, eq := range eqMap {
		if !usedEq[key] {
			problems = append(problems, fmt.Sprintf("%s: stale equivalent %s (%s at %s) did not match a LIVED mutant",
				result.Name, eq.ID, eq.Mutator, eq.Location))
		}
	}
	return problems
}
