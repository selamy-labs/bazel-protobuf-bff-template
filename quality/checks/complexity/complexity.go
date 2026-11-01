// Package complexity measures every function's cognitive and cyclomatic complexity and judges the tree
// against its own budgets (spec 011 FR-004, FR-005). The budgets are a policy input: a ceiling for every
// function, and named debt for the functions measured above it, each with the value measured at the
// revision the file records. Check judges one tree. Compare, built from a base, judges a proposal's
// budgets against that base: a ceiling may only fall, a debt entry may only fall or go, and no debt or
// exclusion is ever added. Nothing here invents a number: a budget is a measurement of this tree.
package complexity

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

// Metric names what was measured.
type Metric string

const (
	// Cognitive is gocognit's measure: nesting and breaks in linear flow.
	Cognitive Metric = "cognitive"
	// Cyclomatic is gocyclo's measure: independent paths.
	Cyclomatic Metric = "cyclomatic"
)

var metrics = []Metric{Cognitive, Cyclomatic}

// Measurement is one function's values. Function is "path name", as the tools print the name.
type Measurement struct {
	Path, Name string
	Values     map[Metric]int
}

// Function is the key a budget names.
func (m Measurement) Function() string { return m.Path + " " + m.Name }

// Exclude drops paths from measurement, with a reason. An exclusion is leniency, so Compare refuses a new one.
type Exclude struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
	re      *regexp.Regexp
}

// Debt is one function measured above the ceiling, budgeted at its measured value.
type Debt struct {
	Function string         `json:"function"`
	Owner    string         `json:"owner"`
	Values   map[Metric]int `json:"values"`
}

// Budgets is the policy input.
type Budgets struct {
	// MeasuredAt is the revision the values were measured at. It names the measurement; nothing reads it.
	MeasuredAt string         `json:"measured_at"`
	Ceilings   map[Metric]int `json:"ceilings"`
	Excludes   []Exclude      `json:"excludes"`
	Debt       []Debt         `json:"debt"`
}

// Load reads and validates a budgets file.
func Load(path string) (*Budgets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	budgets := &Budgets{}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(budgets); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return budgets, budgets.prepare(path)
}

func (b *Budgets) prepare(path string) error {
	if err := b.prepareCeilings(path); err != nil {
		return err
	}
	if err := b.prepareExcludes(path); err != nil {
		return err
	}
	return b.prepareDebt(path)
}

func (b *Budgets) prepareCeilings(path string) error {
	for _, metric := range metrics {
		if value, ok := b.Ceilings[metric]; !ok || value < 1 {
			return fmt.Errorf("%s: the %s ceiling must be a positive integer", path, metric)
		}
	}
	for metric := range b.Ceilings {
		if !slices.Contains(metrics, metric) {
			return fmt.Errorf("%s: unknown metric %q", path, metric)
		}
	}
	return nil
}

func (b *Budgets) prepareExcludes(path string) error {
	for i := range b.Excludes {
		exclude := &b.Excludes[i]
		if exclude.Reason == "" {
			return fmt.Errorf("%s: exclude %q has no reason", path, exclude.Pattern)
		}
		re, err := regexp.Compile(exclude.Pattern)
		if err != nil {
			return fmt.Errorf("%s: exclude %q: %w", path, exclude.Pattern, err)
		}
		exclude.re = re
	}
	return nil
}

func (b *Budgets) prepareDebt(path string) error {
	seen := map[string]bool{}
	for _, debt := range b.Debt {
		if debt.Function == "" || !strings.Contains(debt.Function, " ") {
			return fmt.Errorf("%s: debt %q must be \"path name\"", path, debt.Function)
		}
		if seen[debt.Function] {
			return fmt.Errorf("%s: debt %q is listed twice", path, debt.Function)
		}
		seen[debt.Function] = true
		if debt.Owner == "" {
			return fmt.Errorf("%s: debt %q has no owner", path, debt.Function)
		}
		if err := b.prepareDebtValues(path, debt); err != nil {
			return err
		}
	}
	return nil
}

func (b *Budgets) prepareDebtValues(path string, debt Debt) error {
	for metric, value := range debt.Values {
		if !slices.Contains(metrics, metric) {
			return fmt.Errorf("%s: debt %q: unknown metric %q", path, debt.Function, metric)
		}
		if value <= b.Ceilings[metric] {
			return fmt.Errorf("%s: debt %q budgets %s at %d, which is not above the ceiling %d; it is no debt", path, debt.Function, metric, value, b.Ceilings[metric])
		}
	}
	return nil
}

// Excluded says whether a path is dropped from measurement, and why.
func (b *Budgets) Excluded(path string) (string, bool) {
	for _, exclude := range b.Excludes {
		if exclude.re.MatchString(path) {
			return exclude.Reason, true
		}
	}
	return "", false
}

// limit is what one function may reach for one metric.
func (b *Budgets) limit(function string, metric Metric) (int, bool) {
	for _, debt := range b.Debt {
		if debt.Function == function {
			if value, ok := debt.Values[metric]; ok {
				return value, true
			}
			return b.Ceilings[metric], false
		}
	}
	return b.Ceilings[metric], false
}

