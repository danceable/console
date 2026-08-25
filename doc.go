// Package console builds command line applications out of commands, which are
// optionally organized in groups and subgroups.
//
// # Commands
//
// A command implements the [Command] interface. It defines its flags in
// Configure and does its work in Run:
//
//	app := console.NewConsole(path.Base(os.Args[0]), "the application.", os.Stdout, os.Stderr, provider.Default)
//	app.Register(blog.NewServeCommand())
//
//	os.Exit(app.Run(ctx, os.Args))
//
// A requested help is written to the first writer, while a usage error (and the
// help which follows it) is written to the second one.
//
// A command which also implements [Service] gets its service providers
// registered, booted and terminated around its run.
//
// # Flags
//
// A flag is identified by any combination of a long name (--flag), a short name
// (-f) and an environment variable name. Each of them is optional and only the
// defined ones are enabled, so a flag without a short name can't be provided as
// "-f" and a flag which defines no environment variable never looks at the
// environment:
//
//	func (c *ServeCommand) Configure(flagSet *console.FlagSet) {
//		console.Var(flagSet, &c.port, console.Long("port"), "the port to listen to.", console.Short("p"), console.Env("SERVER_PORT"), console.Default(80))
//	}
//
// A flag is defined by the name it leads with, which is a parameter of [Var]
// rather than an option: a flag which names none of the three could never be
// provided. The other names it answers to are options, along with the value it
// defaults to, which is otherwise the value its variable already holds.
//
// Such a flag is provided as "--port 80", "--port=80", "-p 80", "-p=80" or
// "-p80". When it is not provided at all, its value is loaded from the
// SERVER_PORT environment variable, and when that one is empty or undefined,
// the flag keeps its default value.
//
// Boolean flags take no value ("--all"), may be combined ("-af") and are the
// only flags which don't consume the argument which follows them. Everything
// after the first non-flag argument or after "--" is left untouched and is
// available through [FlagSet.Args].
//
// # Flag types
//
// A flag takes the type of the variable it is bound to, and that type is what
// says how its values are parsed, formatted and presented. The Go types are
// registered by the package itself: the strings, the booleans, the signed and
// unsigned integers, the floating point numbers and [time.Duration].
//
// A type of your own is registered with [Register], which is all it takes for
// it to be usable as a flag and as a struct field:
//
//	console.Register(console.Type[net.IP]{
//		Name:    "ip",
//		Expects: "an IP address",
//		Parse: func(argument string) (net.IP, error) {
//			if ip := net.ParseIP(argument); ip != nil {
//				return ip, nil
//			}
//
//			return nil, errors.New("invalid address")
//		},
//	})
//
// A type which parses itself, by implementing [Value], is defined as a flag
// with [FlagSet.Var] instead.
//
// # Groups
//
// Commands are optionally organized in groups and subgroups, each one having
// its own flags and its own help:
//
//	pods := console.NewGroup("pods", "manages the pods.").
//		Flags(func(flagSet *console.FlagSet) {
//			console.Var(flagSet, &all, console.Long("all"), "targets every namespace.", console.Short("a"))
//		}).
//		Register(pod.NewListCommand()).
//		RegisterGroup(nodes)
//
//	console.Flags(func(flagSet *console.FlagSet) {
//		console.Var(flagSet, &username, console.Long("username"), "the user to authenticate as.", console.Short("u"))
//	})
//	console.RegisterGroup(pods)
//
// Which accepts:
//
//	app --username=admin pods --all list
//	    └ console flags  └ group
//	                          └ group flags
//	                                 └ command
//
// The flags of each level are parsed before the name of the next one, so a
// command can rely on the flags of the groups it belongs to being already
// loaded. A flag is only known by the level it is defined on: "--all" is a
// usage error before "pods" and "--username" is one after it.
//
// Every group answers "--help" (or "-h") with its own help, listing its
// subgroups, its commands and its flags. [Group.WithUsage] overrides the usage
// line of the generated help and [Group.WithHelp] replaces the whole help with
// a text of your own.
package console
