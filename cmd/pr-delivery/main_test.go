package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const boardToken = "test-board-token"

// The logins GitHub gives the two Apps that open mitstack pull requests with no board issue behind them.
const (
	dependabotApp = "dependabot[bot]"
	pinBumpApp    = "mitstack-ci[bot]"
)

// step is one commit: rename moves a file first, keeping or appending to its body, remove deletes files, then each path is written, or, for a gitlink step, pointed at the seed commit as a submodule. A path in content is written with exactly that text. A seed step is committed into the seed itself, so the range starts after it.
type step struct {
	paths      []string
	subject    string
	gitlink    bool
	rename     [2]string
	editRename bool
	remove     []string
	content    map[string]string
	seed       bool
}

// body is long enough that an appended line keeps a renamed file similar enough for git to see a rename.
func body(p string) string { return strings.Repeat("an unchanged line of "+p+"\n", 20) }

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGitEnv(t, dir, nil, args...)
}

func runGitEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=test@example.com", "-c", "user.name=test"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRange commits a seed holding each seed step's files and then each other step, returning the directory, the seed sha and each step's sha.
func newRange(t *testing.T, steps []step) (dir, base string, shas []string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "-q")
	for _, s := range steps {
		if s.seed {
			stagePaths(t, dir, s)
		}
	}
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "seed")
	base = runGit(t, dir, "rev-parse", "HEAD")
	i := 0
	for _, s := range steps {
		if s.seed {
			continue
		}
		if from, to := s.rename[0], s.rename[1]; from != "" {
			runGit(t, dir, "mv", from, to)
			if s.editRename {
				appendLine(t, filepath.Join(dir, to))
				runGit(t, dir, "add", to)
			}
		}
		for _, p := range s.remove {
			runGit(t, dir, "rm", "-q", p)
		}
		if s.gitlink {
			for _, p := range s.paths {
				runGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+base+","+p)
			}
		} else {
			stagePaths(t, dir, s)
		}
		subject := s.subject
		if subject == "" {
			subject = "MIT-12: step " + string(rune('a'+i))
		}
		runGit(t, dir, "commit", "-q", "-m", subject)
		shas = append(shas, runGit(t, dir, "rev-parse", "HEAD"))
		i++
	}
	return dir, base, shas
}

// stagePaths writes each of the step's paths, with its content when it names one, and stages them.
func stagePaths(t *testing.T, dir string, s step) {
	t.Helper()
	for _, p := range s.paths {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		text, ok := s.content[p]
		if !ok {
			text = body(p) + s.subject
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", p)
	}
}

// commitPaths writes and commits each path at a fixed time, so two branches' commits order the same way on every run, and returns the commit's sha.
func commitPaths(t *testing.T, dir, subject string, tick int, paths ...string) string {
	t.Helper()
	stagePaths(t, dir, step{paths: paths, subject: subject})
	when := "@" + strconv.Itoa(2_000_000_000+tick) + " +0000"
	runGitEnv(t, dir, []string{"GIT_AUTHOR_DATE=" + when, "GIT_COMMITTER_DATE=" + when}, "commit", "-q", "-m", subject)
	return runGit(t, dir, "rev-parse", "HEAD")
}

// runClaimed runs the check as a person whose title names an issue that is In Progress on the board.
func runClaimed(t *testing.T, dir, base, head string) (code int, out string) {
	t.Helper()
	url, _ := fakeBoard(t, map[string]string{"MIT-12": "In Progress"})
	t.Setenv("TRACKER_BASE_URL", url)
	t.Setenv("TRACKER_API_TOKEN", boardToken)
	t.Setenv("PR_AUTHOR", "octocat")
	var buf bytes.Buffer
	code = Run([]string{"--repo", dir, "--base", base, "--head", head, "--title", "MIT-12: fix"}, &buf)
	return code, buf.String()
}

func appendLine(t *testing.T, path string) {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(text, "an edited line\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeBoard answers the issue list route for the keys it holds, refusing a request without the board token, and counts its requests.
func fakeBoard(t *testing.T, statuses map[string]string) (url string, requests *int) {
	t.Helper()
	requests = new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if r.URL.Path != "/v1/issues" || r.URL.Query().Get("fields") != "status" || r.Header.Get("Authorization") != "Bearer "+boardToken {
			http.Error(w, "unexpected request "+r.URL.String(), http.StatusUnauthorized)
			return
		}
		issues := []map[string]string{}
		for _, k := range strings.Split(r.URL.Query().Get("keys"), ",") {
			if s, ok := statuses[k]; ok {
				issues = append(issues, map[string]string{"id": k, "status": s})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"issues": issues})
	}))
	t.Cleanup(srv.Close)
	return srv.URL, requests
}

