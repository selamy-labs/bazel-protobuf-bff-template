package coverage

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Metrics a profile may claim. Each names what was counted, so no percentage is mistaken for another.
const (
	Statements      = "statements"        // Go statements, from the raw profile.
	BlockSpanLines  = "block-span-lines"  // Lines inside Go coverage blocks, from rules_go's LCOV.
	CompiledJSLines = "compiled-js-lines" // Lines of compiled JavaScript, from Node's LCOV.
	Unmeasured      = "unmeasured"        // No data exists. Never met, never not applicable.
)

// reasons is the closed list of why a file may be left out. The evaluator enforces that a listed reason
// is present. Whether it is true of the file is a human review (spec 007 FR-017).
var reasons = []string{
	"build-tag-tools", "generated", "interfaces-only", "no-statements", "other-platform",
	"test-code", "test-data", "test-tooling",
}

// Exclude leaves files out of every profile, for a stated reason.
type Exclude struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
}

// Profile measures a set of files with one metric. The first profile that matches a file takes it.
type Profile struct {
	Name     string  `json:"name"`
	Language string  `json:"language"`
	Metric   string  `json:"metric"`
	Include  string  `json:"include"`
	Floor    float64 `json:"floor"`
}

// Config is one tree's coverage contract.
type Config struct {
	// Languages maps a language to its source extensions. Every tracked file with one is enrolled.
	Languages map[string][]string `json:"languages"`
	// GoModules maps a Go import path prefix to its repository path.
	GoModules map[string]string `json:"go_modules"`
	// LcovPrefixes maps a path prefix in LCOV to its repository path.
	LcovPrefixes map[string]string `json:"lcov_prefixes"`
	Excludes     []Exclude         `json:"excludes"`
	Profiles     []Profile         `json:"profiles"`

	excludes []*regexp.Regexp
	includes []*regexp.Regexp
}

// Load reads and validates a contract.
func Load(file string) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	config := &Config{}
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if err := config.prepare(); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return config, nil
}

func (c *Config) prepare() error {
	if len(c.Profiles) == 0 {
		return fmt.Errorf("no profiles")
	}
	for _, exclude := range c.Excludes {
		if !slices.Contains(reasons, exclude.Reason) {
			return fmt.Errorf("exclude %q: reason %q is not one of %v", exclude.Pattern, exclude.Reason, reasons)
		}
		pattern, err := regexp.Compile(exclude.Pattern)
		if err != nil {
			return fmt.Errorf("exclude %q: %w", exclude.Pattern, err)
		}
		c.excludes = append(c.excludes, pattern)
	}
	seen := map[string]bool{}
	for _, profile := range c.Profiles {
		if seen[profile.Name] || profile.Name == "" {
			return fmt.Errorf("profile %q: empty or repeated name", profile.Name)
		}
		seen[profile.Name] = true
		if _, known := c.Languages[profile.Language]; !known {
			return fmt.Errorf("profile %s: unknown language %q", profile.Name, profile.Language)
		}
		if !slices.Contains([]string{Statements, BlockSpanLines, CompiledJSLines, Unmeasured}, profile.Metric) {
			return fmt.Errorf("profile %s: unknown metric %q", profile.Name, profile.Metric)
		}
		if profile.Metric == Unmeasured && profile.Floor != 0 {
			return fmt.Errorf("profile %s: an unmeasured profile cannot have a floor", profile.Name)
		}
		pattern, err := regexp.Compile(profile.Include)
		if err != nil {
			return fmt.Errorf("profile %s: %w", profile.Name, err)
		}
		c.includes = append(c.includes, pattern)
	}
	return nil
}

// Enrollment sorts one tree's tracked files. Measured maps a profile's name to its source set, computed
// from the file list and never from a report. Unenrolled holds files of a profiled language that are
// in no profile and no exclude.
type Enrollment struct {
	Measured   map[string][]string
	Unenrolled []string
}

// Enroll classifies tracked files.
func (c *Config) Enroll(files []string) Enrollment {
	enrollment := Enrollment{Measured: map[string][]string{}}
	for _, file := range files {
		language := c.language(file)
		if language == "" || slices.ContainsFunc(c.excludes, func(p *regexp.Regexp) bool { return p.MatchString(file) }) {
			continue
		}
		index := slices.IndexFunc(c.Profiles, func(profile Profile) bool { return profile.Language == language })
		matched := false
		for ; index >= 0 && index < len(c.Profiles); index++ {
			if c.Profiles[index].Language == language && c.includes[index].MatchString(file) {
				enrollment.Measured[c.Profiles[index].Name] = append(enrollment.Measured[c.Profiles[index].Name], file)
				matched = true
				break
			}
		}
		if !matched {
			enrollment.Unenrolled = append(enrollment.Unenrolled, file)
		}
	}
	return enrollment
}

// IsTestData reports whether this contract excludes a file as test data.
func (c *Config) IsTestData(file string) bool {
	for index, exclude := range c.Excludes {
		if exclude.Reason == "test-data" && c.excludes[index].MatchString(file) {
			return true
		}
	}
	return false
}

func (c *Config) language(file string) string {
	extension := path.Ext(file)
	for language, extensions := range c.Languages {
		if slices.Contains(extensions, extension) {
			return language
		}
	}
	return ""
}
