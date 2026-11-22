package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"strings"
)

type browserSignIn struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"clientId"`
}

type browserConfiguration struct {
	BFFURL string         `json:"bffUrl"`
	SignIn *browserSignIn `json:"signIn,omitempty"`
}

// webHandler serves the packaged browser on the RPC origin. The empty base URL is relative to
// that origin. Only public sign-in metadata is exposed; credentials and signing keys stay outside.
func webHandler(root string, signIn *browserSignIn) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/config.json" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			if r.Method == http.MethodHead {
				return
			}
			if err := json.NewEncoder(w).Encode(browserConfiguration{SignIn: signIn}); err != nil {
				return
			}
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		// OpenInRoot confines symlinks and traversal to the configured asset root.
		// https://pkg.go.dev/os#OpenInRoot; ServeContent handles MIME/ranges/HEAD:
		// https://pkg.go.dev/net/http#ServeContent
		file, err := os.OpenInRoot(root, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
}
