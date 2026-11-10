package main

import (
	"path"
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

// classRule matches a path by a slash-separated prefix, an exact path, or a basename pattern.
type classRule struct {
	class    pathClass
	prefixes []string
	exact    []string
	basename []string
}

// classRules is the one table of path classes, read top to bottom, so a doc or pin under a test directory stays other.
// Docs ship with the code they describe, and a pin or dependency file moves a version and carries no logic a test could pin.
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
		basename: []string{
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
