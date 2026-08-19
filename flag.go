package console

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
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

// boolValuer is implemented by values that may be provided without an explicit
// value, e.g. "--verbose" instead of "--verbose=true".
type boolValuer interface {
	IsBoolFlag() bool
}

// typer is implemented by values that name their own type in the help output.
type typer interface {
	Type() string
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

// FlagOption defines an optional property of a flag.
type FlagOption func(*Flag)

// Long defines the long name of a flag, which is provided as "--name".
func Long(name string) FlagOption {
	return func(f *Flag) { f.long = name }
}

// Short defines the short (single character) name of a flag, which is provided
// as "-n".
func Short(name string) FlagOption {
	return func(f *Flag) { f.short = name }
}

// Env defines the environment variable a flag falls back to when it is not
// provided on the command line. Empty environment variables are ignored.
func Env(name string) FlagOption {
	return func(f *Flag) { f.env = name }
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

// Var defines a flag with the given value and usage. At least one of the Long,
// Short or Env options must be provided.
func (f *FlagSet) Var(value Value, usage string, options ...FlagOption) *Flag {
	flag := &Flag{
		usage:    usage,
		value:    value,
		defValue: value.String(),
	}

	for _, option := range options {
		option(flag)
	}

	f.define(flag)

	return flag
}

// StringVar defines a string flag which stores its value in p.
func (f *FlagSet) StringVar(p *string, value string, usage string, options ...FlagOption) *Flag {
	return f.Var(newStringValue(value, p), usage, options...)
}

// BoolVar defines a boolean flag which stores its value in p.
func (f *FlagSet) BoolVar(p *bool, value bool, usage string, options ...FlagOption) *Flag {
	return f.Var(newBoolValue(value, p), usage, options...)
}

// IntVar defines an int flag which stores its value in p.
func (f *FlagSet) IntVar(p *int, value int, usage string, options ...FlagOption) *Flag {
	return f.Var(newIntValue(value, p), usage, options...)
}

// Int64Var defines an int64 flag which stores its value in p.
func (f *FlagSet) Int64Var(p *int64, value int64, usage string, options ...FlagOption) *Flag {
	return f.Var(newInt64Value(value, p), usage, options...)
}

// UintVar defines a uint flag which stores its value in p.
func (f *FlagSet) UintVar(p *uint, value uint, usage string, options ...FlagOption) *Flag {
	return f.Var(newUintValue(value, p), usage, options...)
}

// Uint64Var defines a uint64 flag which stores its value in p.
func (f *FlagSet) Uint64Var(p *uint64, value uint64, usage string, options ...FlagOption) *Flag {
	return f.Var(newUint64Value(value, p), usage, options...)
}

// Float64Var defines a float64 flag which stores its value in p.
func (f *FlagSet) Float64Var(p *float64, value float64, usage string, options ...FlagOption) *Flag {
	return f.Var(newFloat64Value(value, p), usage, options...)
}

// DurationVar defines a time.Duration flag which stores its value in p.
func (f *FlagSet) DurationVar(p *time.Duration, value time.Duration, usage string, options ...FlagOption) *Flag {
	return f.Var(newDurationValue(value, p), usage, options...)
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
		switch {
		case isBoolValue(flag.value):
			value = "true"
		case i < len(arguments):
			value, i = arguments[i], i+1
		default:
			return i, fmt.Errorf("flag needs an argument: --%s", name)
		}
	}

	if err := flag.value.Set(value); err != nil {
		return i, fmt.Errorf("invalid value %q for flag --%s: %s", value, name, err)
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

		switch {
		case strings.HasPrefix(rest, "="):
			value, rest = rest[1:], ""
		case isBoolValue(flag.value):
			// a boolean flag takes no value, so the rest of the argument is
			// made of other boolean flags, e.g. "-abc".
			value = "true"
		case rest != "":
			value, rest = rest, ""
		case i < len(arguments):
			value, i = arguments[i], i+1
		default:
			return i, fmt.Errorf("flag needs an argument: -%s", name)
		}

		if err := flag.value.Set(value); err != nil {
			return i, fmt.Errorf("invalid value %q for flag -%s: %s", value, name, err)
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
			return f.fail(fmt.Errorf("invalid value %q for environment variable %s: %s", value, flag.env, err))
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

// PrintDefaults prints the defined flags to the given writer.
func (f *FlagSet) PrintDefaults(w io.Writer) {
	names := make([]string, 0, len(f.flags))
	width := 0

	for _, flag := range f.flags {
		name := flagName(flag)
		names = append(names, name)

		if len(name) > width {
			width = len(name)
		}
	}

	// the help flag is always available.
	names = append(names, "  -h, --help")
	if len("  -h, --help") > width {
		width = len("  -h, --help")
	}

	for i, flag := range f.flags {
		fmt.Fprintf(w, "%-*s  %s\n", width, names[i], flagUsage(flag))
	}

	fmt.Fprintf(w, "%-*s  %s\n", width, names[len(names)-1], "shows this help message.")
}

// flagName builds the left (name) column of a flag in the help output.
func flagName(flag *Flag) string {
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

// flagUsage builds the right (usage) column of a flag in the help output.
func flagUsage(flag *Flag) string {
	var b strings.Builder

	fmt.Fprint(&b, flag.usage)

	if !isZeroValue(flag.defValue) {
		fmt.Fprintf(&b, " (default %s)", flag.defValue)
	}

	// an env-only flag is already presented by its environment variable.
	if flag.env != "" && (flag.long != "" || flag.short != "") {
		fmt.Fprintf(&b, " [env: %s]", flag.env)
	}

	return b.String()
}

// flagType returns the name of a value's type. Booleans take no value, so they
// are not presented by a type.
func flagType(value Value) string {
	if isBoolValue(value) {
		return ""
	}

	if v, ok := value.(typer); ok {
		return v.Type()
	}

	return "value"
}

// isBoolValue reports whether a value may be provided without an explicit value.
func isBoolValue(value Value) bool {
	v, ok := value.(boolValuer)

	return ok && v.IsBoolFlag()
}

// isZeroValue reports whether a default value is the zero value of its type,
// in which case it is not worth mentioning in the help output.
func isZeroValue(value string) bool {
	switch value {
	case "", "0", "false", "0s":
		return true
	default:
		return false
	}
}

type stringValue string

func newStringValue(value string, p *string) *stringValue {
	*p = value

	return (*stringValue)(p)
}

func (s *stringValue) Set(value string) error {
	*s = stringValue(value)

	return nil
}

func (s *stringValue) String() string { return string(*s) }
func (s *stringValue) Type() string   { return "string" }

type boolValue bool

func newBoolValue(value bool, p *bool) *boolValue {
	*p = value

	return (*boolValue)(p)
}

func (b *boolValue) Set(value string) error {
	v, err := strconv.ParseBool(value)
	if err != nil {
		return errParse
	}

	*b = boolValue(v)

	return nil
}

func (b *boolValue) String() string   { return strconv.FormatBool(bool(*b)) }
func (b *boolValue) Type() string     { return "bool" }
func (b *boolValue) IsBoolFlag() bool { return true }

type intValue int

func newIntValue(value int, p *int) *intValue {
	*p = value

	return (*intValue)(p)
}

func (i *intValue) Set(value string) error {
	v, err := strconv.ParseInt(value, 0, strconv.IntSize)
	if err != nil {
		return numError(err)
	}

	*i = intValue(v)

	return nil
}

func (i *intValue) String() string { return strconv.Itoa(int(*i)) }
func (i *intValue) Type() string   { return "int" }

type int64Value int64

func newInt64Value(value int64, p *int64) *int64Value {
	*p = value

	return (*int64Value)(p)
}

func (i *int64Value) Set(value string) error {
	v, err := strconv.ParseInt(value, 0, 64)
	if err != nil {
		return numError(err)
	}

	*i = int64Value(v)

	return nil
}

func (i *int64Value) String() string { return strconv.FormatInt(int64(*i), 10) }
func (i *int64Value) Type() string   { return "int" }

type uintValue uint

func newUintValue(value uint, p *uint) *uintValue {
	*p = value

	return (*uintValue)(p)
}

func (u *uintValue) Set(value string) error {
	v, err := strconv.ParseUint(value, 0, strconv.IntSize)
	if err != nil {
		return numError(err)
	}

	*u = uintValue(v)

	return nil
}

func (u *uintValue) String() string { return strconv.FormatUint(uint64(*u), 10) }
func (u *uintValue) Type() string   { return "uint" }

type uint64Value uint64

func newUint64Value(value uint64, p *uint64) *uint64Value {
	*p = value

	return (*uint64Value)(p)
}

func (u *uint64Value) Set(value string) error {
	v, err := strconv.ParseUint(value, 0, 64)
	if err != nil {
		return numError(err)
	}

	*u = uint64Value(v)

	return nil
}

func (u *uint64Value) String() string { return strconv.FormatUint(uint64(*u), 10) }
func (u *uint64Value) Type() string   { return "uint" }

type float64Value float64

func newFloat64Value(value float64, p *float64) *float64Value {
	*p = value

	return (*float64Value)(p)
}

func (f *float64Value) Set(value string) error {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return numError(err)
	}

	*f = float64Value(v)

	return nil
}

func (f *float64Value) String() string { return strconv.FormatFloat(float64(*f), 'g', -1, 64) }
func (f *float64Value) Type() string   { return "float" }

type durationValue time.Duration

func newDurationValue(value time.Duration, p *time.Duration) *durationValue {
	*p = value

	return (*durationValue)(p)
}

func (d *durationValue) Set(value string) error {
	v, err := time.ParseDuration(value)
	if err != nil {
		return errParse
	}

	*d = durationValue(v)

	return nil
}

func (d *durationValue) String() string { return time.Duration(*d).String() }
func (d *durationValue) Type() string   { return "duration" }

var (
	errParse = errors.New("parse error")
	errRange = errors.New("value out of range")
)

// numError unwraps the verbose errors of the strconv package.
func numError(err error) error {
	var numError *strconv.NumError
	if !errors.As(err, &numError) {
		return err
	}

	if errors.Is(numError.Err, strconv.ErrRange) {
		return errRange
	}

	return errParse
}
