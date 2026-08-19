package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/danceable/provider"
)

// ExitStatus represents a Posix exit status that a command expects to be returned to the shell.
type ExitStatus = int

const (
	// ExitSuccess is the exit status for a successful command.
	ExitSuccess ExitStatus = 0

	// ExitFailure is the exit status for a failed command.
	ExitFailure ExitStatus = 1

	// ExitUsageError is the exit status for a usage error.
	ExitUsageError ExitStatus = 2
)

const (
	// terminationDelay is the grace period before service providers are terminated.
	terminationDelay = 1 * time.Second

	// terminationDeadline is the maximum duration allowed for providers to terminate.
	terminationDeadline = 10 * time.Second
)

// Command represents a single command.
type Command interface {
	// Name returns the command's name.
	Name() string

	// Description returns a short string (less than one line) describing the command.
	Description() string

	// Usage returns a long string explaining the command and giving usage information.
	Usage() string

	// Configure configures this command. The arguments which follow the flags
	// of the command are available through the flag set's Args method.
	Configure(*FlagSet)

	// Run attempts to run the command.
	Run(context.Context) ExitStatus
}

// Service is an optional interface that a Command can implement to provide
// service providers whose lifecycle (register, boot and terminate) is managed
// by the danceable service provider manager. Boot is called once all providers
// have been booted, allowing the command to resolve its dependencies before Run.
type Service interface {
	// Providers returns the service providers required by the command.
	Providers() []provider.Provider

	// Boot resolves the command's dependencies from the booted container.
	Boot(ctx context.Context, container provider.Container) error
}

// Console represents a set of commands, which are optionally organized in
// groups and subgroups.
type Console struct {
	name string // normally path.Base(os.Args[0])
	root *Group

	writer    io.Writer // specifies where should write the regular output (normally os.Stdout).
	errWriter io.Writer // specifies where should write errors (normally os.Stderr).
	manager   *provider.Manager
}

// NewConsole returns a new Console.
//
// A requested help is written to writer, while everything which goes along with
// a non successful exit status (a usage error and the help which follows it, a
// failing service) is written to errWriter.
func NewConsole(name, description string, writer, errWriter io.Writer, manager *provider.Manager) *Console {
	if writer == nil {
		writer = os.Stdout
	}

	if errWriter == nil {
		errWriter = os.Stderr
	}

	return &Console{
		name:      name,
		root:      NewGroup(name, description),
		writer:    writer,
		errWriter: errWriter,
		manager:   manager,
	}
}

// Writer returns the writer the regular output is written to.
func (c *Console) Writer() io.Writer { return c.writer }

// ErrWriter returns the writer the errors are written to.
func (c *Console) ErrWriter() io.Writer { return c.errWriter }

// Register registers commands.
func (c *Console) Register(commands ...Command) *Console {
	c.root.Register(commands...)

	return c
}

// RegisterGroup registers groups of commands.
func (c *Console) RegisterGroup(groups ...*Group) *Console {
	c.root.RegisterGroup(groups...)

	return c
}

// Flags defines the global flags, which are provided before the name of the
// first group or command:
//
//	app --username=admin pods --all list
func (c *Console) Flags(configure func(*FlagSet)) *Console {
	c.root.Flags(configure)

	return c
}

// WithUsage overrides the usage line which is shown in the console's help.
func (c *Console) WithUsage(usage string) *Console {
	c.root.WithUsage(usage)

	return c
}

// WithHelp overrides the whole help of the console with the given text.
func (c *Console) WithHelp(help string) *Console {
	c.root.WithHelp(help)

	return c
}

// Run attempts to invoke registered commands.
//
// It walks through the groups the arguments point to, parsing the flags of each
// one of them on its way, until it reaches a command:
//
//	app --username=admin pods --all list --limit=10
func (c *Console) Run(ctx context.Context, arguments []string) ExitStatus {
	if len(arguments) > 0 {
		arguments = arguments[1:] // drops the binary's name.
	}

	group, path := c.root, []string{c.name}

	for {
		flagSet := group.flagSet(path, c.errWriter)
		flagSet.Usage = func() { group.explain(c.errWriter, path, flagSet) }

		if err := flagSet.Parse(arguments); err != nil {
			if errors.Is(err, ErrHelp) {
				group.explain(c.writer, path, flagSet)

				return ExitSuccess
			}

			return ExitUsageError
		}

		arguments = flagSet.Args()
		if len(arguments) == 0 {
			group.explain(c.errWriter, path, flagSet)

			return ExitUsageError
		}

		name := arguments[0]
		arguments = arguments[1:]

		if subgroup, exists := group.groups[name]; exists {
			group, path = subgroup, append(path, name)

			continue
		}

		command, exists := group.commands[name]
		if !exists {
			fmt.Fprintf(c.errWriter, "%q is not a command, See %q.\n", name, strings.Join(path, " ")+" --help")

			return ExitUsageError
		}

		return c.runCommand(ctx, command, append(path, name), arguments)
	}
}

// runCommand parses the command's flags and runs it.
func (c *Console) runCommand(ctx context.Context, command Command, path, arguments []string) ExitStatus {
	flagSet := NewFlagSet(strings.Join(path, " "), c.errWriter)
	flagSet.Usage = func() { explainCommand(c.errWriter, command, flagSet) }

	command.Configure(flagSet)

	if err := flagSet.Parse(arguments); err != nil {
		if errors.Is(err, ErrHelp) {
			explainCommand(c.writer, command, flagSet)

			return ExitSuccess
		}

		return ExitUsageError
	}

	if service, providesServices := command.(Service); providesServices {
		return c.runService(ctx, command, service)
	}

	return command.Run(ctx)
}

// runService registers the command's service providers on the manager, boots
// them, runs the command and finally terminates the providers gracefully.
func (c *Console) runService(ctx context.Context, cmd Command, service Service) ExitStatus {
	for _, p := range service.Providers() {
		c.manager.Register(p)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	exitStatus := make(chan ExitStatus, 1)

	err := c.manager.Run(
		runCtx,
		provider.WithTerminationDelay(terminationDelay),
		provider.WithTerminationDeadline(terminationDeadline),
		provider.WithCallback(func(callbackCtx context.Context, container provider.Container) {
			if err := service.Boot(callbackCtx, container); err != nil {
				fmt.Fprintln(c.errWriter, err)
				exitStatus <- ExitFailure
				cancel()
				return
			}

			exitStatus <- cmd.Run(callbackCtx)
			cancel()
		}),
	)
	if err != nil {
		fmt.Fprintln(c.errWriter, err)
		return ExitFailure
	}

	return <-exitStatus
}
