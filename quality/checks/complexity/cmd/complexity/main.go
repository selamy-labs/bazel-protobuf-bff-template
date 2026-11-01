// Command complexity judges per-function complexity. `check` measures the listed files of one tree
// against the tree's own budgets, and `compare` is built from a base and judges a proposal's budgets
// against that base's.
//
//	complexity check --root <dir> --list <NUL-separated paths> --budgets <path>
//	complexity compare --base-root <dir> --root <dir> [--budgets <path>]
//
// Exit 0 passes, 1 is a finding, and 2 means it could not judge. A list that measures nothing never passes.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/command"
	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/complexity"
)

const defaultBudgets = "quality/complexity_budgets.json"

func main() {
	os.Exit(command.Dispatch("complexity", os.Args[1:], map[string]func([]string) (int, error){"check": check, "compare": compare}, os.Stderr))
}

func listed(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, name := range bytes.Split(data, []byte{0}) {
		if len(name) > 0 {
			paths = append(paths, string(name))
		}
	}
	return paths, nil
}

func check(args []string) (int, error) {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	root := flags.String("root", "", "the repository root")
	list := flags.String("list", "", "a NUL-separated list of repository-relative paths")
	budgetsPath := flags.String("budgets", defaultBudgets, "the budgets file, relative to the root")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *root == "" || *list == "" {
		return 2, fmt.Errorf("--root and --list are required")
	}
	budgets, err := complexity.Load(filepath.Join(*root, *budgetsPath))
	if err != nil {
		return 2, err
	}
	paths, err := listed(*list)
	if err != nil {
		return 2, err
	}
	result, problems, err := budgets.Check(paths, func(path string) ([]byte, error) {
		return os.ReadFile(filepath.Join(*root, path))
	})
	if err != nil {
		return 2, err
	}
	fmt.Println("measurement: complexity " + result.String() + "; budgets measured at " + budgets.MeasuredAt)
	for _, problem := range problems {
		fmt.Println("complexity " + problem)
	}
	if len(problems) > 0 {
		return 1, nil
	}
	return 0, nil
}

func compare(args []string) (int, error) {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	baseRoot := flags.String("base-root", "", "the base tree")
	root := flags.String("root", "", "the proposal tree")
	budgetsPath := flags.String("budgets", defaultBudgets, "the budgets file, relative to each root")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *baseRoot == "" || *root == "" {
		return 2, fmt.Errorf("--base-root and --root are required")
	}
	base, err := complexity.Load(filepath.Join(*baseRoot, *budgetsPath))
	if err != nil {
		return 2, fmt.Errorf("base: %w", err)
	}
	proposal, err := complexity.Load(filepath.Join(*root, *budgetsPath))
	if err != nil {
		return 2, fmt.Errorf("proposal: %w", err)
	}
	problems := complexity.Compare(base, proposal)
	for _, problem := range problems {
		fmt.Println("complexity-from-base " + problem)
	}
	if len(problems) > 0 {
		return 1, nil
	}
	fmt.Println("complexity-from-base the proposal's budgets are at least as strict as the base's")
	return 0, nil
}
