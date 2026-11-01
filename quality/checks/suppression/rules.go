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
	// Spans marks directives written as block comments, which their tools accept across lines.
	Spans bool
}

// Rules is the canonical ban list, one entry per inline control honored by a tool this template
// runs or plans to run. Matching is case-insensitive. Codes are stable and never reused.
var Rules = []Rule{
	{"S001", "Go lint suppression (nolint)", re(`//\s*nolint\b`), ".go", false},
	{"S002", "ESLint disable or inline configuration comment", re(`eslint-disable|/\*\s*eslint-env\b|/\*\s*eslint\s+["']?[@\w/-]+["']?\s*:|/\*\s*globals?\s+\S`), ".ts", true},
	{"S003", "TypeScript checker suppression", re(`@ts-(ignore|expect-error|nocheck)\b`), ".ts", false},
	{"S004", "buf lint ignore comment (use the API-style ledger)", re(`buf:lint:ignore`), ".proto", false},
	{"S005", "coverage exclusion marker", re(`\b(istanbul|c8|v8)\s+ignore\b|node:coverage\s+(ignore|disable)\b|pragma:\s*no\s*cover|coverage:ignore`), ".ts", true},
	{"S006", "ShellCheck directive that changes what is checked", re(`#\s*shellcheck\s[^\n]*\b(disable|enable|shell)\s*=`), ".txt", false},
	{"S007", "Python lint/type suppression", re(`#\s*noqa\b|#\s*(ruff|flake8)\s*:\s*noqa|type:\s*ignore\b|pyright:\s*ignore|pylint:\s*disable|mypy:\s*ignore`), ".py", false},
	{"S008", "gitleaks inline allow", re(`gitleaks:allow`), ".txt", false},
	{"S009", "buildifier or buildozer suppression", re(`(buildifier|buildozer):\s*(disable\s*=|leave-alone)|#\s*@unused\b`), ".txt", false},
	{"S010", "zizmor ignore comment", re(`zizmor:\s*ignore`), ".yml", false},
	{"S011", "staticcheck or gosec suppression", re(`//\s*lint:(file-)?ignore\b|#nosec\b`), ".go", false},
	{"S012", "Prettier ignore comment", re(`prettier-ignore`), ".ts", false},
}

func re(pattern string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)` + pattern)
}

// patternTable is exempt because it must contain the banned strings; exact path, not a prefix.
const patternTable = "quality/checks/suppression/rules.go"
