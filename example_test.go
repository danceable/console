package console_test

import (
	"context"
	"fmt"
	"os"

	"github.com/danceable/provider"

	"github.com/danceable/console"
)

// listCommand lists the pods of a cluster.
type listCommand struct {
	all   *bool
	limit int
}

func (c *listCommand) Name() string        { return "list" }
func (c *listCommand) Description() string { return "lists the pods." }
func (c *listCommand) Usage() string       { return "kubectl pods list [flags]" }

func (c *listCommand) Configure(flagSet *console.FlagSet) {
	flagSet.IntVar(
		&c.limit,
		10,
		"the maximum number of pods to show.",
		console.Long("limit"),
		console.Short("l"),
		console.Env("KUBECTL_LIMIT"),
	)
}

func (c *listCommand) Run(ctx context.Context) console.ExitStatus {
	fmt.Printf("listing at most %d pods (every namespace: %t)\n", c.limit, *c.all)

	return console.ExitSuccess
}

// kubectl builds a console which routes "kubectl pods list" to the list
// command, with a flag on the group and a flag on the command.
func kubectl(all *bool) *console.Console {
	pods := console.NewGroup("pods", "manages the pods.").
		Flags(func(flagSet *console.FlagSet) {
			flagSet.BoolVar(all, false, "targets the pods of every namespace.", console.Long("all"), console.Short("a"))
		}).
		Register(&listCommand{all: all})

	c := console.NewConsole("kubectl", "controls the cluster manager.", os.Stdout, os.Stderr, provider.Default)
	c.RegisterGroup(pods)

	return c
}

func Example() {
	var all bool

	// the flags of each level are parsed before the name of the next one.
	kubectl(&all).Run(context.Background(), []string{"kubectl", "pods", "--all", "list", "-l", "5"})

	// Output:
	// listing at most 5 pods (every namespace: true)
}

func Example_help() {
	var all bool

	// every group answers "--help" with its own help.
	kubectl(&all).Run(context.Background(), []string{"kubectl", "pods", "--help"})

	// Output:
	// manages the pods.
	//
	// Usage:
	//
	//   kubectl pods [flags] <command> [command arguments]
	//
	// The commands are:
	//
	//   list        lists the pods.
	//
	// Flags:
	//
	//   -a, --all   targets the pods of every namespace.
	//   -h, --help  shows this help message.
	//
	// Use "kubectl pods <command> --help" for more information about a command.
}
