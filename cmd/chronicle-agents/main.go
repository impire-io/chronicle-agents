// Command chronicle-agents is the CLI: declare a workspace's types, serve an
// agent from its definition file. Thin main; the logic is in cli.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/impire-io/chronicle-agents/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := cli.Run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "chronicle-agents:", err)
		os.Exit(1)
	}
}
