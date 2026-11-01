package runner

import (
	"context"
	"regexp"
	"strings"
	"unicode"
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

// The evaluator's own source. A base without it cannot judge anything.
const evaluatorPath = "quality/checks/coverage/cmd/floors/main.go"

const contractPath = "quality/coverage_floors.json"

func malformedRevision(named string) bool {
	return strings.HasPrefix(named, "-") || strings.Contains(named, "..") ||
		strings.ContainsFunc(named, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// ResolveBase resolves a named revision once and applies every test to the resolved SHA, so no
// spelling of the tested commit gets past them. A different commit with the same tree is accepted:
// nothing changed, so nothing weakened, and a differing tree would authenticate nothing.
func ResolveBase(ctx context.Context, git Git, named, source, tested string) Base {
	refuse := func(reason string) Base { return Base{Source: source, Refusal: reason} }
	if named == "" {
		return refuse("no policy base was named (rerun with --base=<commit>; for hooks, " +
			"tools/bootstrap/bazel.sh run //tools/hooks:install -- --policy-base=<commit>)")
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
	for _, path := range []string{evaluatorPath, contractPath} {
		if _, err := git.Run(ctx, nil, "cat-file", "-e", sha+":"+path); err != nil {
			return refuse("the base has no evaluator: " + sha + " lacks " + path)
		}
	}
	return Base{Revision: sha, Source: source}
}

func quoted(text string) string { return "'" + text + "'" }

// BasePolicyDigest is the policy digest of a commit, by the same rule as a receipt's own.
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
