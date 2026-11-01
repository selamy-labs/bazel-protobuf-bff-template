// Command release runs the steps of quality/RELEASING.md.
//
//	release notes <old tree> <new tree>
//	release record <record file> <tag> <commit>
//	release check <record file> <repository>
//
// Each tree is a copy of the module at one release. Exit 0 passes, 1 is a finding, 2 is a usage error.
package main

import (
	"fmt"
	"os"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/release"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	arguments := os.Args[2:]
	switch command := os.Args[1]; {
	case command == "notes" && len(arguments) == 2:
		notes, err := release.Notes(arguments[0], arguments[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "release:", err)
			os.Exit(1)
		}
		fmt.Print(notes)
	case command == "record" && len(arguments) == 3:
		if err := release.Record(arguments[0], arguments[1], arguments[2]); err != nil {
			fmt.Fprintln(os.Stderr, "release:", err)
			os.Exit(1)
		}
	case command == "check" && len(arguments) == 2:
		problems, err := release.Check(arguments[0], arguments[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "release:", err)
			os.Exit(1)
		}
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "release:", problem)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: release notes <old> <new> | record <file> <tag> <commit> | check <file> <repository>")
	os.Exit(2)
}
