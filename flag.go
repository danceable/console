package console

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// ErrHelp is the error returned when the help flag (-h or --help) is provided
// but no such flag is defined.
var ErrHelp = errors.New("console: help requested")

// Value is the interface to the dynamic value stored in a flag.
type Value interface {
	// String returns the flag's value as a string.
	String() string

	// Set sets the flag's value from its string representation.
	Set(string) error
}

// implicitValuer is implemented by values that may be provided without an
// explicit value, e.g. "--verbose" instead of "--verbose=true", and that name
// the value they take then.
type implicitValuer interface {
	ImplicitValue() (string, bool)
}

// boolValuer is the boolean only form of implicitValuer, which a value of your
// own may implement instead.
type boolValuer interface {
	IsBoolFlag() bool
}

// typer is implemented by values that name their own type in the help output.
type typer interface {
	Type() string
}

// zeroer is implemented by values that know the string form of the zero value
// of their type, which the help doesn't mention as a default.
type zeroer interface {
	Zero() string
}

// Flag represents the state of a single flag.
//
// A flag is identified by any combination of a long name (--flag), a short name
// (-f) and an environment variable name. Each of them is optional, but at least
// one of them has to be defined. Only the defined ones are enabled: a flag
// without a short name can't be provided as "-f", and a flag without an
// environment variable name never looks at the environment.
type Flag struct {
	long     string
	short    string
	env      string
	usage    string
	value    Value
	defValue string

	provided bool
	fromEnv  bool
}

// Long returns the flag's long name (without the leading "--"), if any.
func (f *Flag) Long() string { return f.long }

// Short returns the flag's short name (without the leading "-"), if any.
func (f *Flag) Short() string { return f.short }

// Env returns the name of the environment variable the flag falls back to, if any.
func (f *Flag) Env() string { return f.env }

// Usage returns the flag's usage message.
func (f *Flag) Usage() string { return f.usage }

// Value returns the flag's value.
func (f *Flag) Value() Value { return f.value }

// DefValue returns the flag's default value as a string.
func (f *Flag) DefValue() string { return f.defValue }

// Provided reports whether the flag was provided on the command line.
func (f *Flag) Provided() bool { return f.provided }

// FromEnv reports whether the flag's value was loaded from the environment.
func (f *Flag) FromEnv() bool { return f.fromEnv }

// FlagOption defines an optional property of a flag, such as the value it
// defaults to or the other names it answers to.
type FlagOption interface {
	apply(*Flag)
}

// FlagName is a name a flag is reachable by: a long name, a short name or an
// environment variable. A flag which defines none of them can never be
// provided, which is why [Var] and [FlagSet.Var] take one apart from their
// options: the first name is what defines a flag, the other ones are optional.
type FlagName interface {
	FlagOption

	// name tells a name from the other options of a flag.
	name()
}

// flagOption turns a function into a FlagOption.
type flagOption func(*Flag)

func (o flagOption) apply(flag *Flag) { o(flag) }

// flagName turns a function into a FlagName.
type flagName func(*Flag)

func (n flagName) apply(flag *Flag) { n(flag) }
func (n flagName) name()            {}

// Long defines the long name of a flag, which is provided as "--name".
func Long(name string) FlagName {
	return flagName(func(f *Flag) { f.long = name })
}

// Short defines the short (single character) name of a flag, which is provided
// as "-n".
func Short(name string) FlagName {
	return flagName(func(f *Flag) { f.short = name })
}

// Env defines the environment variable a flag falls back to when it is not
// provided on the command line. Empty environment variables are ignored.
func Env(name string) FlagName {
	return flagName(func(f *Flag) { f.env = name })
}

// FlagSet represents a set of defined flags.
type FlagSet struct {
	// Usage is called when an error occurs while parsing flags. It is meant to
	// print the help of the command the flag set belongs to.
	Usage func()

	name string // the full path of the command, e.g. "kubectl pods list".

	// errWriter is where the parsing errors are written to. A flag set never
	// writes anything else, the help is written by the console.
	errWriter io.Writer

	flags []*Flag // in definition order.
	long  map[string]*Flag
	short map[string]*Flag

	args []string
}

// NewFlagSet returns a new flag set which writes its errors to the given writer.
func NewFlagSet(name string, errWriter io.Writer) *FlagSet {
	if errWriter == nil {
		errWriter = os.Stderr
	}

	return &FlagSet{
		name:      name,
		errWriter: errWriter,
		long:      make(map[string]*Flag),
		short:     make(map[string]*Flag),
	}
}

