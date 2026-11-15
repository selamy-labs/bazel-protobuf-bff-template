package inputs

import (
	"fmt"
	"math/rand"
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
	}
	for i := 0; i < 800; i++ {
		line := pieces[i%len(pieces)]
		for n := rng.Intn(5); n > 0; n-- {
			line += pieces[rng.Intn(len(pieces))]
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
