[![Go Reference](https://pkg.go.dev/badge/github.com/danceable/console.svg)](https://pkg.go.dev/github.com/danceable/console)
[![CI](https://github.com/danceable/console/actions/workflows/ci.yml/badge.svg)](https://github.com/danceable/console/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/danceable/console)](https://goreportcard.com/report/github.com/danceable/console)
[![Coverage Status](https://coveralls.io/repos/github/danceable/console/badge.svg)](https://coveralls.io/github/danceable/console?branch=main)

# Console

Console is a handy package to build console commands in Go.
It turns a set of commands into a command line application, with flags that come
from the command line **or** from the environment, commands organized in groups
and subgroups, and a help of its own for every level.

Features:

- Flags identified by a long name (`--flag`), a short name (`-f`) and an environment variable — each one optional, and only the defined ones are enabled
- Environment variables as a fallback, so commands never read `os.Getenv` themselves
- GNU-style parsing: `--flag value`, `--flag=value`, `-f value`, `-f=value`, `-fvalue`, combined booleans (`-af`) and `--`
- Commands organized in groups and subgroups, each with flags of its own: `kubectl --username=admin pods --all list`
- A generated help for every group, subgroup and command, which you can override
- Separate writers for the regular output and for the errors
- Posix exit statuses
- An optional lifecycle for the service providers of [danceable/provider](https://github.com/danceable/provider)

## Documentation

### Required Go Versions

It requires Go `v1.26` or newer versions.

### Installation

To install this package, run the following command in your project directory.

```
go get github.com/danceable/console
```

Next, include it in your application:

```go
import "github.com/danceable/console"
```

### Introduction

A command is any type that implements the `Command` interface:

```go
type Command interface {
    Name() string
    Description() string
    Usage() string
    Configure(*console.FlagSet)
    Run(context.Context) console.ExitStatus
}
```

`Configure` defines the flags of the command and `Run` does its work. The
console parses the arguments, routes them to the command they name and returns
the exit status the command returns.

### Quick Start

```go
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
    defer cancel()

    c := console.New(
        path.Base(os.Args[0]),
        "the application.",
        os.Stdout,
        os.Stderr,
    )

    c.Register(blog.NewServeCommand())

    code := c.Run(ctx, os.Args)

    cancel()
    os.Exit(code)
}
```

### Examples

#### Implementing a Command

```go
type ServeCommand struct {
    port int
}

func (c *ServeCommand) Name() string        { return "serve" }
func (c *ServeCommand) Description() string { return "serves a http server." }
func (c *ServeCommand) Usage() string       { return "serve [flags]" }

func (c *ServeCommand) Configure(flagSet *console.FlagSet) {
    console.Var(
        flagSet,
        &c.port,
        console.Long("port"),
        "specifies which port server should listen to.",
        console.Short("p"),
        console.Env("SERVER_PORT"),
        console.Default(80),
    )
}

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
    if err := http.ListenAndServe(fmt.Sprintf(":%d", c.port), nil); err != nil {
        return console.ExitFailure
    }

    return console.ExitSuccess
}
```

#### Flags

A flag is identified by any combination of a long name, a short name and an
environment variable name. Each of them is optional, but at least one of them has
to be defined, and **only the defined ones are enabled**: a flag without a short
name can't be provided as `-f`, and a flag which defines no environment variable
never looks at the environment.

```go
// provided as "--port 80", "--port=80", "-p 80", "-p=80" or "-p80",
// and loaded from SERVER_PORT when it is not provided at all.
console.Var(flagSet, &port, console.Long("port"), "the port to listen to.", console.Short("p"), console.Env("SERVER_PORT"), console.Default(80))

// long name only: "--username=admin".
console.Var(flagSet, &username, console.Long("username"), "the user to authenticate as.")

// environment variable only: it can't be provided on the command line at all.
console.Var(flagSet, &secret, console.Env("SIGNING_SECRET"), "the signing secret.")
```

A flag is defined by the name it leads with, which is why it is a parameter of
`Var` rather than an option: a flag which names none of the three could never be
provided. The other names it answers to are options, along with the value it
defaults to.

The command line always wins over the environment. An environment variable which
is empty or undefined is ignored, so the flag keeps its default value.

| Syntax | Description |
|--------|-------------|
| `--flag value` | Long name, the value is the next argument. |
| `--flag=value` | Long name, inline value. |
| `-f value` | Short name, the value is the next argument. |
| `-f=value`, `-fvalue` | Short name, attached value. |
| `--flag`, `-f` | A boolean flag, which takes no value. |
| `--flag=false` | A boolean flag with an explicit value. |
| `-af` | Combined boolean short names. |
| `--` | Ends the flag parsing. |

Parsing stops right before the first non-flag argument (or `--`), and everything
which follows is available through `flagSet.Args()`. That is what lets the
arguments of a subgroup, of a command and of their own flags stay untouched.

#### Flag types

`console.Var` takes the type of the variable it binds a flag to, so there is one
way to define a flag whatever it holds:

```go
console.Var(flagSet, &port, console.Long("port"), "the port to listen to.", console.Default(80))
console.Var(flagSet, &timeout, console.Long("timeout"), "the request timeout.", console.Default(10*time.Second))
```

Each type is registered once with the parser which reads it, and every flag and
every struct field of that type is then parsed by it. The Go types are
registered by the package itself:

| Type | Shown as | Reads |
|------|----------|-------|
| `string` | `string` | the argument as it is |
| `bool` | *(nothing, it takes no value)* | `true`, `false`, `1`, `0`, `t`, `f` |
| `int`, `int8`, `int16`, `int32`, `int64` | `int` | `42`, `-42`, `0x2a`, `0b101010`, `0o52` |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64` | `uint` | the same, without a sign |
| `float32`, `float64` | `float` | `1.5`, `-1.5`, `1e3` |
| `time.Duration` | `duration` | `300ms`, `1.5h`, `2h45m` |

A type of your own is registered the same way, which is all it takes for it to
be usable as a flag and as a struct field:

```go
console.Register(console.Type[net.IP]{
    Name:    "ip",
    Expects: "an IP address",
    Parse: func(argument string) (net.IP, error) {
        if ip := net.ParseIP(argument); ip != nil {
            return ip, nil
        }

        return nil, errors.New("invalid address")
    },
})

var bind net.IP
console.Var(flagSet, &bind, console.Long("bind"), "the address to bind to.", console.Default(net.IPv4zero))
```

| Field | Description |
|-------|-------------|
| `Parse` | The parser of the type, which is the only field it has to define. The parsing functions of the standard library (`strconv.ParseBool`, `time.ParseDuration`, `net.ParseMAC`) are already of that shape. |
| `Name` | The name of the type in the help, as the `int` of `--port int`. A type which names itself with nothing is shown as a `value`. |
| `Expects` | What the type accepts, as `an integer`. Every value the parser rejects is explained with it (`can't be parsed as an integer`), except the ones it rejects as out of range. A type which leaves it empty reports the error of its parser as it is. |
| `Format` | The string form of a value, which the help shows the default as. Defaults to the one the `fmt` package gives it. |
| `Implicit` | The value the flag takes when it is provided without one, the way a boolean is provided as `--verbose`. Such a flag never consumes the argument which follows it. A type which leaves it empty requires a value. |

Registering a type twice replaces the first registration, which is how a Go type
is given parsing rules of your own. A type is normally registered from an `init`
function: a flag is bound to its type when it is defined, so a flag which is
already defined keeps the parsing it was defined with.

A flag defaults to the value its variable already holds, which `console.Default`
overrides:

```go
port := 80
console.Var(flagSet, &port, console.Long("port"), "the port to listen to.")
console.Var(flagSet, &retries, console.Long("retries"), "the number of retries.", console.Default(3))
```

A default value reaches its flag the way an untyped constant reaches a variable
it is assigned to, so `console.Default(0)` is the zero of whatever number the
flag holds. One the flag could never hold, because it is of another class of
types or because it would overflow or be truncated to reach it, panics when the
flag is defined.

A flag whose default value is the zero value of its type is not presented with a
default in the help, and a value the parser rejects leaves the variable
untouched, so a flag provided with an invalid value keeps its default.

#### Flags from a struct

A whole set of flags is defined at once from a tagged struct, which keeps the
configuration of an application in a single place:

```go
type configs struct {
    AppEnv  string `usage:"the deployment environment." env:"APP_ENV"`
    NodeEnv string `usage:"a fallback for APP_ENV." env:"NODE_ENV"`

    PostgresHost     string `usage:"the host." env:"POSTGRES_HOST" long:"postgres-host" short:"H" default:"localhost"`
    PostgresPort     int    `usage:"the port." env:"POSTGRES_PORT" long:"postgres-port" short:"P" default:"5432"`
    PostgresPassword string `usage:"the password." env:"POSTGRES_PASSWORD" long:"postgres-password" default:"-"`
}

configuration := &configs{}

configure, err := console.StructFlags(configuration)
if err != nil {
    log.Fatal(err)
}

console.New("app", "controls the app.", os.Stdout, os.Stderr).Flags(configure)
```

`StructFlags` validates the struct and returns the configure function of a
console, of a group or of a command, so a mistake in the tags surfaces when the
console is built rather than when it reaches the level the flags belong to.
`flagSet.Struct(&configuration)` binds a struct onto a flag set directly.

Every tag is optional and their order is irrelevant. A field which carries none
of them is not a flag and is left alone, while a field which carries any of them
has to be reachable, which means naming at least one of `long`, `short` and
`env`:

| Tag | Description |
|-----|-------------|
| `usage:"..."` | The usage message of the flag. |
| `long:"name"` | The long name, provided as `--name`. |
| `short:"n"` | The short (single character) name, provided as `-n`. |
| `env:"NAME"` | The environment variable the flag falls back to. |
| `default:"..."` | The default value of the flag. |

A field is of any [registered type](#flag-types), which the Go types are out of
the box, or of a type whose pointer implements `Value` and carries its own
parsing.

##### Default values

The default value of a flag is the value its field already holds, even when it is
the zero value of its type, so a struct is given its defaults by being populated
before it is bound:

```go
configuration := &configs{PostgresHost: "localhost", PostgresPort: 5432}
```

A `default` tag overrides that value, and `default:"-"` drops it: the field is
reset to the zero value of its type whatever the struct holds, and the help shows
no default for it. A `default` tag goes through the flag's own parser, so it is
reported as an error when the type of the field cannot read it.

##### Nested and embedded structs

Nested and embedded structs are flattened into the same flag set, which lets the
settings of a subsystem live in a struct of their own. No name is derived from
the field a struct is nested in: a nested field names its flag the way a field of
the outer struct does. A nil pointer to a struct is allocated to hold its flags.

```go
type postgres struct {
    Host string `usage:"the host." long:"postgres-host" env:"POSTGRES_HOST"`
    Port int    `usage:"the port." long:"postgres-port" default:"5432"`
}

type configs struct {
    common                     // embedded, flattened in as well.
    Verbose  bool     `usage:"verbose output." long:"verbose" short:"v"`
    Postgres postgres // nested, its own fields become flags.
    Redis    *redis   // allocated when it is nil.
}
```

A struct which is nested is never a flag itself, so it carries no flag tag of its
own. A type which parses itself, by implementing `Value`, is a flag rather than a
struct to recurse into.

##### Errors

Binding is all or nothing: the struct is resolved and validated before the first
flag is defined, so a rejected struct leaves both the flag set and the values of
the struct untouched. The error is a `*FieldError`, which names the offending
field and wraps the reason:

| Error | Returned for |
|-------|--------------|
| `ErrNotStruct` | A target which is not a non nil pointer to a struct. |
| `ErrNoName` | A tagged field which names none of `long`, `short` and `env`. |
| `ErrInvalidName` | A short name longer than a character, or a long name the parser could never match. |
| `ErrDuplicateName` | A name two fields claim, or a name the flag set already holds. |
| `ErrInvalidDefault` | A `default` tag the type of the field cannot be parsed from. |
| `ErrUnsupportedType` | A tagged field of a type no flag can be defined for. |
| `ErrUnexportedField` | A tagged field which cannot be addressed. |
| `ErrNestedTags` | A nested or an embedded struct carrying flag tags of its own. |
| `ErrRecursiveType` | A struct which nests itself. |

```go
var fieldError *console.FieldError
if errors.As(err, &fieldError) {
    log.Fatalf("the field %s cannot be a flag: %s", fieldError.Field, fieldError.Err)
}
```

#### Groups and subgroups

Commands are optionally organized in groups and subgroups. A group is not
runnable by itself: it routes to the command (or to the subgroup) which is named
right after it, and it can define flags of its own.

```go
nodes := console.NewGroup("nodes", "manages the nodes of the pods.").
    Register(node.NewListCommand())

pods := console.NewGroup("pods", "manages the pods.").
    Flags(func(flagSet *console.FlagSet) {
        console.Var(flagSet, &all, console.Long("all"), "targets every namespace.", console.Short("a"))
    }).
    Register(pod.NewListCommand()).
    RegisterGroup(nodes)

c.Flags(func(flagSet *console.FlagSet) {
    console.Var(flagSet, &username, console.Long("username"), "the user to authenticate as.", console.Short("u"))
})
c.RegisterGroup(pods)
```

Which accepts:

```
kubectl --username=admin pods --all list
        └ console flags   └ group
                               └ group flags
                                      └ command

kubectl pods nodes list
        └ group
             └ subgroup
                   └ command
```

The flags of each level are parsed before the name of the next one, so a command
can rely on the flags of the groups it belongs to being already loaded. A flag is
only known by the level it is defined on: `--all` is a usage error before `pods`,
and `--username` is one after it.

#### Help

Every group, subgroup and command answers `--help` (or `-h`) with a help of its
own, listing its subgroups, its commands and its flags. Subgroups, commands and
flags are all listed in alphabetical order, whatever the order they were
defined in:

```
$ kubectl pods --help
manages the pods.

Usage:

  kubectl pods [flags] <command> [command arguments]

The command groups are:

  nodes       manages the nodes of the pods.

The commands are:

  list        lists the pods.

Flags:

  -a, --all   targets the pods of every namespace.
  -h, --help  shows this help message.

Use "kubectl pods <command> --help" for more information about a command.
```

`WithUsage` overrides the usage line of the generated help, and `WithHelp`
replaces the whole help with a text of your own:

```go
console.NewGroup("pods", "manages the pods.").
    WithUsage("kubectl pods [flags] <command>").
    WithHelp("a totally custom help.")
```

#### Writers

`New` takes two writers. A requested help is written to the first one
(normally `os.Stdout`), while everything which goes along with a non successful
exit status — a usage error, the help which follows it, a failing service — is
written to the second one (normally `os.Stderr`):

```
$ kubectl --help | less        # the help is on the standard output
$ kubectl --bogus 2>/dev/null  # the usage error is on the standard error
```

Passing `nil` for either one falls back to `os.Stdout` and `os.Stderr`.

#### Service providers

A command which also implements the `Service` interface gets the service
providers of [danceable/provider](https://github.com/danceable/provider)
registered, booted and terminated around its run:

```go
type Service interface {
    Providers() []provider.Provider
}
```

Every provider is registered and booted before the command runs, and they are
terminated gracefully once the command returns or the context is cancelled. A
provider which fails to register or to boot stops the command, which exits with
`ExitFailure`.

```go
func (c *ServeCommand) Providers() []provider.Provider {
    return []provider.Provider{
        NewMySQLProvider(),
        NewNATSProvider(),
    }
}
```

The providers are run by the manager a console is built with by
`NewWithServiceProvider`, which takes the parameters of `New` and the
manager last. Only a console holding a service needs one. The console is not
tied to the manager of danceable/provider: it depends on the `Manager` interface
alone, which a `*provider.Manager` (such as `provider.Default`) implements as it
is:

```go
type Manager interface {
    Register(provider.Provider)
    Run(context.Context, ...provider.Option) error
}
```

```go
c := console.NewWithServiceProvider(
    path.Base(os.Args[0]),
    "the application.",
    os.Stdout,
    os.Stderr,
    provider.Default,
)
```

A console built by `New` has no manager, so it refuses to run a service,
which exits with `ExitFailure`.

#### Exit Statuses

| Status | Value | Description |
|--------|-------|-------------|
| `ExitSuccess` | 0 | The command succeeded, or a help was requested. |
| `ExitFailure` | 1 | The command failed, or its service providers failed to boot. |
| `ExitUsageError` | 2 | The arguments don't name a command, or a flag is invalid. |

#### Console Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| New | `New(name, description string, writer, errWriter io.Writer) *Console` | Creates a console which writes its output to `writer` and its errors to `errWriter`. |
| NewWithServiceProvider | `NewWithServiceProvider(name, description string, writer, errWriter io.Writer, manager Manager) *Console` | Creates a console as `New` does, whose services are run by `manager`. |
| Register | `Register(commands ...Command) *Console` | Registers commands. |
| RegisterGroup | `RegisterGroup(groups ...*Group) *Console` | Registers groups of commands. |
| Flags | `Flags(configure func(*FlagSet)) *Console` | Defines the global flags, provided before the name of the first group or command. |
| WithUsage | `WithUsage(usage string) *Console` | Overrides the usage line of the help. |
| WithHelp | `WithHelp(help string) *Console` | Overrides the whole help. |
| Run | `Run(ctx context.Context, arguments []string) ExitStatus` | Routes the arguments to a command and runs it. |
| Writer | `Writer() io.Writer` | The writer the regular output is written to. |
| ErrWriter | `ErrWriter() io.Writer` | The writer the errors are written to. |

#### Group Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| NewGroup | `NewGroup(name, description string) *Group` | Creates a group of commands. |
| Register | `Register(commands ...Command) *Group` | Registers commands in the group. |
| RegisterGroup | `RegisterGroup(groups ...*Group) *Group` | Registers subgroups in the group. |
| Flags | `Flags(configure func(*FlagSet)) *Group` | Defines the flags of the group, provided right after its name. |
| WithUsage | `WithUsage(usage string) *Group` | Overrides the usage line of the group's help. |
| WithHelp | `WithHelp(help string) *Group` | Overrides the whole help of the group. |
| Commands | `Commands() []string` | The names of the registered commands, in alphabetical order. |
| Groups | `Groups() []string` | The names of the registered subgroups, in alphabetical order. |

#### Flag Names and Options

A `FlagName` is what a flag is defined by, and any of them may also be given as
an option, to name a flag by more than one:

| Name | Description |
|------|-------------|
| `Long(name)` | The long name of the flag, provided as `--name`. |
| `Short(name)` | The short (single character) name of the flag, provided as `-n`. |
| `Env(name)` | The environment variable the flag falls back to when it is not provided. |

| Option | Description |
|--------|-------------|
| `Long`, `Short`, `Env` | The other names the flag answers to. |
| `Default(value)` | The value the flag defaults to, which is otherwise the value its variable already holds. |

Defining a flag with a nameless name, with a short name longer than a character,
or with a name which is already taken, panics: those are wiring mistakes rather
than user input errors.

#### FlagSet Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| Var | `Var(value Value, name FlagName, usage string, options ...FlagOption) *Flag` | Defines a flag with a value of your own. A flag of a registered type is defined with the `console.Var` function, as Go has no generic methods. |
| Struct | `Struct(target any) error` | Defines one flag per tagged field of a struct. |
| Parse | `Parse(arguments []string) error` | Parses the flags, then loads the missing ones from the environment. |
| Args, Arg, NArg | `Args() []string` | The arguments which follow the flags. |
| Lookup | `Lookup(name string) *Flag` | The flag defined with the given long, short or environment variable name. |
| Flags | `Flags() []*Flag` | The defined flags, in definition order. |
| PrintDefaults | `PrintDefaults(w io.Writer)` | Writes the flags as they appear in a help, in alphabetical order. |

A value of your own implements `Value`, which is what a flag holds whatever its
type. Registering a [flag type](#flag-types) is the way to parse a Go type, and
implementing `Value` is the way for a type to parse itself:

```go
type Value interface {
    String() string
    Set(string) error
}
```

It may implement any of the optional methods a registered type describes with a
field:

| Method | Description |
|--------|-------------|
| `Type() string` | Names the type of the value in the help. |
| `ImplicitValue() (string, bool)` | The value the flag takes when it is provided without one, and whether it takes one at all. |
| `IsBoolFlag() bool` | The boolean only form of `ImplicitValue`, which takes `true`. |
| `Zero() string` | The string form of the zero value of the type, which the help doesn't mention as a default. |

#### Interfaces

| Interface | Methods | Description |
|-----------|---------|-------------|
| Command | `Name()`, `Description()`, `Usage()`, `Configure(*FlagSet)`, `Run(ctx)` | A single command of the console. |
| Service | `Providers()` | Optional interface for a command whose service providers are managed around its run. |
| Manager | `Register(provider)`, `Run(ctx, options...)` | Manages the service providers of the services, as a `*provider.Manager` does. |
| Value | `String()`, `Set(string)` | The dynamic value stored in a flag. |

#### Package Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| New | `New(name, description string, writer, errWriter io.Writer) *Console` | A new console. |
| NewWithServiceProvider | `NewWithServiceProvider(name, description string, writer, errWriter io.Writer, manager Manager) *Console` | A new console, whose services are run by the given manager. |
| NewGroup | `NewGroup(name, description string) *Group` | A new group of commands. |
| NewFlagSet | `NewFlagSet(name string, errWriter io.Writer) *FlagSet` | A new set of flags. |
| Var | `Var[T any](flagSet *FlagSet, p *T, name FlagName, usage string, options ...FlagOption) *Flag` | Defines a flag of the type of the variable it stores its value in, named by the name it is provided by. |
| Default | `Default[T any](value T) FlagOption` | The value a flag defaults to. |
| Register | `Register[T any](flagType Type[T])` | Registers how the flags of a Go type are parsed and presented. |
| StructFlags | `StructFlags(target any) (func(*FlagSet), error)` | The configure function of a tagged struct, validated up front. |

## License

[MIT](LICENSE)
