package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun(t *testing.T) {
	capability := writeManifest(t, "type: capability\n")
	cases := []struct {
		name      string
		args      []string
		wantCode  int
		wantFirst string
		mentions  string
	}{
		{"a table repository prints its layer and the table as source", []string{"--repository", "mitstack-eu/talos-cluster-kit"}, 0, "kit", "layer table"},
		{"a capability's manifest prints capability and the manifest as source", []string{"--repository", "mitstack-eu/hcloud-docker-host", "--manifest", capability}, 0, "capability", "manifest.yaml says type: capability"},
		{"an unplaced repository exits one naming it", []string{"--repository", "mitstack-eu/unlisted-tool"}, 1, "unplaced", "mitstack-eu/unlisted-tool"},
		{"an unreadable manifest exits one naming the file", []string{"--repository", "mitstack-eu/x", "--manifest", "/nonexistent/manifest.yaml"}, 1, "repo-layer", "/nonexistent/manifest.yaml"},
		{"a missing --repository exits two", nil, 2, "repo-layer", "--repository is required"},
		{"a repository without an owner exits two", []string{"--repository", "block-kit"}, 2, "repo-layer", "block-kit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			code := Run(tc.args, &out)

			first, _, _ := strings.Cut(out.String(), ":")
			if code != tc.wantCode || first != tc.wantFirst || !strings.Contains(out.String(), tc.mentions) {
				t.Errorf("Run(%q) = %d, %q; want %d, a line starting %q naming %q", tc.args, code, out.String(), tc.wantCode, tc.wantFirst, tc.mentions)
			}
		})
	}
}
