// Witness for the BFF image (spec 009). It reads the image's own content and trusts no file name:
// each blob is reached from the index, and its digest and size are checked against its bytes.
package image_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	refusal      = "bff: no verifier is configured: name the issuer with --verifier-issuer, --verifier-audience and --verifier-jwks-url (or BFF_VERIFIER_ISSUER, BFF_VERIFIER_AUDIENCE, BFF_VERIFIER_JWKS_URL); this repository ships no identity provider\n"
	manifestType = "application/vnd.oci.image.manifest.v1+json"
	configType   = "application/vnd.oci.image.config.v1+json"
	layerType    = "application/vnd.oci.image.layer.v1.tar"
	epoch        = "1970-01-01T00:00:00Z"
)

type descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type configuration struct {
	Created      string `json:"created"`
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		User         string              `json:"User"`
		Entrypoint   []string            `json:"Entrypoint"`
		Cmd          []string            `json:"Cmd"`
		Env          []string            `json:"Env"`
		ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	} `json:"config"`
	RootFS struct {
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

// image is what a layout holds once every descriptor has been checked.
type image struct {
	config configuration
	layer  descriptor
	files  []*tar.Header
	server []byte
}

// blob reads a descriptor's content and checks it against the descriptor.
func blob(root string, want descriptor, mediaType string) ([]byte, error) {
	if want.MediaType != mediaType {
		return nil, fmt.Errorf("%s: media type %q, want %q", want.Digest, want.MediaType, mediaType)
	}
	hash, found := strings.CutPrefix(want.Digest, "sha256:")
	if !found {
		return nil, fmt.Errorf("digest %q is not sha256", want.Digest)
	}
	data, err := os.ReadFile(filepath.Join(root, "blobs", "sha256", hash))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != hash {
		return nil, fmt.Errorf("%s: content hashes to %s", want.Digest, got)
	}
	if int64(len(data)) != want.Size {
		return nil, fmt.Errorf("%s: %d bytes, descriptor says %d", want.Digest, len(data), want.Size)
	}
	return data, nil
}

func decode(root string, want descriptor, mediaType string, into any) ([]byte, error) {
	data, err := blob(root, want, mediaType)
	if err != nil {
		return nil, err
	}
	return data, json.Unmarshal(data, into)
}

// load follows the layout from its index, and fails on anything that does not check out.
func load(root string) (*image, error) {
	data, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		return nil, err
	}
	var index struct {
		Manifests []descriptor `json:"manifests"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if len(index.Manifests) != 1 {
		return nil, fmt.Errorf("%d manifests, want 1", len(index.Manifests))
	}
	var manifest struct {
		Config descriptor   `json:"config"`
		Layers []descriptor `json:"layers"`
	}
	if _, err := decode(root, index.Manifests[0], manifestType, &manifest); err != nil {
		return nil, err
	}
	loaded := &image{}
	if _, err := decode(root, manifest.Config, configType, &loaded.config); err != nil {
		return nil, err
	}
	if len(manifest.Layers) != 1 {
		return nil, fmt.Errorf("%d layers, want 1", len(manifest.Layers))
	}
	loaded.layer = manifest.Layers[0]
	content, err := blob(root, loaded.layer, layerType)
	if err != nil {
		return nil, err
	}
	reader := tar.NewReader(bytes.NewReader(content))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return loaded, nil
		}
		if err != nil {
			return nil, err
		}
		loaded.files = append(loaded.files, header)
		if header.Name == "bff" || header.Name == "./bff" {
			if loaded.server, err = io.ReadAll(reader); err != nil {
				return nil, err
			}
		}
	}
}

func layout(t *testing.T) string {
	t.Helper()
	return filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("IMAGE"))
}

func built(t *testing.T) *image {
	t.Helper()
	loaded, err := load(layout(t))
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestConfigStatesThePlatformUserAndEntrypoint(t *testing.T) {
	loaded := built(t)
	config := loaded.config
	if config.OS != "linux" || config.Architecture != "amd64" {
		t.Errorf("platform %s/%s", config.OS, config.Architecture)
	}
	if config.Config.User != "65532:65532" {
		t.Errorf("user %q is not the numeric non-root user", config.Config.User)
	}
	if !slices.Equal(config.Config.Entrypoint, []string{"/bff"}) {
		t.Errorf("entrypoint %q", config.Config.Entrypoint)
	}
	if len(config.Config.Cmd)+len(config.Config.Env)+len(config.Config.ExposedPorts) != 0 {
		t.Errorf("config sets more than the contract: %+v", config.Config)
	}
	if config.Created != epoch {
		t.Errorf("created %q", config.Created)
	}
	if !slices.Equal(config.RootFS.DiffIDs, []string{loaded.layer.Digest}) {
		t.Errorf("diff ids %q do not name the one layer %s", config.RootFS.DiffIDs, loaded.layer.Digest)
	}
}

func TestTheLayerHoldsOnlyTheServer(t *testing.T) {
	loaded := built(t)
	if len(loaded.files) != 1 {
		names := make([]string, 0, len(loaded.files))
		for _, header := range loaded.files {
			names = append(names, header.Name)
		}
		t.Fatalf("layer holds %q, want only the server", names)
	}
	file := loaded.files[0]
	if file.Typeflag != tar.TypeReg || file.Uid != 65532 || file.Gid != 65532 || file.Mode != 0o555 || !file.ModTime.Equal(time.Unix(0, 0)) {
		t.Errorf("server entry: type %c, %d:%d, mode %o, time %v", file.Typeflag, file.Uid, file.Gid, file.Mode, file.ModTime)
	}
}

func extracted(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bff")
	if err := os.WriteFile(path, built(t).server, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// staticProblems lists what makes an ELF need a loader. A missing interpreter alone is not enough, so
// the dynamic section and its needed entries are read as well.
func staticProblems(path string) ([]string, error) {
	file, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var problems []string
	if file.Machine != elf.EM_X86_64 {
		problems = append(problems, "machine "+file.Machine.String())
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP || program.Type == elf.PT_DYNAMIC {
			problems = append(problems, "program header "+program.Type.String())
		}
	}
	if needed, err := file.DynString(elf.DT_NEEDED); err == nil && len(needed) != 0 {
		problems = append(problems, fmt.Sprintf("dynamic dependencies %q", needed))
	}
	return problems, nil
}

func TestThePackagedBinaryIsStatic(t *testing.T) {
	problems, err := staticProblems(extracted(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("the packaged binary needs a loader: %v", problems)
	}
}

// The static check must see a dynamic binary. The host's shell is one on the supported Linux profile, so a
// host without it fails this required witness rather than passing without it.
func TestADynamicBinaryIsSeen(t *testing.T) {
	problems, err := staticProblems("/bin/sh")
	if err != nil {
		t.Fatal("the supported profile has a dynamic /bin/sh to read:", err)
	}
	if !slices.ContainsFunc(problems, func(p string) bool { return strings.HasPrefix(p, "program header PT_INTERP") }) ||
		!slices.ContainsFunc(problems, func(p string) bool { return strings.HasPrefix(p, "dynamic dependencies") }) {
		t.Errorf("a dynamic binary was not seen as one: %v", problems)
	}
}

func TestThePackagedBinaryRefusesToStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, extracted(t))
	command.Env = []string{}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("got %v, want exit 1", err)
	}
	if stderr.String() != refusal {
		t.Errorf("stderr %q, want %q", stderr.String(), refusal)
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused server printed %q, and its ready event is the only thing it prints", stdout.String())
	}
}

// ownedCopy writes a layout's bytes into new regular files under dir. Runfiles are links into Bazel's
// inputs, read-only under the sandbox, so a copy that kept links would mutate those inputs or fail.
func ownedCopy(t *testing.T, source, dir string) {
	t.Helper()
	err := filepath.WalkDir(source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, relative)
		info, err := os.Stat(name) // follows a link to what it names
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// corruptions are the ways a layer blob can stop matching its descriptor.
var corruptions = map[string]func([]byte) []byte{
	"a changed byte": func(data []byte) []byte { data[0] ^= 1; return data },
	"added bytes":    func(data []byte) []byte { return append(data, 0) },
}

func corrupt(t *testing.T, source string, change func([]byte) []byte) {
	t.Helper()
	loaded, err := load(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ownedCopy(t, source, root)
	path := filepath.Join(root, "blobs", "sha256", strings.TrimPrefix(loaded.layer.Digest, "sha256:"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, change(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(root); err == nil {
		t.Fatal("a layer that does not match its digest was accepted")
	}
}

// The witness must not pass on a layout that lies about its content.
func TestALayoutThatDoesNotMatchItsDescriptorsIsRefused(t *testing.T) {
	for name, change := range corruptions {
		t.Run(name, func(t *testing.T) { corrupt(t, layout(t), change) })
	}
}

// A layout reached through links into a read-only directory, as under the sandbox: the corruption lands on
// the copy, the source stays as it was, and the refusal still holds.
func TestCorruptionNeverReachesALinkedReadOnlySource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	ownedCopy(t, layout(t), source)
	before := sha256Dir(t, source)
	linked := filepath.Join(t.TempDir(), "linked")
	err := filepath.WalkDir(source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(source, name)
		target := filepath.Join(linked, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return os.Symlink(name, target)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(name, 0o555)
		}
		return os.Chmod(name, 0o444)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(source, func(name string, entry fs.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(name, 0o755)
			}
			return nil
		})
	})
	for name, change := range corruptions {
		t.Run(name, func(t *testing.T) { corrupt(t, linked, change) })
	}
	if after := sha256Dir(t, source); after != before {
		t.Fatal("the read-only source changed under the corruption test")
	}
}

// sha256Dir hashes every regular file's bytes under dir, by relative path.
func sha256Dir(t *testing.T, dir string) string {
	t.Helper()
	sum := sha256.New()
	err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(dir, name)
		sum.Write([]byte(relative))
		sum.Write(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sum.Sum(nil))
}
