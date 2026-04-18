// Command poddies is the CLI entrypoint. It constructs the CLI App
// from the process environment and dispatches the cobra command tree.
package main

import (
	"fmt"
	"os"

	"github.com/andrewwormald/poddies/internal/cli"
)

// version is overridden at build time via -ldflags.
var version = "0.0.0-dev"

func main() {
	cli.Version = version
	app, err := cli.NewAppFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := app.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
