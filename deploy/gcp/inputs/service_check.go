package inputs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"go.yaml.in/yaml/v3"
)

var (
	approvedPlanDigestRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)
	forbiddenEnvRegex       = regexp.MustCompile(`\b(TF|OPENTOFU)_[A-Za-z0-9_]*`)
	allowedServingKeys      = []string{"allowed_origin", "image", "max_instances", "port", "revision"}
)

// ValidateApprovedPlanDigest validates that an approval digest is strictly
// 64 lowercase hexadecimal characters with no whitespace or newlines.
func ValidateApprovedPlanDigest(digest string) error {
	if !approvedPlanDigestRegex.MatchString(digest) {
		return fmt.Errorf("inputs: approved plan digest %q must be exactly 64 lowercase hexadecimal characters", digest)
	}
	return nil
}

func isIgnoredFile(name string) bool {
	return strings.HasPrefix(name, ".") ||
		strings.HasSuffix(name, "~") ||
		(strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#"))
}

func isConfigOrTestFileName(name string) bool {
	clean := strings.TrimPrefix(name, ".")
	clean = strings.TrimPrefix(clean, "#")
	clean = strings.TrimSuffix(clean, "~")
	clean = strings.TrimSuffix(clean, "#")
	return strings.HasSuffix(clean, ".tf") ||
		strings.HasSuffix(clean, ".tofu") ||
		strings.HasSuffix(clean, ".tftest.hcl") ||
		strings.HasSuffix(clean, ".tf.json") ||
		strings.HasSuffix(clean, ".tofu.json")
}

func isForbiddenFileName(name string) bool {
	if !isConfigOrTestFileName(name) {
		return false
	}
	if strings.Contains(name, ".tofu") ||
		strings.Contains(name, "override") ||
		strings.HasSuffix(name, ".tf.json") {
		return true
	}
	return isIgnoredFile(name)
}

func isAcceptedTestsFile(name string) bool {
	if isForbiddenFileName(name) {
		return false
	}
	if strings.HasSuffix(name, ".tf") ||
		strings.HasSuffix(name, ".tf.json") ||
		strings.Contains(name, ".tofu") {
		return false
	}
	return strings.HasSuffix(name, ".tftest.hcl") ||
		strings.HasSuffix(name, ".tfvars") ||
		strings.HasSuffix(name, ".json") ||
		strings.HasSuffix(name, ".md")
}

func checkTestsDirEntry(path, name string) error {
	if !isAcceptedTestsFile(name) {
		return fmt.Errorf("offline: only .tftest.hcl, .tfvars, .json, or .md files are accepted in tests/: %s", path)
	}
	return nil
}

func validateRootModuleFiles(path, name string) error {
	if isForbiddenFileName(name) {
		return fmt.Errorf("offline: override, .tofu, JSON or disguised configuration is not accepted here: %s", path)
	}
	return nil
}

func checkUnexpectedModuleFiles(path, name, relDir string) error {
	if relDir == "tests" || strings.HasPrefix(relDir, "tests/") {
		return checkTestsDirEntry(path, name)
	}
	if relDir != "." {
		if isConfigOrTestFileName(name) {
			return fmt.Errorf("offline: configuration files in subdirectories are not accepted here: %s", path)
		}
		return nil
	}
	return validateRootModuleFiles(path, name)
}

func checkConfigDirEntry(path, name, relDir string) error {
	return checkUnexpectedModuleFiles(path, name, relDir)
}

func verifyNoForbiddenFiles(configDir string) error {
	return filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(configDir, path)
		return checkConfigDirEntry(path, info.Name(), filepath.ToSlash(filepath.Dir(rel)))
	})
}

type providerBlockEntry struct {
	block    *hclsyntax.Block
	srcBytes []byte
}

func normalizeExprSource(srcBytes []byte, expr hclsyntax.Expression) string {
	raw := expr.Range().SliceBytes(srcBytes)
	tokens, _ := hclsyntax.LexConfig(raw, "", hcl.Pos{Line: 1, Column: 1})
	var sb strings.Builder
	for _, tok := range tokens {
		if tok.Type != hclsyntax.TokenComment && tok.Type != hclsyntax.TokenNewline && tok.Type != hclsyntax.TokenEOF {
			sb.WriteString(normalizeWhitespace(string(tok.Bytes)))
		}
	}
	return sb.String()
}

func isAllowedProjectBinding(val string) bool {
	return val == "jsondecode(file(var.inputs_file)).contract.project_id" ||
		val == "local.inputs.contract.project_id" ||
		val == "var.project_id"
}

