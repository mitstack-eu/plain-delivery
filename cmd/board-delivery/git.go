package main

import (
	"strings"

	"github.com/mitstack-eu/plain-delivery/internal/gitcmd"
)

// anchorTag marks the last commit whose keys reached the board.
const anchorTag = "board-synced"

// Neither separator byte appears in ordinary commit text.
const (
	commitFieldSep  = "\x1f"
	commitRecordSep = "\x1e"
)

var gitOutput = gitcmd.Output

// readAnchorTag returns "" for an absent tag, which is a first run rather than an error.
func readAnchorTag(repo string) string {
	sha, err := gitOutput(repo, "rev-parse", "-q", "--verify", "refs/tags/"+anchorTag+"^{commit}")
	if err != nil {
		return ""
	}
	return sha
}

type commit struct {
	Subject string
	Body    string
}

func commitsInRange(repo, rng string) ([]commit, error) {
	format := "%s" + commitFieldSep + "%b" + commitRecordSep
	out, err := gitOutput(repo, "log", "--format="+format, rng, "--")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var commits []commit
	for _, rec := range strings.Split(out, commitRecordSep) {
		rec = strings.TrimPrefix(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, commitFieldSep, 2)
		c := commit{Subject: parts[0]}
		if len(parts) == 2 {
			c.Body = parts[1]
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// advanceAnchorTag runs only after a successful board write, since a tag moved past unrecorded commits would strand their keys.
func advanceAnchorTag(repo, remote, sha string) error {
	if _, err := gitOutput(repo, "tag", "-f", anchorTag, sha); err != nil {
		return err
	}
	if _, err := gitOutput(repo, "push", "-f", remote, "refs/tags/"+anchorTag); err != nil {
		return err
	}
	return nil
}
