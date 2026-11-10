package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mitstack-eu/plain-delivery/internal/gitcmd"
)

// shortSHA is how many hex characters name a commit in a message.
const shortSHA = 12

// gitlinkMode is the tree mode git gives a submodule's pinned commit.
const gitlinkMode = "160000"

// A pure rename moves no assertion, so it counts as neither half.
const pureRename = "R100"

type commit struct {
	SHA, Subject, Body string
	// Paths maps each path the commit changes to its class.
	Paths map[string]pathClass
}

func (c commit) label() string {
	return fmt.Sprintf("commit %s %q", c.SHA[:shortSHA], c.Subject)
}

// pathsOf lists the commit's paths of one class, sorted, as a comma-separated string.
func (c commit) pathsOf(class pathClass) string {
	var out []string
	for p, cl := range c.Paths {
		if cl == class {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return strings.Join(out, ", ")
}

var gitOutput = gitcmd.Output

// readCommits reads merge-base(base, head)..head oldest first, leaving out merge commits, whose diff is two branches' union rather than one author's commit.
func readCommits(repo, base, head string) ([]commit, error) {
	mergeBase, err := gitOutput(repo, "merge-base", base, head)
	if err != nil {
		return nil, err
	}
	list, err := gitOutput(repo, "rev-list", "--reverse", "--no-merges", strings.TrimSpace(mergeBase)+".."+head)
	if err != nil {
		return nil, err
	}
	var commits []commit
	for _, sha := range strings.Fields(list) {
		c, err := readCommit(repo, sha)
		if err != nil {
			return nil, err
		}
		commits = append(commits, c)
	}
	return commits, nil
}

func readCommit(repo, sha string) (commit, error) {
	msg, err := gitOutput(repo, "show", "-s", "--format=%s%x00%b", sha)
	if err != nil {
		return commit{}, err
	}
	subject, body, _ := strings.Cut(msg, "\x00")
	raw, err := gitOutput(repo, "diff-tree", "--no-commit-id", "-r", "-M", "--raw", "-z", sha)
	if err != nil {
		return commit{}, err
	}
	paths := classifyRaw(raw)
	for p, class := range paths {
		if class != classCode || !isWorkflow(p) {
			continue
		}
		patch, err := gitOutput(repo, "diff-tree", "--no-commit-id", "-p", "-U0", "--no-color", sha, "--", p)
		if err != nil {
			return commit{}, err
		}
		paths[p] = classifyPatch(p, patch)
	}
	return commit{SHA: sha, Subject: subject, Body: body, Paths: paths}, nil
}

// classifyRaw reads `git diff-tree --raw -z`, whose -z keeps an unusual path unquoted: a ":<old mode> <new mode> <old sha> <new sha> <status>" field, then the path, or the old and new path for a rename or copy.
func classifyRaw(raw string) map[string]pathClass {
	paths := map[string]pathClass{}
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); i++ {
		meta := strings.Fields(strings.TrimPrefix(fields[i], ":"))
		if len(meta) != 5 || i+1 >= len(fields) {
			continue
		}
		oldMode, newMode, status := meta[0], meta[1], meta[4]
		i++
		p := fields[i]
		if status[0] == 'R' || status[0] == 'C' {
			if i+1 >= len(fields) {
				break
			}
			i++
			p = fields[i]
		}
		if status == pureRename {
			continue
		}
		paths[p] = classify(p, newMode == gitlinkMode || oldMode == gitlinkMode)
	}
	return paths
}
