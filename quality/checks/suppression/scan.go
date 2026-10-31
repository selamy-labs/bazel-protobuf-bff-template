package suppression

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Finding is one banned directive at a location.
type Finding struct {
	Path    string
	Line    int
	Code    string
	Message string
}

const rerun = "tools/verify.sh"

// Scanner holds the files exempt from the scan, as path to sha256. A file is exempt only when both
// match, so an unknown name near an exempt one is scanned, and so is a changed exempt file.
type Scanner struct {
	Exempt map[string]string
}

// Scan reports banned directives in one file using no enrolled exemptions.
func Scan(path string, data []byte) []Finding {
	return Scanner{}.Scan(path, data)
}

// Scan reports banned directives in one file. Documentation is skipped: a directive in prose has
// no effect on any tool. NUL bytes are removed rather than treated as a binary marker, because
// bash still runs such a file, and because it reveals directives in UTF-16 source.
func (s Scanner) Scan(path string, data []byte) []Finding {
	if s.isExempt(path, data) || strings.HasSuffix(path, ".md") {
		return nil
	}
	text := string(bytes.ReplaceAll(data, []byte{0}, nil))
	var findings []Finding
	for _, rule := range Rules {
		if !rule.Spans {
			continue
		}
		for _, match := range rule.Pattern.FindAllStringIndex(text, -1) {
			line := strings.Count(text[:match[0]], "\n") + 1
			findings = append(findings, Finding{Path: path, Line: line, Code: rule.Code, Message: rule.Description})
		}
	}
	for i, line := range strings.Split(text, "\n") {
		for _, rule := range Rules {
			if !rule.Spans && rule.Pattern.MatchString(line) {
				findings = append(findings, Finding{Path: path, Line: i + 1, Code: rule.Code, Message: rule.Description})
			}
		}
	}
	sort.SliceStable(findings, func(a, b int) bool { return findings[a].Line < findings[b].Line })
	return findings
}

func (s Scanner) isExempt(path string, data []byte) bool {
	if path == patternTable {
		return true
	}
	want, enrolled := s.Exempt[path]
	sum := sha256.Sum256(data)
	return enrolled && want == hex.EncodeToString(sum[:])
}

// ParseManifest reads path to sha256 from sha256sum-format text. Empty input is an error: a
// missing manifest must not silently change what is scanned.
func ParseManifest(data []byte) (map[string]string, error) {
	exempt := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		sum, path, ok := strings.Cut(line, "  ")
		if !ok || len(sum) != 64 || path == "" {
			return nil, fmt.Errorf("malformed manifest line %q", line)
		}
		exempt[path] = sum
	}
	return exempt, nil
}

// Report writes at most limit diagnostic lines, then a summary of the rest.
func Report(w io.Writer, findings []Finding, limit int) {
	for i, f := range findings {
		if i == limit {
			fmt.Fprintf(w, "suppression-ban: %d more findings not shown\n", len(findings)-limit)
			return
		}
		fmt.Fprintf(w, "suppression-ban %s:%d: [%s] %s; remove the directive and fix the finding (rerun: %s)\n",
			f.Path, f.Line, f.Code, f.Message, rerun)
	}
}

// ParsePaths splits a NUL-delimited path list, dropping empty entries.
func ParsePaths(data []byte) []string {
	var paths []string
	for _, p := range bytes.Split(data, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	return paths
}
