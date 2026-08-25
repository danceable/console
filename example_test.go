package console_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

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
	console.Var(
		flagSet,
		&c.limit,
		console.Long("limit"),
		"the maximum number of pods to show.",
		console.Short("l"),
		console.Env("KUBECTL_LIMIT"),
		console.Default(10),
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
			console.Var(flagSet, all, console.Long("all"), "targets the pods of every namespace.", console.Short("a"))
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

// contextScope holds the value of a "--context" flag defined at one level of
// the console, together with the flag set it belongs to. Keeping the flag set
// is what lets a command tell an explicitly provided value apart from a
// default one, as the console builds a new flag set for every level and drops
// it once the level is parsed.
type contextScope struct {
	context string
	flagSet *console.FlagSet
}

// bind defines the "--context" flag of the level on the given flag set. Every
// level binds its own variable: two levels sharing one would overwrite each
// other, as a flag writes its default value when it is defined.
func (s *contextScope) bind(flagSet *console.FlagSet, defaultContext string) {
	s.flagSet = flagSet

	console.Var(flagSet, &s.context, console.Long("context"), "the context to work against.", console.Default(defaultContext))
}

// provided reports whether the flag was provided at this level.
func (s *contextScope) provided() bool {
	return s.flagSet != nil && s.flagSet.Lookup("context").Provided()
}

// given returns the value the level was provided with, or "-" when the level
// fell back to its default.
func (s *contextScope) given() string {
	if !s.provided() {
		return "-"
	}

	return s.context
}

// podsCommand resolves the context it runs against out of the three levels
// which define one.
type podsCommand struct {
	root  *contextScope
	group *contextScope
	own   *contextScope
}

func (c *podsCommand) Name() string        { return "pods" }
func (c *podsCommand) Description() string { return "gets the pods." }
func (c *podsCommand) Usage() string       { return "kubectl get pods [flags]" }

func (c *podsCommand) Configure(flagSet *console.FlagSet) {
	c.own.bind(flagSet, "minikube")
}

func (c *podsCommand) Run(ctx context.Context) console.ExitStatus {
	// the deepest level which was explicitly provided wins, the root's value
	// applies when none of the deeper ones was.
	effective := c.root.context

	for _, scope := range []*contextScope{c.group, c.own} {
		if scope.provided() {
			effective = scope.context
		}
	}

	fmt.Printf("kubectl=%s get=%s pods=%s => %s\n", c.root.given(), c.group.given(), c.own.given(), effective)

	return console.ExitSuccess
}

// kubectlContexts builds a console where the root, the "get" group and the
// "pods" command each define their own "--context" flag.
func kubectlContexts() *console.Console {
	root, group, own := &contextScope{}, &contextScope{}, &contextScope{}

	get := console.NewGroup("get", "gets resources.").
		Flags(func(flagSet *console.FlagSet) { group.bind(flagSet, "staging") }).
		Register(&podsCommand{root: root, group: group, own: own})

	c := console.NewConsole("kubectl", "controls the cluster manager.", os.Stdout, os.Stderr, provider.Default)
	c.Flags(func(flagSet *console.FlagSet) { root.bind(flagSet, "default") })
	c.RegisterGroup(get)

	return c
}

func Example_scopedFlags() {
	// a flag set is built per level, so the same name may be defined at each
	// one of them without colliding: the three values live side by side.
	kubectlContexts().Run(context.Background(), []string{
		"kubectl", "--context=server-1", "get", "--context=staging", "pods", "--context=running",
	})

	// the levels which are not provided fall back to the closest one which was.
	kubectlContexts().Run(context.Background(), []string{
		"kubectl", "--context=server-1", "get", "pods",
	})

	// a group may be provided while the command it routes to is not.
	kubectlContexts().Run(context.Background(), []string{
		"kubectl", "get", "--context=staging", "pods",
	})

	// without any of them, the root's default applies.
	kubectlContexts().Run(context.Background(), []string{"kubectl", "get", "pods"})

	// Output:
	// kubectl=server-1 get=staging pods=running => running
	// kubectl=server-1 get=- pods=- => server-1
	// kubectl=- get=staging pods=- => staging
	// kubectl=- get=- pods=- => default
}

// logLevel is a type of its own, whose flags are defined by registering how it
// is parsed rather than by writing a value for it.
type logLevel int

const (
	debugLevel logLevel = iota
	infoLevel
	errorLevel
)

// logLevels names the levels, in the order they are defined in.
var logLevels = []string{"debug", "info", "error"}

func (l logLevel) String() string { return logLevels[l] }

// parseLogLevel is the parser the level type is registered with.
func parseLogLevel(argument string) (logLevel, error) {
	if level := slices.Index(logLevels, argument); level >= 0 {
		return logLevel(level), nil
	}

	return 0, errors.New("unknown level")
}

// Registering a type is what makes it usable as a flag: the parser is handed
// the argument, and the flag set writes what it returns into the variable the
// flag was defined with.
func ExampleRegister() {
	console.Register(console.Type[logLevel]{
		Name:    "level",
		Expects: "a level (debug, info or error)",
		Parse:   parseLogLevel,
	})

	var level logLevel

	flagSet := console.NewFlagSet("app", os.Stdout)
	console.Var(flagSet, &level, console.Long("level"), "the level to log at.", console.Short("l"), console.Default(infoLevel))

	if err := flagSet.Parse([]string{"-l", "debug"}); err == nil {
		fmt.Println("logging at the", level, "level")
	}

	// a value the parser rejects is explained with what the type expects, and
	// leaves the flag with the value it already held.
	flagSet.Parse([]string{"--level=loud"})

	// the type names itself in the help, next to the default value of the flag.
	flagSet.PrintDefaults(os.Stdout)

	// Output:
	// logging at the debug level
	// invalid value "loud" for flag --level: can't be parsed as a level (debug, info or error)
	//   -l, --level level  the level to log at. (default info)
	//   -h, --help         shows this help message.
}
