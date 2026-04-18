package main

import (
	"fmt"
	"io"
	"os"
)

// version is the current poddies version. Overridden at build time via -ldflags.
var version = "0.0.0-dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "version" || args[0] == "--version" || args[0] == "-v" {
		_, err := fmt.Fprintf(out, "poddies %s\n", version)
		return err
	}
	return fmt.Errorf("unknown command: %s", args[0])
}