func isAllowedRegionBinding(val string) bool {
	return val == "jsondecode(file(var.inputs_file)).contract.region" ||
		val == "local.inputs.contract.region" ||
		val == "var.region"
}

func verifyGoogleProviderBlock(googleBlock providerBlockEntry) error {
	if len(googleBlock.block.Body.Blocks) != 0 {
		return fmt.Errorf("offline: nested blocks in provider block are not accepted here")
	}
	projectAttr, hasProject := googleBlock.block.Body.Attributes["project"]
	if !hasProject || !isAllowedProjectBinding(normalizeExprSource(googleBlock.srcBytes, projectAttr.Expr)) {
		return fmt.Errorf("offline: the provider is not bound to the funnel's project")
	}
	regionAttr, hasRegion := googleBlock.block.Body.Attributes["region"]
	if !hasRegion || !isAllowedRegionBinding(normalizeExprSource(googleBlock.srcBytes, regionAttr.Expr)) {
		return fmt.Errorf("offline: the provider is not bound to the funnel's region")
	}
	return nil
}

func inspectProviderBlock(block *hclsyntax.Block, srcBytes []byte, googleBlocks *[]providerBlockEntry) error {
	if len(block.Labels) != 1 || block.Labels[0] != "google" {
		name := "unlabeled"
		if len(block.Labels) > 0 {
			name = block.Labels[0]
		}
		return fmt.Errorf("offline: unauthorized provider declared: %s", name)
	}
	*googleBlocks = append(*googleBlocks, providerBlockEntry{
		block:    block,
		srcBytes: srcBytes,
	})
	return nil
}

func inspectTFBlocks(body *hclsyntax.Body, srcBytes []byte, googleBlocks *[]providerBlockEntry) error {
	for _, block := range body.Blocks {
		if block.Type == "import" || block.Type == "module" || block.Type == "data" {
			return fmt.Errorf("offline: a module, import or data source is not accepted here")
		}
		if block.Type == "provider" {
			if err := inspectProviderBlock(block, srcBytes, googleBlocks); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseAndInspectTFFile(path string, googleBlocks *[]providerBlockEntry) error {
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	file, diags := hclsyntax.ParseConfig(srcBytes, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("offline: invalid HCL configuration in %s: %s", path, diags.Error())
	}
	body := file.Body.(*hclsyntax.Body)
	return inspectTFBlocks(body, srcBytes, googleBlocks)
}

func isAcceptedTfFile(name string) bool {
	if isIgnoredFile(name) {
		return false
	}
	return strings.HasSuffix(name, ".tf")
}

func collectTFFiles(configDir string) []string {
	entries, _ := os.ReadDir(configDir)
	var tfFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isAcceptedTfFile(entry.Name()) {
			tfFiles = append(tfFiles, filepath.Join(configDir, entry.Name()))
		}
	}
	sort.Strings(tfFiles)
	return tfFiles
}

// CheckServiceConfig verifies that the OpenTofu service directory contains only
// accepted configuration files, no unauthorized imports/modules/data/providers, and that
// exactly one google provider block is declared and bound to the funnel's contract values.
func CheckServiceConfig(configDir string) error {
	if err := verifyNoForbiddenFiles(configDir); err != nil {
		return err
	}
	tfFiles := collectTFFiles(configDir)
	var googleBlocks []providerBlockEntry
	for _, path := range tfFiles {
		if err := parseAndInspectTFFile(path, &googleBlocks); err != nil {
			return err
		}
	}
	if len(googleBlocks) != 1 {
		return fmt.Errorf("offline: exactly one google provider block required, found %d", len(googleBlocks))
	}
	return verifyGoogleProviderBlock(googleBlocks[0])
}

func parseStreamLine(line string, summary *map[string]any, plans *[]map[string]any) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}
	t, _ := msg["type"].(string)
	if t == "test_summary" {
		if s, ok := msg["test_summary"].(map[string]any); ok {
			*summary = s
		}
	} else if t == "test_plan" {
		if p, ok := msg["test_plan"].(map[string]any); ok {
			*plans = append(*plans, p)
		}
	}
}

func parseTestStream(streamPath string) (map[string]any, []map[string]any, error) {
	f, err := os.Open(streamPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var summary map[string]any
	var plans []map[string]any

	for {
		line, rErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			parseStreamLine(line, &summary, &plans)
		}
		if rErr != nil {
			break
		}
	}
	return summary, plans, nil
}

