package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const catalogText = `
version: "%VERSION%"
checks {
  id: "ban"
  definition: "checks/rules.go"
  stages: [STAGE_PRE_COMMIT, STAGE_FULL]
  classification: CLASSIFICATION_GUARANTEE
  selector { include: "\\.go$" }
  command: ["x"]
}
checks {
  id: "other"
  definition: "adapters/native.sh"
  stages: [STAGE_FULL]
  classification: CLASSIFICATION_GUARANTEE
  command: ["y"]
}
`

const rulesText = "{\"S001\", \"first\", re(`one`)},\n{\"S002\", \"second\", re(`two`)},\n"

func tree(t *testing.T, version, catalog, rules string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range map[string]string{
		Catalog:                strings.ReplaceAll(catalog, "%VERSION%", version),
		"checks/rules.go":      rules,
		"checks/rules_test.go": "ignored\n",
		"adapters/native.sh":   "tool\n",
	} {
		file := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func notes(t *testing.T, older, newer string) string {
	t.Helper()
	text, err := Notes(older, newer)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func wants(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(text, part) {
			t.Errorf("notes lack %q:\n%s", part, text)
		}
	}
}

func TestNoChangeSaysSo(t *testing.T) {
	text := notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", catalogText, rulesText))
	wants(t, text, "1 to 2", "No rule changed")
}

// The case a list of ids could not see: the id stays, and what it bans changes.
func TestAChangeInsideAnExistingDefinitionIsVisible(t *testing.T) {
	changed := strings.Replace(rulesText, "re(`two`)", "re(`two|2`)", 1)
	text := notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", catalogText, changed))
	wants(t, text, "| `ban` | `checks/rules.go` changed |", "a definition line was removed or replaced",
		"- {\"S002\", \"second\", re(`two`)},", "+ {\"S002\", \"second\", re(`two|2`)},")
	if strings.Contains(text, "`other`") {
		t.Errorf("a rule that did not change is listed:\n%s", text)
	}
}

func TestAMessageChangeIsVisible(t *testing.T) {
	changed := strings.Replace(rulesText, `"first"`, `"the first"`, 1)
	wants(t, notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", catalogText, changed)), "`checks/rules.go` changed")
}

func TestAnAddedLineIsNotFlagged(t *testing.T) {
	text := notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", catalogText, rulesText+"{\"S003\", \"third\", re(`three`)},\n"))
	wants(t, text, "`checks/rules.go` changed", "+ {\"S003\"")
	if strings.Contains(text, "removed or replaced") {
		t.Errorf("an added code is not a weakening:\n%s", text)
	}
}

func TestATestFileIsNotADefinition(t *testing.T) {
	older := tree(t, "1", strings.ReplaceAll(catalogText, `"checks/rules.go"`, `"checks"`), rulesText)
	newer := tree(t, "2", strings.ReplaceAll(catalogText, `"checks/rules.go"`, `"checks"`), rulesText)
	if err := os.WriteFile(filepath.Join(newer, "checks/rules_test.go"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wants(t, notes(t, older, newer), "No rule changed")
}

func TestRulesAddedAndRemoved(t *testing.T) {
	added := catalogText + `checks { id: "new" definition: "adapters/native.sh" stages: [STAGE_FULL] command: ["z"] }`
	wants(t, notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", added, rulesText)), "| `new` | added |")
	wants(t, notes(t, tree(t, "1", added, rulesText), tree(t, "2", catalogText, rulesText)), "| `new` | removed | the rule is gone |")
}

func TestWeakeningIsFlagged(t *testing.T) {
	for name, change := range map[string][2]string{
		"dropped from STAGE_PRE_COMMIT": {"[STAGE_PRE_COMMIT, STAGE_FULL]", "[STAGE_FULL]"},
		"no longer a guarantee":         {"classification: CLASSIFICATION_GUARANTEE\n  selector", "classification: CLASSIFICATION_SIGNAL\n  selector"},
		"selector changed":              {`include: "\\.go$"`, `include: "\\.ts$"`},
		"now allows empty input":        {`id: "ban"`, "id: \"ban\"\n  allow_empty: true"},
		"now skipped offline":           {`id: "ban"`, "id: \"ban\"\n  needs_network: true"},
	} {
		text := notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", strings.Replace(catalogText, change[0], change[1], 1), rulesText))
		wants(t, text, "| `ban` | entry changed | "+name)
	}
}

func TestAStrongerEntryIsNotFlagged(t *testing.T) {
	stronger := strings.Replace(catalogText, `command: ["y"]`, `command: ["y", "--strict"]`, 1)
	text := notes(t, tree(t, "1", catalogText, rulesText), tree(t, "2", stronger, rulesText))
	wants(t, text, "| `other` | entry changed |  |")
}

func TestTheModuleVersionMatchesTheCatalog(t *testing.T) {
	root := tree(t, "1.2.3", catalogText, rulesText)
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "MODULE.bazel"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`module(name = "m", version = "1.2.3")`)
	if _, err := Load(root); err != nil {
		t.Fatal(err)
	}
	write(`module(name = "m", version = "1.2.4")`)
	if _, err := Load(root); err == nil {
		t.Fatal("a catalog and module on different versions passed")
	}
}

func TestAMissingDefinitionFails(t *testing.T) {
	root := tree(t, "1", catalogText, rulesText)
	if err := os.Remove(filepath.Join(root, "adapters/native.sh")); err != nil {
		t.Fatal(err)
	}
	if _, err := Notes(root, root); err == nil || !strings.Contains(err.Error(), "other") {
		t.Fatalf("got %v", err)
	}
	bare := tree(t, "1", `version: "1" checks { id: "x" }`, rulesText)
	if _, err := Notes(bare, bare); err == nil || !strings.Contains(err.Error(), "names no definition") {
		t.Fatalf("got %v", err)
	}
}

const secret = "operator-only-text"

func plant(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// Each case changes a definition between two valid trees, so a read outside the tree would be quoted.
func TestNothingOutsideTheTreeIsRead(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	plant(t, filepath.Join(outside, "file"), secret+"\n")
	plant(t, filepath.Join(outside, "other"), secret+" again\n")
	definition := func(path string) string {
		return strings.ReplaceAll(catalogText, `"checks/rules.go"`, `"`+path+`"`)
	}
	cases := map[string]func(older, newer string) (string, string){
		"a dotted path": func(older, newer string) (string, string) {
			plant(t, filepath.Join(filepath.Dir(older), "outside"), secret+"\n")
			plant(t, filepath.Join(filepath.Dir(newer), "outside"), secret+" again\n")
			return definition("../outside"), definition("../outside")
		},
		"a dotted path through the tree": func(older, newer string) (string, string) {
			plant(t, filepath.Join(filepath.Dir(older), "outside"), secret+"\n")
			plant(t, filepath.Join(filepath.Dir(newer), "outside"), secret+" again\n")
			return definition("checks/../../outside"), definition("checks/../../outside")
		},
		"an absolute path": func(_, _ string) (string, string) {
			return definition(filepath.Join(outside, "file")), definition(filepath.Join(outside, "other"))
		},
		"a linked file": func(older, newer string) (string, string) {
			link(t, filepath.Join(outside, "file"), filepath.Join(older, "checks/rules.go"))
			link(t, filepath.Join(outside, "other"), filepath.Join(newer, "checks/rules.go"))
			return catalogText, catalogText
		},
		"a relative link": func(older, newer string) (string, string) {
			plant(t, filepath.Join(filepath.Dir(older), "outside"), secret+"\n")
			plant(t, filepath.Join(filepath.Dir(newer), "outside"), secret+" again\n")
			link(t, "../../outside", filepath.Join(older, "checks/rules.go"))
			link(t, "../../outside", filepath.Join(newer, "checks/rules.go"))
			return catalogText, catalogText
		},
		"a linked parent directory": func(older, newer string) (string, string) {
			link(t, outside, filepath.Join(older, "linked"))
			link(t, outside, filepath.Join(newer, "linked"))
			plant(t, filepath.Join(outside, "both"), secret+"\n")
			return definition("linked/file"), definition("linked/other")
		},
		"a link inside a definition directory": func(older, newer string) (string, string) {
			link(t, filepath.Join(outside, "file"), filepath.Join(older, "checks/extra.go"))
			link(t, filepath.Join(outside, "other"), filepath.Join(newer, "checks/extra.go"))
			return definition("checks"), definition("checks")
		},
		"a link at a path only the old rule names": func(_, newer string) (string, string) {
			link(t, filepath.Join(outside, "other"), filepath.Join(newer, "checks/rules.go"))
			plant(t, filepath.Join(newer, "checks/kept.go"), "kept\n")
			return catalogText, definition("checks/kept.go")
		},
		"a linked catalog": func(_, newer string) (string, string) {
			plant(t, filepath.Join(outside, Catalog), strings.ReplaceAll(catalogText, "%VERSION%", secret))
			link(t, filepath.Join(outside, Catalog), filepath.Join(newer, Catalog))
			return catalogText, catalogText
		},
		"a linked module file": func(_, newer string) (string, string) {
			plant(t, filepath.Join(outside, "module"), `module(version = "2")`)
			link(t, filepath.Join(outside, "module"), filepath.Join(newer, "MODULE.bazel"))
			return catalogText, catalogText
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			older, newer := tree(t, "1", catalogText, rulesText), tree(t, "2", catalogText, rulesText)
			before, after := arrange(older, newer)
			plant(t, filepath.Join(older, Catalog), strings.ReplaceAll(before, "%VERSION%", "1"))
			if fi, err := os.Lstat(filepath.Join(newer, Catalog)); err == nil && fi.Mode().IsRegular() {
				plant(t, filepath.Join(newer, Catalog), strings.ReplaceAll(after, "%VERSION%", "2"))
			}
			text, err := Notes(older, newer)
			if err == nil {
				t.Fatalf("a read outside the tree passed:\n%s", text)
			}
			if strings.Contains(text, secret) || strings.Contains(err.Error(), secret) {
				t.Fatalf("outside text was quoted: %q %v", text, err)
			}
		})
	}
}

// A link that stays inside the tree is an ordinary definition, and a change through it still shows.
func TestALinkInsideTheTreeStillShowsItsChange(t *testing.T) {
	definition := strings.ReplaceAll(catalogText, `"checks/rules.go"`, `"checks/current.go"`)
	make := func(version, rules string) string {
		root := tree(t, version, definition, rules)
		link(t, "rules.go", filepath.Join(root, "checks/current.go"))
		return root
	}
	changed := strings.Replace(rulesText, "re(`two`)", "re(`two|2`)", 1)
	wants(t, notes(t, make("1", rulesText), make("2", changed)), "| `ban` | `checks/current.go` changed |", "+ {\"S002\", \"second\", re(`two|2`)},")
}

const (
	shaOne = "1111111111111111111111111111111111111111"
	shaTwo = "2222222222222222222222222222222222222222"
)

func TestARecordedMappingDoesNotChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "RELEASES")
	if err := Record(path, "v1", shaOne); err != nil {
		t.Fatal(err)
	}
	if err := Record(path, "v1", shaOne); err != nil {
		t.Fatalf("the same mapping again is no change: %v", err)
	}
	if err := Record(path, "v1", shaTwo); err == nil || !strings.Contains(err.Error(), "does not change") {
		t.Fatalf("got %v", err)
	}
	if err := Record(path, "v2", "main"); err == nil {
		t.Fatal("a branch was recorded")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "v1 "+shaOne+"\n" {
		t.Fatalf("record: %q", data)
	}
}

func TestCheckFindsAMovedTag(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "one")
	first := git("rev-parse", "HEAD")
	git("tag", "v1")
	path := filepath.Join(dir, "RELEASES")
	if err := Record(path, "v1", first); err != nil {
		t.Fatal(err)
	}
	if problems, err := Check(path, dir); err != nil || len(problems) != 0 {
		t.Fatalf("control: %v %v", problems, err)
	}
	git("commit", "-q", "--allow-empty", "-m", "two")
	git("tag", "-f", "v1")
	problems, err := Check(path, dir)
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0], "moved") {
		t.Fatalf("got %v %v", problems, err)
	}
	git("tag", "-d", "v1")
	if problems, _ := Check(path, dir); len(problems) != 1 || !strings.Contains(problems[0], "missing") {
		t.Fatalf("got %v", problems)
	}
}

func TestCommand(t *testing.T) {
	binary := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("RELEASE"))
	run := func(want int, args ...string) string {
		t.Helper()
		output, err := exec.Command(binary, args...).CombinedOutput()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if code != want {
			t.Fatalf("%v: exit %d, want %d: %s", args, code, want, output)
		}
		return string(output)
	}
	root := tree(t, "1", catalogText, rulesText)
	wants(t, run(0, "notes", root, root), "No rule changed")
	run(1, "notes", root, filepath.Join(root, "absent"))
	dir := t.TempDir()
	record := filepath.Join(dir, "RELEASES")
	run(0, "record", record, "v1", shaOne)
	run(1, "record", record, "v1", shaTwo)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	wants(t, run(1, "check", record, dir), "v1 is recorded and missing")
	empty := filepath.Join(t.TempDir(), "empty")
	run(0, "check", empty, dir)
	if err := os.WriteFile(empty, []byte("malformed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(1, "check", empty, dir)
	run(2)
	run(2, "unknown")
}