func TestRun(t *testing.T) {
	inProgress := map[string]string{"MIT-12": "In Progress"}
	pinFiles := []string{
		"blocks/pins/managed-ci.yaml", "api/internal/seed/pins.yaml", "api/internal/seed/manifests/managed-ci-block.yaml",
		"app/src/proto/fixtures/seededBlockFacts.json", "scripts/resolve-block-images_test.sh",
	}
	keylessPass := "pr-delivery: 1 commit(s) judged: the tests came first, and the board was not asked about the keys of a GitHub App on the keyless list"
	const (
		workflow     = ".github/workflows/ci.yml"
		oldCheckout  = "34e114876b0b11c390a56381ad16ebd13914f8d5"
		newCheckout  = "08c6903cd8c0fde910a37f88322edcfb5dd907a8"
		workflowHead = "name: ci\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@"
		workflowTail = "      - run: make test\n"
	)
	ciBefore := workflowHead + oldCheckout + " # v7.0.0\n" + workflowTail
	ciBumped := workflowHead + newCheckout + " # v7.0.1\n" + workflowTail
	ciBumpedWithInput := workflowHead + newCheckout + " # v7.0.1\n        with:\n          fetch-depth: 0\n" + workflowTail
	const (
		caddyfile = "app/Dockerfile"
		walgfile  = "deploy/wal-shipper/Dockerfile"
		joseRepl  = "      --replace github.com/go-jose/go-jose/v4=github.com/go-jose/go-jose/v4@v4.1.6"
		otelRepl  = "      --replace go.opentelemetry.io/otel=go.opentelemetry.io/otel@v1.47.0"
		joseGet   = "      github.com/go-jose/go-jose/v4@v4.1.6"
	)
	caddyBuild := func(comment string, replaces ...string) string {
		return "FROM golang:1.26.11-alpine AS caddy\n" + comment +
			"RUN xcaddy build v2.11.7 --output /usr/bin/caddy \\\n" + strings.Join(replaces, " \\\n") + "\n"
	}
	walgBuild := func(comment string, modules ...string) string {
		return "FROM golang:1.26.11-bookworm AS walg\n" + comment +
			"RUN set -eux; \\\n    go get \\\n" + strings.Join(modules, " \\\n") + "; \\\n    go mod tidy\n"
	}
	caddyBefore := caddyBuild("# go-jose is a dependency rather than an xcaddy plugin, so xcaddy\n# moves it through --replace.\n", joseRepl)
	caddyAfter := caddyBuild("# go-jose and OpenTelemetry are dependencies rather than xcaddy\n# plugins, so xcaddy moves each\n# through --replace.\n", joseRepl, otelRepl)
	pinsBefore := map[string]string{
		caddyfile: caddyBefore,
		walgfile:  walgBuild("# go get raises the OpenTelemetry SDK to its fixed version.\n", "      go.opentelemetry.io/otel/sdk@v1.45.0", joseGet),
	}
	pinsAfter := map[string]string{
		caddyfile: caddyAfter,
		walgfile: walgBuild("# go get raises OpenTelemetry's SDK and trace modules to their\n# fixed version.\n",
			"      go.opentelemetry.io/otel/sdk@v1.47.0", "      go.opentelemetry.io/otel/trace@v1.47.0", joseGet),
	}
	caddyRunChanged := strings.Replace(caddyAfter, "/usr/bin/caddy", "/usr/local/bin/caddy", 1)
	cases := []struct {
		name, title, author string
		steps               []step
		board               map[string]string
		wantCode            int
		// wantLine is the lines, newline-separated, the output must hold, with @N standing for the twelve-character sha of range step N, a seed step not counted.
		wantLine string
	}{
		{"a person's test commit and then its code under an In Progress key passes", "MIT-12: fix", "octocat",
			[]step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}}}, inProgress, 0,
			"pr-delivery: 2 commit(s) judged: the tests came first and every key is claimed"},
		{"tests, then code, then a later tests-only commit passes", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}}, {paths: []string{"api/exec_test.go"}}}, inProgress, 0,
			"pr-delivery: 3 commit(s) judged: the tests came first and every key is claimed"},
		{"a docs-only commit ahead of the tests is not the first commit the order judges", "MIT-12: fix", "",
			[]step{{paths: []string{"docs/plan.md"}}, {paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}}}, inProgress, 0,
			"pr-delivery: 3 commit(s) judged: the tests came first and every key is claimed"},
		{"code and its doc in one commit with no test fails as code with no test", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run.go", "docs/run.md"}}}, inProgress, 1,
			"pr-delivery: the code this pull request changes has no test, since no commit changes a test file"},
		{"a pure test rename ahead of a code commit does not count as the tests landing first", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go"}, seed: true}, {rename: [2]string{"api/run_test.go", "api/exec_test.go"}}, {paths: []string{"api/run.go"}}, {paths: []string{"api/new_test.go"}}}, inProgress, 1,
			`pr-delivery: code came before its tests: commit @1 "MIT-12: step b" changes code first: api/run.go`},
		{"code first and a later mixed commit report both violations", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run.go"}}, {paths: []string{"api/exec.go", "api/exec_test.go"}}}, inProgress, 1,
			"pr-delivery: code came before its tests: commit @0 \"MIT-12: step a\" changes code first: api/run.go\n" +
				`pr-delivery: commit @1 "MIT-12: step b" changes a test and code together: tests api/exec_test.go; code api/exec.go`},
		{"two mixed commits are each reported", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run.go", "api/run_test.go"}}, {paths: []string{"api/exec.go", "api/exec_test.go"}}}, inProgress, 1,
			"pr-delivery: commit @0 \"MIT-12: step a\" changes a test and code together: tests api/run_test.go; code api/run.go\n" +
				`pr-delivery: commit @1 "MIT-12: step b" changes a test and code together: tests api/exec_test.go; code api/exec.go`},
		{"a commit that only renames a test file passes", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go"}, seed: true}, {rename: [2]string{"api/run_test.go", "api/exec_test.go"}}}, inProgress, 0,
			"pr-delivery: 1 commit(s) judged: the tests came first and every key is claimed"},
		{"a commit that only renames a non-ASCII test path passes", "MIT-12: fix", "",
			[]step{{paths: []string{"api/café_test.go"}, seed: true}, {rename: [2]string{"api/café_test.go", "api/naïve_test.go"}}}, inProgress, 0,
			"pr-delivery: 1 commit(s) judged: the tests came first and every key is claimed"},
		{"a quoted rename followed by a code edit and a quoted doc deletion reads each record", "MIT-12: fix", "",
			[]step{
				{paths: []string{"api/café_test.go", "docs/café-runbook.md"}, seed: true},
				{paths: []string{"api/other_test.go"}},
				{rename: [2]string{"api/café_test.go", "api/naïve_test.go"}, remove: []string{"docs/café-runbook.md"}, paths: []string{"api/server.go"}},
			}, inProgress, 0,
			"pr-delivery: 2 commit(s) judged: the tests came first and every key is claimed"},
		{"deleting a test file alongside a production change fails as a test and code together", "MIT-12: fix", "",
			[]step{{paths: []string{"api/server_test.go"}, seed: true}, {remove: []string{"api/server_test.go"}, paths: []string{"api/run.go"}}}, inProgress, 1,
			`pr-delivery: commit @0 "MIT-12: step a" changes a test and code together: tests api/server_test.go; code api/run.go`},
		{"a non-ASCII test path beside a production edit fails naming the path unescaped", "MIT-12: fix", "",
			[]step{{paths: []string{"api/café_test.go", "api/handler.go"}}}, inProgress, 1,
			`pr-delivery: commit @0 "MIT-12: step a" changes a test and code together: tests api/café_test.go; code api/handler.go`},
		{"a non-ASCII doc path beside a test passes, since the doc is other", "MIT-12: fix", "",
			[]step{{paths: []string{"docs/café-runbook.md", "api/handler_test.go"}}}, inProgress, 0,
			"pr-delivery: 1 commit(s) judged: the tests came first and every key is claimed"},
		{"a test path containing a space beside a production edit fails", "MIT-12: fix", "",
			[]step{{paths: []string{"api/some file_test.go", "api/server.go"}}}, inProgress, 1,
			`pr-delivery: commit @0 "MIT-12: step a" changes a test and code together: tests api/some file_test.go; code api/server.go`},
		{"a test path containing a double quote beside a production edit fails", "MIT-12: fix", "",
			[]step{{paths: []string{`api/say"quoted"_test.go`, "api/server.go"}}}, inProgress, 1,
			`pr-delivery: commit @0 "MIT-12: step a" changes a test and code together: tests api/say"quoted"_test.go; code api/server.go`},
		{"an empty range fails, since no commit names a board key", "MIT-12: fix", "",
			nil, inProgress, 1, "pr-delivery: no commit names a board key"},
		{"a key in Pull Request passes", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}}}, map[string]string{"MIT-12": "Pull Request"}, 0, ""},
		{"Go code committed before its test fails naming the code commit", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run.go"}}, {paths: []string{"api/run_test.go"}}}, inProgress, 1,
			`pr-delivery: code came before its tests: commit @0 "MIT-12: step a" changes code first: api/run.go`},
		{"a commit changing a test and code together fails naming both", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go", "api/run.go"}}}, inProgress, 1,
			`pr-delivery: commit @0 "MIT-12: step a" changes a test and code together: tests api/run_test.go; code api/run.go`},
		{"code with no test fails", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run.go"}}}, inProgress, 1,
			"pr-delivery: the code this pull request changes has no test, since no commit changes a test file"},
		{"an OpenTofu test committed first and then its OpenTofu code passes", "MIT-12: fix", "",
			[]step{{paths: []string{"deploy/tofu/tests/ha_cluster_ecr_pull.tftest.hcl"}}, {paths: []string{"deploy/tofu/variables.tf"}}}, inProgress, 0,
			"pr-delivery: 2 commit(s) judged: the tests came first and every key is claimed"},
		{"OpenTofu code changed with no OpenTofu test fails as code with no test", "MIT-12: fix", "",
			[]step{{paths: []string{"deploy/tofu/variables.tf"}}}, inProgress, 1,
			"pr-delivery: the code this pull request changes has no test, since no commit changes a test file"},
		{"an OpenTofu test and its OpenTofu code in one commit fails as a test and code together", "MIT-12: fix", "",
			[]step{{paths: []string{"deploy/tofu/tests/ha_cluster_ecr_pull.tftest.hcl", "deploy/tofu/variables.tf"}}}, inProgress, 1,
			`pr-delivery: commit @0 "MIT-12: step a" changes a test and code together: tests deploy/tofu/tests/ha_cluster_ecr_pull.tftest.hcl; code deploy/tofu/variables.tf`},
		{"Markdown alone under a claimed key passes with no test commit", "MIT-12: docs", "",
			[]step{{paths: []string{"README.md", "docs/flow.md"}}}, inProgress, 0, ""},
		{"a pin bump changing only pin files under a claimed key passes", "MIT-12: pin managed-ci to 2.21.0", "",
			[]step{{paths: pinFiles}}, inProgress, 0, ""},
		{"a pure rename of a test beside a code change counts as neither half, so the commit is not mixed", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}, rename: [2]string{"api/run_test.go", "api/exec_test.go"}}}, inProgress, 0, ""},
		{"a renamed and edited test counts as a test at its new path", "MIT-12: fix", "",
			[]step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}, rename: [2]string{"api/run_test.go", "api/exec_test.go"}, editRename: true}}, inProgress, 1,
			`pr-delivery: commit @1 "MIT-12: step b" changes a test and code together: tests api/exec_test.go; code api/run.go`},
		{"a key the board does not hold fails naming it", "MIT-12: fix", "",
			[]step{{paths: []string{"README.md"}}}, map[string]string{}, 1, "pr-delivery: MIT-12 is not an issue on the board"},
		{"a key in Ready fails naming its status", "MIT-12: fix", "",
			[]step{{paths: []string{"README.md"}}}, map[string]string{"MIT-12": "Ready"}, 1,
			"pr-delivery: MIT-12 is Ready on the board, and a pull request may name only an issue that is In Progress or Pull Request"},
		{"a key in Done fails naming its status", "MIT-12: fix", "",
			[]step{{paths: []string{"README.md"}}}, map[string]string{"MIT-12": "Done"}, 1,
			"pr-delivery: MIT-12 is Done on the board, and a pull request may name only an issue that is In Progress or Pull Request"},
		{"a key moved to Removed before a rerun fails naming its new status", "update the thing (refs MIT-12)", "",
			[]step{{paths: []string{"README.md"}, subject: "update the thing (refs MIT-12)"}}, map[string]string{"MIT-12": "Removed"}, 1,
			"pr-delivery: MIT-12 is Removed on the board, and a pull request may name only an issue that is In Progress or Pull Request"},
		{"a title naming no key fails", "fix the thing", "",
			[]step{{paths: []string{"README.md"}}}, inProgress, 1, "pr-delivery: the title names no board key"},
		{"commits naming no key fail", "MIT-12: fix", "",
			[]step{{paths: []string{"README.md"}, subject: "fix the thing"}}, inProgress, 1, "pr-delivery: no commit names a board key"},
		{"no board address fails, since no board can say the key is claimed", "MIT-12: fix", "",
			[]step{{paths: []string{"README.md"}}}, nil, 1, "pr-delivery: no board to read, since TRACKER_BASE_URL is unset"},
		{"the pin-bump App's keyless submodule pin move passes and reads no board", "chore: bump platform submodule", pinBumpApp,
			[]step{{paths: []string{"platform"}, subject: "chore: bump platform submodule", gitlink: true}}, map[string]string{}, 0,
			"pr-delivery: mitstack-ci[bot] is a GitHub App on the keyless list, so the board is not asked about its keys\n" + keylessPass},
		{"Dependabot's keyless module bump passes and reads no board", "gomod(deps): bump the go-all group in /api", dependabotApp,
			[]step{{paths: []string{"api/go.mod", "api/go.sum"}, subject: "gomod(deps): bump the go-all group in /api"}}, map[string]string{}, 0,
			"pr-delivery: dependabot[bot] is a GitHub App on the keyless list, so the board is not asked about its keys\n" + keylessPass},
		{"Dependabot's keyless Actions bump of a uses: ref passes", "chore(deps): bump actions/checkout from 7.0.0 to 7.0.1", dependabotApp,
			[]step{
				{paths: []string{workflow, "api/run_test.go"}, content: map[string]string{workflow: ciBefore}, seed: true},
				{paths: []string{workflow}, content: map[string]string{workflow: ciBumped}, subject: "chore(deps): bump actions/checkout from 7.0.0 to 7.0.1"},
			}, map[string]string{}, 0,
			"pr-delivery: dependabot[bot] is a GitHub App on the keyless list, so the board is not asked about its keys"},
		{"a person's workflow change adding a with: line beside a ref bump, with no test, fails as code with no test", "MIT-12: fetch the full history", "",
			[]step{
				{paths: []string{workflow, "api/run_test.go"}, content: map[string]string{workflow: ciBefore}, seed: true},
				{paths: []string{workflow}, content: map[string]string{workflow: ciBumpedWithInput}},
			}, inProgress, 1,
			"pr-delivery: the code this pull request changes has no test, since no commit changes a test file"},
		{"a listed App's code before its test still fails", "chore: bump", dependabotApp,
			[]step{{paths: []string{"api/run.go"}, subject: "chore: bump"}, {paths: []string{"api/run_test.go"}, subject: "chore: test"}}, nil, 1,
			`pr-delivery: code came before its tests: commit @0 "chore: bump" changes code first: api/run.go`},
		{"an App off the list fails without a key", "chore: bump platform submodule", "renovate[bot]",
			[]step{{paths: []string{"platform"}, subject: "chore: bump platform submodule", gitlink: true}}, nil, 1,
			"pr-delivery: the title names no board key"},
		{"a person fails without a key", "chore: bump platform submodule", "octocat",
			[]step{{paths: []string{"platform"}, subject: "chore: bump platform submodule", gitlink: true}}, nil, 1,
			"pr-delivery: the title names no board key"},
		{"a person's Dockerfile patches moving only module versions and comments pass with no test commit", "MIT-12: take OpenTelemetry 1.47.0", "",
			[]step{
				{paths: []string{caddyfile, walgfile}, content: pinsBefore, seed: true},
				{paths: []string{caddyfile, walgfile}, content: pinsAfter},
			}, inProgress, 0,
			"pr-delivery: 1 commit(s) judged: the tests came first and every key is claimed"},
		{"a Dockerfile patch moving a module version and changing a RUN line, with no test, fails as code with no test", "MIT-12: take OpenTelemetry 1.47.0", "",
			[]step{
				{paths: []string{caddyfile}, content: map[string]string{caddyfile: caddyBefore}, seed: true},
				{paths: []string{caddyfile}, content: map[string]string{caddyfile: caddyRunChanged}},
			}, inProgress, 1,
			"pr-delivery: the code this pull request changes has no test, since no commit changes a test file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, base, shas := newRange(t, tc.steps)
			t.Setenv("TRACKER_BASE_URL", "")
			t.Setenv("TRACKER_API_TOKEN", boardToken)
			requests := new(int)
			if tc.board != nil {
				var url string
				url, requests = fakeBoard(t, tc.board)
				t.Setenv("TRACKER_BASE_URL", url)
			}
			t.Setenv("PR_AUTHOR", tc.author)
			head := base
			if len(shas) > 0 {
				head = shas[len(shas)-1]
			}
			args := []string{"--repo", dir, "--base", base, "--head", head, "--title", tc.title}

			var out bytes.Buffer
			code := Run(args, &out)

			want := tc.wantLine
			for i, sha := range shas {
				want = strings.ReplaceAll(want, "@"+string(rune('0'+i)), sha[:12])
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			missing := false
			for _, w := range strings.Split(want, "\n") {
				missing = missing || (w != "" && !slices.Contains(lines, w))
			}
			if code != tc.wantCode || missing {
				t.Errorf("Run(%q) = %d; want %d and the lines %q\noutput:\n%s", args, code, tc.wantCode, want, out.String())
			}
			listedApp := tc.author == dependabotApp || tc.author == pinBumpApp
			if listedApp && tc.wantCode == 0 && strings.Contains(out.String(), "every key is claimed") {
				t.Errorf("%s's pull request claimed every key; the board was not asked\noutput:\n%s", tc.author, out.String())
			}
			if listedApp && *requests != 0 {
				t.Errorf("%s's pull request asked the board %d time(s); want none", tc.author, *requests)
			}
			if !listedApp && tc.board != nil && *requests != 1 {
				t.Errorf("a person's pull request asked the board %d time(s); want one request for every key", *requests)
			}
		})
	}
}

