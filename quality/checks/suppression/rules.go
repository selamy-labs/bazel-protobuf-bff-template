// Package suppression bans inline directives that silence a check (spec 001 FR-019).
//
// This file is exempt from its own scan because it holds the pattern table.
package suppression

import "regexp"

// Rule is one banned suppression directive with a stable code.
type Rule struct {
	Code        string
	Description string
	Pattern     *regexp.Regexp
	fixtureExt  string
}

// Rules is the canonical ban list. Codes are stable and never reused.
var Rules = []Rule{
	{"S001", "Go lint suppression (nolint)", regexp.MustCompile(`//\s*nolint\b`), ".go"},
	{"S002", "ESLint disable directive", regexp.MustCompile(`eslint-disable`), ".ts"},
	{"S003", "TypeScript checker suppression", regexp.MustCompile(`@ts-(ignore|expect-error|nocheck)\b`), ".ts"},
	{"S004", "buf lint ignore comment (use the API-style ledger)", regexp.MustCompile(`buf:lint:ignore`), ".proto"},
	{"S005", "coverage exclusion marker", regexp.MustCompile(`\b(istanbul|c8|v8) ignore\b|pragma:\s*no\s*cover|coverage:ignore`), ".ts"},
	{"S006", "ShellCheck disable directive", regexp.MustCompile(`shellcheck\s+disable=`), ".sh"},
	{"S007", "Python lint/type suppression", regexp.MustCompile(`#\s*noqa\b|type:\s*ignore|pyright:\s*ignore`), ".py"},
	{"S008", "gitleaks inline allow", regexp.MustCompile(`gitleaks:allow`), ".txt"},
	{"S009", "buildifier disable directive", regexp.MustCompile(`buildifier:\s*disable=`), ".bzl"},
	{"S010", "zizmor ignore comment", regexp.MustCompile(`zizmor:\s*ignore`), ".yml"},
}

// exemptPrefixes are paths whose content is verified by other means: the vendored Spec Kit
// scaffold is pinned by hash manifest (spec 002 R4), and this package's pattern table and
// fixtures must contain the banned strings.
var exemptPrefixes = []string{
	".specify/",
	".claude/skills/speckit-",
	"quality/checks/suppression/testdata/",
	"quality/checks/suppression/rules.go",
}