func verifyTestSummary(summary map[string]any) error {
	if summary == nil {
		return fmt.Errorf("offline: a mock test failed")
	}
	status, _ := summary["status"].(string)
	failed, _ := summary["failed"].(float64)
	errored, _ := summary["errored"].(float64)
	passed, _ := summary["passed"].(float64)

	if status != "pass" || failed > 0 || errored > 0 {
		return fmt.Errorf("offline: a mock test failed")
	}
	if passed < 9 {
		return fmt.Errorf("offline: not every mock test ran")
	}
	return nil
}

func verifyDeclaredResources(resources []any) error {
	var declared []string
	for _, item := range resources {
		rMap, _ := item.(map[string]any)
		if mode, _ := rMap["mode"].(string); mode != "managed" {
			return fmt.Errorf("offline: a module, import or data source is not accepted here")
		}
		addr, _ := rMap["address"].(string)
		declared = append(declared, addr)
	}
	sort.Strings(declared)
	expected := []string{
		"google_cloud_run_v2_service.bff",
		"google_cloud_run_v2_service_iam_member.public_invoker",
	}
	if !equalSlices(declared, expected) {
		return fmt.Errorf("offline: the resource graph is not the two declared resources")
	}
	return nil
}

func verifyPlannedResources(pvResources []any) error {
	var planned []string
	for _, item := range pvResources {
		rMap, _ := item.(map[string]any)
		addr, _ := rMap["address"].(string)
		planned = append(planned, addr)
	}
	sort.Strings(planned)
	expected := []string{
		"google_cloud_run_v2_service.bff",
		"google_cloud_run_v2_service_iam_member.public_invoker",
	}
	if !equalSlices(planned, expected) {
		return fmt.Errorf("offline: the resource graph is not the two declared resources")
	}
	return nil
}

func checkScalingBounds(tExpr map[string]any) error {
	scalingList, _ := tExpr["scaling"].([]any)
	if len(scalingList) == 0 {
		return fmt.Errorf("offline: service scaling is missing")
	}
	sMap, _ := scalingList[0].(map[string]any)
	maxInstRefs := getNestedReferences(sMap, "max_instance_count")
	if !containsString(maxInstRefs, "local.inputs.bounds.max_instances") {
		return fmt.Errorf("offline: max_instance_count does not derive from the inputs bounds")
	}
	for _, ref := range maxInstRefs {
		if strings.Contains(ref, "budget_alert") {
			return fmt.Errorf("offline: max_instance_count must not depend on budget alert fields")
		}
	}
	return nil
}

func checkBFFServiceWiring(rMap map[string]any) error {
	exprs, _ := rMap["expressions"].(map[string]any)
	tmplList, _ := exprs["template"].([]any)
	if len(tmplList) == 0 {
		return fmt.Errorf("offline: service template is missing")
	}
	tExpr, _ := tmplList[0].(map[string]any)
	concRefs := getNestedReferences(tExpr, "max_instance_request_concurrency")
	if !containsString(concRefs, "local.inputs.bounds.concurrency") {
		return fmt.Errorf("offline: concurrency does not derive from the inputs bounds")
	}
	timeoutRefs := getNestedReferences(tExpr, "timeout")
	if !containsString(timeoutRefs, "local.inputs.bounds.request_timeout_seconds") {
		return fmt.Errorf("offline: timeout does not derive from the inputs bounds")
	}
	for _, ref := range append(concRefs, timeoutRefs...) {
		if strings.Contains(ref, "budget_alert") {
			return fmt.Errorf("offline: bounds must not depend on budget alert fields")
		}
	}
	return checkScalingBounds(tExpr)
}

func verifyServiceBoundsWiring(resources []any) error {
	for _, item := range resources {
		rMap, _ := item.(map[string]any)
		if addr, _ := rMap["address"].(string); addr == "google_cloud_run_v2_service.bff" {
			return checkBFFServiceWiring(rMap)
		}
	}
	return nil
}

func verifyServingOutputKeys(val map[string]any) error {
	for k := range val {
		if !containsString(allowedServingKeys, k) {
			return fmt.Errorf("offline: output serving contains unauthorized key: %s", k)
		}
	}
	for _, requiredKey := range allowedServingKeys {
		if _, ok := val[requiredKey]; !ok {
			return fmt.Errorf("offline: output serving missing required key: %s", requiredKey)
		}
	}
	return nil
}

func verifyOutputNames(outputs map[string]any) error {
	for k := range outputs {
		if k != "serving" {
			return fmt.Errorf("offline: output %s consumes budget alert fields or is not authorized", k)
		}
	}
	return nil
}