// Name returns the name of the flag set.
func (f *FlagSet) Name() string { return f.name }

// ErrWriter returns the writer the parsing errors are written to.
func (f *FlagSet) ErrWriter() io.Writer { return f.errWriter }

// SetErrWriter sets the writer the parsing errors are written to.
func (f *FlagSet) SetErrWriter(errWriter io.Writer) { f.errWriter = errWriter }

// Flags returns the defined flags in definition order.
func (f *FlagSet) Flags() []*Flag { return f.flags }

// Lookup returns the flag defined with the given long, short or environment
// variable name, or nil when no such flag is defined.
func (f *FlagSet) Lookup(name string) *Flag {
	if flag, ok := f.long[name]; ok {
		return flag
	}

	if flag, ok := f.short[name]; ok {
		return flag
	}

	for _, flag := range f.flags {
		if flag.env != "" && flag.env == name {
			return flag
		}
	}

	return nil
}

// Args returns the non-flag arguments. Flag parsing stops right before the
// first non-flag argument, which lets the arguments of subgroups, subcommands
// and their own flags stay untouched.
func (f *FlagSet) Args() []string { return f.args }

// Arg returns the i'th non-flag argument, or an empty string when it doesn't exist.
func (f *FlagSet) Arg(i int) string {
	if i < 0 || i >= len(f.args) {
		return ""
	}

	return f.args[i]
}

// NArg returns the number of non-flag arguments.
func (f *FlagSet) NArg() int { return len(f.args) }

// Var defines a flag with the given value, name and usage. The name is what the
// flag is provided by, and the other names it answers to are given as options:
//
//	flagSet.Var(&level, console.Long("level"), "the level to log at.", console.Short("l"), console.Env("LOG_LEVEL"))
func (f *FlagSet) Var(value Value, name FlagName, usage string, options ...FlagOption) *Flag {
	flag := &Flag{usage: usage, value: value}

	if name != nil {
		name.apply(flag)
	}

	for _, option := range options {
		option.apply(flag)
	}

	// the options have their say before the default value is read, as a
	// Default option writes it into the variable the flag is bound to.
	flag.defValue = value.String()

	f.define(flag)

	return flag
}

// Var defines a flag of the type of the variable it stores its value in, which
// is where the parsed value is written:
//
//	var port int
//
//	console.Var(flagSet, &port, console.Long("port"), "the port to listen to.", console.Short("p"), console.Default(80))
//
// A flag is defined by the name it is provided by, which is a [Long] name, a
// [Short] name or an [Env] variable. The other names it answers to are given
// as options, along with the value it defaults to.
//
// The type of the variable has to be registered, which the Go types are by the
// package itself. [Register] is what a type of your own is made usable by, and
// a variable whose pointer implements [Value] carries its own parsing.
//
// The flag defaults to the value the variable already holds, which a [Default]
// option overrides.
//
// It panics when no flag can be defined for the type of the variable, as a
// definition error is a programming mistake rather than user input.
func Var[T any](flagSet *FlagSet, p *T, name FlagName, usage string, options ...FlagOption) *Flag {
	flagValue, err := valueOf(p)
	if err != nil {
		panic(fmt.Sprintf("console: %s: %s", flagSet.name, err))
	}

	return flagSet.Var(flagValue, name, usage, options...)
}

// define validates and registers a flag. It panics on definition errors, as
// they are programming mistakes rather than user input errors.
func (f *FlagSet) define(flag *Flag) {
	switch {
	case flag.long == "" && flag.short == "" && flag.env == "":
		panic(fmt.Sprintf("console: %s: a flag needs at least one of a long, short or env name", f.name))
	case strings.HasPrefix(flag.long, "-") || strings.Contains(flag.long, "="):
		panic(fmt.Sprintf("console: %s: invalid long flag name %q", f.name, flag.long))
	case flag.short != "" && len(flag.short) != 1:
		panic(fmt.Sprintf("console: %s: short flag name %q must be a single character", f.name, flag.short))
	case flag.short == "-" || flag.short == "=":
		panic(fmt.Sprintf("console: %s: invalid short flag name %q", f.name, flag.short))
	}

	if flag.long != "" {
		if _, exists := f.long[flag.long]; exists {
			panic(fmt.Sprintf("console: %s: flag --%s is defined twice", f.name, flag.long))
		}

		f.long[flag.long] = flag
	}

	if flag.short != "" {
		if _, exists := f.short[flag.short]; exists {
			panic(fmt.Sprintf("console: %s: flag -%s is defined twice", f.name, flag.short))
		}

		f.short[flag.short] = flag
	}

	f.flags = append(f.flags, flag)
}

