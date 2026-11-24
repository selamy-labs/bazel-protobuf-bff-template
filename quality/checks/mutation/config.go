// Package mutation enforces Go mutation-testing floors and reviewed equivalent-mutant proofs
// (spec 011 FR-004, FR-010, T008, T013; issue #10).
package mutation

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// SourceMapping binds one repository-relative source directory to its unique mirror destination.
type SourceMapping struct {
	Source string
	Dest   string
}

func (m SourceMapping) String() string {
	return m.Source + "->" + m.Dest
}

var locationPattern = regexp.MustCompile(`^[^:\s]+:[1-9][0-9]*:[1-9][0-9]*$`)

// PackageConfig declares one Go package evaluated by offline mutation testing.
type PackageConfig struct {
	Name        string   `json:"name"`
	Module      string   `json:"module"`
	GoVersion   string   `json:"go_version"`
	Package     string   `json:"package"`
	StripPrefix string   `json:"strip_prefix,omitempty"`
	Sources     []string `json:"sources"`
	Floor       float64  `json:"floor"`
}

// Floors is the versioned policy input declaring enrolled packages and ratcheting floors.
type Floors struct {
	MeasuredAt         string          `json:"measured_at"`
	EquivalentsPath    string          `json:"equivalents"`
	TimeoutCoefficient int             `json:"timeout_coefficient"`
	Packages           []PackageConfig `json:"packages"`
}

// Equivalent records one reviewed, proven equivalent surviving mutant.
type Equivalent struct {
	ID       string `json:"id"`
	Package  string `json:"package"`
	Location string `json:"location"`
	Mutator  string `json:"mutator"`
	Proof    string `json:"proof"`
}

// Key uniquely identifies a mutant site within a package.
func (e Equivalent) Key() string {
	return e.Package + "|" + e.Location + "|" + e.Mutator
}

// EquivalentsFile is the reviewed equivalents ledger referenced by Floors.
type EquivalentsFile struct {
	Equivalents []Equivalent `json:"equivalents"`
}

// LoadFloors reads and validates a mutation floors JSON file.
func LoadFloors(path string) (*Floors, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	floors := &Floors{}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(floors); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := floors.validate(path); err != nil {
		return nil, err
	}
	return floors, nil
}

func isSafeRelPath(p string) bool {
	if strings.TrimSpace(p) == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return false
	}
	cleaned := path.Clean(filepath.ToSlash(p))
	return cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, "../") && !strings.HasPrefix(cleaned, "/")
}

func normalizeStripPrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}
	trimmed := strings.TrimSuffix(filepath.ToSlash(prefix), "/")
	if !isSafeRelPath(trimmed) {
		return "", fmt.Errorf("strip_prefix %q must be a clean relative path", prefix)
	}
	return path.Clean(trimmed) + "/", nil
}

// CanonicalMappings validates and returns the sorted, unique source-to-mirror directory mappings.
func (pkg PackageConfig) CanonicalMappings() ([]SourceMapping, error) {
	prefix, err := normalizeStripPrefix(pkg.StripPrefix)
	if err != nil {
		return nil, err
	}
	seenSrc := map[string]bool{}
	seenDst := map[string]bool{}
	mappings := make([]SourceMapping, 0, len(pkg.Sources))
	for _, rawSrc := range pkg.Sources {
		if !isSafeRelPath(rawSrc) {
			return nil, fmt.Errorf("source %q must be a clean relative path", rawSrc)
		}
		src := path.Clean(filepath.ToSlash(rawSrc))
		if seenSrc[src] {
			return nil, fmt.Errorf("duplicate source %q", src)
		}
		dst := src
		if prefix != "" && strings.HasPrefix(src, prefix) {
			dst = strings.TrimPrefix(src, prefix)
		}
		if !isSafeRelPath(dst) {
			return nil, fmt.Errorf("source %q maps to invalid mirror destination %q", rawSrc, dst)
		}
		dst = path.Clean(dst)
		if seenDst[dst] {
			return nil, fmt.Errorf("duplicate mirror destination %q", dst)
		}
		seenSrc[src] = true
		seenDst[dst] = true
		mappings = append(mappings, SourceMapping{Source: src, Dest: dst})
	}
	slices.SortFunc(mappings, func(a, b SourceMapping) int {
		if a.Dest != b.Dest {
			return strings.Compare(a.Dest, b.Dest)
		}
		return strings.Compare(a.Source, b.Source)
	})
	return mappings, nil
}

func (f *Floors) validate(path string) error {
	if strings.TrimSpace(f.MeasuredAt) == "" {
		return fmt.Errorf("%s: measured_at is required", path)
	}
	if !isSafeRelPath(f.EquivalentsPath) {
		return fmt.Errorf("%s: equivalents path %q must be a clean relative path", path, f.EquivalentsPath)
	}
	if f.TimeoutCoefficient < 1 {
		return fmt.Errorf("%s: timeout_coefficient must be >= 1", path)
	}
	if len(f.Packages) == 0 {
		return fmt.Errorf("%s: at least one package must be enrolled", path)
	}
	return f.validatePackages(path)
}

func (f *Floors) validatePackages(path string) error {
	seen := map[string]bool{}
	for _, pkg := range f.Packages {
		if err := validatePackage(path, pkg, seen); err != nil {
			return err
		}
		seen[pkg.Name] = true
	}
	return nil
}

