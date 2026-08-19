package console

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danceable/provider"
	"github.com/google/go-cmp/cmp"
)

func TestConsole(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		t.Run("console help without registered command", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
			}{
				{
					name:      "-h flag",
					arguments: []string{"", "-h"},
				},
				{
					name:      "--help flag",
					arguments: []string{"", "--help"},
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitSuccess {
						t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
					}

					golden(t, "help-without-commands.txt", writer.String())
					empty(t, "error", errWriter.String())
				})
			}
		})

		t.Run("console help with registered command", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
			}{
				{
					name:      "-h flag",
					arguments: []string{"", "-h"},
				},
				{
					name:      "--help flag",
					arguments: []string{"", "--help"},
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)

					var (
						boolArg bool
						intArg  int
					)

					command := NewSpyCommand(
						"test-command",
						"this is a test description",
						"this is a test usage",
						0,
						func(fs *FlagSet) {
							fs.BoolVar(&boolArg, false, "test bool argument", Long("boolArg"))
							fs.IntVar(&intArg, 666, "test int argument", Long("intArg"))
						},
					)

					console.Register(command)
					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitSuccess {
						t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
					}

					golden(t, "help-with-commands.txt", writer.String())
					empty(t, "error", errWriter.String())
				})
			}
		})

		t.Run("command help", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
			}{
				{
					name:      "-h flag",
					arguments: []string{"binary-name", "test-command", "-h"},
				},
				{
					name:      "--help flag",
					arguments: []string{"binary-name", "test-command", "--help"},
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)

					var (
						port int
						name string
						all  bool
					)

					console.Register(NewSpyCommand(
						"test-command",
						"this is a test description",
						"this is a test usage",
						0,
						func(fs *FlagSet) {
							fs.IntVar(&port, 80, "specifies which port server should listen to.", Long("port"), Short("p"), Env("SERVER_PORT"))
							fs.StringVar(&name, "", "specifies the unique name of the worker.", Long("name"), Env("WORKER_NAME"))
							fs.BoolVar(&all, false, "runs on every namespace.", Short("a"))
						},
					))

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitSuccess {
						t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
					}

					golden(t, "command-help.txt", writer.String())
					empty(t, "error", errWriter.String())
				})
			}
		})
	})

	t.Run("invalid attempts", func(t *testing.T) {
		testCases := []struct {
			name       string
			arguments  []string
			exitStatus int
			outputErr  string
		}{
			{
				name:       "no arguments",
				arguments:  []string{},
				exitStatus: ExitUsageError,
				outputErr:  testdata(t, "help-without-commands.txt"),
			},
			{
				name:       "0-either only binary or command",
				arguments:  []string{""},
				exitStatus: ExitUsageError,
				outputErr:  testdata(t, "help-without-commands.txt"),
			},
			{
				name:       "1-either only binary or command",
				arguments:  []string{"command"},
				exitStatus: ExitUsageError,
				outputErr:  testdata(t, "help-without-commands.txt"),
			},
			{
				name:       "not registered command (help)",
				arguments:  []string{"", "help"},
				exitStatus: ExitUsageError,
				outputErr:  "\"help\" is not a command, See \"Test --help\".\n",
			},
			{
				name:       "not registered command with -h flag",
				arguments:  []string{"", "command", "-h"},
				exitStatus: ExitUsageError,
				outputErr:  "\"command\" is not a command, See \"Test --help\".\n",
			},
			{
				name:       "not registered command with --help flag",
				arguments:  []string{"binary", "command", "--help"},
				exitStatus: ExitUsageError,
				outputErr:  "\"command\" is not a command, See \"Test --help\".\n",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var writer, errWriter bytes.Buffer
				console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)

				if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != testCase.exitStatus {
					t.Errorf("unexpected exit code, want %d got %d", testCase.exitStatus, exitStatus)
				}

				want := testCase.outputErr
				got := errWriter.String()
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("console error output mismatch (-want +got):\n%s", diff)
				}

				empty(t, "regular", writer.String())
			})
		}
	})

	t.Run("exit status will be returned to caller", func(t *testing.T) {
		statuses := []int{0, 1, 2, 3, 4}

		for _, status := range statuses {
			var writer, errWriter bytes.Buffer
			console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)

			command := NewSpyCommand(
				"command",
				"this is a test description",
				"this is a test usage",
				status,
				nil,
			)

			console.Register(command)

			if exitStatus := console.Run(context.Background(), []string{"binary-name", "command"}); exitStatus != status {
				t.Errorf("unexpected exit code, want %d got %d", status, exitStatus)
			}

			empty(t, "regular", writer.String())
			empty(t, "error", errWriter.String())

			if command.NameCount != 1 {
				t.Errorf("%q method should be called once", "Name")
			}

			if command.DescriptionCount != 0 {
				t.Errorf("%q method should not be called", "Description")
			}

			if command.UsageCount != 0 {
				t.Errorf("%q method should not be called", "Usage")
			}

			if command.RunCount != 1 {
				t.Errorf("%q method should be called once", "Run")
			}

			if command.ConfigureCount != 1 {
				t.Errorf("%q method should be called once", "Configure")
			}
		}
	})

	t.Run("test command arguments", func(t *testing.T) {
		var writer, errWriter bytes.Buffer
		console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)

		var (
			boolArg bool
			intArg  int
		)

		command := NewSpyCommand(
			"test-command",
			"this is a test description",
			"this is a test usage",
			0,
			func(fs *FlagSet) {
				fs.BoolVar(&boolArg, false, "test bool argument", Long("boolArg"))
				fs.IntVar(&intArg, 666, "test int argument", Long("intArg"))
			},
		)

		console.Register(command)

		t.Run("args should be filled with provided values", func(t *testing.T) {
			errWriter.Reset()
			arguments := []string{"binary-name", "test-command", "--intArg", "100", "--boolArg", "true"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != ExitSuccess {
				t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
			}

			if command.RunCount != 1 {
				t.Errorf("%q method should be called once", "Run")
			}

			if command.ConfigureCount != 1 {
				t.Errorf("%q method should be called once", "Configure")
			}

			empty(t, "regular", writer.String())
			empty(t, "error", errWriter.String())

			if boolArg != true {
				t.Errorf("unexpected argument, want true got false")
			}

			if intArg != 100 {
				t.Errorf("unexpected argument, want %d got %d", 100, intArg)
			}
		})

		t.Run("flag type mismatch", func(t *testing.T) {
			errWriter.Reset()
			arguments := []string{"binary-name", "test-command", "--intArg", "100.2", "--boolArg", "true"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != ExitUsageError {
				t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
			}

			golden(t, "flag-type-mismatch.txt", errWriter.String())
			empty(t, "regular", writer.String())

			if boolArg != false {
				t.Errorf("unexpected argument, want false got true")
			}

			if intArg != 666 {
				t.Errorf("unexpected argument, want %d got %d", 666, intArg)
			}
		})

		t.Run("non existing arg", func(t *testing.T) {
			errWriter.Reset()
			arguments := []string{"binary-name", "test-command", "--nonexisting", "abc", "--intArg", "100"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != ExitUsageError {
				t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
			}

			golden(t, "flag-provided-but-not-defined.txt", errWriter.String())
			empty(t, "regular", writer.String())

			if boolArg != false {
				t.Errorf("unexpected argument, want false got true")
			}

			if intArg != 666 {
				t.Errorf("unexpected argument, want %d got %d", 666, intArg)
			}
		})
	})

	t.Run("writers", func(t *testing.T) {
		t.Run("a requested help is written to the writer, everything else to the error writer", func(t *testing.T) {
			testCases := []struct {
				name       string
				arguments  []string
				exitStatus int
				writer     bool // the regular output is expected to be written to.
			}{
				{
					name:       "the help of the console",
					arguments:  []string{"kubectl", "--help"},
					exitStatus: ExitSuccess,
					writer:     true,
				},
				{
					name:       "the help of a group",
					arguments:  []string{"kubectl", "pods", "--help"},
					exitStatus: ExitSuccess,
					writer:     true,
				},
				{
					name:       "the help of a command",
					arguments:  []string{"kubectl", "pods", "list", "--help"},
					exitStatus: ExitSuccess,
					writer:     true,
				},
				{
					name:       "a missing command",
					arguments:  []string{"kubectl"},
					exitStatus: ExitUsageError,
				},
				{
					name:       "an unknown command",
					arguments:  []string{"kubectl", "destroy"},
					exitStatus: ExitUsageError,
				},
				{
					name:       "an unknown flag of a group",
					arguments:  []string{"kubectl", "pods", "--unknown"},
					exitStatus: ExitUsageError,
				},
				{
					name:       "an invalid flag of a command",
					arguments:  []string{"kubectl", "pods", "list", "--limit", "many"},
					exitStatus: ExitUsageError,
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console, _ := kubectl(&writer, &errWriter)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != testCase.exitStatus {
						t.Errorf("unexpected exit code, want %d got %d", testCase.exitStatus, exitStatus)
					}

					if testCase.writer {
						empty(t, "error", errWriter.String())

						if writer.Len() == 0 {
							t.Error("the regular output should not be empty")
						}

						return
					}

					empty(t, "regular", writer.String())

					if errWriter.Len() == 0 {
						t.Error("the error output should not be empty")
					}
				})
			}
		})

		t.Run("the writers fall back to the standard ones", func(t *testing.T) {
			console := NewConsole("Test", "Test description", nil, nil, provider.Default)

			if console.Writer() != os.Stdout {
				t.Error("the regular output should fall back to os.Stdout")
			}

			if console.ErrWriter() != os.Stderr {
				t.Error("the error output should fall back to os.Stderr")
			}
		})
	})

	t.Run("groups", func(t *testing.T) {
		t.Run("commands of a group and a subgroup are reachable", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
				want      string
			}{
				{
					name:      "command of the console",
					arguments: []string{"kubectl", "version"},
					want:      "version",
				},
				{
					name:      "command of a group",
					arguments: []string{"kubectl", "pods", "list"},
					want:      "list",
				},
				{
					name:      "command of a subgroup",
					arguments: []string{"kubectl", "pods", "nodes", "list"},
					want:      "nodes:list",
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console, fixture := kubectl(&writer, &errWriter)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitSuccess {
						t.Errorf("unexpected exit code, want %d got %d, output: %s", ExitSuccess, exitStatus, errWriter.String())
					}

					if fixture.executed != testCase.want {
						t.Errorf("unexpected executed command, want %q got %q", testCase.want, fixture.executed)
					}
				})
			}
		})

		t.Run("flags are parsed at the level they are defined on", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console, fixture := kubectl(&writer, &errWriter)

			arguments := []string{"kubectl", "--username=admin", "pods", "--all", "list", "-l", "5", "extra-argument"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != ExitSuccess {
				t.Errorf("unexpected exit code, want %d got %d, output: %s", ExitSuccess, exitStatus, errWriter.String())
			}

			if fixture.username != "admin" {
				t.Errorf("unexpected console flag, want %q got %q", "admin", fixture.username)
			}

			if !fixture.all {
				t.Error("unexpected group flag, want true got false")
			}

			if fixture.limit != 5 {
				t.Errorf("unexpected command flag, want %d got %d", 5, fixture.limit)
			}

			if want, got := []string{"extra-argument"}, fixture.arguments; !cmp.Equal(want, got) {
				t.Errorf("unexpected command arguments, want %v got %v", want, got)
			}
		})

		t.Run("a flag of a group is not available on another level", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
			}{
				{
					name:      "group flag before the group's name",
					arguments: []string{"kubectl", "--all", "pods", "list"},
				},
				{
					name:      "console flag after the group's name",
					arguments: []string{"kubectl", "pods", "--username=admin", "list"},
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console, fixture := kubectl(&writer, &errWriter)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitUsageError {
						t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
					}

					if fixture.executed != "" {
						t.Errorf("no command should have been executed, got %q", fixture.executed)
					}
				})
			}
		})

		t.Run("an unknown command of a group is reported", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console, _ := kubectl(&writer, &errWriter)

			if exitStatus := console.Run(context.Background(), []string{"kubectl", "pods", "destroy"}); exitStatus != ExitUsageError {
				t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
			}

			want := "\"destroy\" is not a command, See \"kubectl pods --help\".\n"
			got := errWriter.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console error output mismatch (-want +got):\n%s", diff)
			}

			empty(t, "regular", writer.String())
		})

		t.Run("each group has its own help", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
				file      string
			}{
				{
					name:      "console",
					arguments: []string{"kubectl", "--help"},
					file:      "group-console-help.txt",
				},
				{
					name:      "group",
					arguments: []string{"kubectl", "pods", "--help"},
					file:      "group-help.txt",
				},
				{
					name:      "subgroup",
					arguments: []string{"kubectl", "pods", "nodes", "--help"},
					file:      "subgroup-help.txt",
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console, _ := kubectl(&writer, &errWriter)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitSuccess {
						t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
					}

					golden(t, testCase.file, writer.String())
					empty(t, "error", errWriter.String())
				})
			}
		})

		t.Run("a group can define its own help", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)
			console.RegisterGroup(NewGroup("pods", "manages the pods.").WithHelp("a totally custom help."))

			if exitStatus := console.Run(context.Background(), []string{"Test", "pods", "--help"}); exitStatus != ExitSuccess {
				t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
			}

			want := "a totally custom help.\n"
			got := writer.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console output mismatch (-want +got):\n%s", diff)
			}

			empty(t, "error", errWriter.String())
		})

		t.Run("a group without a command shows its help", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console, _ := kubectl(&writer, &errWriter)

			if exitStatus := console.Run(context.Background(), []string{"kubectl", "pods"}); exitStatus != ExitUsageError {
				t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
			}

			want := testdata(t, "group-help.txt")
			got := errWriter.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console error output mismatch (-want +got):\n%s", diff)
			}

			empty(t, "regular", writer.String())
		})

		t.Run("a group which only routes to subgroups lists no command", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)
			console.RegisterGroup(
				NewGroup("pods", "manages the pods.").
					RegisterGroup(NewGroup("nodes", "manages the nodes of the pods.")),
			)

			if exitStatus := console.Run(context.Background(), []string{"Test", "pods", "--help"}); exitStatus != ExitSuccess {
				t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
			}

			golden(t, "routing-group-help.txt", writer.String())
			empty(t, "error", errWriter.String())
		})

		t.Run("a group knows its commands and subgroups", func(t *testing.T) {
			nodes := NewGroup("nodes", "manages the nodes of the pods.")
			pods := NewGroup("pods", "manages the pods.").
				WithUsage("kubectl pods [flags] <command>").
				Register(
					NewSpyCommand("list", "lists the pods.", "", 0, nil),
					NewSpyCommand("delete", "deletes a pod.", "", 0, nil),
				).
				RegisterGroup(nodes)

			if want, got := []string{"delete", "list"}, pods.Commands(); !cmp.Equal(want, got) {
				t.Errorf("unexpected commands, want %v got %v", want, got)
			}

			if want, got := []string{"nodes"}, pods.Groups(); !cmp.Equal(want, got) {
				t.Errorf("unexpected groups, want %v got %v", want, got)
			}

			if want, got := "kubectl pods [flags] <command>", pods.Usage(); want != got {
				t.Errorf("unexpected usage, want %q got %q", want, got)
			}

			if want, got := "manages the pods.", pods.Description(); want != got {
				t.Errorf("unexpected description, want %q got %q", want, got)
			}
		})

		t.Run("the console has its own usage and help", func(t *testing.T) {
			t.Run("usage", func(t *testing.T) {
				var writer, errWriter bytes.Buffer
				console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)
				console.WithUsage("Test <command>")

				console.Run(context.Background(), []string{"Test", "--help"})

				if want := "  Test <command>\n"; !strings.Contains(writer.String(), want) {
					t.Errorf("the help should contain %q, got:\n%s", want, writer.String())
				}
			})

			t.Run("help", func(t *testing.T) {
				var writer, errWriter bytes.Buffer
				console := NewConsole("Test", "Test description", &writer, &errWriter, provider.Default)
				console.WithHelp("a totally custom help.")

				console.Run(context.Background(), []string{"Test", "--help"})

				if want, got := "a totally custom help.\n", writer.String(); want != got {
					t.Errorf("unexpected help, want %q got %q", want, got)
				}
			})
		})

		t.Run("registering a command without a name panics", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("registering a command without a name should panic")
				}
			}()

			NewGroup("pods", "manages the pods.").Register(NewSpyCommand("", "", "", 0, nil))
		})

		t.Run("registering a name twice panics", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("registering the same name twice should panic")
				}
			}()

			group := NewGroup("pods", "manages the pods.")
			group.Register(NewSpyCommand("list", "", "", 0, nil))
			group.RegisterGroup(NewGroup("list", "duplicated name."))
		})
	})
}