func TestRun_FailsWithTheBoardsAnswerAndTheTokenRedacted(t *testing.T) {
	dir, base, shas := newRange(t, []step{{paths: []string{"README.md"}}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down for "+boardToken, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TRACKER_BASE_URL", srv.URL)
	t.Setenv("TRACKER_API_TOKEN", boardToken)

	var out bytes.Buffer
	code := Run([]string{"--repo", dir, "--base", base, "--head", shas[0], "--title", "MIT-12: fix"}, &out)

	want := "pr-delivery: reading the board: board returned 500: down for [REDACTED]"
	if code != 1 || !slices.Contains(strings.Split(strings.TrimSpace(out.String()), "\n"), want) {
		t.Errorf("Run = %d; want 1 and the line %q\noutput:\n%s", code, want, out.String())
	}
}

func TestRun_ExitsTwoOnAMissingRangeFlag(t *testing.T) {
	for _, omit := range []string{"--base", "--head"} {
		args := []string{"--base", "a", "--head", "b"}
		i := slices.Index(args, omit)
		args = slices.Delete(args, i, i+2)

		var out bytes.Buffer
		code := Run(args, &out)

		if want := "pr-delivery: " + omit + " is required\n"; code != 2 || out.String() != want {
			t.Errorf("Run(%q) = %d, %q; want 2, %q", args, code, out.String(), want)
		}
	}
}

func TestRun_FailsOnARefItCannotResolve(t *testing.T) {
	cases := []struct {
		name    string
		badBase bool
	}{
		{"an unresolvable base ref fails naming it", true},
		{"an unresolvable head ref fails naming it", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, base, shas := newRange(t, []step{{paths: []string{"api/run.go", "api/run_test.go"}}})
			head := shas[0]
			if tc.badBase {
				base = "refs/heads/does-not-exist"
			} else {
				head = "refs/heads/no-such-head"
			}

			code, out := runClaimed(t, dir, base, head)

			prefix := "pr-delivery: reading the commits from " + base + " to " + head + ": "
			named := slices.ContainsFunc(strings.Split(out, "\n"), func(l string) bool { return strings.HasPrefix(l, prefix) })
			if code != 1 || !named {
				t.Errorf("Run(base %q, head %q) = %d; want 1 and a line starting %q\noutput:\n%s", base, head, code, prefix, out)
			}
		})
	}
}