func verifyNoBudgetStrings(cfgOutput, pvOutput any) error {
	cfgBytes, _ := json.Marshal(cfgOutput)
	pvBytes, _ := json.Marshal(pvOutput)
	for _, raw := range []string{string(cfgBytes), string(pvBytes)} {
		if strings.Contains(raw, "budget_alert_amount") || strings.Contains(raw, "budget_alert_currency") {
			return fmt.Errorf("offline: output serving consumes budget alert fields")
		}
	}
	return nil
}

func verifyPlanOutputs(root, pv map[string]any) error {
	cfgOutputs, _ := root["outputs"].(map[string]any)
	pvOutputs, _ := pv["outputs"].(map[string]any)
	if err := verifyOutputNames(cfgOutputs); err != nil {
		return err
	}
	if err := verifyOutputNames(pvOutputs); err != nil {
		return err
	}
	servingPV, ok := pvOutputs["serving"].(map[string]any)
	if !ok {
		return fmt.Errorf("offline: output serving missing from planned values")
	}
	val, ok := servingPV["value"].(map[string]any)
	if !ok {
		return fmt.Errorf("offline: output serving value is not a map")
	}
	if err := verifyServingOutputKeys(val); err != nil {
		return err
	}
	return verifyNoBudgetStrings(cfgOutputs["serving"], servingPV)
}

func verifyProviderConfig(cfg map[string]any) error {
	providerCfg, _ := cfg["provider_config"].(map[string]any)
	googleCfg, _ := providerCfg["google"].(map[string]any)
	googleExprs, _ := googleCfg["expressions"].(map[string]any)

	projRefs := getNestedReferences(googleExprs, "project")
	if !containsString(projRefs, "var.inputs_file") &&
		!containsString(projRefs, "local.inputs") &&
		!containsString(projRefs, "local.inputs.contract.project_id") {
		return fmt.Errorf("offline: the provider is not bound to the funnel's project")
	}

	regRefs := getNestedReferences(googleExprs, "region")
	if !containsString(regRefs, "var.inputs_file") &&
		!containsString(regRefs, "local.inputs") &&
		!containsString(regRefs, "local.inputs.contract.region") {
		return fmt.Errorf("offline: the provider is not bound to the funnel's region")
	}
	return nil
}

func verifySinglePlan(plan map[string]any) error {
	cfg, _ := plan["configuration"].(map[string]any)
	root, _ := cfg["root_module"].(map[string]any)
	if mc, ok := root["module_calls"].(map[string]any); ok && len(mc) > 0 {
		return fmt.Errorf("offline: a module, import or data source is not accepted here")
	}
	rawRes, _ := root["resources"].([]any)
	if err := verifyDeclaredResources(rawRes); err != nil {
		return err
	}
	pv, _ := plan["planned_values"].(map[string]any)
	pvRoot, _ := pv["root_module"].(map[string]any)
	pvRawRes, _ := pvRoot["resources"].([]any)
	if err := verifyPlannedResources(pvRawRes); err != nil {
		return err
	}
	if err := verifyServiceBoundsWiring(rawRes); err != nil {
		return err
	}
	if err := verifyPlanOutputs(root, pv); err != nil {
		return err
	}
	return verifyProviderConfig(cfg)
}

func getPlanInputsFile(plan map[string]any) string {
	vars, ok := plan["variables"].(map[string]any)
	if !ok {
		return ""
	}
	inFile, ok := vars["inputs_file"].(map[string]any)
	if !ok {
		return ""
	}
	val, _ := inFile["value"].(string)
	return val
}

func getPlanServingValue(plan map[string]any) map[string]any {
	pv, ok := plan["planned_values"].(map[string]any)
	if !ok {
		return nil
	}
	pvOutputs, ok := pv["outputs"].(map[string]any)
	if !ok {
		return nil
	}
	servingPV, ok := pvOutputs["serving"].(map[string]any)
	if !ok {
		return nil
	}
	val, _ := servingPV["value"].(map[string]any)
	return val
}

func getPlanServiceResource(plan map[string]any) map[string]any {
	pv, ok := plan["planned_values"].(map[string]any)
	if !ok {
		return nil
	}
	root, ok := pv["root_module"].(map[string]any)
	if !ok {
		return nil
	}
	resources, ok := root["resources"].([]any)
	if !ok {
		return nil
	}
	for _, r := range resources {
		rMap, ok := r.(map[string]any)
		if ok && rMap["address"] == "google_cloud_run_v2_service.bff" {
			vals, _ := rMap["values"].(map[string]any)
			return vals
		}
	}
	return nil
}

