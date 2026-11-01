package runner

import (
	"os"
	"slices"
	"strings"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// LoadBound reads a host's registry and resolves its rule bindings against the catalog, and validates
// the result. With no catalog, the registry stands alone and binds nothing.
func LoadBound(catalogPath, registryPath string) (*registrypb.Registry, error) {
	if catalogPath == "" {
		return LoadRegistry(registryPath)
	}
	read := func(path string) (*registrypb.Registry, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, configErrorf("registry: %v", err)
		}
		registry := &registrypb.Registry{}
		if err := prototext.Unmarshal(data, registry); err != nil {
			return nil, configErrorf("registry: %v", err)
		}
		return registry, nil
	}
	catalog, err := read(catalogPath)
	if err != nil {
		return nil, err
	}
	host, err := read(registryPath)
	if err != nil {
		return nil, err
	}
	resolved, problems := Resolve(catalog, host)
	if len(problems) == 0 {
		problems = Validate(resolved)
	}
	if len(problems) > 0 {
		return nil, configErrorf("registry: %s", strings.Join(problems, "; "))
	}
	return resolved, nil
}

// Resolve turns a catalog and a host's registry into the one list the runner executes. A binding only
// appends, and it cannot change a rule's stages, class or command (spec 008 R8).
func Resolve(catalog, host *registrypb.Registry) (*registrypb.Registry, []string) {
	var problems []string
	add := func(message string) { problems = append(problems, message) }
	rules := map[string]*registrypb.Check{}
	for _, rule := range catalog.GetChecks() {
		switch {
		case rules[rule.GetId()] != nil:
			add("catalog: duplicate rule " + rule.GetId())
		case rule.GetShard() != "":
			add("catalog: " + rule.GetId() + " names a leg, which is the host's")
		case len(rule.GetTargets()) > 0:
			add("catalog: " + rule.GetId() + " names targets, which are the host's")
		}
		rules[rule.GetId()] = rule
	}
	if catalog.GetVersion() == "" {
		add("catalog: no version")
	}
	resolved := proto.Clone(host).(*registrypb.Registry)
	resolved.Rules, resolved.Checks = nil, nil
	resolved.Version = catalog.GetVersion()
	bound := map[string]bool{}
	for _, binding := range host.GetRules() {
		id := binding.GetId()
		rule := rules[id]
		switch {
		case rule == nil:
			add("rule " + id + " is not in the catalog")
			continue
		case bound[id]:
			add("rule " + id + " is bound twice")
			continue
		case rule.GetKind() == registrypb.Kind_KIND_BAZEL && len(binding.GetArgs()) > 0:
			add("rule " + id + " is a Bazel rule, so it takes targets, not args")
			continue
		case rule.GetKind() != registrypb.Kind_KIND_BAZEL && len(binding.GetTargets()) > 0:
			add("rule " + id + " is not a Bazel rule, so it takes args, not targets")
			continue
		}
		bound[id] = true
		check := proto.Clone(rule).(*registrypb.Check)
		check.Shard = binding.GetShard()
		check.Command = append(check.Command, binding.GetArgs()...)
		check.Targets = binding.GetTargets()
		if len(binding.GetContext()) > 0 {
			if check.Selector == nil {
				add("rule " + id + " has no selector, so it reads no context")
				continue
			}
			check.Selector.Context = append(check.Selector.Context, binding.GetContext()...)
		}
		resolved.Checks = append(resolved.Checks, check)
	}
	resolved.Checks = append(resolved.Checks, host.GetChecks()...)
	for _, tool := range catalog.GetTools() {
		used := slices.ContainsFunc(resolved.GetChecks(), func(check *registrypb.Check) bool {
			return check.GetTool() == tool.GetName() || slices.Contains(check.GetCommand(), "{"+tool.GetName()+"}")
		})
		if used {
			resolved.Tools = append(resolved.Tools, tool)
		}
	}
	return resolved, problems
}
