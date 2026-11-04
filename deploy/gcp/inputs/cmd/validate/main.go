// Command validate is the hosted example's first step (spec 012 FR-003): it resolves the inputs through the one
// typed funnel, prints every effective value with the file that gave it, and writes the effective document a
// plan reads. The base file comes first; each --overlay is an operator's file whose leaves win over the files
// before it, so the last named file has precedence. Exit 0 when valid, 2 with the field named.
//
//	validate [--overlay <file>]... [--effective <path>] <inputs file>
//	validate --check-service <service dir>
//	validate --check-test-stream <test.jsonl>
//	validate --check-pipeline <cloudbuild.yaml>
package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/bfftemplate/deploy/gcp/inputs"
)

type files []string

func (f *files) String() string { return fmt.Sprint([]string(*f)) }

func (f *files) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func handleCheckService(dir string) {
	if err := inputs.CheckServiceConfig(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func handleCheckTestStream(streamPath string) {
	if err := inputs.CheckTestStream(streamPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func handleCheckPipeline(pipelinePath string) {
	if err := inputs.CheckPipelineConfig(pipelinePath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func handleValidateInputs(base string, overlays []string, effectivePath string) {
	in, provenance, effective, err := inputs.Resolve(base, overlays...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate: %v\n", err)
		os.Exit(2)
	}
	lines, err := inputs.Render(effective, provenance)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate: %v\n", err)
		os.Exit(2)
	}
	if effectivePath != "" {
		if err := os.WriteFile(effectivePath, effective, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "validate: %v\n", err)
			os.Exit(2)
		}
	}
	fmt.Printf("validate: %s with %d overlay(s) is valid against schema version %d; serving %s\n", base, len(overlays), in.Version, in.Release.ServingDigest)
	for _, line := range lines {
		fmt.Println(line)
	}
}

func main() {
	var overlays files
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.Var(&overlays, "overlay", "an operator's file whose leaves win over the files before it; repeatable")
	effectivePath := flags.String("effective", "", "write the effective document here, for the plan to read")
	checkService := flags.String("check-service", "", "verify service configuration in this directory")
	checkTestStream := flags.String("check-test-stream", "", "verify test plan stream from tofu test")
	checkPipeline := flags.String("check-pipeline", "", "verify cloudbuild pipeline configuration file")
	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *checkService != "" {
		handleCheckService(*checkService)
	}
	if *checkTestStream != "" {
		handleCheckTestStream(*checkTestStream)
	}
	if *checkPipeline != "" {
		handleCheckPipeline(*checkPipeline)
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: validate [--overlay <file>]... [--effective <path>] <inputs file>")
		os.Exit(2)
	}
	handleValidateInputs(flags.Arg(0), overlays, *effectivePath)
}