type keyPlanWitness struct {
	serving  map[string]any
	resource map[string]any
}

func extractKeyPlanWitness(plans []map[string]any) (keyPlanWitness, keyPlanWitness) {
	var example, budget keyPlanWitness
	for _, plan := range plans {
		inputsFile := getPlanInputsFile(plan)
		sVal := getPlanServingValue(plan)
		rVal := getPlanServiceResource(plan)
		if strings.HasSuffix(inputsFile, "example.inputs.json") {
			example = keyPlanWitness{serving: sVal, resource: rVal}
		} else if strings.HasSuffix(inputsFile, "budget.inputs.json") {
			budget = keyPlanWitness{serving: sVal, resource: rVal}
		}
	}
	return example, budget
}

func verifyBudgetWitnessInvariance(plans []map[string]any) error {
	example, budget := extractKeyPlanWitness(plans)
	if example.serving == nil || budget.serving == nil || example.resource == nil || budget.resource == nil {
		return fmt.Errorf("offline: required comparison plan missing (example or budget)")
	}
	if !reflect.DeepEqual(example.serving, budget.serving) {
		return fmt.Errorf("offline: output serving varies under budget alert changes")
	}
	if !reflect.DeepEqual(example.resource, budget.resource) {
		return fmt.Errorf("offline: planned resource bounds vary under budget alert changes")
	}
	return nil
}

// CheckTestStream parses and verifies the JSON stream output from `tofu test -json`.
func CheckTestStream(streamPath string) error {
	summary, plans, err := parseTestStream(streamPath)
	if err != nil {
		return err
	}
	if err := verifyTestSummary(summary); err != nil {
		return err
	}
	if len(plans) == 0 {
		return fmt.Errorf("offline: no planned inventory produced by tofu test")
	}
	for _, plan := range plans {
		if err := verifySinglePlan(plan); err != nil {
			return err
		}
	}
	return verifyBudgetWitnessInvariance(plans)
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\r' && r != '\n' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func getNestedReferences(parent map[string]any, key string) []string {
	field, ok := parent[key].(map[string]any)
	if !ok {
		return nil
	}
	rawRefs, ok := field["references"].([]any)
	if !ok {
		return nil
	}
	var res []string
	for _, r := range rawRefs {
		if s, ok := r.(string); ok {
			res = append(res, s)
		}
	}
	return res
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type tofuInvocation struct {
	globalFlags []string
	subcommand  string
	args        []string
}

func isGlobalTofuFlag(arg string) bool {
	if strings.HasPrefix(arg, "-chdir=") || arg == "-no-color" ||
		arg == "-help" || arg == "--help" || arg == "-h" ||
		arg == "-version" || arg == "-v" || arg == "--version" {
		return true
	}
	return strings.HasPrefix(arg, "-")
}

func isTofuSeparator(f string) bool {
	return f == ";" || f == "&&" || f == "||" || f == "|" || f == "&"
}

func parseTofuField(fields []string, j int, inv *tofuInvocation) int {
	f := fields[j]
	if f == "-chdir" && j+1 < len(fields) {
		inv.globalFlags = append(inv.globalFlags, f, fields[j+1])
		return j + 2
	}
	if isGlobalTofuFlag(f) && inv.subcommand == "" {
		inv.globalFlags = append(inv.globalFlags, f)
		return j + 1
	}
	if inv.subcommand == "" {
		inv.subcommand = f
		return j + 1
	}
	inv.args = append(inv.args, f)
	return j + 1
}

func parseOneTofu(fields []string, idx int) (tofuInvocation, int) {
	inv := tofuInvocation{}
	j := idx + 1
	for j < len(fields) {
		if isTofuSeparator(fields[j]) {
			return inv, j + 1
		}
		j = parseTofuField(fields, j, &inv)
	}
	return inv, j
}

func parseTofuCommands(line string) []tofuInvocation {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return nil
	}
	if idx := strings.Index(line, "#"); idx != -1 {
		line = line[:idx]
	}
	fields := strings.Fields(line)
	var invocations []tofuInvocation
	for i := 0; i < len(fields); i++ {
		if fields[i] == "tofu" || strings.HasSuffix(fields[i], "/tofu") {
			inv, next := parseOneTofu(fields, i)
			invocations = append(invocations, inv)
			i = next - 1
		}
	}
	return invocations
}

