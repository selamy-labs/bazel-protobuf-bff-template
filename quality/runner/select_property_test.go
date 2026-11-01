package runner

import (
	"math/rand"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/quick"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// The selector's invariants, named before the tests (spec 011 FR-009, research R5): the NUL list round-trips;
// selection keeps the discovery order, is a subset of what was discovered, is idempotent, ignores the order of
// the patterns, and skips vendored and fixture paths unless asked; compaction of a sorted list loses nothing
// but repeats; the policy digest ignores entries outside the policy and the order of the patterns.

// A path is any non-empty string without NUL; the generator draws from a small alphabet so collisions,
// prefixes and repeats are common.
type path string

func (path) Generate(rand *rand.Rand, size int) reflect.Value {
	parts := []string{"a", "b", "go", "sh", "web", "src", "x.go", "y.sh", "z.md", "README", "_test.go"}
	n := 1 + rand.Intn(4)
	segments := make([]string, n)
	for i := range segments {
		segments[i] = parts[rand.Intn(len(parts))]
	}
	return reflect.ValueOf(path(strings.Join(segments, "/")))
}

func strs(paths []path) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = string(p)
	}
	return out
}

func TestSplitRoundTripsAnyNulFreeList(t *testing.T) {
	property := func(paths []path) bool {
		list := strs(paths)
		joined := []byte(strings.Join(list, "\x00") + "\x00")
		return slices.Equal(Split(joined), list)
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCompactKeepsEveryDistinctSortedPathOnce(t *testing.T) {
	property := func(paths []path) bool {
		list := strs(paths)
		sort := slices.Clone(list)
		slices.Sort(sort)
		compacted := compact(slices.Clone(sort))
		unique := slices.Compact(slices.Clone(sort))
		return slices.Equal(compacted, unique) && slices.IsSorted(compacted)
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}

var includeChoices = []string{`\.go$`, `\.sh$`, `^web/`, `README`, `_test\.go$`, `^src/.*\.go$`}

func selectorOf(rand *rand.Rand, vendored, fixtures bool) *registrypb.Check {
	n := rand.Intn(4)
	include := make([]string, n)
	for i := range include {
		include[i] = includeChoices[rand.Intn(len(includeChoices))]
	}
	check := &registrypb.Check{Selector: &registrypb.Selector{Include: include, IncludeVendored: vendored, IncludeFixtures: fixtures}}
	return check
}

func TestSelectIsAnOrderedSubsetIdempotentAndPatternOrderBlind(t *testing.T) {
	seed := rand.New(rand.NewSource(1))
	property := func(paths []path, vendoredMask, fixtureMask uint8, vendored, fixtures bool) bool {
		list := strs(paths)
		inputs := &Inputs{Root: t.TempDir(), All: list, Vendored: map[string]bool{}, Fixtures: map[string]bool{}}
		for i, p := range list {
			if vendoredMask&(1<<(i%8)) != 0 {
				inputs.Vendored[p] = true
			}
			if fixtureMask&(1<<(i%8)) != 0 {
				inputs.Fixtures[p] = true
			}
		}
		check := selectorOf(seed, vendored, fixtures)
		selected := Select(check, inputs)
		// An ordered subset: every selected path appears in the inputs in the same order.
		cursor := 0
		for _, p := range selected {
			index := slices.Index(inputs.All[cursor:], p)
			if index < 0 {
				return false
			}
			cursor += index + 1
		}
		// Exactly the paths the patterns admit and the manifests do not exclude.
		patterns := make([]*regexp.Regexp, 0)
		for _, pattern := range check.GetSelector().GetInclude() {
			patterns = append(patterns, regexp.MustCompile(pattern))
		}
		var expected []string
		for _, p := range inputs.All {
			if inputs.Vendored[p] && !vendored || inputs.Fixtures[p] && !fixtures {
				continue
			}
			if len(patterns) == 0 || matchesAny(p, patterns) {
				expected = append(expected, p)
			}
		}
		if !slices.Equal(selected, expected) {
			return false
		}
		// Idempotent: selecting from the selection changes nothing.
		again := Select(check, &Inputs{Root: inputs.Root, All: selected, Vendored: inputs.Vendored, Fixtures: inputs.Fixtures})
		if !slices.Equal(again, selected) {
			return false
		}
		// Pattern order is irrelevant.
		reversed := slices.Clone(check.GetSelector().GetInclude())
		slices.Reverse(reversed)
		check.Selector.Include = reversed
		return slices.Equal(Select(check, inputs), selected)
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyDigestSeesOnlyPolicyEntriesWhateverTheirPatternOrder(t *testing.T) {
	policy := []*regexp.Regexp{regexp.MustCompile(`\.go$`), regexp.MustCompile(`^web/`)}
	reversed := []*regexp.Regexp{policy[1], policy[0]}
	property := func(paths []path, extra path) bool {
		var entries []string
		for i, p := range strs(paths) {
			entries = append(entries, "100644 blob "+strings.Repeat("0", 39)+string(rune('0'+i%10))+"\t"+p)
		}
		listing := []byte(strings.Join(entries, "\x00") + "\x00")
		digest := policyDigest(listing, policy)
		if policyDigest(listing, reversed) != digest {
			return false
		}
		// An entry outside the policy changes nothing; one inside it changes the digest.
		outside := []byte(strings.Join(append(slices.Clone(entries), "100644 blob "+strings.Repeat("1", 40)+"\tnotes/"+string(extra)+".md"), "\x00") + "\x00")
		inside := []byte(strings.Join(append(slices.Clone(entries), "100644 blob "+strings.Repeat("1", 40)+"\tweb/"+string(extra)), "\x00") + "\x00")
		return policyDigest(outside, policy) == digest && policyDigest(inside, policy) != digest
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}
