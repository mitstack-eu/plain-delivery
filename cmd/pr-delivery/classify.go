package main

import (
	"path"
	"regexp"
	"strings"
)

// pathClass says which half of the commit-order rule a changed path belongs to.
type pathClass int

const (
	classOther pathClass = iota
	classTest
	classCode
)

func (c pathClass) String() string {
	return [...]string{"other", "test", "code"}[c]
}

// classRule matches a path by a slash-separated prefix, an exact path, a trailing path at any depth, or a basename pattern.
type classRule struct {
	class    pathClass
	prefixes []string
	exact    []string
	tails    []string
	basename []string
}

// classRules is the one table of path classes, read top to bottom, so a doc or pin under a test directory stays other, since it carries no logic a test could pin.
var classRules = []classRule{
	{
		class:    classOther,
		prefixes: []string{"docs/", "product/", ".shared/", "blocks/pins/", "blocks/capability-pins/", "api/internal/seed/manifests/"},
		exact:    []string{"LICENSE", "api/internal/seed/pins.yaml", "app/src/proto/fixtures/seededBlockFacts.json"},
		basename: []string{"*.md", "go.mod", "go.sum", "package.json", "package-lock.json"},
	},
	{
		// A fixture directory and a test library are imported by tests alone, though their names carry no suffix.
		class:    classTest,
		prefixes: []string{"*/testdata/", "*/__tests__/", "*/repotest/", "app/src/test/", "app/e2e/", "scripts/test/"},
		// tofu test loads a terraform.tfvars from its tests directory alone, so the file never reaches a real plan.
		tails: []string{"tests/terraform.tfvars"},
		basename: []string{
			"*.tftest.hcl",
			"*_test.go", "*_test.py", "test_*.py", "*_test.sh", "*.test.sh",
			"*_test.js", "*_test.jsx", "*_test.ts", "*_test.tsx",
			"*.test.js", "*.test.jsx", "*.test.ts", "*.test.tsx",
			"*.spec.js", "*.spec.jsx", "*.spec.ts", "*.spec.tsx",
			"testlib-*.sh", "testlib_*.py",
		},
	},
}

// classify places a changed path in its class; a submodule's gitlink is a pin, and anything no rule matches is code.
func classify(p string, gitlink bool) pathClass {
	if gitlink {
		return classOther
	}
	for _, r := range classRules {
		if r.matches(p) {
			return r.class
		}
	}
	return classCode
}

