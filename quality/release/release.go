// Package release holds the two small steps of cutting a quality release (spec 008 FR-006, FR-008):
// notes from a diff of the rule catalog, and a record of which commit each tag named. The notes inform
// a reader and gate nothing, and upstream #26 owns detecting and authorizing policy changes.
package release

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// Catalog is the rule catalog's path in a module's tree.
const Catalog = "rules.textproto"

// maxLines bounds what the notes quote of one changed definition file.
const maxLines = 20

// Load reads a tree's catalog, and fails if a rule names a definition the tree does not hold. Every read
// here and in files goes through os.Root, because the notes quote what they read: a path or a symbolic
// link that leaves the selected tree is refused. A link that stays inside it is an ordinary path.
func Load(root string) (*registrypb.Registry, error) {
	tree, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	data, err := tree.ReadFile(Catalog)
	if err != nil {
		return nil, err
	}
	catalog := &registrypb.Registry{}
	if err := (prototext.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, catalog); err != nil {
		return nil, fmt.Errorf("%s: %w", root, err)
	}
	module, err := tree.ReadFile("MODULE.bazel")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", root, err)
	}
	if want := `version = "` + catalog.GetVersion() + `"`; err == nil && !strings.Contains(string(module), want) {
		return nil, fmt.Errorf("%s: MODULE.bazel lacks %s", root, want)
	}
	for _, rule := range catalog.GetChecks() {
		if len(rule.GetDefinition()) == 0 {
			return nil, fmt.Errorf("%s: rule %s names no definition", root, rule.GetId())
		}
		for _, path := range rule.GetDefinition() {
			if _, err := tree.Stat(path); err != nil {
				return nil, fmt.Errorf("%s: rule %s: %w", root, rule.GetId(), err)
			}
		}
	}
	return catalog, nil
}

// files lists the regular files of a definition: the path itself, or a directory's files without tests.
func files(root, path string) (map[string]string, error) {
	tree, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	files := tree.FS()
	found := map[string]string{}
	err = fs.WalkDir(files, path, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, err := fs.ReadFile(files, name)
		if err == nil {
			found[name] = string(data)
		}
		return err
	})
	return found, err
}

// lineChange is what changed in one file: lines only the old tree has, and lines only the new one has.
type lineChange struct {
	path         string
	removed, add []string
}

func onlyIn(lines, other []string) []string {
	var kept []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !slices.Contains(other, line) {
			kept = append(kept, line)
		}
	}
	return kept
}

func definitionChanges(oldRoot, newRoot string, paths []string) ([]lineChange, error) {
	var changes []lineChange
	for _, path := range paths {
		before, err := files(oldRoot, path)
		if err != nil {
			return nil, err
		}
		after, err := files(newRoot, path)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(before)+len(after))
		for name := range before {
			names = append(names, name)
		}
		for name := range after {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range slices.Compact(names) {
			if before[name] == after[name] {
				continue
			}
			a, b := strings.Split(before[name], "\n"), strings.Split(after[name], "\n")
			changes = append(changes, lineChange{name, onlyIn(a, b), onlyIn(b, a)})
		}
	}
	return changes, nil
}

// weakening names what in a rule's own entry can only make it judge less.
func weakening(before, after *registrypb.Check) []string {
	var found []string
	for _, stage := range before.GetStages() {
		if !slices.Contains(after.GetStages(), stage) {
			found = append(found, "dropped from "+stage.String())
		}
	}
	if before.GetClassification() == registrypb.Classification_CLASSIFICATION_GUARANTEE &&
		after.GetClassification() != registrypb.Classification_CLASSIFICATION_GUARANTEE {
		found = append(found, "no longer a guarantee")
	}
	if !proto.Equal(before.GetSelector(), after.GetSelector()) {
		found = append(found, "selector changed")
	}
	if !before.GetAllowEmpty() && after.GetAllowEmpty() {
		found = append(found, "now allows empty input")
	}
	if !before.GetNeedsNetwork() && after.GetNeedsNetwork() {
		found = append(found, "now skipped offline")
	}
	return found
}

func text(check *registrypb.Check) string {
	copied := proto.Clone(check).(*registrypb.Check)
	copied.Witness = ""
	return prototext.Format(copied)
}

