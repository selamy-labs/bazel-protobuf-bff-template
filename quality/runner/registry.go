// Package runner executes the quality registry: one definition of each check, one runner for every
// stage (spec 006, contracts/check-registry.md).
package runner

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

var checkID = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ConfigError is a broken registry or invocation: exit 2, never a pass.
type ConfigError struct{ Message string }

func (e *ConfigError) Error() string { return e.Message }

func configErrorf(format string, args ...any) error {
	return &ConfigError{Message: fmt.Sprintf(format, args...)}
}

// LoadRegistry reads and validates a registry. Unknown fields are rejected by the parser.
func LoadRegistry(path string) (*registrypb.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, configErrorf("registry: %v", err)
	}
	return ParseRegistry(data)
}

// ParseRegistry validates registry text.
func ParseRegistry(data []byte) (*registrypb.Registry, error) {
	registry := &registrypb.Registry{}
	if err := prototext.Unmarshal(data, registry); err != nil {
		return nil, configErrorf("registry: %v", err)
	}
	if problems := Validate(registry); len(problems) > 0 {
		return nil, configErrorf("registry: %s", strings.Join(problems, "; "))
	}
	return registry, nil
}

// Validate returns every rule the registry breaks.
func Validate(registry *registrypb.Registry) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if len(registry.GetChecks()) == 0 {
		add("no checks")
	}
	tools := map[string]bool{}
	for _, tool := range registry.GetTools() {
		tools[tool.GetName()] = true
	}
	for _, pattern := range registry.GetEscalate() {
		if _, err := regexp.Compile(pattern); err != nil {
			add("escalate %q: %v", pattern, err)
		}
	}
	seen := map[string]bool{}
	for _, check := range registry.GetChecks() {
		id := check.GetId()
		if !checkID.MatchString(id) {
			add("check id %q is not kebab-case", id)
		}
		if seen[id] {
			add("duplicate check id %q", id)
		}
		seen[id] = true
		problems = append(problems, validateCheck(check, tools)...)
	}
	return problems
}

func validateCheck(check *registrypb.Check, tools map[string]bool) []string {
	var problems []string
	id := check.GetId()
	add := func(format string, args ...any) {
		problems = append(problems, id+": "+fmt.Sprintf(format, args...))
	}
	full := false
	for _, stage := range check.GetStages() {
		full = full || stage == registrypb.Stage_STAGE_FULL
	}
	if !full {
		add("every check must run at full")
	}
	switch check.GetClassification() {
	case registrypb.Classification_CLASSIFICATION_GUARANTEE:
		if check.GetWitness() == "" {
			add("a GUARANTEE needs a witness with defect and corrected cases")
		}
	case registrypb.Classification_CLASSIFICATION_SIGNAL:
		if check.GetThreshold() != nil {
			add("a SIGNAL cannot carry a failing threshold")
		}
	default:
		add("missing classification")
	}
	if check.GetTimeoutSeconds() == 0 {
		add("missing timeout")
	}
	switch check.GetKind() {
	case registrypb.Kind_KIND_BAZEL:
		if len(check.GetTargets()) == 0 {
			add("a Bazel check needs targets")
		}
		if len(check.GetCommand()) > 0 {
			add("a Bazel check has no command")
		}
	case registrypb.Kind_KIND_NATIVE:
		if len(check.GetCommand()) == 0 {
			add("a native check needs a command")
		}
		if len(check.GetTargets()) > 0 {
			add("a native check has no targets")
		}
	default:
		add("missing kind")
	}
	if tool := check.GetTool(); tool != "" && !tools[tool] {
		add("unknown tool %q", tool)
	}
	for _, pattern := range append(check.GetSelector().GetInclude(), check.GetSelector().GetContext()...) {
		if _, err := regexp.Compile(pattern); err != nil {
			add("selector %q: %v", pattern, err)
		}
	}
	return problems
}
