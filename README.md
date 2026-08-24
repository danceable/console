[![Go Reference](https://pkg.go.dev/badge/github.com/danceable/console.svg)](https://pkg.go.dev/github.com/danceable/console)
[![CI](https://github.com/danceable/console/actions/workflows/ci.yml/badge.svg)](https://github.com/danceable/console/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/danceable/console)](https://goreportcard.com/report/github.com/danceable/console)

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

    c := console.NewConsole(
        path.Base(os.Args[0]),
        "the application.",
        os.Stdout,
        os.Stderr,
        provider.Default,
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
    flagSet.IntVar(
        &c.port,
        80,
        "specifies which port server should listen to.",
        console.Long("port"),
        console.Short("p"),
        console.Env("SERVER_PORT"),
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
flagSet.IntVar(&port, 80, "the port to listen to.", console.Long("port"), console.Short("p"), console.Env("SERVER_PORT"))

// long name only: "--username=admin".
flagSet.StringVar(&username, "", "the user to authenticate as.", console.Long("username"))

// environment variable only: it can't be provided on the command line at all.
flagSet.StringVar(&secret, "", "the signing secret.", console.Env("SIGNING_SECRET"))
```

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

console.NewConsole("app", "controls the app.", os.Stdout, os.Stderr, provider.Default).Flags(configure)
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

The supported field types are `string`, `bool`, `int`, `int64`, `uint`, `uint64`,
`float64` and `time.Duration`, along with any type whose pointer implements
`Value`.

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
        flagSet.BoolVar(&all, false, "targets every namespace.", console.Long("all"), console.Short("a"))
    }).
    Register(pod.NewListCommand()).
    RegisterGroup(nodes)

c.Flags(func(flagSet *console.FlagSet) {
    flagSet.StringVar(&username, "", "the user to authenticate as.", console.Long("username"), console.Short("u"))
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
own, listing its subgroups, its commands and its flags:

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

`NewConsole` takes two writers. A requested help is written to the first one
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
    Boot(ctx context.Context, container provider.Container) error
}
```

`Boot` is called once every provider has been booted, which is where the command
resolves its dependencies, and the providers are terminated gracefully once the
command returns or the context is cancelled.

```go
func (c *ServeCommand) Providers() []provider.Provider {
    return providers.BlogProviders()
}

func (c *ServeCommand) Boot(ctx context.Context, container provider.Container) error {
    return container.Resolve(&c.handler)
}
```

#### Exit Statuses

| Status | Value | Description |
|--------|-------|-------------|
| `ExitSuccess` | 0 | The command succeeded, or a help was requested. |
| `ExitFailure` | 1 | The command failed, or its service providers failed to boot. |
| `ExitUsageError` | 2 | The arguments don't name a command, or a flag is invalid. |

#### Console Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| NewConsole | `NewConsole(name, description string, writer, errWriter io.Writer, manager *provider.Manager) *Console` | Creates a console which writes its output to `writer` and its errors to `errWriter`. |
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

#### Flag Options

| Option | Description |
|--------|-------------|
| `Long(name)` | The long name of the flag, provided as `--name`. |
| `Short(name)` | The short (single character) name of the flag, provided as `-n`. |
| `Env(name)` | The environment variable the flag falls back to when it is not provided. |

Defining a flag with none of them, with a short name longer than a character, or
with a name which is already taken, panics: those are wiring mistakes rather than
user input errors.

#### FlagSet Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| StringVar | `StringVar(p *string, value string, usage string, options ...FlagOption) *Flag` | Defines a string flag. |
| BoolVar | `BoolVar(p *bool, value bool, usage string, options ...FlagOption) *Flag` | Defines a boolean flag, which takes no value. |
| IntVar, Int64Var | `IntVar(p *int, value int, usage string, options ...FlagOption) *Flag` | Defines a signed integer flag. |
| UintVar, Uint64Var | `UintVar(p *uint, value uint, usage string, options ...FlagOption) *Flag` | Defines an unsigned integer flag. |
| Float64Var | `Float64Var(p *float64, value float64, usage string, options ...FlagOption) *Flag` | Defines a floating point flag. |
| DurationVar | `DurationVar(p *time.Duration, value time.Duration, usage string, options ...FlagOption) *Flag` | Defines a `time.Duration` flag. |
| Var | `Var(value Value, usage string, options ...FlagOption) *Flag` | Defines a flag with a value of your own. |
| Struct | `Struct(target any) error` | Defines one flag per tagged field of a struct. |
| Parse | `Parse(arguments []string) error` | Parses the flags, then loads the missing ones from the environment. |
| Args, Arg, NArg | `Args() []string` | The arguments which follow the flags. |
| Lookup | `Lookup(name string) *Flag` | The flag defined with the given long, short or environment variable name. |
| Flags | `Flags() []*Flag` | The defined flags, in definition order. |
| PrintDefaults | `PrintDefaults(w io.Writer)` | Writes the flags as they appear in a help. |

A flag of your own implements `Value`, and may implement `IsBoolFlag() bool` to
take no value and `Type() string` to name its type in the help:

```go
type Value interface {
    String() string
    Set(string) error
}
```

#### Interfaces

| Interface | Methods | Description |
|-----------|---------|-------------|
| Command | `Name()`, `Description()`, `Usage()`, `Configure(*FlagSet)`, `Run(ctx)` | A single command of the console. |
| Service | `Providers()`, `Boot(ctx, container)` | Optional interface for a command whose service providers are managed around its run. |
| Value | `String()`, `Set(string)` | The dynamic value stored in a flag. |

#### Package Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| NewConsole | `NewConsole(name, description string, writer, errWriter io.Writer, manager *provider.Manager) *Console` | A new console. |
| NewGroup | `NewGroup(name, description string) *Group` | A new group of commands. |
| NewFlagSet | `NewFlagSet(name string, errWriter io.Writer) *FlagSet` | A new set of flags. |
| StructFlags | `StructFlags(target any) (func(*FlagSet), error)` | The configure function of a tagged struct, validated up front. |

## License

[MIT](LICENSE)
