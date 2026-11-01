package runner

import (
	"slices"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

// Selected reports whether a check runs in a stage and leg. An empty leg is the whole stage. Every
// place that asks what runs asks here, so the Bazel plan, the native pool and the accounting of
// checks that never ran cannot disagree (spec 007 FR-001, FR-002).
func Selected(registry *registrypb.Registry, check *registrypb.Check, stage registrypb.Stage, shard string) bool {
	if !slices.Contains(check.GetStages(), stage) {
		return false
	}
	if shard == "" {
		return true
	}
	if check.GetPostcondition() {
		return bazelLeg(registry, shard)
	}
	return check.GetShard() == shard
}

// bazelLeg reports whether a leg has Bazel work, which is what a postcondition judges.
func bazelLeg(registry *registrypb.Registry, shard string) bool {
	return slices.ContainsFunc(registry.GetChecks(), func(check *registrypb.Check) bool {
		return check.GetKind() == registrypb.Kind_KIND_BAZEL && check.GetShard() == shard
	})
}

// Obligations names the checks each leg owes at full, derived from the registry alone. A postcondition
// appears in every leg with Bazel work, and it has no per-leg setting that could leave one out.
func Obligations(registry *registrypb.Registry) map[string][]string {
	legs := map[string][]string{}
	for _, shard := range registry.GetShards() {
		legs[shard] = []string{}
		for _, check := range registry.GetChecks() {
			if Selected(registry, check, registrypb.Stage_STAGE_FULL, shard) {
				legs[shard] = append(legs[shard], check.GetId())
			}
		}
	}
	return legs
}

func validateLegs(registry *registrypb.Registry) []string {
	var problems []string
	shards := registry.GetShards()
	declared := map[string]bool{}
	for _, shard := range shards {
		if !checkID.MatchString(shard) {
			problems = append(problems, "shard "+shard+" is not kebab-case")
		}
		if declared[shard] {
			problems = append(problems, "duplicate shard "+shard)
		}
		declared[shard] = true
	}
	owned := map[string]bool{}
	for _, check := range registry.GetChecks() {
		id, shard := check.GetId(), check.GetShard()
		owned[shard] = true
		switch {
		case check.GetPostcondition() && !check.GetAfterBazel():
			problems = append(problems, id+": a postcondition must set after_bazel")
		case check.GetPostcondition() && shard != "":
			problems = append(problems, id+": is both owned by "+shard+" and a postcondition")
		case shard != "" && !declared[shard]:
			problems = append(problems, id+": unknown shard "+shard)
		case len(shards) > 0 && shard == "" && !check.GetPostcondition():
			problems = append(problems, id+": is neither owned by a shard nor a postcondition")
		}
	}
	for _, shard := range shards {
		if !owned[shard] {
			problems = append(problems, "shard "+shard+" owns no check")
		}
	}
	return problems
}
