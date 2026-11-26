// Package layer_test pins which layer each repository belongs to and the source that decides it.
package layer_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mitstack-eu/plain-delivery/layer"
)

func TestOf_OwnerRuledRepositoriesTakeTheirTableLayer(t *testing.T) {
	ruled := map[string]layer.Name{
		"mitstack": layer.Core, "mitstack-plugins": layer.Core,
		"block-kit": layer.Kit, "opentofu-operator": layer.Kit, "talos-cluster-kit": layer.Kit,
		"k3s-cluster-kit": layer.Kit, "cluster-bases": layer.Kit, "operator-kit": layer.Kit,
		"operator-kit-archive": layer.Kit, "operator-sidecar": layer.Kit,
		"mitstack-workspace": layer.Internal, "plain": layer.Internal,
		"plain-delivery": layer.Internal, "home-infra": layer.Internal,
		"home": layer.Internal, "webshop-starter": layer.Internal, "alpha-site-preview": layer.Internal,
		"mitstack-home-gitops": layer.Internal, "management": layer.Internal,
		"plain-ci": layer.Internal, "plain-cd": layer.Internal, "home-ci": layer.Internal,
		"scratch-proof-1118b-ci": layer.Internal, "mitstack-home-cluster": layer.Internal,
		"cost-model": layer.Internal, "workplace-shop": layer.Internal, "workplace-desk": layer.Internal,
	}
	for name, want := range ruled {
		repo := "mitstack-eu/" + name

		got, err := layer.Of(repo, nil)

		if err != nil || got.Layer != want || !strings.Contains(got.Source, "layer table") {
			t.Errorf("Of(%q, nil) = %+v, %v; want %s from the layer table", repo, got, err, want)
		}
	}
}

func TestOf_ManifestTypeLine(t *testing.T) {
	cases := []struct {
		name, manifest string
		want           layer.Name
	}{
		{"a builder's block takes block from its manifest", "name: managed-ci\ntype: block\n", layer.Block},
		{"a capability block takes capability from its manifest", "name: hcloud-docker-host\ntype: capability\n", layer.Capability},
		{"a block whose nested type hosts kubernetes stays a block", "spec:\n  hosts: kubernetes\n  type: capability\ntype: block\n", layer.Block},
		{"a quoted type with a trailing comment is read", "type: \"capability\" # serves the whole org\n", layer.Capability},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := layer.Of("mitstack-eu/some-operator", []byte(tc.manifest))

			if err != nil || got.Layer != tc.want || !strings.Contains(got.Source, "manifest.yaml says type: "+string(tc.want)) {
				t.Errorf("Of(manifest %q) = %+v, %v; want %s from the manifest", tc.manifest, got, err, tc.want)
			}
		})
	}
}

func TestOf_Refusals(t *testing.T) {
	cases := []struct {
		name, repo string
		manifest   []byte
		want       error
		mentions   []string
	}{
		{"an unknown repository with no manifest is unplaced", "mitstack-eu/unlisted-tool", nil, layer.ErrUnplaced, []string{"mitstack-eu/unlisted-tool", "no manifest.yaml"}},
		{"a manifest type that is neither block nor capability is unplaced", "mitstack-eu/odd", []byte("type: app\n"), layer.ErrUnplaced, []string{"type: app"}},
		{"a manifest with no top-level type line is unplaced", "mitstack-eu/odd", []byte("spec:\n  type: block\n"), layer.ErrUnplaced, []string{"no top-level type: line"}},
		{"a table repository whose manifest names a block type is unplaced", "mitstack-eu/block-kit", []byte("type: block\n"), layer.ErrUnplaced, []string{"kit", "type: block"}},
		{"another owner's repository named like a table row reads only its manifest", "someone/block-kit", nil, layer.ErrUnplaced, []string{"someone/block-kit"}},
		{"a name without an owner is refused", "block-kit", nil, layer.ErrRepositoryName, []string{"block-kit"}},
		{"a name with two slashes is refused", "a/b/c", nil, layer.ErrRepositoryName, []string{"a/b/c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := layer.Of(tc.repo, tc.manifest)

			if !errors.Is(err, tc.want) {
				t.Fatalf("Of(%q, %q) = %+v, %v; want %v", tc.repo, tc.manifest, got, err, tc.want)
			}
			for _, m := range tc.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("Of(%q) error %q; want it to name %q", tc.repo, err, m)
				}
			}
		})
	}
}
