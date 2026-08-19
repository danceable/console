package console

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// Group represents a set of commands and subgroups which are reachable through
// a common name, share the group's flags and have their own help.
//
// A group is not runnable by itself, it only routes to the command (or to the
// subgroup) which is named right after it:
//
//	app --username=admin pods --all list
//	    └ app flags      └ group
//	                          └ group flags
//	                                 └ command
type Group struct {
	name        string
	description string
	usage       string
	help        string
	configure   func(*FlagSet)

	commands map[string]Command
	groups   map[string]*Group
}

// NewGroup returns a new group of commands.
func NewGroup(name, description string) *Group {
	return &Group{
		name:        name,
		description: description,
		commands:    make(map[string]Command),
		groups:      make(map[string]*Group),
	}
}

// Name returns the group's name, which is used to identify it.
func (g *Group) Name() string { return g.name }

// Description returns a short string (less than one line) describing the group.
func (g *Group) Description() string { return g.description }

// Usage returns the usage line of the group.
func (g *Group) Usage() string { return g.usage }

// WithUsage overrides the usage line which is shown in the group's help.
func (g *Group) WithUsage(usage string) *Group {
	g.usage = usage

	return g
}

// WithHelp overrides the whole help of the group with the given text. It lets a
// group document itself instead of relying on the generated help.
func (g *Group) WithHelp(help string) *Group {
	g.help = help

	return g
}

// Flags defines the flags of the group, which are provided right after the
// group's name and before the name of the subgroup or the command:
//
//	app pods --all list
//
// The flags of a group are parsed before the ones of its subgroups and
// commands, so a command can rely on them being already loaded.
func (g *Group) Flags(configure func(*FlagSet)) *Group {
	g.configure = configure

	return g
}

// Register registers commands in the group.
func (g *Group) Register(commands ...Command) *Group {
	for _, command := range commands {
		name := command.Name()

		g.claim(name)
		g.commands[name] = command
	}

	return g
}

// RegisterGroup registers subgroups in the group.
func (g *Group) RegisterGroup(groups ...*Group) *Group {
	for _, group := range groups {
		name := group.Name()

		g.claim(name)
		g.groups[name] = group
	}

	return g
}

// Commands returns the names of the registered commands in alphabetical order.
func (g *Group) Commands() []string { return sorted(g.commands) }

// Groups returns the names of the registered subgroups in alphabetical order.
func (g *Group) Groups() []string { return sorted(g.groups) }

// claim insures a name is used by a single command or subgroup.
func (g *Group) claim(name string) {
	if name == "" {
		panic(fmt.Sprintf("console: %s: a command or a group needs a name", g.name))
	}

	if _, exists := g.commands[name]; exists {
		panic(fmt.Sprintf("console: %s: command %q is registered twice", g.name, name))
	}

	if _, exists := g.groups[name]; exists {
		panic(fmt.Sprintf("console: %s: group %q is registered twice", g.name, name))
	}
}

// flagSet returns the flag set of the group at the given path.
func (g *Group) flagSet(path []string, errWriter io.Writer) *FlagSet {
	flagSet := NewFlagSet(strings.Join(path, " "), errWriter)

	if g.configure != nil {
		g.configure(flagSet)
	}

	return flagSet
}

// explain writes the help of the group.
func (g *Group) explain(w io.Writer, path []string, flagSet *FlagSet) {
	if g.help != "" {
		fmt.Fprintln(w, g.help)

		return
	}

	name := strings.Join(path, " ")

	var b strings.Builder

	fmt.Fprintf(&b, "%s\n\nUsage:\n", g.description)

	if g.usage != "" {
		fmt.Fprintf(&b, "\n  %s\n", g.usage)
	} else {
		fmt.Fprintf(&b, "\n  %s %s\n", name, "[flags] <command> [command arguments]")
	}

	if len(g.groups) > 0 {
		fmt.Fprint(&b, "\nThe command groups are:\n\n")
		writeList(&b, g.groups, func(group *Group) string { return group.Description() })
	}

	// a group which only routes to subgroups has no commands to list.
	if len(g.commands) > 0 || len(g.groups) == 0 {
		fmt.Fprint(&b, "\nThe commands are:\n\n")
		writeList(&b, g.commands, func(command Command) string { return command.Description() })
	}

	fmt.Fprint(&b, "\nFlags:\n\n")
	flagSet.PrintDefaults(&b)

	fmt.Fprintf(&b, "\nUse \"%s <command> --help\" for more information about a command.\n", name)

	fmt.Fprint(w, b.String())
}

// explainCommand writes the help of a single command.
func explainCommand(w io.Writer, command Command, flagSet *FlagSet) {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n\nUsage:\n", command.Description())
	fmt.Fprintf(&b, "\n  %s\n", command.Usage())
	fmt.Fprint(&b, "\nFlags:\n\n")
	flagSet.PrintDefaults(&b)

	fmt.Fprint(w, b.String())
}

// writeList writes an aligned "name  description" list of the given items.
func writeList[T any](w io.Writer, items map[string]T, describe func(T) string) {
	width := 10
	for name := range items {
		if len(name) > width {
			width = len(name)
		}
	}

	for _, name := range sorted(items) {
		fmt.Fprintf(w, "  %-*s  %s\n", width, name, describe(items[name]))
	}
}

// sorted returns the keys of a map in alphabetical order.
func sorted[T any](items map[string]T) []string {
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}
