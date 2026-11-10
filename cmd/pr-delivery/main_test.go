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
	"strings"
	"testing"
)

const boardToken = "test-board-token"

// The logins GitHub gives the two Apps that open mitstack pull requests with no board issue behind them.
const (
	dependabotApp = "dependabot[bot]"
	pinBumpApp    = "mitstack-ci[bot]"
)

// step is one commit: rename moves a file first, keeping or appending to its body, then each path is written, or, for a gitlink step, pointed at the seed commit as a submodule.
type step struct {
	paths      []string
	subject    string
	gitlink    bool
	rename     [2]string
	editRename bool
}

// body is long enough that an appended line keeps a renamed file similar enough for git to see a rename.
func body(p string) string { return strings.Repeat("an unchanged line of "+p+"\n", 20) }

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=test@example.com", "-c", "user.name=test"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRange commits a seed and then each step, returning the directory, the seed sha and each step's sha.
func newRange(t *testing.T, steps []step) (dir, base string, shas []string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "seed")
	base = runGit(t, dir, "rev-parse", "HEAD")
	for i, s := range steps {
		if from, to := s.rename[0], s.rename[1]; from != "" {
			runGit(t, dir, "mv", from, to)
			if s.editRename {
				appendLine(t, filepath.Join(dir, to))
				runGit(t, dir, "add", to)
			}
		}
		for _, p := range s.paths {
			if s.gitlink {
				runGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+base+","+p)
				continue
			}
			full := filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(body(p)+s.subject), 0o644); err != nil {
				t.Fatal(err)
			}
			runGit(t, dir, "add", p)
		}
		subject := s.subject
		if subject == "" {
			subject = "MIT-12: step " + string(rune('a'+i))
		}
		runGit(t, dir, "commit", "-q", "-m", subject)
		shas = append(shas, runGit(t, dir, "rev-parse", "HEAD"))
	}
	return dir, base, shas
}

func appendLine(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("an edited line\n"); err != nil {
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
	cases := []struct {
		name, title, author string
		steps               []step
		board               map[string]string
		wantCode            int
		// wantLine is one line the output must hold, with @N standing for the twelve-character sha of step N.
		wantLine string
	}{
		{"a person's test commit and then its code under an In Progress key passes", "MIT-12: fix", "octocat",
			[]step{{paths: []string{"api/run_test.go"}}, {paths: []string{"api/run.go"}}}, inProgress, 0, ""},
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
			"pr-delivery: mitstack-ci[bot] is a GitHub App on the keyless list, so the board is not asked about its keys"},
		{"Dependabot's keyless module bump passes and reads no board", "gomod(deps): bump the go-all group in /api", dependabotApp,
			[]step{{paths: []string{"api/go.mod", "api/go.sum"}, subject: "gomod(deps): bump the go-all group in /api"}}, map[string]string{}, 0,
			"pr-delivery: dependabot[bot] is a GitHub App on the keyless list, so the board is not asked about its keys"},
		{"a listed App's code before its test still fails", "chore: bump", dependabotApp,
			[]step{{paths: []string{"api/run.go"}, subject: "chore: bump"}, {paths: []string{"api/run_test.go"}, subject: "chore: test"}}, nil, 1,
			`pr-delivery: code came before its tests: commit @0 "chore: bump" changes code first: api/run.go`},
		{"an App off the list fails without a key", "chore: bump platform submodule", "renovate[bot]",
			[]step{{paths: []string{"platform"}, subject: "chore: bump platform submodule", gitlink: true}}, nil, 1,
			"pr-delivery: the title names no board key"},
		{"a person fails without a key", "chore: bump platform submodule", "octocat",
			[]step{{paths: []string{"platform"}, subject: "chore: bump platform submodule", gitlink: true}}, nil, 1,
			"pr-delivery: the title names no board key"},
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
			args := []string{"--repo", dir, "--base", base, "--head", shas[len(shas)-1], "--title", tc.title}

			var out bytes.Buffer
			code := Run(args, &out)

			want := tc.wantLine
			for i, sha := range shas {
				want = strings.ReplaceAll(want, "@"+string(rune('0'+i)), sha[:12])
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if code != tc.wantCode || (want != "" && !slices.Contains(lines, want)) {
				t.Errorf("Run(%q) = %d; want %d and the line %q\noutput:\n%s", args, code, tc.wantCode, want, out.String())
			}
			listedApp := tc.author == dependabotApp || tc.author == pinBumpApp
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
		var args []string
		for _, f := range [][2]string{{"--base", "a"}, {"--head", "b"}} {
			if f[0] != omit {
				args = append(args, f[:]...)
			}
		}

		var out bytes.Buffer
		code := Run(args, &out)

		if want := "pr-delivery: " + omit + " is required\n"; code != 2 || out.String() != want {
			t.Errorf("Run(%q) = %d, %q; want 2, %q", args, code, out.String(), want)
		}
	}
}
