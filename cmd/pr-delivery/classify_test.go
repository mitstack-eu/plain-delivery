package main

import (
	"strings"
	"testing"
)

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
		{"a Python suite is a test", "scripts/thing_test.py", false, classTest},
		{"a JavaScript test is a test", "app/src/thing.test.js", false, classTest},
		{"a JSX test is a test", "app/src/thing.test.jsx", false, classTest},
		{"a dotted TypeScript test is a test", "app/src/thing.test.ts", false, classTest},
		{"a TypeScript spec outside e2e is a test", "app/src/thing.spec.ts", false, classTest},
		{"a TSX spec outside e2e is a test", "app/src/Thing.spec.tsx", false, classTest},
		{"a JavaScript spec is a test", "app/src/a.spec.js", false, classTest},
		{"a JSX spec is a test", "app/src/c.spec.jsx", false, classTest},
		{"an underscore TypeScript test is a test", "app/src/b_test.ts", false, classTest},
		{"an underscore TSX test is a test", "app/src/d_test.tsx", false, classTest},
		{"a file under __tests__ is a test", "app/src/__tests__/thing.ts", false, classTest},
		{"a Python test library is a test", "scripts/testlib_dotenv_git.py", false, classTest},
		{"an MSW handler is a test", "app/src/test/msw/handlers/apps.ts", false, classTest},
		{"a render helper under app/src/test is a test", "app/src/test/renderPage.tsx", false, classTest},
		{"a Playwright helper under e2e is a test", "app/e2e/helpers.ts", false, classTest},
		{"a seeding helper under repotest is a test", "api/internal/repo/repotest/seed.go", false, classTest},
		{"a pipeline fixture under scripts/test is a test", "scripts/test/fixtures/required-checks/pipeline.yml", false, classTest},
		{"a root testdata file is a test", "testdata/golden.json", false, classTest},
		{"a Python module is code", "scripts/check_something.py", false, classCode},
		{"a JavaScript module is code", "app/src/a.js", false, classCode},
		{"a TypeScript module is code", "app/src/b.ts", false, classCode},
		{"a JSX component is code", "app/src/c.jsx", false, classCode},
		{"a TSX component is code", "app/src/d.tsx", false, classCode},
		{"a proto fixture the dev route renders is code", "app/src/proto/fixtures/stack.ts", false, classCode},
		{"a JSON file beside the regenerated block facts is code", "app/src/proto/fixtures/otherFacts.json", false, classCode},
		{"a dated timeline under product is other", "product/roadmap/2026-08-28-timeline.json", false, classOther},
		{"an OpenTofu test under the platform's tofu tests directory is a test", "deploy/tofu/tests/ha_cluster_ecr_pull.tftest.hcl", false, classTest},
		{"an OpenTofu test beside its module is a test", "deploy/tofu/atlassian-archive/bucket.tftest.hcl", false, classTest},
		{"an OpenTofu test under a nested module's tests directory is a test", "deploy/tofu/org/tests/org.tftest.hcl", false, classTest},
		{"an OpenTofu test at a block repository's root tests directory is a test", "tests/main.tftest.hcl", false, classTest},
		{"an OpenTofu test under a block's perapp module is a test", "perapp/tests/mandatory_tags.tftest.hcl", false, classTest},
		{"the variable file tofu test loads from a tests directory is a test", "perapp/tests/terraform.tfvars", false, classTest},
		{"the variable file in a root tests directory is a test", "tests/terraform.tfvars", false, classTest},
		{"an OpenTofu variables file is code", "deploy/tofu/variables.tf", false, classCode},
		{"an OpenTofu resource file is code", "deploy/tofu/ecr.tf", false, classCode},
		{"an OpenTofu file under a tests directory is code", "deploy/tofu/tests/helper.tf", false, classCode},
		{"a module's variable file outside a tests directory is code", "perapp/terraform.tfvars", false, classCode},
		{"a variable file whose name only starts like the test one is code", "perapp/tests/terraform.tfvars.example", false, classCode},
		{"an OpenTofu lock file is code", "deploy/tofu/.terraform.lock.hcl", false, classCode},
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

