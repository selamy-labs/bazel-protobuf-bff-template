package inputs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Provenance names, for each leaf of the schema by its dotted path, the file that gave its effective value.
type Provenance map[string]string

// Load reads and validates one inputs file: Resolve with no overlay.
func Load(path string) (*Inputs, error) {
	in, _, _, err := Resolve(path)
	return in, err
}

// Resolve reads the base file and the overlays in order and merges them leaf by leaf: a later file wins. Each
// file may hold any subset of the schema's leaves, in the schema's shape; a key outside the schema, a section
// given as a value or a value given as a section is refused naming the file and the key. A leaf given by no
// file is refused by name: nothing is defaulted here. The merged document is validated as one file would be,
// and returned with the provenance of every leaf and its canonical JSON, which is what a plan reads.
func Resolve(base string, overlays ...string) (*Inputs, Provenance, []byte, error) {
	leaves, sections, nullable := shape()
	values, provenance, err := collectDocuments(append([]string{base}, overlays...), leaves, sections)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := validateLeaves(leaves, nullable, values, provenance); err != nil {
		return nil, nil, nil, err
	}
	in, effective, err := decodeInputs(values, base)
	if err != nil {
		return nil, nil, nil, err
	}
	return in, provenance, effective, nil
}

func collectDocuments(paths []string, leaves, sections map[string]bool) (map[string]any, Provenance, error) {
	values := map[string]any{}
	provenance := Provenance{}
	for _, path := range paths {
		document, err := readDocument(path)
		if err != nil {
			return nil, nil, err
		}
		if err := collect(filepath.Base(path), "", document, leaves, sections, values, provenance); err != nil {
			return nil, nil, err
		}
	}
	return values, provenance, nil
}

func validateLeaves(leaves, nullable map[string]bool, values map[string]any, provenance Provenance) error {
	for _, leaf := range sortedKeys(leaves) {
		value, ok := values[leaf]
		if !ok {
			return fmt.Errorf("%s: given by no file, and nothing is defaulted here", leaf)
		}
		// A struct decode reads null as a zero value, and the effective document would keep the null.
		if value == nil && !nullable[leaf] {
			return fmt.Errorf("%s: null, given by %s, is not a value here", leaf, provenance[leaf])
		}
	}
	return nil
}

func decodeInputs(values map[string]any, base string) (*Inputs, []byte, error) {
	// Values that came out of a JSON decode always marshal.
	effective, _ := json.MarshalIndent(nest(values), "", "  ")
	decoder := json.NewDecoder(bytes.NewReader(effective))
	decoder.DisallowUnknownFields()
	var in Inputs
	if err := decoder.Decode(&in); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", filepath.Base(base), err)
	}
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}
	return &in, append(effective, '\n'), nil
}

// readDocument reads one file as the generic JSON shape, so that the presence of each key can be seen: a struct
// decode cannot tell an omitted leaf from its zero value, nor say which file gave a value.
func readDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return document, nil
}

// shape derives the schema's leaves and sections from the struct, which the schema test keeps equal to the
// schema: a leaf is a field that is not a nested struct, named by its dotted path. A pointer field is the schema's
// nullable leaf, and no other leaf may be null.
func shape() (leaves, sections, nullable map[string]bool) {
	leaves, sections, nullable = map[string]bool{}, map[string]bool{}, map[string]bool{}
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			path := prefix + field.Tag.Get("json")
			if field.Type.Kind() == reflect.Struct {
				sections[path] = true
				walk(field.Type, path+".")
			} else {
				leaves[path] = true
				nullable[path] = field.Type.Kind() == reflect.Pointer
			}
		}
	}
	walk(reflect.TypeOf(Inputs{}), "")
	return leaves, sections, nullable
}

// collect walks one document against the shape, recording each leaf's value and the file that gave it.
func collect(file, prefix string, node map[string]any, leaves, sections map[string]bool, values map[string]any, provenance Provenance) error {
	for _, key := range sortedKeys(node) {
		path := prefix + key
		value := node[key]
		child, isSection := value.(map[string]any)
		switch {
		case sections[path] && isSection:
			if err := collect(file, path+".", child, leaves, sections, values, provenance); err != nil {
				return err
			}
		case sections[path]:
			return fmt.Errorf("%s: %s: is a section of the schema, not a value", file, path)
		case leaves[path] && isSection:
			return fmt.Errorf("%s: %s: is a value of the schema, not a section", file, path)
		case leaves[path]:
			values[path] = value
			provenance[path] = file
		default:
			return fmt.Errorf("%s: %s: not in the schema", file, path)
		}
	}
	return nil
}

// nest rebuilds the document from the dotted leaf paths.
func nest(values map[string]any) map[string]any {
	document := map[string]any{}
	for path, value := range values {
		node := document
		parts := strings.Split(path, ".")
		for _, part := range parts[:len(parts)-1] {
			next, ok := node[part].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[part] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = value
	}
	return document
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Render prints every effective leaf with its source, one `path=value # from file` line each, in path order.
func Render(values []byte, provenance Provenance) ([]string, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(values))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	flat := map[string]any{}
	flatten("", document, flat)
	var lines []string
	for _, path := range sortedKeys(flat) {
		text, _ := json.Marshal(flat[path]) // decoded from JSON, so it marshals
		lines = append(lines, fmt.Sprintf("%s=%s # from %s", path, text, provenance[path]))
	}
	return lines, nil
}

func flatten(prefix string, node map[string]any, into map[string]any) {
	for key, value := range node {
		if child, ok := value.(map[string]any); ok {
			flatten(prefix+key+".", child, into)
		} else {
			into[prefix+key] = value
		}
	}
}