func checkNoEnvOverride(line string) error {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return nil
	}
	if idx := strings.Index(line, "#"); idx != -1 {
		line = line[:idx]
	}
	if m := forbiddenEnvRegex.FindString(line); m != "" {
		return fmt.Errorf("offline: apply step must not set or export %s", m)
	}
	return nil
}

func isPlanPath(target string) bool {
	clean := filepath.Clean(target)
	return clean == "/workspace/plan.tfplan" || clean == "plan.tfplan" || filepath.Base(clean) == "plan.tfplan"
}

func checkPlanOverwriteArg(arg string) bool {
	if strings.HasPrefix(arg, "-out=") {
		return isPlanPath(strings.TrimPrefix(arg, "-out="))
	}
	if strings.HasPrefix(arg, ">") {
		return isPlanPath(strings.TrimLeft(arg, ">"))
	}
	return false
}

func anyFieldIsPlanPath(fields []string) bool {
	for _, f := range fields {
		if isPlanPath(f) {
			return true
		}
	}
	return false
}

func isFieldPlanOverwrite(fields []string, i int) bool {
	f := fields[i]
	if checkPlanOverwriteArg(f) {
		return true
	}
	if (f == "-out" || f == ">" || f == ">>") && i+1 < len(fields) {
		return isPlanPath(fields[i+1])
	}
	return false
}

func checkCopyOverwrite(fields []string, i int) bool {
	f := fields[i]
	if (f == "cp" || f == "mv") && i+1 < len(fields) {
		return anyFieldIsPlanPath(fields[i+1:])
	}
	return false
}

func checkNoPlanOverwriteInApply(line string) error {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return nil
	}
	if idx := strings.Index(line, "#"); idx != -1 {
		line = line[:idx]
	}
	fields := strings.Fields(line)
	for i := range fields {
		if isFieldPlanOverwrite(fields, i) || checkCopyOverwrite(fields, i) {
			return fmt.Errorf("offline: apply step must not overwrite /workspace/plan.tfplan")
		}
	}
	return nil
}

func checkTofuInitCmd(inv tofuInvocation) error {
	if len(inv.globalFlags) > 0 {
		return fmt.Errorf("offline: tofu init must not contain global flags in apply step")
	}
	expected := []string{"-input=false", "-lockfile=readonly"}
	if !equalSlices(inv.args, expected) {
		return fmt.Errorf("offline: tofu init must accept only exact arguments -input=false -lockfile=readonly")
	}
	return nil
}

func checkTofuApplyCmd(inv tofuInvocation) error {
	if len(inv.globalFlags) > 0 {
		return fmt.Errorf("offline: tofu apply must accept only exact arguments -input=false -auto-approve /workspace/plan.tfplan")
	}
	expected := []string{"-input=false", "-auto-approve", "/workspace/plan.tfplan"}
	if !equalSlices(inv.args, expected) {
		return fmt.Errorf("offline: tofu apply must accept only exact arguments -input=false -auto-approve /workspace/plan.tfplan")
	}
	return nil
}

type applyScriptValidator struct {
	hasInit  bool
	hasApply bool
}

func (v *applyScriptValidator) validateInit(inv tofuInvocation) error {
	if v.hasInit {
		return fmt.Errorf("offline: duplicate tofu init in apply step")
	}
	if v.hasApply {
		return fmt.Errorf("offline: tofu init cannot occur after tofu apply in apply step")
	}
	if err := checkTofuInitCmd(inv); err != nil {
		return err
	}
	v.hasInit = true
	return nil
}

func (v *applyScriptValidator) validateApply(inv tofuInvocation) error {
	if !v.hasInit {
		return fmt.Errorf("offline: tofu apply must occur after tofu init in apply step")
	}
	if v.hasApply {
		return fmt.Errorf("offline: duplicate tofu apply in apply step")
	}
	if err := checkTofuApplyCmd(inv); err != nil {
		return err
	}
	v.hasApply = true
	return nil
}

func (v *applyScriptValidator) validateInvocation(inv tofuInvocation) error {
	if inv.subcommand == "" {
		return fmt.Errorf("offline: apply step contains tofu invocation with missing subcommand")
	}
	if inv.subcommand == "init" {
		return v.validateInit(inv)
	}
	if inv.subcommand == "apply" {
		return v.validateApply(inv)
	}
	return fmt.Errorf("offline: apply step must not invoke tofu %s", inv.subcommand)
}