// kubectlFixture holds the values the kubectl fixture's flags are parsed into.
type kubectlFixture struct {
	username  string
	all       bool
	limit     int
	arguments []string
	executed  string
}

// kubectl builds a console which mimics the "kubectl" command line, having
// flags on the console, on a group and on a subgroup:
//
//	kubectl --username=admin pods --all list -l 5
func kubectl(writer, errWriter *bytes.Buffer) (*Console, *kubectlFixture) {
	fixture := &kubectlFixture{}

	console := NewConsole("kubectl", "controls the cluster manager.", writer, errWriter, provider.Default)
	console.Flags(func(fs *FlagSet) {
		fs.StringVar(&fixture.username, "", "the user to authenticate as.", Long("username"), Short("u"), Env("KUBECTL_USERNAME"))
	})

	list := NewSpyCommand("list", "lists the pods.", "kubectl pods list [flags]", 0, func(fs *FlagSet) {
		fs.IntVar(&fixture.limit, 10, "the maximum number of pods to show.", Long("limit"), Short("l"), Env("KUBECTL_LIMIT"))
	})
	list.runFunc = func(fs *FlagSet) {
		fixture.executed, fixture.arguments = "list", fs.Args()
	}

	nodesList := NewSpyCommand("list", "lists the nodes the pods run on.", "kubectl pods nodes list [flags]", 0, nil)
	nodesList.runFunc = func(fs *FlagSet) { fixture.executed = "nodes:list" }

	nodes := NewGroup("nodes", "manages the nodes of the pods.").
		Register(nodesList)

	pods := NewGroup("pods", "manages the pods.").
		WithUsage("kubectl pods [flags] <command> [command arguments]").
		Flags(func(fs *FlagSet) {
			fs.BoolVar(&fixture.all, false, "targets the pods of every namespace.", Long("all"), Short("a"))
		}).
		Register(list).
		RegisterGroup(nodes)

	version := NewSpyCommand("version", "prints the version.", "kubectl version", 0, nil)
	version.runFunc = func(fs *FlagSet) { fixture.executed = "version" }

	console.Register(version)
	console.RegisterGroup(pods)

	return console, fixture
}

