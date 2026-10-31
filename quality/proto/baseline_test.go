package baseline

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const schemaFile = "api/template/v1/a.proto"

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) repo {
	t.Helper()
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "test")
	r.git("config", "user.email", "test@example.com")
	return r
}

func (r repo) git(args ...string) string {
	r.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", r.dir}, args...)...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r repo) write(path, text string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r repo) commit(message string) string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", message)
	return r.git("rev-parse", "HEAD")
}

func (r repo) read(t *testing.T) (Result, string, error) {
	t.Helper()
	out := t.TempDir()
	reader := Reader{Repo: r.dir, Marker: "api/BASELINE", Schema: "api"}
	result, err := reader.Materialize(out)
	return result, out, err
}

func requireUnavailable(t *testing.T, err error, want string) {
	t.Helper()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("want a message containing %q, got %v", want, err)
	}
}

func TestReadsTheDeclaredRevisionNotTheWorkingTree(t *testing.T) {
	r := newRepo(t)
	r.write(schemaFile, "first\n")
	r.write("api/BASELINE", "bootstrap\n")
	first := r.commit("schema")
	r.write("api/BASELINE", first+"\n")
	r.write(schemaFile, "second\n")
	r.commit("edit")
	r.write(schemaFile, "uncommitted\n")

	result, out, err := r.read(t)
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != first || result.Files != 1 || result.Bootstrap {
		t.Fatalf("unexpected result %+v", result)
	}
	got, err := os.ReadFile(filepath.Join(out, "template/v1/a.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first\n" {
		t.Fatalf("read %q from the wrong revision", got)
	}
}

func TestOnlyProtoFilesAreRead(t *testing.T) {
	r := newRepo(t)
	r.write(schemaFile, "first\n")
	r.write("api/notes.txt", "not a schema\n")
	first := r.commit("schema")
	r.write("api/BASELINE", first+"\n")
	r.commit("marker")
	result, out, err := r.read(t)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 {
		t.Fatalf("want one schema file, got %d", result.Files)
	}
	if _, err := os.Stat(filepath.Join(out, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("a non-schema file was written: %v", err)
	}
}

func TestFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		marker func(r repo, schema string) string
		want   string
	}{
		{"branch name", func(repo, string) string { return "main\n" }, "must hold a full"},
		{"short hash", func(_ repo, schema string) string { return schema[:12] + "\n" }, "must hold a full"},
		{"uppercase hash", func(_ repo, schema string) string { return strings.ToUpper(schema) + "\n" }, "must hold a full"},
		{"empty", func(repo, string) string { return "" }, "must hold a full"},
		{"two lines", func(_ repo, schema string) string { return schema + "\n" + schema + "\n" }, "must hold a full"},
		{"unknown commit", func(repo, string) string { return strings.Repeat("a", 40) + "\n" }, "not in this clone"},
		{"commit outside history", func(r repo, _ string) string {
			head := r.git("rev-parse", "HEAD")
			r.git("checkout", "-q", "--orphan", "other")
			other := r.commit("unrelated")
			r.git("checkout", "-q", "-f", head)
			return other + "\n"
		}, "not an ancestor"},
		{"commit without a schema", func(r repo, _ string) string {
			return r.git("rev-list", "--max-parents=0", "HEAD") + "\n"
		}, "no schema under"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("README", "no schema yet\n")
			r.commit("root")
			r.write(schemaFile, "first\n")
			schema := r.commit("schema")
			r.write("api/BASELINE", c.marker(r, schema))
			_, _, err := r.read(t)
			requireUnavailable(t, err, c.want)
		})
	}
}

func TestMissingMarkerFailsClosed(t *testing.T) {
	r := newRepo(t)
	r.write(schemaFile, "first\n")
	r.commit("schema")
	_, _, err := r.read(t)
	requireUnavailable(t, err, "cannot be read")
}

func TestShallowCloneNamesTheFix(t *testing.T) {
	origin := newRepo(t)
	origin.write(schemaFile, "first\n")
	first := origin.commit("schema")
	origin.write("api/BASELINE", first+"\n")
	origin.commit("marker")

	clone := repo{t: t, dir: filepath.Join(t.TempDir(), "clone")}
	if out, err := exec.Command("git", "clone", "-q", "--depth=1", "file://"+origin.dir, clone.dir).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v: %s", err, out)
	}
	_, _, err := clone.read(t)
	requireUnavailable(t, err, "git fetch --unshallow")
}