// MeasureFile measures every function of one parsed Go file. Both tools read the same syntax tree.
func MeasureFile(path string, source []byte) ([]Measurement, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		return nil, err
	}
	byName := map[string]*Measurement{}
	var ordered []*Measurement
	record := func(name string, metric Metric, value int) {
		m := byName[name]
		if m == nil {
			m = &Measurement{Path: path, Name: name, Values: map[Metric]int{}}
			byName[name] = m
			ordered = append(ordered, m)
		}
		m.Values[metric] = max(m.Values[metric], value)
	}
	for _, stat := range gocognit.ComplexityStats(file, fset, nil) {
		record(stat.FuncName, Cognitive, stat.Complexity)
	}
	for _, stat := range gocyclo.AnalyzeASTFile(file, fset, nil) {
		record(stat.FuncName, Cyclomatic, stat.Complexity)
	}
	// Every declared function must have both values, or the measurement is not one.
	declared := 0
	for _, decl := range file.Decls {
		if _, ok := decl.(*ast.FuncDecl); ok {
			declared++
		}
	}
	if len(ordered) < declared {
		return nil, fmt.Errorf("%s: %d functions declared, %d measured", path, declared, len(ordered))
	}
	var result []Measurement
	for _, m := range ordered {
		for _, metric := range metrics {
			if _, ok := m.Values[metric]; !ok {
				return nil, fmt.Errorf("%s: %s has no %s value", path, m.Name, metric)
			}
		}
		result = append(result, *m)
	}
	return result, nil
}

// Result is what a check measured.
type Result struct {
	Files, Excluded, Functions int
	Max                        map[Metric]int
}

func (r Result) String() string {
	return fmt.Sprintf("%d files measured, %d excluded, %d functions; max cognitive %d, max cyclomatic %d",
		r.Files, r.Excluded, r.Functions, r.Max[Cognitive], r.Max[Cyclomatic])
}

// Check judges every listed file. Problems name the function and both numbers. A list that measures no
// function is an error, never a pass. read gives each path's source.
func (b *Budgets) Check(paths []string, read func(string) ([]byte, error)) (Result, []string, error) {
	result := Result{Max: map[Metric]int{}}
	var problems []string
	for _, path := range paths {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if _, excluded := b.Excluded(path); excluded {
			result.Excluded++
			continue
		}
		source, err := read(path)
		if err != nil {
			return result, nil, err
		}
		measured, err := MeasureFile(path, source)
		if err != nil {
			return result, nil, err
		}
		result.Files++
		problems = append(problems, b.judge(measured, &result)...)
	}
	if result.Functions == 0 {
		return result, nil, errors.New("no function was measured")
	}
	return result, problems, nil
}

// judge compares one file's functions with their limits and records the maxima.
func (b *Budgets) judge(measured []Measurement, result *Result) []string {
	var problems []string
	for _, m := range measured {
		result.Functions++
		for _, metric := range metrics {
			result.Max[metric] = max(result.Max[metric], m.Values[metric])
			limit, isDebt := b.limit(m.Function(), metric)
			if m.Values[metric] <= limit {
				continue
			}
			kind := "ceiling"
			if isDebt {
				kind = "debt budget"
			}
			problems = append(problems, fmt.Sprintf("%s: %s %d is over its %s %d", m.Function(), metric, m.Values[metric], kind, limit))
		}
	}
	return problems
}

// Compare judges a proposal's budgets against a base's. Every difference that could make the check judge
// less is refused: a ceiling that rises, a debt value that rises, a debt entry or an exclusion that
// appears. A ceiling or a debt value that falls, or a debt entry that goes, is stricter and passes.
func Compare(base, proposal *Budgets) []string {
	var problems []string
	for _, metric := range metrics {
		if proposal.Ceilings[metric] > base.Ceilings[metric] {
			problems = append(problems, fmt.Sprintf("the %s ceiling rises from %d to %d", metric, base.Ceilings[metric], proposal.Ceilings[metric]))
		}
	}
	known := map[string]Debt{}
	for _, debt := range base.Debt {
		known[debt.Function] = debt
	}
	for _, debt := range proposal.Debt {
		before, ok := known[debt.Function]
		if !ok {
			problems = append(problems, fmt.Sprintf("debt %q is new; a budget is measured, not granted", debt.Function))
			continue
		}
		for metric, value := range debt.Values {
			if was, had := before.Values[metric]; !had || value > was {
				problems = append(problems, fmt.Sprintf("debt %q: %s rises to %d", debt.Function, metric, value))
			}
		}
	}
	patterns := map[string]bool{}
	for _, exclude := range base.Excludes {
		patterns[exclude.Pattern] = true
	}
	for _, exclude := range proposal.Excludes {
		if !patterns[exclude.Pattern] {
			problems = append(problems, fmt.Sprintf("exclude %q is new", exclude.Pattern))
		}
	}
	return problems
}