func TestRun_JudgesFromTheMergeBase(t *testing.T) {
	dir, base, shas := newRange(t, []step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}}})
	featureTip := shas[1]
	runGit(t, dir, "checkout", "-q", "-b", "mainline", base)
	mainTip := commitPaths(t, dir, "MIT-12: main moves on with a mixed commit", 1, "api/exec.go", "api/exec_test.go")

	code, out := runClaimed(t, dir, mainTip, featureTip)

	want := "pr-delivery: 2 commit(s) judged: the tests came first and every key is claimed"
	if code != 0 || !slices.Contains(strings.Split(strings.TrimSpace(out), "\n"), want) {
		t.Errorf("Run(base %s, head %s) = %d; want 0 and the line %q, since a commit on the base after the branch point is outside the range\noutput:\n%s", mainTip[:12], featureTip[:12], code, want, out)
	}
}

func TestRun_LeavesAMergeCommitOutOfTheRange(t *testing.T) {
	dir, base, _ := newRange(t, nil)
	testsTip := commitPaths(t, dir, "MIT-12: tests only", 1, "api/run_test.go")
	runGit(t, dir, "checkout", "-q", "-b", "side", base)
	commitPaths(t, dir, "MIT-12: code only", 2, "api/run.go")
	runGit(t, dir, "checkout", "-q", "--detach", testsTip)
	when := "@" + strconv.Itoa(2_000_000_003) + " +0000"
	runGitEnv(t, dir, []string{"GIT_AUTHOR_DATE=" + when, "GIT_COMMITTER_DATE=" + when}, "merge", "-q", "--no-ff", "-m", "MIT-12: merge side into the branch", "side")
	mergeTip := runGit(t, dir, "rev-parse", "HEAD")

	code, out := runClaimed(t, dir, base, mergeTip)

	want := "pr-delivery: 2 commit(s) judged: the tests came first and every key is claimed"
	if code != 0 || !slices.Contains(strings.Split(strings.TrimSpace(out), "\n"), want) {
		t.Errorf("Run(base %s, head %s) = %d; want 0 and the line %q, since the merge commit is not judged\noutput:\n%s", base[:12], mergeTip[:12], code, want, out)
	}
}
