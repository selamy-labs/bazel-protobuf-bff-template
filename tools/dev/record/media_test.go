// The recording's witness (spec 012 FR-024): the GIF in the tree is a real animation of the stated size, its
// record names the revision and the frames, and the two agree byte for byte. It cannot prove the frames show the
// real stack; the recorder does that by construction, and the record names the command.
package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/gif"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type record struct {
	Label    string   `json:"label"`
	Revision string   `json:"revision"`
	Command  string   `json:"command"`
	Frames   []string `json:"frames"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Bytes    int      `json:"bytes"`
	SHA256   string   `json:"sha256"`
	Encoder  string   `json:"encoder"`
}

func TestTheRecordingAndItsRecordAgree(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("GIF")))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("RECORD")))
	if err != nil {
		t.Fatal(err)
	}
	var r record
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Label, "LOCAL demo") || strings.Contains(strings.ToLower(r.Label), "hosted demo") {
		t.Errorf("the label must say it is the local demo and claim nothing hosted: %q", r.Label)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(r.Revision) {
		t.Errorf("the record names no full revision: %q", r.Revision)
	}
	if r.Command != "tools/dev/record.sh" {
		t.Errorf("the record names another command: %q", r.Command)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != r.SHA256 || len(data) != r.Bytes {
		t.Errorf("the GIF (%d bytes, %s) is not the one the record describes (%d bytes, %s)", len(data), hex.EncodeToString(sum[:]), r.Bytes, r.SHA256)
	}
	if len(data) > 1_500_000 {
		t.Errorf("the GIF is %d bytes, over the bound", len(data))
	}
	animation, err := gif.DecodeAll(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(animation.Image) != len(r.Frames) || len(animation.Image) < 2 {
		t.Errorf("the GIF holds %d frames, the record names %d", len(animation.Image), len(r.Frames))
	}
	if animation.Config.Width != r.Width || animation.Config.Height != r.Height {
		t.Errorf("the GIF is %dx%d, the record says %dx%d", animation.Config.Width, animation.Config.Height, r.Width, r.Height)
	}
	if animation.LoopCount != 0 {
		t.Errorf("the GIF does not loop: %d", animation.LoopCount)
	}
	for i, want := range []string{"01-open", "02-sign-in", "03-typed", "04-created", "05-reloaded", "06-name-typed", "07-opened"} {
		if i >= len(r.Frames) || r.Frames[i] != want {
			t.Errorf("frame %d is %v, want %s: the journey is sign-in, create, reload, open", i, r.Frames, want)
			break
		}
	}
}
