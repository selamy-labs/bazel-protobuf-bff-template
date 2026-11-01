// Package coverage reads what a Bazel coverage run leaves and judges it against the repository's
// coverage profiles (spec 007 FR-011 to FR-021). It is pure Go, because Bazel's default merger needs
// a C++ toolchain and a JDK, and this template has neither.
package coverage

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ProfilePrefix marks a line of a raw Go coverage profile inside a merged report. LCOV has no place
// for statement counts, and rules_go's conversion drops them, so the profile travels beside the LCOV.
const ProfilePrefix = "#go-profile "

// Merge writes one report from what a test left: every LCOV file in dir, then the raw Go profile
// that rules_go leaves next to the output file. Bazel gives this tool --coverage_dir and --output_file.
func Merge(dir, output string) error {
	merged := &bytes.Buffer{}
	names, err := reports(dir, output)
	if err != nil {
		return err
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		merged.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			merged.WriteByte('\n')
		}
	}
	if err := appendProfile(merged, output+".cover"); err != nil {
		return err
	}
	return os.WriteFile(output, merged.Bytes(), 0o644)
}

// Combine writes one report from the files named, one per line, in reports. It is Bazel's report
// generator step. A floor reads each test's own report, so this is a convenience for a reader.
func Combine(reports, output string) error {
	list, err := os.ReadFile(reports)
	if err != nil {
		return err
	}
	merged := &bytes.Buffer{}
	for name := range strings.Lines(string(list)) {
		data, err := os.ReadFile(strings.TrimSpace(name))
		if err != nil {
			return err
		}
		merged.Write(data)
	}
	return os.WriteFile(output, merged.Bytes(), 0o644)
}

// reports lists the LCOV files in dir, in a stable order, without the output itself.
func reports(dir, output string) ([]string, error) {
	var names []string
	for _, pattern := range []string{"*.dat", "*.lcov"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		names = append(names, matches...)
	}
	slices.Sort(names)
	return slices.DeleteFunc(names, func(name string) bool { return filepath.Clean(name) == filepath.Clean(output) }), nil
}

func appendProfile(merged io.Writer, path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		if line := lines.Text(); line != "" && !strings.HasPrefix(line, "mode: ") {
			if _, err := fmt.Fprintln(merged, ProfilePrefix+line); err != nil {
				return err
			}
		}
	}
	return lines.Err()
}