func TestBootstrapMarker(t *testing.T) {
	t.Run("before the first commit", func(t *testing.T) {
		r := newRepo(t)
		r.write(schemaFile, "first\n")
		r.write("api/BASELINE", "bootstrap\n")
		result, _, err := r.read(t)
		if err != nil || !result.Bootstrap {
			t.Fatalf("want a bootstrap result, got %+v, %v", result, err)
		}
	})
	t.Run("in the root commit", func(t *testing.T) {
		r := newRepo(t)
		r.write(schemaFile, "first\n")
		r.write("api/BASELINE", "bootstrap\n")
		r.commit("schema")
		result, _, err := r.read(t)
		if err != nil || !result.Bootstrap {
			t.Fatalf("want a bootstrap result, got %+v, %v", result, err)
		}
	})
	t.Run("in the commit that adds the schema", func(t *testing.T) {
		r := newRepo(t)
		r.write("README", "no schema yet\n")
		r.commit("root")
		r.write(schemaFile, "first\n")
		r.write("api/BASELINE", "bootstrap\n")
		r.commit("schema")
		result, _, err := r.read(t)
		if err != nil || !result.Bootstrap {
			t.Fatalf("want a bootstrap result, got %+v, %v", result, err)
		}
	})
	t.Run("refused once a parent has a schema", func(t *testing.T) {
		r := newRepo(t)
		r.write(schemaFile, "first\n")
		r.write("api/BASELINE", "bootstrap\n")
		first := r.commit("schema")
		r.write(schemaFile, "second\n")
		r.commit("edit")
		_, _, err := r.read(t)
		requireUnavailable(t, err, first)
	})
	t.Run("refused after the schema was deleted", func(t *testing.T) {
		r := newRepo(t)
		r.write(schemaFile, "first\n")
		r.commit("schema")
		r.git("rm", "-q", schemaFile)
		r.commit("delete")
		r.write(schemaFile, "replacement\n")
		r.write("api/BASELINE", "bootstrap\n")
		_, _, err := r.read(t)
		requireUnavailable(t, err, "already has a schema")
	})
}

// A copy of a template has new history, so the hash it inherited names no commit in it. These are
// the two routes an adopter has; api/README.md documents them.
func TestFreshHistoryCopy(t *testing.T) {
	upstream := newRepo(t)
	upstream.write(schemaFile, "first\n")
	inherited := upstream.commit("schema in the template")

	t.Run("the inherited hash fails closed", func(t *testing.T) {
		r := newRepo(t)
		r.write(schemaFile, "first\n")
		r.write("api/BASELINE", inherited+"\n")
		r.commit("copy of the template")
		_, _, err := r.read(t)
		requireUnavailable(t, err, "not in this clone")
	})
	t.Run("a copy with a schema in its first commit declares that commit", func(t *testing.T) {
		r := newRepo(t)
		r.write(schemaFile, "first\n")
		r.write("api/BASELINE", inherited+"\n")
		first := r.commit("copy of the template")

		r.write("api/BASELINE", "bootstrap\n")
		r.commit("try the marker")
		_, _, err := r.read(t)
		requireUnavailable(t, err, "already has a schema")

		r.write("api/BASELINE", first+"\n")
		r.commit("declare the baseline")
		result, out, err := r.read(t)
		if err != nil || result.Revision != first {
			t.Fatalf("want a comparison with %s, got %+v, %v", first, result, err)
		}
		if _, err := os.Stat(filepath.Join(out, "template/v1/a.proto")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a copy that adds its schema later bootstraps in that commit", func(t *testing.T) {
		r := newRepo(t)
		r.write("README", "a copy with the example removed\n")
		r.commit("copy of the template")
		r.write(schemaFile, "our first schema\n")
		r.write("api/BASELINE", "bootstrap\n")
		schema := r.commit("add our schema")
		result, _, err := r.read(t)
		if err != nil || !result.Bootstrap {
			t.Fatalf("want a bootstrap result, got %+v, %v", result, err)
		}
		r.write("api/BASELINE", schema+"\n")
		r.commit("declare the baseline")
		result, _, err = r.read(t)
		if err != nil || result.Revision != schema {
			t.Fatalf("want a comparison with %s, got %+v, %v", schema, result, err)
		}
	})
}