type SpyCommand struct {
	name          string
	description   string
	usage         string
	exitStatus    int
	configureFunc func(*FlagSet)
	runFunc       func(*FlagSet)
	flagSet       *FlagSet

	NameCount        int
	DescriptionCount int
	UsageCount       int
	RunCount         int
	ConfigureCount   int
}

var _ Command = &SpyCommand{}

func NewSpyCommand(
	name, description,
	usage string,
	exitStatus int,
	configureFunc func(*FlagSet),
) *SpyCommand {
	return &SpyCommand{
		name:          name,
		description:   description,
		usage:         usage,
		exitStatus:    exitStatus,
		configureFunc: configureFunc,
	}
}

func (c *SpyCommand) Name() string {
	c.NameCount++
	return c.name
}

func (c *SpyCommand) Description() string {
	c.DescriptionCount++
	return c.description
}

func (c *SpyCommand) Usage() string {
	c.UsageCount++
	return c.usage
}

func (c *SpyCommand) Configure(flagSet *FlagSet) {
	c.ConfigureCount++
	c.flagSet = flagSet

	if c.configureFunc != nil {
		c.configureFunc(flagSet)
	}
}

func (c *SpyCommand) Run(ctx context.Context) ExitStatus {
	c.RunCount++

	if c.runFunc != nil {
		c.runFunc(c.flagSet)
	}

	return c.exitStatus
}

// empty fails when the given output of a writer is not empty.
func empty(t *testing.T, writer, got string) {
	t.Helper()

	if got != "" {
		t.Errorf("the %s output should be empty, got:\n%s", writer, got)
	}
}

func testdata(t *testing.T, filename string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	return string(b)
}

// golden compares the given output with the content of a testdata file. Setting
// the UPDATE_GOLDEN environment variable rewrites the file with the output.
func golden(t *testing.T, filename, got string) {
	t.Helper()

	path := filepath.Join("testdata", filename)

	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		fmt.Printf("updated %s\n", path)
	}

	if diff := cmp.Diff(testdata(t, filename), got); diff != "" {
		t.Errorf("console error output mismatch (-want +got):\n%s", diff)
	}
}
