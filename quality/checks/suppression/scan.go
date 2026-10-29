package suppression

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Finding is one banned directive at a location.
type Finding struct {
	Path    string
	Line    int
	Code    string
	Message string
}

const rerun = "bazel run //:verify"

// Scan reports banned directives in one file. Documentation and binary content are skipped:
// a directive in prose has no effect on any tool.
func Scan(path string, data []byte) []Finding {
	if isExempt(path) || strings.HasSuffix(path, ".md") || isBinary(data) {
		return nil
	}
	var findings []Finding
	for i, line := range strings.Split(string(data), "\n") {
		for _, rule := range Rules {
			if rule.Pattern.MatchString(line) {
				findings = append(findings, Finding{Path: path, Line: i + 1, Code: rule.Code, Message: rule.Description})
			}
		}
	}
	return findings
}

func isExempt(path string) bool {
	for _, prefix := range exemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isBinary(data []byte) bool {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	return bytes.IndexByte(head, 0) >= 0
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
