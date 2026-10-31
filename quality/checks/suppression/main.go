// Command suppression-ban reads a NUL-delimited list of repository-relative paths from the file
// named by -paths, scans each under -root, and exits 1 if any banned directive is found.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/suppression"
)

func main() {
	os.Exit(run())
}

func run() int {
	root := flag.String("root", ".", "repository root")
	pathsFile := flag.String("paths", "", "file containing NUL-delimited paths")
	scanner := suppression.Scanner{Exempt: map[string]string{}}
	flag.Func("exempt", "sha256sum manifest of exempt files (repeatable)", func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		exempt, err := suppression.ParseManifest(data)
		if err != nil {
			return err
		}
		for name, sum := range exempt {
			scanner.Exempt[name] = sum
		}
		return nil
	})
	flag.Parse()
	if *pathsFile == "" {
		fmt.Fprintln(os.Stderr, "suppression-ban: -paths is required")
		return 2
	}
	list, err := os.ReadFile(*pathsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suppression-ban: %v\n", err)
		return 2
	}
	paths := suppression.ParsePaths(list)
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "suppression-ban: the path list is empty")
		return 2
	}
	var findings []suppression.Finding
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(*root, path))
		if errors.Is(err, fs.ErrNotExist) {
			continue // deleted in the working tree
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "suppression-ban: %v\n", err)
			return 2
		}
		findings = append(findings, scanner.Scan(path, data)...)
	}
	suppression.Report(os.Stdout, findings, 50)
	if len(findings) > 0 {
		return 1
	}
	return 0
}