func validatePackage(path string, pkg PackageConfig, seen map[string]bool) error {
	if strings.TrimSpace(pkg.Name) == "" {
		return fmt.Errorf("%s: package name is required", path)
	}
	if seen[pkg.Name] {
		return fmt.Errorf("%s: package %q is listed twice", path, pkg.Name)
	}
	if strings.TrimSpace(pkg.Module) == "" || strings.TrimSpace(pkg.GoVersion) == "" || strings.TrimSpace(pkg.Package) == "" {
		return fmt.Errorf("%s: package %q requires module, go_version, and package", path, pkg.Name)
	}
	if pkg.Package != "." && (!strings.HasPrefix(pkg.Package, "./") || !isSafeRelPath(strings.TrimPrefix(pkg.Package, "./"))) {
		return fmt.Errorf("%s: package %q target %q must be '.' or a clean relative './...' path", path, pkg.Name, pkg.Package)
	}
	if len(pkg.Sources) == 0 {
		return fmt.Errorf("%s: package %q requires at least one source directory", path, pkg.Name)
	}
	if _, err := pkg.CanonicalMappings(); err != nil {
		return fmt.Errorf("%s: package %q: %w", path, pkg.Name, err)
	}
	if pkg.Floor < 0 || pkg.Floor > 100 {
		return fmt.Errorf("%s: package %q floor %.2f must be between 0 and 100", path, pkg.Name, pkg.Floor)
	}
	return nil
}

// LoadEquivalents reads and validates the reviewed equivalents ledger against enrolled packages.
func LoadEquivalents(path string, floors *Floors) (*EquivalentsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file := &EquivalentsFile{}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := file.validate(path, floors); err != nil {
		return nil, err
	}
	return file, nil
}

func (e *EquivalentsFile) validate(path string, floors *Floors) error {
	enrolled := map[string]bool{}
	for _, pkg := range floors.Packages {
		enrolled[pkg.Name] = true
	}
	seenIDs := map[string]bool{}
	seenKeys := map[string]bool{}
	for _, entry := range e.Equivalents {
		if err := validateEquivalent(path, entry, enrolled, seenIDs, seenKeys); err != nil {
			return err
		}
		seenIDs[entry.ID] = true
		seenKeys[entry.Key()] = true
	}
	return nil
}

func validateEquivalent(path string, entry Equivalent, enrolled, seenIDs, seenKeys map[string]bool) error {
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Mutator) == "" || strings.TrimSpace(entry.Proof) == "" {
		return fmt.Errorf("%s: equivalent entry requires non-empty id, mutator, and proof", path)
	}
	if !enrolled[entry.Package] {
		return fmt.Errorf("%s: equivalent %q references unenrolled package %q", path, entry.ID, entry.Package)
	}
	if !locationPattern.MatchString(entry.Location) {
		return fmt.Errorf("%s: equivalent %q location %q must be file:line:col", path, entry.ID, entry.Location)
	}
	if seenIDs[entry.ID] {
		return fmt.Errorf("%s: equivalent id %q is listed twice", path, entry.ID)
	}
	if seenKeys[entry.Key()] {
		return fmt.Errorf("%s: equivalent %q duplicates site %s", path, entry.ID, entry.Key())
	}
	return nil
}

// ResolveEquivalentsPath returns the absolute path to the equivalents ledger inside root.
func ResolveEquivalentsPath(root string, floors *Floors) string {
	return filepath.Join(root, floors.EquivalentsPath)
}

// Compare judges proposal mutation floors against a trusted base. Removing an enrolled package,
// lowering a package floor, or altering an existing package's target identity is refused.
func Compare(base, proposal *Floors) []string {
	var problems []string
	proposalByName := map[string]PackageConfig{}
	for _, pkg := range proposal.Packages {
		proposalByName[pkg.Name] = pkg
	}
	for _, want := range base.Packages {
		have, ok := proposalByName[want.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: enrolled package was removed", want.Name))
			continue
		}
		if have.Module != want.Module || have.Package != want.Package {
			problems = append(problems, fmt.Sprintf("%s: package target changed from %s (%s) to %s (%s)",
				want.Name, want.Module, want.Package, have.Module, have.Package))
		}
		if msg := compareSourceMappings(want, have); msg != "" {
			problems = append(problems, msg)
		}
		if have.Floor < want.Floor {
			problems = append(problems, fmt.Sprintf("%s: floor lowered from %.1f%% to %.1f%%",
				want.Name, want.Floor, have.Floor))
		}
	}
	return problems
}

func compareSourceMappings(want, have PackageConfig) string {
	wantMaps, wantErr := want.CanonicalMappings()
	haveMaps, haveErr := have.CanonicalMappings()
	if wantErr != nil || haveErr != nil {
		return fmt.Sprintf("%s: invalid source mapping (%v / %v)", want.Name, wantErr, haveErr)
	}
	if slices.Equal(wantMaps, haveMaps) {
		return ""
	}
	return fmt.Sprintf("%s: source mapping changed from %s to %s",
		want.Name, formatMappings(wantMaps), formatMappings(haveMaps))
}

func formatMappings(mappings []SourceMapping) string {
	parts := make([]string, len(mappings))
	for i, m := range mappings {
		parts[i] = m.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
