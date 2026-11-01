package coverage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var profileLine = regexp.MustCompile(`^(.+):(\d+\.\d+,\d+\.\d+) (\d+) (\d+)$`)

type block struct {
	statements int
	covered    bool
}

// Report is what the tests of one run showed. A block or line counts as covered if any test covered
// it, and repeated records from instrumented subprocesses are one line.
type Report struct {
	// Statements maps a Go import path to its blocks, from raw profiles only.
	Statements map[string]map[string]block
	// Lines maps an LCOV path to its lines.
	Lines map[string]map[int]bool
	// Branches maps an LCOV path to its branch counts.
	Branches map[string]map[string]bool
}

// NewReport returns an empty report.
func NewReport() *Report {
	return &Report{
		Statements: map[string]map[string]block{},
		Lines:      map[string]map[int]bool{},
		Branches:   map[string]map[string]bool{},
	}
}

// Add parses one test's coverage.dat. A record that does not close, or a line that does not parse, is
// an error: a truncated report must not read as a smaller one.
func (r *Report) Add(name, text string) error {
	source := ""
	for number, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		var err error
		switch {
		case strings.HasPrefix(line, ProfilePrefix):
			err = r.addProfile(strings.TrimPrefix(line, ProfilePrefix))
		case strings.HasPrefix(line, "SF:"):
			if source != "" {
				err = fmt.Errorf("record for %s did not end", source)
			}
			source = strings.TrimPrefix(line, "SF:")
		case line == "end_of_record":
			if source == "" {
				err = fmt.Errorf("end of a record that did not start")
			}
			source = ""
		case strings.HasPrefix(line, "DA:"):
			err = r.addLine(source, strings.TrimPrefix(line, "DA:"))
		case strings.HasPrefix(line, "BRDA:"):
			err = r.addBranch(source, strings.TrimPrefix(line, "BRDA:"))
		}
		if err != nil {
			return fmt.Errorf("%s:%d: %w", name, number+1, err)
		}
	}
	if source != "" {
		return fmt.Errorf("%s: truncated: the record for %s did not end", name, source)
	}
	return nil
}

func (r *Report) addProfile(line string) error {
	match := profileLine.FindStringSubmatch(line)
	if match == nil {
		return fmt.Errorf("not a coverage profile line: %q", line)
	}
	statements, err := strconv.Atoi(match[3])
	if err != nil {
		return err
	}
	count, err := strconv.Atoi(match[4])
	if err != nil {
		return err
	}
	blocks := r.Statements[match[1]]
	if blocks == nil {
		blocks = map[string]block{}
		r.Statements[match[1]] = blocks
	}
	blocks[match[2]] = block{statements: statements, covered: blocks[match[2]].covered || count > 0}
	return nil
}

func (r *Report) addLine(source, record string) error {
	if source == "" {
		return fmt.Errorf("DA outside a record")
	}
	number, count, found := strings.Cut(record, ",")
	if !found {
		return fmt.Errorf("not a DA line: %q", record)
	}
	line, err := strconv.Atoi(number)
	if err != nil {
		return fmt.Errorf("not a DA line: %q", record)
	}
	count, _, _ = strings.Cut(count, ",")
	hits, err := strconv.Atoi(count)
	if err != nil {
		return fmt.Errorf("not a DA line: %q", record)
	}
	lines := r.Lines[source]
	if lines == nil {
		lines = map[int]bool{}
		r.Lines[source] = lines
	}
	lines[line] = lines[line] || hits > 0
	return nil
}

func (r *Report) addBranch(source, record string) error {
	if source == "" {
		return fmt.Errorf("BRDA outside a record")
	}
	fields := strings.Split(record, ",")
	if len(fields) != 4 {
		return fmt.Errorf("not a BRDA line: %q", record)
	}
	branches := r.Branches[source]
	if branches == nil {
		branches = map[string]bool{}
		r.Branches[source] = branches
	}
	taken := 0
	if fields[3] != "-" { // LCOV's mark for a branch that was never reached.
		count, err := strconv.Atoi(fields[3])
		if err != nil || count < 0 {
			return fmt.Errorf("not a BRDA line: %q", record)
		}
		taken = count
	}
	key := strings.Join(fields[:3], ",")
	branches[key] = branches[key] || taken > 0
	return nil
}
