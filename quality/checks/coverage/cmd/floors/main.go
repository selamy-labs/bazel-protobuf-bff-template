// Command floors judges coverage. `check` reads a finished run against the tree's own contract, and
// `compare` is built from a base and judges a proposal's contract and suppressions against that base.
//
//	floors check --root <dir> --log <bazel log> --testlogs <dir> --bazel-exit <outcome>
//	floors compare --base-root <dir> --root <dir>
//
// Exit 0 passes, 1 is a finding, and 2 means it could not judge. A run that it cannot read never passes.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/coverage"
	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/suppression"
)

const (
	contract = "quality/coverage_floors.json"
	vendored = "tools/speckit/scaffold.sha256"
)

var testLine = regexp.MustCompile(`^(\S+)\s+(?:\(cached\)\s+)?(PASSED|FAILED|TIMEOUT|FLAKY|NO STATUS|SKIPPED|FAILED TO BUILD)\b`)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: floors check|compare ...")
		os.Exit(2)
	}
	var code int
	var err error
	switch os.Args[1] {
	case "check":
		code, err = check(os.Args[2:])
	case "compare":
		code, err = compare(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "floors: %v\n", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func tracked(root string) ([]string, error) {
	command := exec.Command("git", "ls-files", "-z")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", root, err)
	}
	var files []string
	for _, name := range bytes.Split(output, []byte{0}) {
		if len(name) > 0 {
			files = append(files, string(name))
		}
	}
	return files, nil
}

func report(problems []string) int {
	for _, problem := range problems {
		fmt.Println("coverage-floor " + problem)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

// passedTargets reads the run's own log, so a report left by an older run is never read. A test that
// did not pass ends the judgment: a report beside a failure is not evidence.
func passedTargets(log string) ([]string, []string, error) {
	data, err := os.ReadFile(log)
	if err != nil {
		return nil, nil, err
	}
	var passed, problems []string
	for line := range strings.Lines(string(data)) {
		if match := testLine.FindStringSubmatch(line); match != nil {
			if match[2] == "PASSED" {
				passed = append(passed, match[1])
			} else {
				problems = append(problems, match[1]+" is "+match[2]+", so its coverage is not evidence")
			}
		}
	}
	if len(passed) == 0 && len(problems) == 0 {
		problems = append(problems, "the log shows no test")
	}
	return passed, problems, nil
}

// reportPath maps a label to its coverage.dat, under the one test log tree of this run.
func reportPath(testlogs, label string) string {
	label, external := strings.CutPrefix(label, "@bff_template_quality//")
	label = strings.TrimPrefix(label, "//")
	label = strings.ReplaceAll(label, ":", "/")
	if external {
		label = filepath.Join("external", "bff_template_quality+", label)
	}
	return filepath.Join(testlogs, label, "coverage.dat")
}

func check(args []string) (int, error) {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	log := flags.String("log", "", "the Bazel log of this run")
	testlogs := flags.String("testlogs", "", "the test log tree")
	exit := flags.String("bazel-exit", "", "how the run's Bazel invocation ended; only 0 can pass")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *exit == "" {
		return 2, errors.New("--bazel-exit is required")
	}
	if *exit != "0" {
		// Summary lines and reports can both look complete beside a failed invocation.
		return report([]string{"the Bazel invocation exited " + *exit + ", so its coverage is not evidence"}), nil
	}
	config, err := coverage.Load(filepath.Join(*root, contract))
	if err != nil {
		return 2, err
	}
	files, err := tracked(*root)
	if err != nil {
		return 2, err
	}
	targets, problems, err := passedTargets(*log)
	if err != nil {
		return 2, err
	}
	if len(problems) > 0 {
		return report(problems), nil
	}
	combined := coverage.NewReport()
	for _, target := range targets {
		path := reportPath(*testlogs, target)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 2, err
		}
		if err := combined.Add(target, string(data)); err != nil {
			return report([]string{err.Error()}), nil
		}
	}
	results, problems := config.Check(files, combined)
	for _, result := range results {
		fmt.Println(result)
	}
	return report(problems), nil
}

func compare(args []string) (int, error) {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	baseRoot := flags.String("base-root", "", "a worktree of the base")
	root := flags.String("root", ".", "the tested tree")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	base, err := coverage.Load(filepath.Join(*baseRoot, contract))
	if err != nil {
		return 2, err
	}
	config, err := coverage.Load(filepath.Join(*root, contract))
	if err != nil {
		return 2, err
	}
	baseFiles, err := tracked(*baseRoot)
	if err != nil {
		return 2, err
	}
	files, err := tracked(*root)
	if err != nil {
		return 2, err
	}
	problems := coverage.Compare(base, baseFiles, config, files)
	findings, err := suppressions(base, *baseRoot, *root, files)
	if err != nil {
		return 2, err
	}
	return report(append(problems, findings...)), nil
}

// suppressions runs this binary's patterns, which are the base's, over the tested tree. Only the base's
// own test data and vendored manifest are skipped, so a proposal cannot enroll a file to hide it.
func suppressions(base *coverage.Config, baseRoot, root string, files []string) ([]string, error) {
	var problems []string
	scanner := suppression.Scanner{Exempt: map[string]string{}}
	if data, err := os.ReadFile(filepath.Join(baseRoot, vendored)); err == nil {
		if scanner.Exempt, err = suppression.ParseManifest(data); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for _, file := range slices.DeleteFunc(slices.Clone(files), base.IsTestData) {
		data, err := os.ReadFile(filepath.Join(root, file))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, finding := range scanner.Scan(file, data) {
			problems = append(problems, fmt.Sprintf("%s:%d: %s %s", file, finding.Line, finding.Code, finding.Message))
		}
	}
	return problems, nil
}
