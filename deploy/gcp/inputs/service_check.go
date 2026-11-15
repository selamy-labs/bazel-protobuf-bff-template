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
	"unicode"
	"unicode/utf8"

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

type applyToken struct {
	raw   string
	value string
	isOp  bool
}

var applyOperators = []string{
	"&>>", "&&", "||", "|&", ";;", ";&",
	">>", ">|", ">&", "&>", "<>", "<&",
	";", "|", "&", "(", ")", ">", "<",
}

func matchApplyOp(s string) string {
	for _, op := range applyOperators {
		if strings.HasPrefix(s, op) {
			return op
		}
	}
	return ""
}

func consumeSingleQuoted(clean string, i int, buf *strings.Builder) int {
	for i++; i < len(clean) && clean[i] != '\''; i++ {
		buf.WriteByte(clean[i])
	}
	return min(i+1, len(clean))
}

func consumeDoubleQuoted(clean string, i int, buf *strings.Builder) int {
	i++
	for i < len(clean) && clean[i] != '"' {
		if clean[i] == '\\' && i+1 < len(clean) && strings.ContainsRune("\"\\$`", rune(clean[i+1])) {
			buf.WriteByte(clean[i+1])
			i += 2
			continue
		}
		buf.WriteByte(clean[i])
		i++
	}
	return min(i+1, len(clean))
}

func scanApplyWord(clean string, start int) (applyToken, int) {
	var buf strings.Builder
	i := start
	for i < len(clean) {
		b := clean[i]
		if b == ' ' || b == '\t' || matchApplyOp(clean[i:]) != "" {
			break
		}
		if b == '\'' {
			i = consumeSingleQuoted(clean, i, &buf)
			continue
		}
		if b == '"' {
			i = consumeDoubleQuoted(clean, i, &buf)
			continue
		}
		if b == '\\' && i+1 < len(clean) {
			r, w := utf8.DecodeRuneInString(clean[i+1:])
			buf.WriteRune(r)
			i += 1 + w
			continue
		}
		r, w := utf8.DecodeRuneInString(clean[i:])
		buf.WriteRune(r)
		i += w
	}
	return applyToken{raw: clean[start:i], value: buf.String()}, i
}

func tokenizeApplyLine(clean string) []applyToken {
	var tokens []applyToken
	for i := 0; i < len(clean); {
		if clean[i] == ' ' || clean[i] == '\t' {
			i++
			continue
		}
		if op := matchApplyOp(clean[i:]); op != "" {
			tokens = append(tokens, applyToken{raw: op, value: op, isOp: true})
			i += len(op)
			continue
		}
		tok, next := scanApplyWord(clean, i)
		tokens = append(tokens, tok)
		i = next
	}
	return tokens
}

func isTofuSeparatorToken(t applyToken) bool {
	if t.isOp {
		return isTofuSeparator(t.raw) || t.raw == "|&"
	}
	return t.raw == "{" || t.raw == "}"
}

func isTofuCommandToken(t applyToken) bool {
	return !t.isOp && (t.value == "tofu" || strings.HasSuffix(t.value, "/tofu"))
}

func parseTofuToken(tokens []applyToken, j int, inv *tofuInvocation) int {
	tok := tokens[j]
	if tok.value == "-chdir" && j+1 < len(tokens) {
		inv.globalFlags = append(inv.globalFlags, tok.raw, tokens[j+1].raw)
		return j + 2
	}
	if !tok.isOp && isGlobalTofuFlag(tok.value) && inv.subcommand == "" {
		inv.globalFlags = append(inv.globalFlags, tok.raw)
		return j + 1
	}
	if inv.subcommand == "" {
		inv.subcommand = tok.raw
		return j + 1
	}
	inv.args = append(inv.args, tok.raw)
	return j + 1
}

func parseOneTofu(tokens []applyToken, idx int) (tofuInvocation, int) {
	inv := tofuInvocation{}
	j := idx + 1
	for j < len(tokens) {
		if isTofuSeparatorToken(tokens[j]) {
			return inv, j + 1
		}
		j = parseTofuToken(tokens, j, &inv)
	}
	return inv, j
}

