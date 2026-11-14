// Command dstate supervises long-running processes from a declarative config.
package main

import (
	"fmt"
	"os"

	"github.com/akamstar/durastate/internal/cli"
)

// Set by the linker: -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version string
	commit  string
	date    string
)

func main() {
	root := cli.NewRootCmd(cli.BuildInfo{Version: version, Commit: commit, Date: date})
	err := root.Execute()
	if err != nil && !cli.Silent(err) {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(cli.ExitCode(err))
}
