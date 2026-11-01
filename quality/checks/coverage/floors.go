package coverage

import (
	"fmt"
	"slices"
	"strings"
)

// Result is one profile's measurement.
type Result struct {
	Profile, Metric string
	Covered, Total  int
	Floor           float64
	Branches        string
}

// Percent is the measured percentage.
func (r Result) Percent() float64 {
	if r.Total == 0 {
		return 0
	}
	return 100 * float64(r.Covered) / float64(r.Total)
}

func (r Result) String() string {
	if r.Metric == Unmeasured {
		return fmt.Sprintf("%s: unmeasured, %d files, no data", r.Profile, r.Total)
	}
	return fmt.Sprintf("%s: %.1f%% %s (%d of %d), floor %.1f%%, branches %s",
		r.Profile, r.Percent(), r.Metric, r.Covered, r.Total, r.Floor, r.Branches)
}

// Check judges one tree against its own contract. Results are what was measured, and problems deny
// the check. An absent file fails and is never counted as covered.
func (c *Config) Check(files []string, report *Report) ([]Result, []string) {
	var results []Result
	var problems []string
	enrollment := c.Enroll(files)
	for _, file := range enrollment.Unenrolled {
		problems = append(problems, "unenrolled source: "+file+" is in no profile and no exclude")
	}
	for _, profile := range c.Profiles {
		sources := enrollment.Measured[profile.Name]
		result := Result{Profile: profile.Name, Metric: profile.Metric, Floor: profile.Floor, Branches: "unmeasured"}
		if profile.Metric == Unmeasured {
			result.Total = len(sources)
			results = append(results, result)
			continue
		}
		if len(sources) == 0 {
			problems = append(problems, profile.Name+": measures no files")
		}
		var branchCovered, branchTotal int
		for _, source := range sources {
			covered, total, found := c.measure(profile.Metric, source, report)
			if !found {
				problems = append(problems, profile.Name+": "+source+" is absent from the report")
				continue
			}
			result.Covered += covered
			result.Total += total
			if profile.Metric == CompiledJSLines {
				for _, hit := range report.Branches[compiled(source)] {
					branchTotal++
					if hit {
						branchCovered++
					}
				}
			}
		}
		if branchTotal > 0 {
			result.Branches = fmt.Sprintf("%d of %d", branchCovered, branchTotal)
		}
		if result.Total == 0 && len(sources) > 0 {
			problems = append(problems, profile.Name+": no countable data, so its floor cannot be judged")
		}
		if result.Total > 0 && result.Percent() < profile.Floor {
			problems = append(problems, fmt.Sprintf("%s: %.1f%% is below its floor of %.1f%%", profile.Name, result.Percent(), profile.Floor))
		}
		results = append(results, result)
	}
	return results, problems
}

func (c *Config) measure(metric, source string, report *Report) (covered, total int, found bool) {
	switch metric {
	case Statements:
		for path, blocks := range report.Statements {
			if c.repositoryPath(path) != source {
				continue
			}
			for _, block := range blocks {
				total += block.statements
				if block.covered {
					covered += block.statements
				}
			}
		}
		// A file that counts nothing was not measured. A true no-statement file is a reviewed exclude.
		found = total > 0
	case BlockSpanLines:
		return c.lines(source, report)
	case CompiledJSLines:
		return countLines(report.Lines[compiled(source)])
	}
	return covered, total, found
}

func (c *Config) lines(source string, report *Report) (covered, total int, found bool) {
	merged := map[int]bool{}
	for path, lines := range report.Lines {
		if c.lcovPath(path) != source {
			continue
		}
		for line, hit := range lines {
			merged[line] = merged[line] || hit
		}
	}
	return countLines(merged)
}

func countLines(lines map[int]bool) (covered, total int, found bool) {
	for _, hit := range lines {
		total++
		if hit {
			covered++
		}
	}
	return covered, total, total > 0
}

// compiled maps a TypeScript source to the JavaScript that V8 saw. Lines are of that output, so the
// mapping is by file and not by line.
func compiled(source string) string {
	for _, extension := range []string{".tsx", ".ts"} {
		if stem, ok := strings.CutSuffix(source, extension); ok {
			return stem + ".js"
		}
	}
	return source
}

func (c *Config) repositoryPath(path string) string {
	for prefix, replacement := range c.GoModules {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			return replacement + rest
		}
	}
	return path
}

func (c *Config) lcovPath(path string) string {
	for prefix, replacement := range c.LcovPrefixes {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			return replacement + rest
		}
	}
	return path
}

// Compare judges a tree's contract against its base's, and nothing wider: floors only rise, and no
// surviving path leaves the source set of the profile that measured it (spec 007 FR-016). There is no
// move detection. A deleted path is simply gone, and a new path must enroll like any other.
func Compare(base *Config, baseFiles []string, proposal *Config, files []string) []string {
	var problems []string
	baseSets, sets := base.Enroll(baseFiles).Measured, proposal.Enroll(files).Measured
	for _, want := range base.Profiles {
		index := slices.IndexFunc(proposal.Profiles, func(p Profile) bool { return p.Name == want.Name })
		if index < 0 {
			problems = append(problems, want.Name+": the profile was removed")
			continue
		}
		have := proposal.Profiles[index]
		switch {
		case have.Metric != want.Metric:
			problems = append(problems, fmt.Sprintf("%s: metric changed from %s to %s", want.Name, want.Metric, have.Metric))
		case have.Language != want.Language:
			problems = append(problems, fmt.Sprintf("%s: language changed from %s to %s", want.Name, want.Language, have.Language))
		case have.Floor < want.Floor:
			problems = append(problems, fmt.Sprintf("%s: floor lowered from %.1f to %.1f", want.Name, want.Floor, have.Floor))
		}
		for _, file := range baseSets[want.Name] {
			if slices.Contains(files, file) && !slices.Contains(sets[want.Name], file) {
				problems = append(problems, fmt.Sprintf("%s: narrowed: %s left the measured source set", want.Name, file))
			}
		}
	}
	return problems
}
