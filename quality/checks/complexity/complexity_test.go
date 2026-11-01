package complexity

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// nested has cognitive 3 (if 1, nested if 2) and cyclomatic 3 (two branches plus one).
const nested = "package p\n\nfunc nested(a, b bool) int {\n\tif a {\n\t\tif b {\n\t\t\treturn 1\n\t\t}\n\t}\n\treturn 0\n}\n"

// plainFunc has cognitive 0 and cyclomatic 1.
const plainFunc = "func plain() int { return 0 }\n"

const plain = "package p\n\n" + plainFunc

func budgets(t *testing.T, text string) *Budgets {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budgets.json")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const base = `{"measured_at": "test", "ceilings": {"cognitive": 2, "cyclomatic": 2},
 "typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 2}},
 "excludes": [{"pattern": "_test\\.go$", "reason": "test-code"}],
 "debt": [{"function": "a.go nested", "owner": "t", "values": {"cognitive": 6, "cyclomatic": 6}}]}`

const debtAtThree = `{"measured_at": "test", "ceilings": {"cognitive": 2, "cyclomatic": 2},
 "excludes": [{"pattern": "_test\\.go$", "reason": "test-code"}],
 "debt": [{"function": "a.go nested", "owner": "t", "values": {"cognitive": 3, "cyclomatic": 3}}]}`

func sources(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		text, ok := files[path]
		if !ok {
			return nil, errors.New(path + " is missing")
		}
		return []byte(text), nil
	}
}

func TestMeasurementMatchesTheTools(t *testing.T) {
	measured, err := MeasureFile("a.go", []byte(nested+plainFunc))
	if err != nil {
		t.Fatal(err)
	}
	if len(measured) != 2 {
		t.Fatalf("measured %d functions, want 2", len(measured))
	}
	if got := measured[0].Values; got[Cognitive] != 3 || got[Cyclomatic] != 3 || measured[0].Function() != "a.go nested" {
		t.Errorf("nested: %v %q", got, measured[0].Function())
	}
	if got := measured[1].Values; got[Cognitive] != 0 || got[Cyclomatic] != 1 {
		t.Errorf("plain: %v", got)
	}
}

// deepBody is ten nested ifs: cognitive 55 and cyclomatic 11 for a declaration, by the tools' own counts.
func deepBody() string {
	body := ""
	for i := 0; i < 10; i++ {
		body += strings.Repeat("\t", i+1) + fmt.Sprintf("if x > %d {\n", i)
	}
	body += strings.Repeat("\t", 11) + "return 1\n"
	for i := 9; i >= 0; i-- {
		body += strings.Repeat("\t", i+1) + "}\n"
	}
	return body
}

// TestPackageLevelLiteralsAreMeasured is the regression for two independent controls: the first planted ten
// nested ifs in a map callback and saw the file pass with no function measured; the second, after a
// hand-enumerated fix, planted the same literal as an immediately invoked call and as an operand and saw both
// pass. Every expression shape of a package-level initializer is measured now, by one walk.
func TestPackageLevelLiteralsAreMeasured(t *testing.T) {
	deep := "func(x int) int {\n" + deepBody() + "\treturn 0\n}"
	cases := map[string]struct {
		source string
		names  []string
	}{
		"a declaration":     {"package p\n\nfunc Deep(x int) int {\n" + deepBody() + "\treturn 0\n}\n", []string{"Deep"}},
		"a variable":        {"package p\n\nvar Deep = " + deep + "\n", []string{"Deep@3:12"}},
		"a map callback":    {"package p\n\nvar callbacks = map[string]func(int) int{\"deep\": " + deep + "}\n", []string{"callbacks@3:50"}},
		"a slice element":   {"package p\n\nvar handlers = []func(int) int{" + deep + "}\n", []string{"handlers@3:32"}},
		"a struct field":    {"package p\n\ntype T struct{ Run func(int) int }\n\nvar cfg = T{Run: " + deep + "}\n", []string{"cfg@5:18"}},
		"a call argument":   {"package p\n\nfunc wrap(f func(int) int) func(int) int { return f }\n\nvar wrapped = wrap(" + deep + ")\n", []string{"wrap", "wrapped@5:20"}},
		"an immediate call": {"package p\n\nvar result = (" + deep + ")(1)\n", []string{"result@3:15"}},
		"an operand":        {"package p\n\nvar sum = (" + deep + ")(1) + 1\n", []string{"sum@3:12"}},
		"an index base":     {"package p\n\nvar first = []func(int) int{" + deep + "}[0]\n", []string{"first@3:29"}},
		"a map key":         {"package p\n\nvar keyed = map[string]int{(" + deep + ")(1) == 0: 1}\n", []string{"keyed@3:29"}},
	}
	b := budgets(t, `{"measured_at": "test", "ceilings": {"cognitive": 20, "cyclomatic": 10}}`)
	for name, c := range cases {
		measured, err := MeasureFile("a.go", []byte(c.source))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var names []string
		for _, m := range measured {
			names = append(names, m.Name)
		}
		if strings.Join(names, ",") != strings.Join(c.names, ",") {
			t.Errorf("%s: measured %v, want %v", name, names, c.names)
		}
		last := measured[len(measured)-1]
		if last.Values[Cognitive] != 55 || last.Values[Cyclomatic] != 11 {
			t.Errorf("%s: %s measured %v, want cognitive 55 and cyclomatic 11", name, last.Name, last.Values)
		}
		_, problems, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": c.source}))
		if err != nil || len(problems) != 2 || !strings.Contains(problems[0], last.Name+": cognitive 55 is over its ceiling 20") {
			t.Errorf("%s: problems %v, err %v", name, problems, err)
		}
	}
	// A closure inside a package-level literal is part of that literal's measure, and a literal inside a declared
	// function is part of that function's, so neither is listed twice; a file with no function is still a measurement.
	nestedSource := "package p\n\nvar outer = func() func() int {\n\treturn func() int {\n\t\tif true {\n\t\t\treturn 1\n\t\t}\n\t\treturn 0\n\t}\n}\n\nfunc Decl() func() int { return func() int { return 2 } }\n"
	measured, err := MeasureFile("a.go", []byte(nestedSource))
	if err != nil || len(measured) != 2 || measured[0].Name != "Decl" || measured[1].Name != "outer@3:13" || measured[1].Values[Cognitive] != 2 {
		t.Errorf("nested: %+v %v", measured, err)
	}
	if measured, err := MeasureFile("a.go", []byte("package p\n\nvar x = 1\n\nvar y = []int{1, 2}\n")); err != nil || len(measured) != 0 {
		t.Errorf("no function: %+v %v", measured, err)
	}
	if _, problems, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": "package p\n\nvar simple = func() int { return 0 }\n"})); err != nil || len(problems) != 0 {
		t.Errorf("restored control: %v %v", problems, err)
	}
}

func TestPlantedFunctionOverTheCeilingIsNamedWithBothNumbers(t *testing.T) {
	b := budgets(t, `{"measured_at": "test", "ceilings": {"cognitive": 2, "cyclomatic": 2}}`)
	_, problems, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": nested}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.go nested: cognitive 3 is over its ceiling 2", "a.go nested: cyclomatic 3 is over its ceiling 2"}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
	result, problems, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": plain}))
	if err != nil || len(problems) != 0 {
		t.Fatalf("the restored control failed: %v %v", problems, err)
	}
	if result.Files != 1 || result.Functions != 1 || result.Max[Cyclomatic] != 1 {
		t.Errorf("result %+v", result)
	}
}

func TestDebtPassesAtItsBudgetAndFailsAboveIt(t *testing.T) {
	b := budgets(t, debtAtThree)
	if _, problems, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": nested})); err != nil || len(problems) != 0 {
		t.Fatalf("debt at its budget: %v %v", problems, err)
	}
	deeper := strings.Replace(nested, "\t\t\treturn 1", "\t\t\tif a && b {\n\t\t\t\treturn 2\n\t\t\t}\n\t\t\treturn 1", 1)
	_, problems, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": deeper}))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 2 || !strings.Contains(problems[0], "a.go nested: cognitive 7 is over its debt budget 3") || !strings.Contains(problems[1], "cyclomatic 5 is over its debt budget 3") {
		t.Errorf("problems: %v", problems)
	}
}

func TestNothingMeasuredIsAnErrorAndExcludedFilesAreCounted(t *testing.T) {
	b := budgets(t, debtAtThree)
	if _, _, err := b.Check(nil, sources(nil)); err == nil {
		t.Error("an empty list passed")
	}
	// A staged change of test files only is nothing to judge: the hook refused such a commit once.
	result, problems, err := b.Check([]string{"a_test.go", "b.md"}, sources(map[string]string{"a_test.go": nested}))
	if err != nil || len(problems) != 0 || result.Excluded != 1 || result.Files != 0 {
		t.Errorf("only excluded input: %+v %v %v", result, problems, err)
	}
	if _, _, err := b.Check([]string{"b.md"}, sources(nil)); err == nil {
		t.Error("a list with no Go file passed")
	}
	if result, _, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": "package p\n\nvar x = 1\n"})); err != nil || result.Files != 1 || result.Functions != 0 {
		t.Errorf("a file with no function is a measurement: %+v %v", result, err)
	}
	if _, _, err := b.Check([]string{"a.go"}, sources(map[string]string{"a.go": "package p\n\nfunc broken( {\n"})); err == nil {
		t.Error("a file that does not parse passed")
	}
}

func TestLoadRefusesBudgetsThatAreNotMeasurementsOrNotDebt(t *testing.T) {
	for name, text := range map[string]string{
		"debt not above the ceiling":     `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "debt": [{"function": "a.go f", "owner": "t", "values": {"cognitive": 5}}]}`,
		"exclude without a reason":       `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "excludes": [{"pattern": "x"}]}`,
		"debt without an owner":          `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "debt": [{"function": "a.go f", "values": {"cognitive": 6}}]}`,
		"missing ceiling":                `{"ceilings": {"cognitive": 5}}`,
		"unknown field":                  `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "waiver": true}`,
		"unknown metric":                 `{"ceilings": {"cognitive": 5, "cyclomatic": 5, "lines": 1}}`,
		"typescript without its ceiling": `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "typescript": {"ceilings": {}}}`,
		"typescript ceiling of zero":     `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "typescript": {"ceilings": {"cyclomatic": 0}}}`,
		"unknown typescript metric":      `{"ceilings": {"cognitive": 5, "cyclomatic": 5}, "typescript": {"ceilings": {"cyclomatic": 5, "cognitive": 5}}}`,
	} {
		path := filepath.Join(t.TempDir(), "b.json")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestCompareRefusesEveryLeniencyAndPassesEveryStrictness(t *testing.T) {
	before := budgets(t, base)
	cases := []struct {
		name, text, refused string
	}{
		{"the cognitive ceiling falls", strings.Replace(base, `"cognitive": 2,`, `"cognitive": 1,`, 1), ""},
		{"the cognitive ceiling rises", strings.Replace(base, `"cognitive": 2,`, `"cognitive": 4,`, 1), "the cognitive ceiling rises from 2 to 4"},
		{"the cyclomatic ceiling falls", strings.Replace(base, `"cyclomatic": 2}`, `"cyclomatic": 1}`, 1), ""},
		{"the cyclomatic ceiling rises", strings.Replace(base, `"cyclomatic": 2}`, `"cyclomatic": 3}`, 1), "the cyclomatic ceiling rises from 2 to 3"},
		{"a debt value falls", strings.Replace(base, `"cyclomatic": 6}`, `"cyclomatic": 5}`, 1), ""},
		{"a debt value rises", strings.Replace(base, `"cognitive": 6,`, `"cognitive": 9,`, 1), `debt "a.go nested": cognitive rises to 9`},
		{"a debt entry goes", strings.Replace(base, `[{"function": "a.go nested", "owner": "t", "values": {"cognitive": 6, "cyclomatic": 6}}]`, `[]`, 1), ""},
		{"a debt entry appears", strings.Replace(base, `"debt": [`, `"debt": [{"function": "b.go g", "owner": "t", "values": {"cognitive": 3}}, `, 1), `debt "b.go g" is new; a budget is measured, not granted`},
		{"an exclusion appears", strings.Replace(base, `"excludes": [`, `"excludes": [{"pattern": "^legacy/", "reason": "old"}, `, 1), `exclude "^legacy/" is new`},
		{"the typescript ceiling falls", strings.Replace(base, `"typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 2}}`, `"typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 1}}`, 1), ""},
		{"the typescript ceiling rises", strings.Replace(base, `"typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 2}}`, `"typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 3}}`, 1), "the typescript cyclomatic ceiling rises from 2 to 3"},
		{"the typescript ceilings go", strings.Replace(base, ` "typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 2}},`, ``, 1), "the typescript ceilings were removed"},
	}
	for _, c := range cases {
		problems := Compare(before, budgets(t, c.text))
		if c.refused == "" && len(problems) != 0 {
			t.Errorf("%s: refused: %v", c.name, problems)
		}
		if c.refused != "" && (len(problems) != 1 || problems[0] != c.refused) {
			t.Errorf("%s: %v, want %q", c.name, problems, c.refused)
		}
	}
	if problems := Compare(before, before); len(problems) != 0 {
		t.Errorf("an unchanged file was refused: %v", problems)
	}
	// A base from before the typescript section judges nothing about it, and a section that appears is stricter.
	without := budgets(t, strings.Replace(base, ` "typescript": {"measured_at": "test", "ceilings": {"cyclomatic": 2}},`, ``, 1))
	if problems := Compare(without, before); len(problems) != 0 {
		t.Errorf("the typescript section appearing was refused: %v", problems)
	}
}