// Notes describes every rule change between two trees of the module. A line removed from a definition
// is flagged for review, because this tool cannot know that a replacement is at least as strict.
func Notes(oldRoot, newRoot string) (string, error) {
	before, err := Load(oldRoot)
	if err != nil {
		return "", err
	}
	after, err := Load(newRoot)
	if err != nil {
		return "", err
	}
	previous := map[string]*registrypb.Check{}
	for _, rule := range before.GetChecks() {
		previous[rule.GetId()] = rule
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# Rule changes: %s to %s\n\n| Rule | Change | Weakening to review |\n| --- | --- | --- |\n", before.GetVersion(), after.GetVersion())
	quoted := map[string]lineChange{}
	changed := 0
	row := func(id, change, flagged string) {
		changed++
		fmt.Fprintf(&out, "| `%s` | %s | %s |\n", id, change, flagged)
	}
	for _, rule := range after.GetChecks() {
		old := previous[rule.GetId()]
		delete(previous, rule.GetId())
		if old == nil {
			row(rule.GetId(), "added", "")
			continue
		}
		paths := append(slices.Clone(rule.GetDefinition()), old.GetDefinition()...)
		slices.Sort(paths)
		changes, err := definitionChanges(oldRoot, newRoot, slices.Compact(paths))
		if err != nil {
			return "", err
		}
		flagged := weakening(old, rule)
		var what []string
		if text(old) != text(rule) {
			what = append(what, "entry changed")
		}
		for _, change := range changes {
			what = append(what, "`"+change.path+"` changed")
			quoted[change.path] = change
			if len(change.removed) > 0 && !slices.Contains(flagged, "a definition line was removed or replaced") {
				flagged = append(flagged, "a definition line was removed or replaced")
			}
		}
		if len(what) > 0 {
			row(rule.GetId(), strings.Join(what, ", "), strings.Join(flagged, "; "))
		}
	}
	gone := make([]string, 0, len(previous))
	for id := range previous {
		gone = append(gone, id)
	}
	slices.Sort(gone)
	for _, id := range gone {
		row(id, "removed", "the rule is gone")
	}
	if changed == 0 {
		return fmt.Sprintf("# Rule changes: %s to %s\n\nNo rule changed.\n", before.GetVersion(), after.GetVersion()), nil
	}
	names := make([]string, 0, len(quoted))
	for name := range quoted {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(&out, "\n## `%s`\n\n```diff\n", name)
		lines := make([]string, 0, len(quoted[name].removed)+len(quoted[name].add))
		for _, line := range quoted[name].removed {
			lines = append(lines, "- "+line)
		}
		for _, line := range quoted[name].add {
			lines = append(lines, "+ "+line)
		}
		for index, line := range lines {
			if index == maxLines {
				fmt.Fprintf(&out, "… %d more lines\n", len(lines)-maxLines)
				break
			}
			out.WriteString(line + "\n")
		}
		out.WriteString("```\n")
	}
	return out.String(), nil
}

// readRecord reads the release record: one line per tag, naming the commit it named.
func readRecord(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	record := map[string]string{}
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) != 2 || len(fields[1]) != 40 {
			return nil, fmt.Errorf("%s: malformed line: %q", path, strings.TrimSpace(line))
		}
		if _, repeated := record[fields[0]]; repeated {
			return nil, fmt.Errorf("%s: %s is recorded twice", path, fields[0])
		}
		record[fields[0]] = fields[1]
	}
	return record, nil
}

// Record adds a tag's commit to the record. It refuses to change a mapping it already holds.
func Record(path, tag, commit string) error {
	record, err := readRecord(path)
	if err != nil {
		return err
	}
	if len(commit) != 40 || strings.Trim(commit, "0123456789abcdef") != "" {
		return fmt.Errorf("%q is not a full commit SHA", commit)
	}
	if recorded, known := record[tag]; known {
		if recorded != commit {
			return fmt.Errorf("%s is recorded as %s, and a recorded mapping does not change", tag, recorded)
		}
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(file, "%s %s\n", tag, commit); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// Check compares each recorded tag with what a repository's tags name now. A tag that moved is a
// finding for whoever cuts a release. A consumer pinned to a full SHA never reads it.
func Check(path, repository string) ([]string, error) {
	record, err := readRecord(path)
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(record))
	for tag := range record {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	var problems []string
	for _, tag := range tags {
		command := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
		command.Dir = repository
		output, err := command.Output()
		if got := strings.TrimSpace(string(output)); err != nil {
			problems = append(problems, tag+" is recorded and missing")
		} else if got != record[tag] {
			problems = append(problems, fmt.Sprintf("%s moved: recorded %s, now %s", tag, record[tag], got))
		}
	}
	return problems, nil
}