func TestClassifyPatch(t *testing.T) {
	const (
		workflow = ".github/workflows/ci.yml"
		oldRef   = "34e114876b0b11c390a56381ad16ebd13914f8d5"
		newRef   = "08c6903cd8c0fde910a37f88322edcfb5dd907a8"
	)
	patchOf := func(path string, hunks ...string) string {
		head := "diff --git a/" + path + " b/" + path + "\nindex 1d2f3a4..5b6c7d8 100644\n--- a/" + path + "\n+++ b/" + path + "\n"
		return head + strings.Join(hunks, "")
	}
	checkoutBump := "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@" + oldRef + " # v7.0.0\n+      - uses: actions/checkout@" + newRef + " # v7.0.1\n"
	goBump := "@@ -15 +15 @@ jobs:\n-      - uses: actions/setup-go@" + oldRef + " # v6.0.0\n+      - uses: actions/setup-go@" + newRef + " # v6.1.0\n"
	cases := []struct {
		name, path, patch string
		want              pathClass
	}{
		{"a ref-only bump of one uses line with its comment version changed is other", workflow,
			patchOf(workflow, checkoutBump), classOther},
		{"a ref-only bump in a .yaml workflow is other", ".github/workflows/release.yaml",
			patchOf(".github/workflows/release.yaml", checkoutBump), classOther},
		{"two ref-only bumps in two hunks are other", workflow,
			patchOf(workflow, checkoutBump, goBump), classOther},
		{"a ref-only bump of a uses key with no list dash is other", workflow,
			patchOf(workflow, "@@ -20 +20 @@ jobs:\n-        uses: actions/cache@"+oldRef+" # v4.0.0\n+        uses: actions/cache@"+newRef+" # v4.1.0\n"), classOther},
		{"a bump whose ref has no comment on either side is other", workflow,
			patchOf(workflow, "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+"\n+      - uses: actions/checkout@"+newRef+"\n"), classOther},
		{"a bump that also changes the action path is code", workflow,
			patchOf(workflow, "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n+      - uses: actions/checkout-fork@"+newRef+" # v7.0.1\n"), classCode},
		{"a bump plus an added with line is code", workflow,
			patchOf(workflow, checkoutBump, "@@ -12,0 +13,2 @@\n+        with:\n+          fetch-depth: 0\n"), classCode},
		{"an added step with only added lines is code", workflow,
			patchOf(workflow, "@@ -13,0 +14,2 @@ jobs:\n+      - uses: actions/setup-go@"+newRef+" # v6.1.0\n+      - run: go vet ./...\n"), classCode},
		{"a hunk that only removes a uses line is code", workflow,
			patchOf(workflow, "@@ -12 +11,0 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n"), classCode},
		{"a hunk removing two uses lines and adding one is code", workflow,
			patchOf(workflow, "@@ -12,2 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n-      - uses: actions/setup-go@"+oldRef+" # v6.0.0\n+      - uses: actions/checkout@"+newRef+" # v7.0.1\n"), classCode},
		{"a bump plus a changed run line is code", workflow,
			patchOf(workflow, checkoutBump, "@@ -14 +14 @@\n-      - run: make test\n+      - run: make check\n"), classCode},
		{"a changed indentation is code", workflow,
			patchOf(workflow, "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n+        - uses: actions/checkout@"+newRef+" # v7.0.0\n"), classCode},
		{"text after the ref other than a comment is code", workflow,
			patchOf(workflow, "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n+      - uses: actions/checkout@"+newRef+" with-extra-token\n"), classCode},
		{"a second ref token after the first is code", workflow,
			patchOf(workflow, "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n+      - uses: actions/checkout@"+newRef+" "+oldRef+" # v7.0.1\n"), classCode},
		{"a changed key before uses is code", workflow,
			patchOf(workflow, "@@ -12 +12 @@ jobs:\n-      - uses: actions/checkout@"+oldRef+" # v7.0.0\n+      - name: actions/checkout@"+newRef+" # v7.0.1\n"), classCode},
		{"the same ref-only patch outside .github/workflows keeps the path's class, code", "scripts/ci.yml",
			patchOf("scripts/ci.yml", checkoutBump), classCode},
		{"a ref-only bump of a test path keeps it a test", "scripts/test/fixtures/required-checks/pipeline.yml",
			patchOf("scripts/test/fixtures/required-checks/pipeline.yml", checkoutBump), classTest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPatch(tc.path, tc.patch)

			if got != tc.want {
				t.Errorf("classifyPatch(%q, patch) = %v; want %v\npatch:\n%s", tc.path, got, tc.want, tc.patch)
			}
		})
	}
}
