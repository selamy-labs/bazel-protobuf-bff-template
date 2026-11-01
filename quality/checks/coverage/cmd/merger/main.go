// Command merger is Bazel's coverage output generator and report generator for this repository. The
// first runs per test with --coverage_dir, and the second once after the tests with --reports_file.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/coverage"
)

func main() {
	var dir, reports, output string
	for _, argument := range os.Args[1:] {
		if value, ok := strings.CutPrefix(argument, "--coverage_dir="); ok {
			dir = value
		}
		if value, ok := strings.CutPrefix(argument, "--reports_file="); ok {
			reports = value
		}
		if value, ok := strings.CutPrefix(argument, "--output_file="); ok {
			output = value
		}
	}
	var err error
	switch {
	case output != "" && dir != "":
		err = coverage.Merge(dir, output)
	case output != "" && reports != "":
		err = coverage.Combine(reports, output)
	default:
		err = errors.New("--output_file and one of --coverage_dir and --reports_file are required")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "merger: %v\n", err)
		os.Exit(1)
	}
}