func unsupportedApplyWhitespace(quote, r rune) bool {
	return quote == 0 && r != ' ' && r != '\t' && unicode.IsSpace(r)
}

func isUnsupportedUnquotedRedirect(line string, i int) bool {
	rest := line[i:]
	return strings.HasPrefix(rest, "<<") || strings.HasPrefix(rest, "<(") || strings.HasPrefix(rest, ">(")
}

func isApplyOperator(r rune) bool {
	switch r {
	case ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

func isApplyIdentByte(b byte, first bool) bool {
	if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_' {
		return true
	}
	return !first && b >= '0' && b <= '9'
}

func simpleBracedVarLen(rest string) (int, bool) {
	if len(rest) < 3 || rest[0] != '{' || !isApplyIdentByte(rest[1], true) {
		return 0, false
	}
	i := 2
	for i < len(rest) && isApplyIdentByte(rest[i], false) {
		i++
	}
	if strings.HasPrefix(rest[i:], ":-") {
		i += 2
	}
	if i < len(rest) && rest[i] == '}' {
		return i + 1, true
	}
	return 0, false
}

func checkApplyDollar(rest string, quote rune) (int, error) {
	if quote == 0 && (strings.HasPrefix(rest, "'") || strings.HasPrefix(rest, "\"")) {
		return 0, fmt.Errorf("offline: dollar-quoted string is not supported in apply step")
	}
	if strings.HasPrefix(rest, "(") || strings.HasPrefix(rest, "[") {
		return 0, fmt.Errorf("offline: command or arithmetic expansion is not supported in apply step")
	}
	if strings.HasPrefix(rest, "{") {
		if n, ok := simpleBracedVarLen(rest); ok {
			return n, nil
		}
		return 0, fmt.Errorf("offline: unsupported parameter expansion in apply step")
	}
	return 0, nil
}

type applyCommentScanner struct {
	quote       rune
	escaped     bool
	atWordStart bool
	afterOp     bool
}

func (s *applyCommentScanner) stepUnquoted(line string, i int, r rune) (bool, error) {
	switch {
	case r == '\'' || r == '"':
		s.quote, s.atWordStart, s.afterOp = r, false, false
	case (r == '<' || r == '>') && isUnsupportedUnquotedRedirect(line, i):
		return false, fmt.Errorf("offline: heredoc or process substitution is not supported in apply step")
	case r == '#' && s.afterOp:
		return false, fmt.Errorf("offline: ambiguous hash after shell operator in apply step")
	case r == '#' && s.atWordStart:
		return true, nil
	case r == ' ' || r == '\t':
		s.atWordStart, s.afterOp = true, false
	case isApplyOperator(r):
		s.atWordStart, s.afterOp = false, true
	default:
		s.atWordStart, s.afterOp = false, false
	}
	return false, nil
}

func (s *applyCommentScanner) step(line string, i int, r rune, width int) (bool, int, error) {
	if unsupportedApplyWhitespace(s.quote, r) {
		return false, 0, fmt.Errorf("offline: unsupported whitespace in apply step")
	}
	if s.escaped {
		s.escaped, s.atWordStart, s.afterOp = false, false, false
		return false, width, nil
	}
	if r == '\\' && s.quote != '\'' {
		s.escaped, s.atWordStart, s.afterOp = true, false, false
		return false, width, nil
	}
	if s.quote != 0 && r == s.quote {
		s.quote, s.atWordStart, s.afterOp = 0, false, false
		return false, width, nil
	}
	if s.quote == '\'' {
		s.atWordStart, s.afterOp = false, false
		return false, width, nil
	}
	if r == '`' {
		return false, 0, fmt.Errorf("offline: backtick expansion is not supported in apply step")
	}
	if r == '$' {
		skip, err := checkApplyDollar(line[i+1:], s.quote)
		if err != nil {
			return false, 0, err
		}
		s.atWordStart, s.afterOp = false, false
		return false, 1 + skip, nil
	}
	if s.quote == '"' {
		s.atWordStart, s.afterOp = false, false
		return false, width, nil
	}
	done, err := s.stepUnquoted(line, i, r)
	return done, width, err
}

// stripApplyComment preserves supported quotes and parameter expansions, rejects unsafe shell syntax,
// and trims unquoted shell comments.
func stripApplyComment(line string) (string, error) {
	scanner := applyCommentScanner{atWordStart: true}
	for i := 0; i < len(line); {
		r, width := utf8.DecodeRuneInString(line[i:])
		done, adv, err := scanner.step(line, i, r, width)
		if err != nil {
			return "", err
		}
		if done {
			return line[:i], nil
		}
		i += adv
	}
	if scanner.escaped {
		return "", fmt.Errorf("offline: line continuation is not supported in apply step")
	}
	if scanner.quote != 0 {
		return "", fmt.Errorf("offline: unterminated quote in apply step")
	}
	return line, nil
}

func parseTofuCommands(line string) []tofuInvocation {
	clean, err := stripApplyComment(line)
	if err != nil {
		return nil
	}
	tokens := tokenizeApplyLine(clean)
	var invocations []tofuInvocation
	for i := 0; i < len(tokens); i++ {
		if isTofuCommandToken(tokens[i]) {
			inv, next := parseOneTofu(tokens, i)
			invocations = append(invocations, inv)
			i = next - 1
		}
	}
	return invocations
}

func checkNoEnvOverride(line string) error {
	clean, err := stripApplyComment(line)
	if err != nil {
		return err
	}
	if m := forbiddenEnvRegex.FindString(clean); m != "" {
		return fmt.Errorf("offline: apply step must not set or export %s", m)
	}
	for _, tok := range tokenizeApplyLine(clean) {
		if !tok.isOp {
			if m := forbiddenEnvRegex.FindString(tok.value); m != "" {
				return fmt.Errorf("offline: apply step must not set or export %s", m)
			}
		}
	}
	return nil
}

func isPlanPath(target string) bool {
	if target == "" {
		return false
	}
	base := filepath.Base(filepath.Clean(target))
	return base == "plan.tfplan" || base == "plan.tfplan.sha256"
}

func isOutputRedirectOp(op string) bool {
	switch op {
	case ">", ">>", ">|", ">&", "&>", "&>>", "<>":
		return true
	}
	return false
}

func anyFieldIsPlanPath(fields []string) bool {
	for _, f := range fields {
		if isPlanPath(strings.TrimPrefix(f, "of=")) {
			return true
		}
	}
	return false
}

func isPlanMutatingCmd(cmd string) bool {
	switch filepath.Base(cmd) {
	case "cp", "mv", "tee", "install", "ln", "dd", "rm", "truncate":
		return true
	}
	return false
}

func checkCopyOverwrite(fields []string, i int) bool {
	if i+1 < len(fields) && isPlanMutatingCmd(fields[i]) {
		return anyFieldIsPlanPath(fields[i+1:])
	}
	return false
}

func tokenAtIsPlanOverwrite(tokens []applyToken, values []string, i int) bool {
	tok := tokens[i]
	if tok.isOp {
		return isOutputRedirectOp(tok.raw) && i+1 < len(tokens) && !tokens[i+1].isOp && isPlanPath(tokens[i+1].value)
	}
	if strings.HasPrefix(tok.value, "-out=") && isPlanPath(strings.TrimPrefix(tok.value, "-out=")) {
		return true
	}
	if tok.value == "-out" && i+1 < len(tokens) && !tokens[i+1].isOp && isPlanPath(tokens[i+1].value) {
		return true
	}
	return checkCopyOverwrite(values, i)
}

func checkNoPlanOverwriteInApply(line string) error {
	clean, err := stripApplyComment(line)
	if err != nil {
		return err
	}
	tokens := tokenizeApplyLine(clean)
	values := make([]string, len(tokens))
	for i, tok := range tokens {
		values[i] = tok.value
	}
	for i := range tokens {
		if tokenAtIsPlanOverwrite(tokens, values, i) {
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
	clean, _ := stripApplyComment(line)
	if err := checkSha256Strict(clean); err != nil {
		return err
	}
	for _, inv := range parseTofuCommands(line) {
		if err := v.validateInvocation(inv); err != nil {
			return err
		}
	}
	return nil
}

func isSha256CmdToken(tok applyToken) bool {
	return !tok.isOp && (tok.value == "sha256sum" || strings.HasSuffix(tok.value, "/sha256sum"))
}

func isGrepCmdToken(tok applyToken) bool {
	return !tok.isOp && (tok.value == "grep" || strings.HasSuffix(tok.value, "/grep"))
}

func hasSha256CheckFlag(arg string) bool {
	if arg == "--check" || strings.HasPrefix(arg, "--check=") {
		return true
	}
	return strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg, 'c')
}

func checkTokensSha256Strict(tokens []applyToken) error {
	for i, tok := range tokens {
		if !isSha256CmdToken(tok) {
			continue
		}
		hasCheck, hasStrict := false, false
		for j := i + 1; j < len(tokens) && !isTofuSeparatorToken(tokens[j]); j++ {
			if tokens[j].value == "--strict" {
				hasStrict = true
			}
			if hasSha256CheckFlag(tokens[j].value) {
				hasCheck = true
			}
		}
		if hasCheck && !hasStrict {
			return fmt.Errorf("offline: apply step must invoke sha256sum with --strict flag")
		}
	}
	return nil
}

func checkSha256Strict(norm string) error {
	return checkTokensSha256Strict(tokenizeApplyLine(norm))
}

func isUnsupportedApplyToken(tok applyToken) bool {
	if !tok.isOp {
		return false
	}
	switch tok.raw {
	case "(", ")", "&", "|&", ";;", ";&":
		return true
	}
	return false
}

func isUnsupportedApplyKeyword(val string) bool {
	switch val {
	case "function", "for", "while", "until", "case", "select":
		return true
	}
	return false
}

func continuesApplyLine(tok applyToken) bool {
	if tok.isOp {
		return tok.raw == "|" || tok.raw == "||" || tok.raw == "&&" || tok.raw == ";"
	}
	return tok.raw == "{"
}

func buildApplyTokenStream(script string) ([]applyToken, error) {
	var all []applyToken
	for _, line := range strings.Split(script, "\n") {
		clean, err := stripApplyComment(line)
		if err != nil {
			return nil, err
		}
		ltoks := tokenizeApplyLine(clean)
		if len(ltoks) == 0 {
			continue
		}
		for _, tok := range ltoks {
			if isUnsupportedApplyToken(tok) {
				return nil, fmt.Errorf("offline: unsupported shell construct in apply step")
			}
		}
		if len(all) > 0 && !continuesApplyLine(all[len(all)-1]) {
			all = append(all, applyToken{raw: ";", value: ";", isOp: true})
		}
		all = append(all, ltoks...)
	}
	if len(all) > 0 && all[len(all)-1].isOp {
		if all[len(all)-1].raw == "||" {
			return nil, fmt.Errorf("offline: apply step approved plan digest check must not be masked with || true")
		}
		if all[len(all)-1].raw == "|" || all[len(all)-1].raw == "&&" {
			return nil, fmt.Errorf("offline: incomplete command operator in apply step")
		}
	}
	return all, nil
}

func flushApplyChain(chains [][]applyToken, cur []applyToken) [][]applyToken {
	for len(cur) > 1 && !cur[0].isOp && (cur[0].raw == "then" || cur[0].raw == "else") {
		chains = append(chains, cur[:1])
		cur = cur[1:]
	}
	if len(cur) > 0 {
		chains = append(chains, cur)
	}
	return chains
}

func updateBraceDepth(depth int, tok applyToken) (int, error) {
	if !tok.isOp && tok.raw == "{" {
		return depth + 1, nil
	}
	if !tok.isOp && tok.raw == "}" {
		if depth == 0 {
			return 0, fmt.Errorf("offline: unmatched closing brace in apply step")
		}
		return depth - 1, nil
	}
	return depth, nil
}

func splitApplyChains(all []applyToken) ([][]applyToken, error) {
	var chains [][]applyToken
	var cur []applyToken
	depth := 0
	for _, tok := range all {
		if tok.isOp && tok.raw == ";" && depth == 0 {
			chains = flushApplyChain(chains, cur)
			cur = nil
			continue
		}
		nextDepth, err := updateBraceDepth(depth, tok)
		if err != nil {
			return nil, err
		}
		depth = nextDepth
		cur = append(cur, tok)
	}
	if depth != 0 {
		return nil, fmt.Errorf("offline: unclosed brace group in apply step")
	}
	return flushApplyChain(chains, cur), nil
}

type applyListItem struct {
	opBefore string
	tokens   []applyToken
}

func splitApplyListItems(chain []applyToken) ([]applyListItem, error) {
	var items []applyListItem
	var cur []applyToken
	opBefore := ""
	depth := 0
	for _, tok := range chain {
		depth, _ = updateBraceDepth(depth, tok)
		if depth == 0 && tok.isOp && (tok.raw == "&&" || tok.raw == "||") {
			if len(cur) == 0 {
				return nil, fmt.Errorf("offline: incomplete command operator in apply step")
			}
			items = append(items, applyListItem{opBefore: opBefore, tokens: cur})
			opBefore, cur = tok.raw, nil
			continue
		}
		cur = append(cur, tok)
	}
	if len(cur) == 0 {
		return nil, fmt.Errorf("offline: incomplete command operator in apply step")
	}
	items = append(items, applyListItem{opBefore: opBefore, tokens: cur})
	return items, nil
}

func splitPipelineStages(tokens []applyToken) ([][]applyToken, error) {
	var stages [][]applyToken
	var cur []applyToken
	depth := 0
	for _, tok := range tokens {
		depth, _ = updateBraceDepth(depth, tok)
		if depth == 0 && tok.isOp && tok.raw == "|" {
			if len(cur) == 0 {
				return nil, fmt.Errorf("offline: incomplete command operator in apply step")
			}
			stages = append(stages, cur)
			cur = nil
			continue
		}
		cur = append(cur, tok)
	}
	if len(cur) == 0 {
		return nil, fmt.Errorf("offline: incomplete command operator in apply step")
	}
	stages = append(stages, cur)
	return stages, nil
}

func isDigestGrepFrom(tokens []applyToken, start int) bool {
	for j := start + 1; j < len(tokens) && !isTofuSeparatorToken(tokens[j]); j++ {
		val := tokens[j].value
		if strings.Contains(val, "plan.tfplan.sha256") || strings.Contains(val, "_APPROVED_PLAN_SHA256") {
			return true
		}
	}
	return false
}

func itemContainsDigestCmd(tokens []applyToken) bool {
	for i, tok := range tokens {
		if isSha256CmdToken(tok) {
			return true
		}
		if isGrepCmdToken(tok) && isDigestGrepFrom(tokens, i) {
			return true
		}
	}
	return false
}

func hasDigestWrapper(stages [][]applyToken) bool {
	for _, st := range stages {
		if !st[0].isOp && (st[0].raw == "!" || st[0].raw == "{") {
			return true
		}
	}
	return false
}

func isNonZeroExitCode(tok applyToken) bool {
	if tok.isOp || tok.raw != tok.value || len(tok.raw) == 0 || len(tok.raw) > 3 {
		return false
	}
	val := 0
	for i := 0; i < len(tok.raw); i++ {
		c := tok.raw[i]
		if c < '0' || c > '9' {
			return false
		}
		val = val*10 + int(c-'0')
	}
	return val >= 1 && val <= 255
}

func isExitNonZeroCmd(toks []applyToken) bool {
	return len(toks) == 2 && !toks[0].isOp && toks[0].raw == "exit" && isNonZeroExitCode(toks[1])
}

func isSafeErrorEchoCmd(cmd []applyToken) bool {
	if len(cmd) == 0 || cmd[0].isOp || cmd[0].raw != "echo" {
		return false
	}
	for i, tok := range cmd[1:] {
		if tok.isOp {
			rest := cmd[i+1:]
			return len(rest) == 2 && rest[0].raw == ">&" && !rest[1].isOp && (rest[1].raw == "1" || rest[1].raw == "2")
		}
	}
	return true
}

func isDisallowedBraceInnerToken(tok applyToken) bool {
	if !tok.isOp {
		return tok.raw == "{" || tok.raw == "}"
	}
	return tok.raw == "&&" || tok.raw == "||" || tok.raw == "|"
}

func splitBraceBlockCmds(inner []applyToken) ([][]applyToken, bool) {
	var cmds [][]applyToken
	var cur []applyToken
	for _, tok := range inner {
		if isDisallowedBraceInnerToken(tok) {
			return nil, false
		}
		if tok.isOp && tok.raw == ";" {
			if len(cur) > 0 {
				cmds = append(cmds, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, tok)
	}
	if len(cur) > 0 {
		cmds = append(cmds, cur)
	}
	return cmds, true
}

func isFailClosedBraceBlock(toks []applyToken) bool {
	if len(toks) < 4 || toks[0].isOp || toks[0].raw != "{" || toks[len(toks)-1].isOp || toks[len(toks)-1].raw != "}" {
		return false
	}
	cmds, ok := splitBraceBlockCmds(toks[1 : len(toks)-1])
	if !ok || len(cmds) == 0 || !isExitNonZeroCmd(cmds[len(cmds)-1]) {
		return false
	}
	for _, prev := range cmds[:len(cmds)-1] {
		if !isSafeErrorEchoCmd(prev) {
			return false
		}
	}
	return true
}

func isFailClosedOrHandler(toks []applyToken) bool {
	return isExitNonZeroCmd(toks) || isFailClosedBraceBlock(toks)
}

func rawTokensEqual(tokens []applyToken, want ...string) bool {
	if len(tokens) != len(want) {
		return false
	}
	for i, w := range want {
		if tokens[i].isOp || tokens[i].raw != w {
			return false
		}
	}
	return true
}

func isValidShaPipeline(stages [][]applyToken) bool {
	if len(stages) == 1 {
		return rawTokensEqual(stages[0], "sha256sum", "--strict", "-c", "-")
	}
	if len(stages) == 2 {
		return rawTokensEqual(stages[0], "printf", `'%s  /workspace/plan.tfplan\n'`, `"${_APPROVED_PLAN_SHA256}"`) &&
			rawTokensEqual(stages[1], "sha256sum", "--strict", "-c", "-")
	}
	return false
}

func isValidGrepPipeline(stages [][]applyToken) bool {
	return len(stages) == 1 && rawTokensEqual(stages[0], "grep", "-Fq", `"${_APPROVED_PLAN_SHA256}"`, "/workspace/plan.tfplan.sha256")
}

func itemHasTofuApply(stages [][]applyToken) bool {
	for _, st := range stages {
		for i, tok := range st {
			if isTofuCommandToken(tok) {
				inv, _ := parseOneTofu(st, i)
				if inv.subcommand == "apply" {
					return true
				}
			}
		}
	}
	return false
}

type applyIfFrame struct {
	allowSidecar bool
	inElse       bool
}

type digestOrderTracker struct {
	shaSeq   int
	grepSeq  int
	applySeq int
	seq      int
	awaiting bool
	frames   []applyIfFrame
}

func checkDigestItemMask(items []applyListItem, idx int, stages [][]applyToken, allowDigest bool) error {
	item := items[idx]
	if !allowDigest || item.opBefore != "" || hasDigestWrapper(stages) {
		return fmt.Errorf("offline: apply step approved plan digest check must not be masked with || true")
	}
	if idx+1 < len(items) {
		if items[idx+1].opBefore != "||" || idx+2 != len(items) || !isFailClosedOrHandler(items[idx+1].tokens) {
			return fmt.Errorf("offline: apply step approved plan digest check must not be masked with || true")
		}
	}
	return nil
}

func validatePipelineStages(stages [][]applyToken) error {
	for _, st := range stages {
		if !st[0].isOp && isUnsupportedApplyKeyword(st[0].value) {
			return fmt.Errorf("offline: unsupported shell construct in apply step")
		}
		if err := checkTokensSha256Strict(st); err != nil {
			return err
		}
	}
	return nil
}

func (t *digestOrderTracker) recordDigestSeqs(stages [][]applyToken, itemSeq int) {
	if t.shaSeq < 0 && len(t.frames) == 0 && isValidShaPipeline(stages) {
		t.shaSeq = itemSeq
	}
	inTopOrSidecar := len(t.frames) == 0 || (len(t.frames) == 1 && t.frames[0].allowSidecar && !t.frames[0].inElse)
	if t.grepSeq < 0 && inTopOrSidecar && isValidGrepPipeline(stages) {
		t.grepSeq = itemSeq
	}
}

func (t *digestOrderTracker) visitItem(items []applyListItem, idx int, allowDigest bool) error {
	item := items[idx]
	stages, err := splitPipelineStages(item.tokens)
	if err != nil {
		return err
	}
	if err := validatePipelineStages(stages); err != nil {
		return err
	}
	t.seq++
	itemSeq := t.seq
	if itemHasTofuApply(stages) && t.applySeq < 0 {
		t.applySeq = itemSeq
	}
	if !itemContainsDigestCmd(item.tokens) {
		return nil
	}
	if err := checkDigestItemMask(items, idx, stages, allowDigest); err != nil {
		return err
	}
	if idx+1 < len(items) && items[idx+1].opBefore == "&&" {
		return nil
	}
	t.recordDigestSeqs(stages, itemSeq)
	return nil
}

func (t *digestOrderTracker) evalChain(chain []applyToken, allowDigest bool) error {
	items, err := splitApplyListItems(chain)
	if err != nil {
		return err
	}
	for idx := range items {
		if err := t.visitItem(items, idx, allowDigest); err != nil {
			return err
		}
	}
	return nil
}

func (t *digestOrderTracker) handleIfControl(chain []applyToken, kw string) error {
	if kw == "if" {
		if t.awaiting {
			return fmt.Errorf("offline: unexpected if before then in apply step")
		}
		cond := chain[1:]
		if err := t.evalChain(cond, false); err != nil {
			return err
		}
		t.frames = append(t.frames, applyIfFrame{
			allowSidecar: len(t.frames) == 0 && rawTokensEqual(cond, "[[", "-f", "/workspace/plan.tfplan.sha256", "]]"),
		})
		t.awaiting = true
		return nil
	}
	if t.awaiting || len(t.frames) == 0 {
		return fmt.Errorf("offline: unexpected elif in apply step")
	}
	if err := t.evalChain(chain[1:], false); err != nil {
		return err
	}
	top := &t.frames[len(t.frames)-1]
	top.allowSidecar = false
	top.inElse = true
	t.awaiting = true
	return nil
}

func (t *digestOrderTracker) stepChain(chain []applyToken) error {
	head := chain[0]
	if !head.isOp {
		switch head.raw {
		case "if", "elif":
			return t.handleIfControl(chain, head.raw)
		case "then":
			if !t.awaiting {
				return fmt.Errorf("offline: unexpected then in apply step")
			}
			t.awaiting = false
			return nil
		case "else":
			if t.awaiting || len(t.frames) == 0 {
				return fmt.Errorf("offline: unexpected else in apply step")
			}
			t.frames[len(t.frames)-1].inElse = true
			return nil
		case "fi":
			if t.awaiting || len(t.frames) == 0 || len(chain) != 1 {
				return fmt.Errorf("offline: unexpected fi in apply step")
			}
			t.frames = t.frames[:len(t.frames)-1]
			return nil
		}
	}
	if t.awaiting {
		return fmt.Errorf("offline: missing then in conditional in apply step")
	}
	return t.evalChain(chain, true)
}

func (t digestOrderTracker) verify() error {
	if t.awaiting || len(t.frames) != 0 {
		return fmt.Errorf("offline: unclosed conditional in apply step")
	}
	if t.shaSeq < 0 {
		return fmt.Errorf("offline: apply step must verify plan digest with sha256sum --strict -c -")
	}
	if t.grepSeq < 0 {
		return fmt.Errorf("offline: apply step must verify plan sidecar checksum with grep against approved digest")
	}
	if t.applySeq >= 0 && (t.shaSeq >= t.applySeq || t.grepSeq >= t.applySeq) {
		return fmt.Errorf("offline: apply step must verify plan digest before tofu apply")
	}
	return nil
}

func checkApprovalDigestVerification(script string) error {
	all, err := buildApplyTokenStream(script)
	if err != nil {
		return err
	}
	chains, err := splitApplyChains(all)
	if err != nil {
		return err
	}
	tracker := digestOrderTracker{shaSeq: -1, grepSeq: -1, applySeq: -1}
	for _, chain := range chains {
		if err := tracker.stepChain(chain); err != nil {
			return err
		}
	}
	return tracker.verify()
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
