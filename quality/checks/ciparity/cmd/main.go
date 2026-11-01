// Command ciparity validates a workflow, given as JSON, against a JSON policy. Exit 1 lists every
// violation; exit 2 means the input could not be judged. Malformed input never passes.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/prototext"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/checks/ciparity"
	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
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
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: ciparity <workflow.json> <policy.json> <registry.textproto>")
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
	data, err := os.ReadFile(os.Args[3])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci-parity: %v\n", err)
		return 2
	}
	registry := &registrypb.Registry{}
	if err := prototext.Unmarshal(data, registry); err != nil {
		fmt.Fprintf(os.Stderr, "ci-parity: %v\n", err)
		return 2
	}
	if len(registry.GetShards()) == 0 {
		fmt.Fprintln(os.Stderr, "ci-parity: the registry declares no legs")
		return 2
	}
	errs := ciparity.Validate(workflow, policy, registry.GetShards())
	for _, e := range errs {
		fmt.Printf("ci-parity %s (rerun: tools/verify.sh)\n", e)
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}
