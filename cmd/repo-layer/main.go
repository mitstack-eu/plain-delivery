// Command repo-layer, part of mitstack's own tooling, prints the layer a repository belongs to and the source that decided it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mitstack-eu/plain-delivery/layer"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout))
}

// Run prints "<layer>: <source>" for --repository, reading its root manifest from --manifest when given, and returns 2 on a bad flag or name, 1 on an unplaced repository or unreadable manifest and 0 otherwise.
func Run(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("repo-layer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repo := fs.String("repository", "", "the repository, as owner/name")
	manifestPath := fs.String("manifest", "", "the repository's root manifest.yaml, omitted when it has none")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stdout, "repo-layer:", err)
		return 2
	}
	if *repo == "" {
		fmt.Fprintln(stdout, "repo-layer: --repository is required")
		return 2
	}

	var manifest []byte
	if *manifestPath != "" {
		var err error
		if manifest, err = os.ReadFile(*manifestPath); err != nil {
			fmt.Fprintf(stdout, "repo-layer: reading manifest %s: %v\n", *manifestPath, err)
			return 1
		}
	}

	verdict, err := layer.Of(*repo, manifest)
	switch {
	case errors.Is(err, layer.ErrRepositoryName):
		fmt.Fprintln(stdout, "repo-layer:", err)
		return 2
	case err != nil:
		fmt.Fprintln(stdout, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %s\n", verdict.Layer, verdict.Source)
	return 0
}
