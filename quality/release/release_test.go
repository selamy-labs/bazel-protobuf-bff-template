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
