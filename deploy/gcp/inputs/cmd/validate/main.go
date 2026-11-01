// Command validate is the hosted example's first step (spec 012 FR-003): it validates one inputs file through the
// one typed funnel and prints the effective values a plan will read. Exit 0 when valid, 2 with the field named.
//
//	validate <inputs file>
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/bfftemplate/deploy/gcp/inputs"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: validate <inputs file>")
		os.Exit(2)
	}
	in, err := inputs.Load(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate: %v\n", err)
		os.Exit(2)
	}
	effective, _ := json.MarshalIndent(map[string]any{
		"project":        in.Contract.ProjectID,
		"region":         in.Contract.Region,
		"runtime":        in.Contract.RuntimeIdentity,
		"image":          in.Image.Digest,
		"serving_digest": in.Release.ServingDigest,
		"allowed_origin": in.Runtime.AllowedOrigin,
		"rpc_base_url":   in.Frontend.RPCBaseURL,
		"max_instances":  in.Bounds.MaxInstances,
	}, "", "  ")
	fmt.Printf("validate: %s is valid against schema version 1\n%s\n", os.Args[1], effective)
}
