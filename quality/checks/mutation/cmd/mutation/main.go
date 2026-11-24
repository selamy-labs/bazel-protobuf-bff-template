// Command mutation judges Go mutation-testing floors and reviewed equivalent-mutant proofs.
//
//	mutation check --root <dir> --floors <path> --gremlins <bin> --go <bin>
//	mutation compare --base-root <dir> --root <dir> [--floors <path>]
//
// Exit 0 passes, 1 is a finding, and 2 means it could not judge.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/command"
	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/mutation"
)

const defaultFloors = "quality/mutation_floors.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return command.Dispatch("mutation", args, map[string]func([]string) (int, error){
		"check":   func(a []string) (int, error) { return check(a, stdout) },
		"compare": func(a []string) (int, error) { return compare(a, stdout) },
	}, stderr)
}

func check(args []string, stdout io.Writer) (int, error) {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "the repository root")
	floorsPath := flags.String("floors", defaultFloors, "the mutation floors file, relative to root")
	gremlinsBin := flags.String("gremlins", "", "path to the pinned gremlins binary")
	goBin := flags.String("go", "", "path to the pinned go binary")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *root == "" || *gremlinsBin == "" || *goBin == "" {
		return 2, fmt.Errorf("--root, --gremlins, and --go are required")
	}
	floors, err := mutation.LoadFloors(filepath.Join(*root, *floorsPath))
	if err != nil {
		return 2, err
	}
	eqFile, err := mutation.LoadEquivalents(mutation.ResolveEquivalentsPath(*root, floors), floors)
	if err != nil {
		return 2, err
	}
	runner := mutation.Runner{GremlinsBin: *gremlinsBin, GoBin: *goBin}
	summary, problems, err := runner.RunAll(*root, floors, eqFile)
	if err != nil {
		return 2, err
	}
	fmt.Fprintln(stdout, "measurement: mutation "+summary.String())
	for _, problem := range problems {
		fmt.Fprintln(stdout, "mutation "+problem)
	}
	if len(problems) > 0 {
		return 1, nil
	}
	return 0, nil
}

func compare(args []string, stdout io.Writer) (int, error) {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseRoot := flags.String("base-root", "", "the base tree")
	root := flags.String("root", "", "the proposal tree")
	floorsPath := flags.String("floors", defaultFloors, "the mutation floors file, relative to each root")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *baseRoot == "" || *root == "" {
		return 2, fmt.Errorf("--base-root and --root are required")
	}
	base, err := mutation.LoadFloors(filepath.Join(*baseRoot, *floorsPath))
	if err != nil {
		return 2, fmt.Errorf("base: %w", err)
	}
	proposal, err := mutation.LoadFloors(filepath.Join(*root, *floorsPath))
	if err != nil {
		return 2, fmt.Errorf("proposal: %w", err)
	}
	if _, err := mutation.LoadEquivalents(mutation.ResolveEquivalentsPath(*root, proposal), proposal); err != nil {
		return 2, fmt.Errorf("proposal equivalents: %w", err)
	}
	problems := mutation.Compare(base, proposal)
	for _, problem := range problems {
		fmt.Fprintln(stdout, "mutation-from-base "+problem)
	}
	if len(problems) > 0 {
		return 1, nil
	}
	fmt.Fprintln(stdout, "mutation-from-base the proposal's mutation floors are at least as strict as the base's")
	return 0, nil
}
