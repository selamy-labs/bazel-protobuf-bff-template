package inputs

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSameOriginImageProfile(t *testing.T) {
	name := findTestFile("SAME_ORIGIN_EXAMPLE", "same-origin.inputs.json", "deploy/gcp/inputs/same-origin.inputs.json")
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	in, err := Load(write(t, string(data)))
	if err != nil {
		t.Fatalf("same-origin image profile refused: %v", err)
	}
	if in.Frontend.Host != "same-origin" || in.Frontend.BasePath != "/" || in.Frontend.RPCBaseURL != in.Runtime.AllowedOrigin {
		t.Fatal("profile does not bind one root origin")
	}
	for _, c := range []struct{ name, path, rpc, want string }{
		{"foreign RPC origin", "/", "https://other.example.invalid", "frontend.rpc_base_url"},
		{"separate frontend mount", "/separate/", in.Runtime.AllowedOrigin, "frontend.base_path"},
	} {
		t.Run(c.name, func(t *testing.T) {
			changed := *in
			changed.Frontend.BasePath = c.path
			changed.Frontend.RPCBaseURL = c.rpc
			raw, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Load(write(t, string(raw)))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %s refusal, got %v", c.want, err)
			}
		})
	}
}
