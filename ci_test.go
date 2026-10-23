package plaindelivery

import (
	"os"
	"strings"
	"testing"
)

// ciJobs returns each job of ci.yml as its uncommented lines, trimmed and joined, keyed by the job's id.
func ciJobs(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	jobs, id, inJobs := map[string]string{}, "", false
	for _, line := range strings.Split(string(raw), "\n") {
		text := strings.TrimPrefix(strings.TrimSpace(line), "- ")
		switch {
		case line == "jobs:":
			inJobs = true
		case !inJobs || text == "" || strings.HasPrefix(text, "#"):
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   "):
			id = strings.TrimSuffix(text, ":")
		default:
			jobs[id] += "\n" + text
		}
	}
	return jobs
}

func TestCI_GatesPullRequestsAndRecordsDeliveriesWithThisRepositorysOwnCommands(t *testing.T) {
	jobs := ciJobs(t)
	for _, tc := range []struct{ job, line string }{
		{"test", "run: go vet ./..."},
		{"test", "run: go test ./..."},
		{"pr-key-gate", "if: github.event_name == 'pull_request'"},
		{"pr-key-gate", "fetch-depth: 0"},
		{"pr-key-gate", "PR_TITLE: ${{ github.event.pull_request.title }}"},
		{"pr-key-gate", `run: go run ./cmd/pr-keys --repo . --base "$BASE_SHA" --head "$HEAD_SHA"`},
		{"record-delivery", "if: (github.event_name == 'push' || github.event_name == 'workflow_dispatch') && github.ref == 'refs/heads/main'"},
		{"record-delivery", "needs: test"},
		{"record-delivery", "fetch-depth: 0"},
		{"record-delivery", "contents: write"},
		{"record-delivery", "group: record-delivery"},
		{"record-delivery", "cancel-in-progress: false"},
		{"record-delivery", "TRACKER_BASE_URL: ${{ vars.TRACKER_BASE_URL }}"},
		{"record-delivery", "TRACKER_API_TOKEN: ${{ secrets.TRACKER_API_TOKEN }}"},
		{"record-delivery", `run: go run ./cmd/board-delivery --repo . --before "$BEFORE_SHA" --after "$GITHUB_SHA"`},
	} {
		if !strings.Contains(jobs[tc.job]+"\n", "\n"+tc.line+"\n") {
			t.Errorf("the %s job has no line %q", tc.job, tc.line)
		}
	}
	for _, tc := range []struct{ job, banned string }{
		{"pr-key-gate", "secrets."},
		{"pr-key-gate", "continue-on-error"},
		{"record-delivery", "continue-on-error"},
	} {
		if strings.Contains(jobs[tc.job], tc.banned) {
			t.Errorf("the %s job names %q", tc.job, tc.banned)
		}
	}
}
