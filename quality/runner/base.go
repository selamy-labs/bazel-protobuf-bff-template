package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"google.golang.org/protobuf/encoding/prototext"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// Base is the policy base of a run: what a tested commit is compared with. Naming one says nothing
// about review, which is a separate record. --from is the selection range and never fills this in.
type Base struct {
	// Revision is the resolved SHA, empty when Refusal is set.
	Revision string
	// Source says how it was chosen: argument, hook-config or ci-event.
	Source string
	// Refusal names why no comparison can happen. It denies the checks that need a base.
	Refusal string
}

func malformedRevision(named string) bool {
	return strings.HasPrefix(named, "-") || strings.Contains(named, "..") ||
		strings.ContainsFunc(named, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// ResolveBase resolves a named revision once and applies every test to the resolved SHA, so no
// spelling of the tested commit gets past them. A different commit with the same tree is accepted:
// nothing changed, so nothing weakened, and a differing tree would authenticate nothing.
// A base without the host's contract has nothing to compare with. Whether it has an evaluator is decided
// by building it, because a consumer's evaluator comes from the pin at that commit (spec 008 R8).
func ResolveBase(ctx context.Context, git Git, named, source, tested, contract string) Base {
	refuse := func(reason string) Base { return Base{Source: source, Refusal: reason} }
	if named == "" {
		return refuse("no policy base was named (rerun with --base=<commit>; for hooks, " +
			"run the install target with --policy-base=<commit>)")
	}
	if malformedRevision(named) {
		return refuse("malformed revision " + quoted(named))
	}
	if shallow, err := git.Run(ctx, nil, "rev-parse", "--is-shallow-repository"); err != nil {
		return refuse("the repository cannot be read: " + err.Error())
	} else if strings.TrimSpace(string(shallow)) == "true" {
		return refuse("history is shallow, so a base cannot be established")
	}
	resolved, err := git.Run(ctx, nil, "rev-parse", "--verify", "--quiet", "--end-of-options", named+"^{commit}")
	if err != nil {
		return refuse("unknown revision " + quoted(named))
	}
	sha := strings.TrimSpace(string(resolved))
	if sha == tested {
		return refuse("a commit cannot be its own base (" + quoted(named) + " resolves to the tested commit)")
	}
	if _, err := git.Run(ctx, nil, "merge-base", "--is-ancestor", sha, tested); err != nil {
		return refuse(sha + " is not an ancestor of the tested commit")
	}
	if _, err := git.Run(ctx, nil, "cat-file", "-e", sha+":"+contract); err != nil {
		return refuse("the base has no coverage contract: " + sha + " lacks " + contract)
	}
	return Base{Revision: sha, Source: source}
}

func quoted(text string) string { return "'" + text + "'" }

// baseEscalate reads a base's own definition of its policy inputs. Fields a later schema added are
// ignored, because a base can be older or newer than this runner.
func baseEscalate(ctx context.Context, git Git, revision, path string) ([]string, error) {
	text, err := git.Run(ctx, nil, "show", revision+":"+path)
	if err != nil {
		return nil, fmt.Errorf("the base has no %s: %w", path, err)
	}
	registry := &registrypb.Registry{}
	if err := (prototext.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(text, registry); err != nil {
		return nil, fmt.Errorf("the base's %s does not parse: %w", path, err)
	}
	if len(registry.GetEscalate()) == 0 {
		return nil, fmt.Errorf("the base's %s names no policy inputs", path)
	}
	return registry.GetEscalate(), nil
}

// BaseOwnPolicyDigest is a commit's policy digest by that commit's own definition, so nothing in the
// tested tree can change it. It is identity metadata, not a judgment of the proposal.
func BaseOwnPolicyDigest(ctx context.Context, git Git, revision, registry string) (string, error) {
	policy, err := baseEscalate(ctx, git, revision, registry)
	if err != nil {
		return "", err
	}
	return BasePolicyDigest(ctx, git, revision, policy)
}

// BasePolicyDigest is the policy digest of a commit under a given definition, by the same rule as a
// receipt's own.
func BasePolicyDigest(ctx context.Context, git Git, revision string, policy []string) (string, error) {
	patterns := make([]*regexp.Regexp, 0, len(policy))
	for _, pattern := range policy {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return "", err
		}
		patterns = append(patterns, compiled)
	}
	listing, err := git.Run(ctx, nil, "ls-tree", "-r", "-z", revision)
	if err != nil {
		return "", err
	}
	return policyDigest(listing, patterns), nil
}
