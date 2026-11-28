// Package layer, part of mitstack's own tooling, names the layer a repository belongs to, from one table of mitstack's own repositories and a block's manifest type line.
package layer

import (
	"errors"
	"fmt"
	"strings"
)

// Name is one of the five values a task's layer field takes.
type Name string

const (
	Core       Name = "core"
	Kit        Name = "kit"
	Block      Name = "block"
	Capability Name = "capability"
	Internal   Name = "internal"
)

// A table row binds only this owner's repository, so another owner's repository of the same name reads its manifest.
const owner = "mitstack-eu"

// A block names its own layer in its manifest, so the table lists only the core, kit and internal repositories.
var repositories = map[string]Name{
	"mitstack":              Core,
	"mitstack-plugins":      Core,
	"block-kit":             Kit,
	"opentofu-operator":     Kit,
	"talos-cluster-kit":     Kit,
	"k3s-cluster-kit":       Kit,
	"cluster-bases":         Kit,
	"operator-kit":          Kit,
	"operator-kit-archive":  Kit,
	"operator-sidecar":      Kit,
	"mitstack-workspace":    Internal,
	"plain":                 Internal,
	"plain-delivery":        Internal,
	"home-infra":            Internal,
	"home":                  Internal,
	"webshop-starter":       Internal,
	"alpha-site-preview":    Internal,
	"mitstack-home-gitops":  Internal,
	"management":            Internal,
	"plain-ci":              Internal,
	"plain-cd":              Internal,
	"home-ci":               Internal,
	"mitstack-home-cluster": Internal,
	"cost-model":            Internal,
	"workplace-shop":        Internal,
	"workplace-desk":        Internal,
}

// Every scratch proof app mitstack creates for itself, and its -ci and -cd repositories, is named with this prefix, so one rule places them all.
const scratchPrefix = "scratch-"

// manifestLayers are the manifest type words that are a layer themselves.
var manifestLayers = map[string]Name{string(Block): Block, string(Capability): Capability}

// ErrUnplaced marks a repository no rule places in a layer.
var ErrUnplaced = errors.New("unplaced")

// ErrRepositoryName marks a repository name that is not owner/name.
var ErrRepositoryName = errors.New("a repository is named owner/name")

// Verdict is a repository's layer and the source that decided it.
type Verdict struct {
	Layer  Name
	Source string
}

// Of returns the layer of repo, named owner/name, whose root manifest.yaml is manifest, or nil when it has none.
func Of(repo string, manifest []byte) (Verdict, error) {
	org, name, ok := strings.Cut(repo, "/")
	if !ok || org == "" || name == "" || strings.Contains(name, "/") {
		return Verdict{}, fmt.Errorf("%w, and %q is not", ErrRepositoryName, repo)
	}
	typ, hasType := manifestType(manifest)
	if org == owner {
		if listed, rule, ok := tableRule(name); ok {
			return tableVerdict(repo, listed, rule, typ)
		}
	}
	if manifest == nil {
		return Verdict{}, fmt.Errorf("%w: %s is in no row of plain-delivery's layer table and has no manifest.yaml", ErrUnplaced, repo)
	}
	if !hasType {
		return Verdict{}, fmt.Errorf("%w: %s is in no row of plain-delivery's layer table and its manifest.yaml has no top-level type: line", ErrUnplaced, repo)
	}
	layer, ok := manifestLayers[typ]
	if !ok {
		return Verdict{}, fmt.Errorf("%w: %s's manifest.yaml says type: %s, which names neither block nor capability", ErrUnplaced, repo, typ)
	}
	return Verdict{Layer: layer, Source: fmt.Sprintf("%s's manifest.yaml says type: %s", repo, typ)}, nil
}

// tableRule returns the layer the table gives an owner repository's name and the words saying why, a named row winning over the scratch rule.
func tableRule(name string) (Name, string, bool) {
	if listed, ok := repositories[name]; ok {
		return listed, fmt.Sprintf("is listed as %s in plain-delivery's layer table", listed), true
	}
	if strings.HasPrefix(name, scratchPrefix) {
		return Internal, fmt.Sprintf("has a name starting %s, which plain-delivery's layer table places as %s", scratchPrefix, Internal), true
	}
	return "", "", false
}

// tableVerdict places a table repository, refusing one whose manifest says block or capability, since a repository belongs to one layer.
func tableVerdict(repo string, listed Name, rule, manifestTyp string) (Verdict, error) {
	if _, claimsBlock := manifestLayers[manifestTyp]; claimsBlock {
		return Verdict{}, fmt.Errorf("%w: %s is %s in plain-delivery's layer table, yet its manifest.yaml says type: %s", ErrUnplaced, repo, listed, manifestTyp)
	}
	return Verdict{Layer: listed, Source: fmt.Sprintf("%s %s", repo, rule)}, nil
}

// Only an unindented type: line counts, since a nested type: belongs to another field.
func manifestType(manifest []byte) (string, bool) {
	for line := range strings.Lines(string(manifest)) {
		value, ok := strings.CutPrefix(line, "type:")
		if !ok {
			continue
		}
		value, _, _ = strings.Cut(value, " #")
		return strings.Trim(strings.TrimSpace(value), `"'`), true
	}
	return "", false
}
