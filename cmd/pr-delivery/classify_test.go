package main

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name, path string
		gitlink    bool
		want       pathClass
	}{
		{"a Go file is code", "api/internal/http/server.go", false, classCode},
		{"a Go test is a test", "api/internal/http/server_test.go", false, classTest},
		{"a shell suite is a test", "scripts/operator-branch-protection_test.sh", false, classTest},
		{"a dotted shell suite is a test", "scripts/pipeline-watch.test.sh", false, classTest},
		{"a TypeScript test is a test", "app/src/pages/Home.test.tsx", false, classTest},
		{"a Playwright spec is a test", "app/e2e/login.spec.ts", false, classTest},
		{"a pytest module is a test", "scripts/test_flow_ledger.py", false, classTest},
		{"a file under testdata is a test", "api/internal/seed/testdata/golden.json", false, classTest},
		{"a shell test library is a test", "scripts/testlib-plain-stub.sh", false, classTest},
		{"a repository fixture is a test", "api/internal/repotest/fixtures.go", false, classTest},
		{"a shell script is code", "scripts/operator-branch-protection.sh", false, classCode},
		{"a workflow is code", ".github/workflows/pipeline.yml", false, classCode},
		{"an extensionless file is code", "Makefile", false, classCode},
		{"Markdown is other", "README.md", false, classOther},
		{"a file under docs is other", "docs/diagram.svg", false, classOther},
		{"a planning file is other", "product/roadmap/plan.yaml", false, classOther},
		{"the generated shared mirror is other", ".shared/check-tracker-keys.sh", false, classOther},
		{"a block pin is other", "blocks/pins/managed-ci.yaml", false, classOther},
		{"a capability pin is other", "blocks/capability-pins/postgres.yaml", false, classOther},
		{"the seeded pin list is other", "api/internal/seed/pins.yaml", false, classOther},
		{"a seeded block manifest is other", "api/internal/seed/manifests/managed-ci-block.yaml", false, classOther},
		{"the regenerated block facts are other", "app/src/proto/fixtures/seededBlockFacts.json", false, classOther},
		{"a Go module file is other", "api/go.mod", false, classOther},
		{"a Go checksum file is other", "api/go.sum", false, classOther},
		{"an npm lock file is other", "app/package-lock.json", false, classOther},
		{"a submodule pin is other", "platform", true, classOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.path, tc.gitlink)

			if got != tc.want {
				t.Errorf("classify(%q, %v) = %v; want %v", tc.path, tc.gitlink, got, tc.want)
			}
		})
	}
}
