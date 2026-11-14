package main

import (
	"path"
	"regexp"
	"slices"
	"strconv"
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

// classifyPatch places a changed path in its class, where a workflow whose patch only moves `uses:` refs, or a Dockerfile whose patch only moves module versions or comments, is a version pin, so other.
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

// hunk is one block of a `git diff-tree -p -U0` patch: the line each side starts at, and the lines it removes and adds.
type hunk struct {
	oldStart, newStart int
	removed, added     []string
}

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

// parseHunks reads a -U0 patch into its hunks, failing on a malformed header or a context line, which -U0 never prints.
func parseHunks(patch string) ([]hunk, bool) {
	var hunks []hunk
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return nil, false
			}
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[2])
			hunks = append(hunks, hunk{oldStart: oldStart, newStart: newStart})
		case len(hunks) == 0, line == "", strings.HasPrefix(line, `\`):
		case line[0] == '-':
			hunks[len(hunks)-1].removed = append(hunks[len(hunks)-1].removed, line[1:])
		case line[0] == '+':
			hunks[len(hunks)-1].added = append(hunks[len(hunks)-1].added, line[1:])
		default:
			return nil, false
		}
	}
	return hunks, len(hunks) > 0
}

// refOnlyBump reports whether every hunk of a -U0 patch replaces uses: lines one for one, changing only the ref after "@" and its comment.
func refOnlyBump(patch string) bool {
	hunks, ok := parseHunks(patch)
	if !ok {
		return false
	}
	for _, h := range hunks {
		if len(h.removed) == 0 || len(h.removed) != len(h.added) {
			return false
		}
		for i := range h.removed {
			if !sameUsesLine(h.removed[i], h.added[i]) {
				return false
			}
		}
	}
	return true
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

// A pin's first path element holds a dot, as Go requires of a module path, and its version is a full semver tag, so `@latest` never counts; lineEnd allows a trailing `;`, `\`, or both in that order.
const (
	moduleDomain  = `[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)+`
	modulePath    = moduleDomain + `(?:/[A-Za-z0-9._~-]+)*`
	moduleVersion = `v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?`
	modulePin     = modulePath + `@` + moduleVersion
	lineEnd       = `[ \t]*;?[ \t]*\\?[ \t]*$`
)

var (
	// replacePin is an xcaddy `--replace M=M@V` line, whose two module paths modulePinLine compares.
	replacePin      = regexp.MustCompile(`^[ \t]*--replace[ \t]+(` + modulePath + `)=(` + modulePath + `)@` + moduleVersion + lineEnd)
	goGetPins       = regexp.MustCompile(`^[ \t]*(?:go[ \t]+get[ \t]+)?` + modulePin + `(?:[ \t]+` + modulePin + `)*` + lineEnd)
	dockerComment   = regexp.MustCompile(`^[ \t]*#`)
	parserDirective = regexp.MustCompile(`^[ \t]*#[ \t]*[A-Za-z][A-Za-z0-9]*[ \t]*=`)
)

// modulePinsOnly reports whether every added and removed line of a -U0 patch, judged on its own, passes modulePinLine, and no hunk reaches line 1, where a changed comment can make or unmake a parser directive.
func modulePinsOnly(patch string) bool {
	hunks, ok := parseHunks(patch)
	if !ok {
		return false
	}
	for _, h := range hunks {
		if h.oldStart <= 1 || h.newStart <= 1 {
			return false
		}
		for _, line := range slices.Concat(h.removed, h.added) {
			if !modulePinLine(line) {
				return false
			}
		}
	}
	return true
}

// modulePinLine reports whether a Dockerfile line only names module versions or is a comment, where a directive-shaped comment stays code anywhere, since a -U0 patch cannot show whether only directives sit above it.
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