// Parse parses flags from the given arguments. Parsing stops at the first
// non-flag argument or at "--", and the remaining arguments are available
// through Args. Flags which are not provided fall back to their environment
// variable, when they define one.
func (f *FlagSet) Parse(arguments []string) error {
	f.args = nil

	var (
		i   int
		err error
	)

	for i < len(arguments) {
		argument := arguments[i]

		// everything that is not a flag terminates the flag parsing.
		if len(argument) < 2 || argument[0] != '-' {
			break
		}

		// "--" explicitly terminates the flag parsing.
		if argument == "--" {
			i++
			break
		}

		i++

		if strings.HasPrefix(argument, "--") {
			i, err = f.parseLong(argument[2:], arguments, i)
		} else {
			i, err = f.parseShort(argument[1:], arguments, i)
		}

		if err != nil {
			return f.fail(err)
		}
	}

	f.args = arguments[i:]

	return f.parseEnv()
}

// parseLong parses a "--flag", "--flag=value" or "--flag value" argument and
// returns the index of the next argument to parse.
func (f *FlagSet) parseLong(argument string, arguments []string, i int) (int, error) {
	name, value, hasValue := strings.Cut(argument, "=")

	flag, defined := f.long[name]
	if !defined {
		if name == "help" {
			return i, ErrHelp
		}

		return i, fmt.Errorf("flag provided but not defined: --%s", name)
	}

	if !hasValue {
		switch implicit, takesNoValue := implicitValue(flag.value); {
		case takesNoValue:
			value = implicit
		case i < len(arguments):
			value, i = arguments[i], i+1
		default:
			return i, fmt.Errorf("flag needs an argument: --%s", name)
		}
	}

	if err := flag.value.Set(value); err != nil {
		return i, fmt.Errorf("invalid value %q for flag --%s: %w", value, name, err)
	}

	flag.provided = true

	return i, nil
}

// parseShort parses a "-f", "-f=value", "-fvalue", "-f value" or a combination
// of boolean short flags like "-abc", and returns the index of the next
// argument to parse.
func (f *FlagSet) parseShort(argument string, arguments []string, i int) (int, error) {
	// a long name provided with a single dash, e.g. "-port" instead of "--port".
	if name, _, _ := strings.Cut(argument, "="); len(name) > 1 && f.short[name[:1]] == nil {
		if _, defined := f.long[name]; defined {
			return i, fmt.Errorf("flag provided but not defined: -%s (did you mean --%s?)", name, name)
		}
	}

	for len(argument) > 0 {
		name, rest := argument[:1], argument[1:]

		flag, defined := f.short[name]
		if !defined {
			if name == "h" {
				return i, ErrHelp
			}

			return i, fmt.Errorf("flag provided but not defined: -%s", name)
		}

		var value string

		switch implicit, takesNoValue := implicitValue(flag.value); {
		case strings.HasPrefix(rest, "="):
			value, rest = rest[1:], ""
		case takesNoValue:
			// a flag which takes no value leaves the rest of the argument to
			// the other flags it is combined with, e.g. "-abc".
			value = implicit
		case rest != "":
			value, rest = rest, ""
		case i < len(arguments):
			value, i = arguments[i], i+1
		default:
			return i, fmt.Errorf("flag needs an argument: -%s", name)
		}

		if err := flag.value.Set(value); err != nil {
			return i, fmt.Errorf("invalid value %q for flag -%s: %w", value, name, err)
		}

		flag.provided = true
		argument = rest
	}

	return i, nil
}

// parseEnv loads the value of the flags which were not provided on the command
// line from their environment variable. Undefined and empty environment
// variables are ignored, so that the flag keeps its default value.
func (f *FlagSet) parseEnv() error {
	for _, flag := range f.flags {
		if flag.provided || flag.env == "" {
			continue
		}

		value, exists := os.LookupEnv(flag.env)
		if !exists || value == "" {
			continue
		}

		if err := flag.value.Set(value); err != nil {
			return f.fail(fmt.Errorf("invalid value %q for environment variable %s: %w", value, flag.env, err))
		}

		flag.fromEnv = true
	}

	return nil
}

