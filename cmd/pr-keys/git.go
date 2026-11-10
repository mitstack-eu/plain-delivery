package main

import (
	"strings"

	"github.com/mitstack-eu/plain-delivery/internal/gitcmd"
)

// Neither separator byte appears in ordinary commit text.
const (
	commitFieldSep  = "\x1f"
	commitRecordSep = "\x1e"
)

var gitOutput = gitcmd.Output

// commitsInRange reads every commit's subject and body in rng.
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
