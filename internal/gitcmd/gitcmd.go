// Package gitcmd runs git for this module's commands, the one place their git calls go through.
package gitcmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Output runs git with args in repo and returns its trimmed stdout, keeping stderr apart so a warning never reaches a parser.
func Output(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