func (v *applyScriptValidator) validateLine(line string) error {
	if err := checkNoEnvOverride(line); err != nil {
		return err
	}
	if err := checkNoPlanOverwriteInApply(line); err != nil {
		return err
	}
	for _, inv := range parseTofuCommands(line) {
		if err := v.validateInvocation(inv); err != nil {
			return err
		}
	}
	return nil
}

func checkApprovalDigestMask(line string) error {
	if strings.Contains(line, "_APPROVED_PLAN_SHA256") && strings.Contains(line, "sha256sum") {
		if strings.Contains(line, "|| true") || strings.Contains(line, "|| :") {
			return fmt.Errorf("offline: apply step approved plan digest check must not be masked with || true")
		}
	}
	return nil
}

func checkSha256Strict(line string) error {
	if !strings.Contains(line, "sha256sum") {
		return nil
	}
	if strings.Contains(line, "-c") || strings.Contains(line, "--check") {
		if !strings.Contains(line, "--strict") {
			return fmt.Errorf("offline: apply step must invoke sha256sum with --strict flag")
		}
	}
	return nil
}

func checkApprovalDigestVerification(script string) error {
	if !strings.Contains(script, "sha256sum --strict -c -") {
		return fmt.Errorf("offline: apply step must verify plan digest with sha256sum --strict -c -")
	}
	if !strings.Contains(script, `grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256`) {
		return fmt.Errorf("offline: apply step must verify plan sidecar checksum with grep against approved digest")
	}
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if err := checkApprovalDigestMask(trimmed); err != nil {
			return err
		}
		if err := checkSha256Strict(trimmed); err != nil {
			return err
		}
	}
	return nil
}

// ValidateApplyStepScript verifies that an apply step script contains no unauthorized tofu commands,
// no TF_* or OPENTOFU_* overrides, does not overwrite /workspace/plan.tfplan, and uses exact approved
// tofu init and tofu apply arguments.
func ValidateApplyStepScript(script string) error {
	if err := checkApprovalDigestVerification(script); err != nil {
		return err
	}
	v := &applyScriptValidator{}
	for _, line := range strings.Split(script, "\n") {
		if err := v.validateLine(line); err != nil {
			return err
		}
	}
	if !v.hasApply {
		return fmt.Errorf("offline: apply step must invoke tofu apply")
	}
	return nil
}

func isScriptTerminator(trimmed, rawLine string) bool {
	if len(rawLine) > 0 && !strings.HasPrefix(rawLine, " ") && !strings.HasPrefix(rawLine, "\t") {
		return true
	}
	return strings.HasPrefix(trimmed, "- id:") || strings.HasPrefix(trimmed, "options:") || strings.HasPrefix(trimmed, "timeout:")
}

type stepExtractor struct {
	targetID string
	inStep   bool
	inScript bool
	lines    []string
}

func (e *stepExtractor) matchStepHeader(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "- id:") {
		return false
	}
	if e.inStep {
		return true
	}
	idVal := strings.Trim(strings.TrimPrefix(trimmed, "- id:"), " \"'")
	if idVal == e.targetID {
		e.inStep = true
	}
	return false
}

func (e *stepExtractor) step(rawLine string) bool {
	trimmed := strings.TrimSpace(rawLine)
	if e.matchStepHeader(trimmed) {
		return false
	}
	if !e.inStep {
		return true
	}
	if !e.inScript {
		if trimmed == "|" || trimmed == ">" || strings.HasSuffix(trimmed, "|") || strings.HasSuffix(trimmed, ">") {
			e.inScript = true
		}
		return true
	}
	if isScriptTerminator(trimmed, rawLine) {
		return false
	}
	e.lines = append(e.lines, rawLine)
	return true
}

func extractStepScript(content, stepID string) string {
	e := &stepExtractor{targetID: stepID}
	for _, line := range strings.Split(content, "\n") {
		if !e.step(line) {
			break
		}
	}
	return strings.Join(e.lines, "\n")
}

func containsActiveCommand(script, command string) bool {
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, command) {
			return true
		}
	}
	return false
}

func verifyPipelinePromoteStep(content string) error {
	script := extractStepScript(content, "promote-by-digest")
	if !containsActiveCommand(script, "crane digest") {
		return fmt.Errorf("offline: promote-by-digest step must verify digest with crane digest")
	}
	return nil
}

func verifyPipelineCancelStep(content string) error {
	script := extractStepScript(content, "cancel-in-flight")
	if !containsActiveCommand(script, "id!=${BUILD_ID}") {
		return fmt.Errorf("offline: cancel-in-flight step must exclude current build id from filter")
	}
	return nil
}

