package main

import (
	"fmt"
	"slices"

	"github.com/mitstack-eu/plain-delivery/boardkeys"
)

// claimedStatuses are the board statuses an issue holds between its claim and its delivery.
var claimedStatuses = []string{"In Progress", "Pull Request"}

// judgeOrder fails code that lands before the first test commit, a commit carrying both halves, and code with no test at all.
func judgeOrder(commits []commit) []string {
	var failures []string
	firstSeen, anyTest, anyCode := false, false, false
	for _, c := range commits {
		tests, code := c.pathsOf(classTest), c.pathsOf(classCode)
		anyTest = anyTest || tests != ""
		anyCode = anyCode || code != ""
		if !firstSeen && (tests != "" || code != "") {
			firstSeen = true
			if tests == "" {
				failures = append(failures, fmt.Sprintf("code came before its tests: %s changes code first: %s", c.label(), code))
			}
		}
		if tests != "" && code != "" {
			failures = append(failures, fmt.Sprintf("%s changes a test and code together: tests %s; code %s", c.label(), tests, code))
		}
	}
	if anyCode && !anyTest {
		return []string{"the code this pull request changes has no test, since no commit changes a test file"}
	}
	return failures
}

// statusReader answers each key's board status, leaving out a key the board does not hold.
type statusReader func(keys []string) (map[string]string, error)

// judgeKeys fails a title or a range that names no key, since a squash lands the title and a rebase the commits, and a key the board does not hold as claimed.
func judgeKeys(title string, commits []commit, read statusReader) []string {
	var failures []string
	titleKeys := boardkeys.Named(title, "")
	if len(titleKeys) == 0 {
		failures = append(failures, "the title names no board key")
	}
	var commitKeys []string
	for _, c := range commits {
		commitKeys = boardkeys.Merge(commitKeys, boardkeys.Named(c.Subject, c.Body))
	}
	if len(commitKeys) == 0 {
		failures = append(failures, "no commit names a board key")
	}
	keys := boardkeys.Merge(titleKeys, commitKeys)
	if len(keys) == 0 {
		return failures
	}
	statuses, err := read(keys)
	if err != nil {
		return append(failures, err.Error())
	}
	for _, k := range keys {
		status, held := statuses[k]
		switch {
		case !held:
			failures = append(failures, k+" is not an issue on the board")
		case !slices.Contains(claimedStatuses, status):
			failures = append(failures, fmt.Sprintf("%s is %s on the board, and a pull request may name only an issue that is In Progress or Pull Request", k, status))
		}
	}
	return failures
}