func run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	binary := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("COMPLEXITY"))
	if _, err := os.Stat(binary); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, args...)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return command.ProcessState.ExitCode(), string(output)
}

func TestBinaryExitCodes(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("quality/complexity_budgets.json", `{"measured_at": "test", "ceilings": {"cognitive": 2, "cyclomatic": 2}}`)
	write("src/a.go", plain)
	write("list", "src/a.go\x00")
	if code, out := run(t, "check", "--root="+root, "--list="+filepath.Join(root, "list")); code != 0 || !strings.Contains(out, "1 files measured") {
		t.Errorf("control: exit %d\n%s", code, out)
	}
	write("src/a.go", nested)
	if code, out := run(t, "check", "--root="+root, "--list="+filepath.Join(root, "list")); code != 1 || !strings.Contains(out, "src/a.go nested: cognitive 3 is over its ceiling 2") {
		t.Errorf("planted defect: exit %d\n%s", code, out)
	}
	write("list", "")
	if code, out := run(t, "check", "--root="+root, "--list="+filepath.Join(root, "list")); code != 2 || !strings.Contains(out, "no Go file was measured") {
		t.Errorf("empty list: exit %d\n%s", code, out)
	}
	if code, _ := run(t, "check", "--root="+root); code != 2 {
		t.Errorf("missing list: exit %d", code)
	}
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "quality"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "quality/complexity_budgets.json"), []byte(`{"measured_at": "test", "ceilings": {"cognitive": 3, "cyclomatic": 2}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, "compare", "--base-root="+root, "--root="+other); code != 1 || !strings.Contains(out, "the cognitive ceiling rises from 2 to 3") {
		t.Errorf("weaker proposal: exit %d\n%s", code, out)
	}
	if code, out := run(t, "compare", "--base-root="+other, "--root="+root); code != 0 || !strings.Contains(out, "at least as strict") {
		t.Errorf("stricter proposal: exit %d\n%s", code, out)
	}
	if code, _ := run(t, "compare", "--base-root="+root, "--root="+filepath.Join(root, "absent")); code != 2 {
		t.Errorf("unreadable proposal: exit %d", code)
	}
}