func verifyPipelineRecordStep(content string) error {
	script := extractStepScript(content, "record")
	if !containsActiveCommand(script, "gcloud run services describe bff") {
		return fmt.Errorf("offline: record step must describe service")
	}
	return nil
}

type cloudbuildStep struct {
	ID   string   `yaml:"id"`
	Env  []string `yaml:"env"`
	Args []string `yaml:"args"`
}

type cloudbuildOptions struct {
	Env                  []string `yaml:"env"`
	AutomapSubstitutions bool     `yaml:"automapSubstitutions"`
	Logging              string   `yaml:"logging"`
}

type cloudbuildConfig struct {
	Steps   []cloudbuildStep  `yaml:"steps"`
	Options cloudbuildOptions `yaml:"options"`
}

func isForbiddenEnvName(entry string) bool {
	name := entry
	if idx := strings.IndexByte(entry, '='); idx != -1 {
		name = entry[:idx]
	}
	name = strings.TrimSpace(name)
	upper := strings.ToUpper(name)
	return strings.HasPrefix(upper, "TF_") ||
		strings.HasPrefix(upper, "OPENTOFU_") ||
		strings.HasPrefix(upper, "GOOGLE_") ||
		strings.HasPrefix(upper, "GCLOUD_") ||
		strings.HasPrefix(upper, "CLOUDSDK_")
}

func checkEnvList(entries []string, context string) error {
	for _, entry := range entries {
		if isForbiddenEnvName(entry) {
			return fmt.Errorf("offline: %s must not set TF_*, OPENTOFU_*, GOOGLE_*, GCLOUD_*, or CLOUDSDK_* environment variables: %q", context, entry)
		}
	}
	return nil
}

func checkPipelineEnv(cfg cloudbuildConfig) error {
	if err := checkEnvList(cfg.Options.Env, "options.env"); err != nil {
		return err
	}
	for _, step := range cfg.Steps {
		if err := checkEnvList(step.Env, fmt.Sprintf("step %s env", step.ID)); err != nil {
			return err
		}
	}
	return nil
}

func hasStepEnvEntry(env []string, expected string) bool {
	for _, entry := range env {
		if strings.TrimSpace(entry) == expected {
			return true
		}
	}
	return false
}

func verifyRequiredStepEnv(step cloudbuildStep, required []string) error {
	for _, req := range required {
		if !hasStepEnvEntry(step.Env, req) {
			return fmt.Errorf("offline: step %s must declare %s in env", step.ID, req)
		}
	}
	return nil
}

func verifyStepEnvMappings(cfg cloudbuildConfig) error {
	stepMap := make(map[string]cloudbuildStep)
	for _, step := range cfg.Steps {
		stepMap[step.ID] = step
	}
	planReq := []string{"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}", "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"}
	if err := verifyRequiredStepEnv(stepMap["plan"], planReq); err != nil {
		return err
	}
	applyReq := []string{"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}", "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"}
	if err := verifyRequiredStepEnv(stepMap["apply-behind-the-gate"], applyReq); err != nil {
		return err
	}
	recordReq := []string{"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"}
	return verifyRequiredStepEnv(stepMap["record"], recordReq)
}

// CheckPipelineConfig verifies that the Cloud Build pipeline file obeys all approval,
// step-behavior, and apply-step argument constraints.
func CheckPipelineConfig(pipelinePath string) error {
	bytes, err := os.ReadFile(pipelinePath)
	if err != nil {
		return err
	}
	var cfg cloudbuildConfig
	if err := yaml.Unmarshal(bytes, &cfg); err != nil {
		return fmt.Errorf("offline: invalid pipeline yaml: %w", err)
	}
	if !cfg.Options.AutomapSubstitutions {
		return fmt.Errorf("offline: options.automapSubstitutions must be true")
	}
	if err := checkPipelineEnv(cfg); err != nil {
		return err
	}
	if err := verifyStepEnvMappings(cfg); err != nil {
		return err
	}
	content := string(bytes)
	applyScript := extractStepScript(content, "apply-behind-the-gate")
	if applyScript == "" {
		return fmt.Errorf("offline: missing apply-behind-the-gate step in pipeline")
	}
	if err := ValidateApplyStepScript(applyScript); err != nil {
		return err
	}
	if err := verifyPipelinePromoteStep(content); err != nil {
		return err
	}
	if err := verifyPipelineCancelStep(content); err != nil {
		return err
	}
	return verifyPipelineRecordStep(content)
}