// fail prints the given error followed by the usage message and returns it.
func (f *FlagSet) fail(err error) error {
	if errors.Is(err, ErrHelp) {
		return err
	}

	fmt.Fprintln(f.errWriter, err)

	if f.Usage != nil {
		f.Usage()
	}

	return err
}

// PrintDefaults prints the defined flags to the given writer, in alphabetical
// order of the name they are presented by.
func (f *FlagSet) PrintDefaults(w io.Writer) {
	// the help flag is always available, and it closes the list.
	const help = "  -h, --help"

	flags := sortedFlags(f.flags)

	names := make([]string, 0, len(flags))
	width := len(help)

	for _, flag := range flags {
		name := nameColumn(flag)
		names = append(names, name)

		if len(name) > width {
			width = len(name)
		}
	}

	for i, flag := range flags {
		fmt.Fprintf(w, "%-*s  %s\n", width, names[i], usageColumn(flag))
	}

	fmt.Fprintf(w, "%-*s  %s\n", width, help, "shows this help message.")
}

// sortedFlags returns the given flags in alphabetical order, leaving the
// definition order of the flag set untouched.
func sortedFlags(flags []*Flag) []*Flag {
	sorted := slices.Clone(flags)

	slices.SortFunc(sorted, func(a, b *Flag) int {
		nameA, nameB := sortName(a), sortName(b)

		// the case only breaks the ties, so that an env-only flag doesn't sort
		// away from the others just because its name is upper case.
		return cmp.Or(
			strings.Compare(strings.ToLower(nameA), strings.ToLower(nameB)),
			strings.Compare(nameA, nameB),
		)
	})

	return sorted
}

// sortName returns the name a flag is sorted by, which is the one it leads with
// in the help output.
func sortName(flag *Flag) string {
	switch {
	case flag.long != "":
		return flag.long
	case flag.short != "":
		return flag.short
	default:
		return flag.env
	}
}

// nameColumn builds the left (name) column of a flag in the help output.
func nameColumn(flag *Flag) string {
	var b strings.Builder

	switch {
	// an env-only flag can't be provided on the command line, so it is
	// presented by the name of its environment variable.
	case flag.long == "" && flag.short == "":
		fmt.Fprintf(&b, "  %s", flag.env)
	case flag.short == "":
		fmt.Fprintf(&b, "      --%s", flag.long)
	case flag.long == "":
		fmt.Fprintf(&b, "  -%s", flag.short)
	default:
		fmt.Fprintf(&b, "  -%s, --%s", flag.short, flag.long)
	}

	if valueType := flagType(flag.value); valueType != "" {
		fmt.Fprintf(&b, " %s", valueType)
	}

	return b.String()
}

// usageColumn builds the right (usage) column of a flag in the help output.
func usageColumn(flag *Flag) string {
	var b strings.Builder

	fmt.Fprint(&b, flag.usage)

	if !isZeroValue(flag) {
		fmt.Fprintf(&b, " (default %s)", flag.defValue)
	}

	// an env-only flag is already presented by its environment variable.
	if flag.env != "" && (flag.long != "" || flag.short != "") {
		fmt.Fprintf(&b, " [env: %s]", flag.env)
	}

	return b.String()
}

// flagType returns the name of a value's type. A value which takes no value on
// the command line is presented by its name alone, and one which names no type
// of its own is called a value.
func flagType(value Value) string {
	if _, takesNoValue := implicitValue(value); takesNoValue {
		return ""
	}

	if v, ok := value.(typer); ok {
		if name := v.Type(); name != "" {
			return name
		}
	}

	return "value"
}

// implicitValue returns the value a flag takes when it is provided without one,
// and reports whether it takes one at all.
func implicitValue(value Value) (string, bool) {
	switch v := value.(type) {
	case implicitValuer:
		return v.ImplicitValue()
	case boolValuer:
		if v.IsBoolFlag() {
			return "true", true
		}
	}

	return "", false
}

// isZeroValue reports whether a flag defaults to the zero value of its type, in
// which case the default is not worth mentioning in the help output.
//
// A registered type knows its own zero value, while a value of your own is
// compared against the string forms the Go types give theirs.
func isZeroValue(flag *Flag) bool {
	if v, ok := flag.value.(zeroer); ok {
		return flag.defValue == v.Zero()
	}

	switch flag.defValue {
	case "", "0", "false", "0s":
		return true
	default:
		return false
	}
}
