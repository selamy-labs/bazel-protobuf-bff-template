// Command ciparity validates a workflow, given as JSON, against a JSON policy. Exit 1 lists every
// violation; exit 2 means the input could not be judged. Malformed input never passes.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/ciparity"
)

func main() {
	os.Exit(run())
}

func load(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	return json.Unmarshal(data, into)
}

func run() int {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: ciparity <workflow.json> <policy.json>")
		return 2
	}
	var workflow map[string]any
	if err := load(os.Args[1], &workflow); err != nil {
		fmt.Fprintf(os.Stderr, "ci-parity: %v\n", err)
		return 2
	}
	var policy ciparity.Policy
	if err := load(os.Args[2], &policy); err != nil {
		fmt.Fprintf(os.Stderr, "ci-parity: %v\n", err)
		return 2
	}
	errs := ciparity.Validate(workflow, policy)
	for _, e := range errs {
		fmt.Printf("ci-parity %s (rerun: tools/verify.sh)\n", e)
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}
