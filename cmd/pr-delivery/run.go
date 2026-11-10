// Command pr-delivery, part of mitstack's own tooling, fails a pull request whose code lands before its tests or whose board keys are not claimed issues.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mitstack-eu/block-kit/safe"
)

// appAuthor is the account type GitHub gives a pull request a GitHub App opened, such as Dependabot or a pin bump, which no board issue stands behind.
const appAuthor = "Bot"

// Run judges the commits in merge-base(--base, --head)..--head and --title, writing every message to stdout and returning 2 on a bad flag, 1 on a failed check and 0 on a pass.
func Run(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("pr-delivery", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repo := fs.String("repo", ".", "the git repository to read commits from")
	base := fs.String("base", "", "the pull request's base sha")
	head := fs.String("head", "", "the pull request's head sha")
	title := fs.String("title", "", "the pull request's title (default: the PR_TITLE environment variable)")
	authorType := fs.String("author-type", "", "the pull request author's GitHub account type (default: the PR_AUTHOR_TYPE environment variable)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stdout, err)
		return 2
	}
	for _, f := range [][2]string{{"--base", *base}, {"--head", *head}} {
		if f[1] == "" {
			fmt.Fprintf(stdout, "pr-delivery: %s is required\n", f[0])
			return 2
		}
	}
	*title = valueOrEnv(*title, "PR_TITLE")
	*authorType = valueOrEnv(*authorType, "PR_AUTHOR_TYPE")

	commits, err := readCommits(*repo, *base, *head)
	if err != nil {
		fmt.Fprintf(stdout, "pr-delivery: reading the commits from %s to %s: %v\n", *base, *head, err)
		return 1
	}
	failures := judgeOrder(commits)
	token := os.Getenv("TRACKER_API_TOKEN")
	if *authorType == appAuthor {
		fmt.Fprintln(stdout, "pr-delivery: the author is a GitHub App, so the board is not asked about its keys")
	} else {
		failures = append(failures, judgeKeys(*title, commits, boardStatuses(os.Getenv("TRACKER_BASE_URL"), token))...)
	}
	for _, f := range failures {
		fmt.Fprintln(stdout, "pr-delivery: "+safe.LogSafe(f, token))
	}
	if len(failures) > 0 {
		fmt.Fprintln(stdout, "pr-delivery: commit the failing tests first and the code that passes them after, each alone, and name a claimed issue in the title and a commit as \"MIT-###:\" or \"(refs MIT-###)\"")
		return 1
	}
	fmt.Fprintf(stdout, "pr-delivery: %d commit(s) judged: the tests came first and every key is claimed\n", len(commits))
	return 0
}

func valueOrEnv(value, env string) string {
	if value != "" {
		return value
	}
	return os.Getenv(env)
}
