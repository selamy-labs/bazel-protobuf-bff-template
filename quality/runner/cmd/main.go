// Command quality runs one stage of the registry. See contracts/check-registry.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/runner"
)

type toolFlag map[string]string

func (t toolFlag) String() string { return "" }

func (t toolFlag) Set(value string) error {
	name, path, found := strings.Cut(value, "=")
	if !found {
		return fmt.Errorf("want NAME=path, got %q", value)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	t[name] = absolute
	return nil
}

// deadlines bound a stage inside its caller: hooks by their budgets, full inside tools/verify.sh.
var deadlines = map[registrypb.Stage]time.Duration{
	registrypb.Stage_STAGE_PRE_COMMIT: 2 * time.Minute,
	registrypb.Stage_STAGE_COMMIT_MSG: time.Minute,
	registrypb.Stage_STAGE_PRE_PUSH:   30 * time.Minute,
	registrypb.Stage_STAGE_FULL:       38 * time.Minute,
}

// A failed flag or registry prints a configuration diagnostic and exits 2, never a silent pass.
func main() {
	if len(os.Args) > 2 && os.Args[1] == runner.SuperviseArg {
		os.Exit(runner.Supervise(os.Args[2:]))
	}
	os.Exit(run())
}

func run() int {
	tools := toolFlag{}
	flags := flag.NewFlagSet("quality", flag.ContinueOnError)
	stageName := flags.String("stage", "", "pre-commit, commit-msg, pre-push or full")
	registryPath := flags.String("registry", "", "registry textproto")
	adapter := flags.String("adapter", "", "the native adapter")
	isolation := flags.String("isolation", "full", "full or weakened")
	offline := flags.Bool("offline", false, "use only fetched artifacts")
	receipt := flags.String("receipt", "", "receipt path")
	rerun := flags.String("rerun", "", "the rerun command to print")
	jobs := flags.Int("jobs", runner.DefaultJobs(), "native pool size")
	slots := flags.Int("slots", 2, "concurrent heavy stages on this host")
	deadline := flags.Duration("deadline", 0, "override the stage deadline")
	from := flags.String("from", "", "pushed range start")
	to := flags.String("to", "", "pushed commit")
	inner := flags.Bool("inner", false, "judge the current worktree as the pushed commit")
	flags.Var(tools, "tool", "NAME=path, repeated")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return runner.ExitConfig
	}
	stage, known := runner.StageNames[*stageName]
	if !known {
		fmt.Fprintf(os.Stderr, "quality: unknown stage %q\n", *stageName)
		return runner.ExitConfig
	}
	if *isolation != "full" && *isolation != "weakened" {
		fmt.Fprintf(os.Stderr, "quality: unknown isolation %q\n", *isolation)
		return runner.ExitConfig
	}
	root := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if root == "" {
		root, _ = os.Getwd()
	}
	registry, err := runner.LoadRegistry(*registryPath)
	if err != nil {
		return fail(err)
	}
	runfiles, err := filepath.Abs("..")
	if err != nil {
		return fail(err)
	}
	adapterPath, err := filepath.Abs(*adapter)
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	env := environment(root, tools, runfiles)
	bazel := filepath.Join(root, "tools", "bootstrap", "bazel.sh")
	limit := deadlines[stage]
	if *deadline > 0 {
		limit = *deadline
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	cache = filepath.Join(cache, "bff-template")
	if stage == registrypb.Stage_STAGE_PRE_PUSH && !*inner {
		code, err := runner.RunPush(ctx, runner.PushOptions{
			Root: root, Bazel: bazel, Env: env, From: *from, To: *to, Isolation: *isolation,
			Offline: *offline, CacheDir: cache, Deadline: limit, Stdout: os.Stdout,
		})
		if err != nil {
			return fail(err)
		}
		return code
	}
	options := runner.Options{
		Registry: registry, Stage: stage, Requested: stage, Root: root, Adapter: adapterPath, Bazel: bazel,
		Tools: tools, Env: env, Isolation: *isolation, Offline: *offline, ReceiptPath: *receipt,
		Rerun: *rerun, Jobs: *jobs, Deadline: limit, Stdout: os.Stdout,
	}
	if options.Rerun == "" {
		options.Rerun = "tools/hooks/stage.sh " + *stageName
	}
	git := runner.Git{Dir: root, Env: env}
	if stage == registrypb.Stage_STAGE_PRE_PUSH {
		escalate, err := runner.Escalates(ctx, git, registry.GetEscalate(), *from, *to)
		if err != nil {
			return fail(err)
		}
		if escalate {
			options.Stage = registrypb.Stage_STAGE_FULL
			options.Deadline = deadlines[registrypb.Stage_STAGE_FULL]
			fmt.Println("quality: policy input changed or base unknown; escalated to full")
		}
	}
	if options.Stage == registrypb.Stage_STAGE_PRE_COMMIT {
		options.Inputs = func(ctx context.Context, git runner.Git, scratch string) (*runner.Inputs, error) {
			return runner.Snapshot(ctx, git, registry, scratch)
		}
	} else {
		narrow := stage == registrypb.Stage_STAGE_PRE_PUSH && options.Stage == registrypb.Stage_STAGE_PRE_PUSH
		options.Inputs = func(ctx context.Context, git runner.Git, _ string) (*runner.Inputs, error) {
			inputs, err := runner.Discover(ctx, git, registry)
			if err == nil && narrow {
				err = runner.PushInputs(ctx, git, inputs, *from, *to)
			}
			return inputs, err
		}
	}
	if options.Stage != registrypb.Stage_STAGE_PRE_COMMIT && options.Stage != registrypb.Stage_STAGE_COMMIT_MSG {
		paths, err := runner.SlotPaths(filepath.Join(cache, "slots"), max(1, *slots))
		if err != nil {
			return fail(err)
		}
		options.Slots = paths
	}
	code, err := runner.Run(ctx, options)
	if err != nil {
		return fail(err)
	}
	return code
}

func fail(err error) int {
	var config *runner.ConfigError
	if errors.As(err, &config) {
		fmt.Fprintf(os.Stderr, "quality: configuration error: %s\n", config.Message)
	} else {
		fmt.Fprintf(os.Stderr, "quality: %v\n", err)
	}
	return runner.ExitConfig
}

// environment hands native checks the scrubbed environment plus NAME=path for each tool, and resolves
// a relative GIT_INDEX_FILE, which Git names relative to the checkout.
func environment(root string, tools toolFlag, runfiles string) []string {
	env := make([]string, 0, len(os.Environ())+len(tools)+1)
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if name == "GIT_INDEX_FILE" && !filepath.IsAbs(value) {
			entry = name + "=" + filepath.Join(root, value)
		}
		env = append(env, entry)
	}
	for name, path := range tools {
		env = append(env, name+"="+path)
	}
	return append(env, "RUNFILES_ROOT="+runfiles)
}