func (r classRule) matches(p string) bool {
	for _, e := range r.exact {
		if p == e {
			return true
		}
	}
	for _, pre := range r.prefixes {
		if hasDirPrefix(p, pre) {
			return true
		}
	}
	for _, tail := range r.tails {
		if p == tail || strings.HasSuffix(p, "/"+tail) {
			return true
		}
	}
	base := path.Base(p)
	for _, pat := range r.basename {
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	return false
}

// hasDirPrefix reports whether p sits under prefix, where a leading "*/" lets the directory sit at any depth.
func hasDirPrefix(p, prefix string) bool {
	if dir, ok := strings.CutPrefix(prefix, "*/"); ok {
		return strings.Contains("/"+p, "/"+dir)
	}
	return strings.HasPrefix(p, prefix)
}

// usesPrefix is a workflow line up to the "@" of an action's ref, and refSuffix the ref with an optional trailing comment.
var (
	usesPrefix = regexp.MustCompile(`^\s*(?:- )?uses:\s+[^\s@#]+$`)
	refSuffix  = regexp.MustCompile(`^[^\s#]+(?:\s+#.*)?$`)
)

// classifyPatch places a changed path in its class, where a workflow whose patch only moves `uses:` refs, or a Dockerfile whose patch only moves module versions, is a version pin, so other.
func classifyPatch(p, patch string) pathClass {
	class := classify(p, false)
	if class != classCode {
		return class
	}
	if (isWorkflow(p) && refOnlyBump(patch)) || (isDockerfile(p) && modulePinsOnly(patch)) {
		return classOther
	}
	return class
}

// patchDecides reports whether a code path's patch can make it a version pin, so its patch is worth reading.
func patchDecides(p string) bool {
	return isWorkflow(p) || isDockerfile(p)
}

func isWorkflow(p string) bool {
	ext := path.Ext(p)
	return strings.HasPrefix(p, ".github/workflows/") && (ext == ".yml" || ext == ".yaml")
}

func isDockerfile(p string) bool {
	return path.Base(p) == "Dockerfile"
}

// refOnlyBump reads a `git diff-tree -p -U0` patch and reports whether every hunk replaces uses: lines one for one, changing only the ref after "@" and its comment.
func refOnlyBump(patch string) bool {
	var removed, added []string
	hunks := 0
	pairsMatch := func() bool {
		if len(removed) == 0 || len(removed) != len(added) {
			return false
		}
		for i := range removed {
			if !sameUsesLine(removed[i], added[i]) {
				return false
			}
		}
		removed, added = nil, nil
		return true
	}
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			if hunks > 0 && !pairsMatch() {
				return false
			}
			hunks++
		case hunks == 0, line == "", strings.HasPrefix(line, `\`):
		case line[0] == '-':
			removed = append(removed, line[1:])
		case line[0] == '+':
			added = append(added, line[1:])
		default:
			return false
		}
	}
	return hunks > 0 && pairsMatch()
}

func sameUsesLine(old, new string) bool {
	oldPrefix, oldRef, ok := strings.Cut(old, "@")
	if !ok {
		return false
	}
	newPrefix, newRef, ok := strings.Cut(new, "@")
	return ok && oldPrefix == newPrefix && usesPrefix.MatchString(oldPrefix) &&
		refSuffix.MatchString(oldRef) && refSuffix.MatchString(newRef)
}

// modulePath and moduleVersion follow Go's module syntax, and lineEnd is the semicolon and backslash a RUN continuation line may end with.
const (
	modulePath    = `[A-Za-z0-9][A-Za-z0-9._~-]*(?:/[A-Za-z0-9._~-]+)*`
	moduleVersion = `v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?`
	modulePin     = modulePath + `@` + moduleVersion
	lineEnd       = `[ \t]*;?[ \t]*\\?[ \t]*$`
)

var (
	// replacePin is an xcaddy `--replace M=M@V` line, whose two module paths modulePinLine compares.
	replacePin = regexp.MustCompile(`^[ \t]*--replace[ \t]+(` + modulePath + `)=(` + modulePath + `)@` + moduleVersion + lineEnd)
	// goGetPins is a line of `go get` arguments, each a module at a version, optionally after `go get` itself.
	goGetPins       = regexp.MustCompile(`^[ \t]*(?:go[ \t]+get[ \t]+)?` + modulePin + `(?:[ \t]+` + modulePin + `)*` + lineEnd)
	dockerComment   = regexp.MustCompile(`^[ \t]*#`)
	parserDirective = regexp.MustCompile(`^[ \t]*#[ \t]*[A-Za-z][A-Za-z0-9]*[ \t]*=`)
)

// modulePinsOnly reads a `git diff-tree -p -U0` patch and reports whether it changes at least one line and every added and removed line, judged on its own, passes modulePinLine.
func modulePinsOnly(patch string) bool {
	inHunk, changed := false, false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case !inHunk, line == "", strings.HasPrefix(line, `\`):
		case line[0] == '-' || line[0] == '+':
			if !modulePinLine(line[1:]) {
				return false
			}
			changed = true
		default:
			return false
		}
	}
	return changed
}

// modulePinLine reports whether a Dockerfile line only names module versions or is a comment; a comment shaped like a parser directive stays code wherever it sits, since the patch alone cannot show whether it heads the file, where it changes how Docker reads it.
func modulePinLine(line string) bool {
	switch {
	case parserDirective.MatchString(line):
		return false
	case dockerComment.MatchString(line), goGetPins.MatchString(line):
		return true
	}
	m := replacePin.FindStringSubmatch(line)
	return m != nil && m[1] == m[2]
}
