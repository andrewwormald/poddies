// Command poddies is the CLI entrypoint. It constructs the CLI App
// from the process environment and dispatches the cobra command tree.
package main

import (
	"fmt"
	"os"

	"github.com/andrewwormald/poddies/internal/cli"

	// Blank import: registers the claude adapter in the global registry.
	// Add additional adapter imports here as they are implemented.
	_ "github.com/andrewwormald/poddies/internal/adapter/claude"
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
