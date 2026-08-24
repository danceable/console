package console

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danceable/container"
	"github.com/danceable/provider"
	"github.com/danceable/provider/adapters/danceable"
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

		t.Run("registering a group name twice panics", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("registering the same group twice should panic")
				}
			}()

			group := NewGroup("pods", "manages the pods.")
			group.RegisterGroup(NewGroup("nodes", "manages the nodes."))
			group.RegisterGroup(NewGroup("nodes", "duplicated name."))
		})
	})

	t.Run("service commands", func(t *testing.T) {
		t.Run("the providers are managed around the run", func(t *testing.T) {
			var writer, errWriter bytes.Buffer

			serviceProvider := &SpyProvider{}
			service := NewSpyService("serve", ExitSuccess, []provider.Provider{serviceProvider}, nil)

			console := NewConsole("Test", "Test description", &writer, &errWriter, newManager())
			console.Register(service)

			if exitStatus := console.Run(context.Background(), []string{"", "serve"}); exitStatus != ExitSuccess {
				t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
			}

			if service.BootCount != 1 {
				t.Errorf("the command should have booted once, got %d", service.BootCount)
			}

			if service.RunCount != 1 {
				t.Errorf("the command should have run once, got %d", service.RunCount)
			}

			if service.Container == nil {
				t.Error("the command should have been booted with the container")
			}

			counts := [3]int{serviceProvider.RegisterCount, serviceProvider.BootCount, serviceProvider.TerminateCount}
			if counts != [3]int{1, 1, 1} {
				t.Errorf("unexpected provider lifecycle, want [1 1 1] got %v", counts)
			}

			empty(t, "error", errWriter.String())
		})

		t.Run("the exit status of the command is returned", func(t *testing.T) {
			var writer, errWriter bytes.Buffer

			service := NewSpyService("serve", ExitFailure, nil, nil)

			console := NewConsole("Test", "Test description", &writer, &errWriter, newManager())
			console.Register(service)

			if exitStatus := console.Run(context.Background(), []string{"", "serve"}); exitStatus != ExitFailure {
				t.Errorf("unexpected exit code, want %d got %d", ExitFailure, exitStatus)
			}

			empty(t, "error", errWriter.String())
		})

		t.Run("a command which fails to boot does not run", func(t *testing.T) {
			var writer, errWriter bytes.Buffer

			bootErr := errors.New("the dependency cannot be resolved")
			service := NewSpyService("serve", ExitSuccess, nil, bootErr)

			console := NewConsole("Test", "Test description", &writer, &errWriter, newManager())
			console.Register(service)

			if exitStatus := console.Run(context.Background(), []string{"", "serve"}); exitStatus != ExitFailure {
				t.Errorf("unexpected exit code, want %d got %d", ExitFailure, exitStatus)
			}

			if service.RunCount != 0 {
				t.Errorf("the command should not have run, got %d", service.RunCount)
			}

			if !strings.Contains(errWriter.String(), bootErr.Error()) {
				t.Errorf("unexpected error output: %s", errWriter.String())
			}
		})

		t.Run("a provider which fails to register stops the command", func(t *testing.T) {
			var writer, errWriter bytes.Buffer

			registerErr := errors.New("the provider cannot be registered")
			service := NewSpyService("serve", ExitSuccess, []provider.Provider{&SpyProvider{registerErr: registerErr}}, nil)

			console := NewConsole("Test", "Test description", &writer, &errWriter, newManager())
			console.Register(service)

			if exitStatus := console.Run(context.Background(), []string{"", "serve"}); exitStatus != ExitFailure {
				t.Errorf("unexpected exit code, want %d got %d", ExitFailure, exitStatus)
			}

			if service.BootCount != 0 {
				t.Errorf("the command should not have booted, got %d", service.BootCount)
			}

			if service.RunCount != 0 {
				t.Errorf("the command should not have run, got %d", service.RunCount)
			}

			if !strings.Contains(errWriter.String(), registerErr.Error()) {
				t.Errorf("unexpected error output: %s", errWriter.String())
			}
		})
	})

	t.Run("scoped flags", func(t *testing.T) {
		t.Run("a flag name defined at several levels keeps one value per level", func(t *testing.T) {
			testCases := []struct {
				name      string
				arguments []string
				root      contextLevel
				group     contextLevel
				command   contextLevel
				effective string
			}{
				{
					name:      "every level is provided",
					arguments: []string{"kubectl", "--context=server-1", "get", "--context=eu-west", "pods", "--context=running"},
					root:      contextLevel{"server-1", true},
					group:     contextLevel{"eu-west", true},
					command:   contextLevel{"running", true},
					effective: "running",
				},
				{
					name:      "only the console is provided",
					arguments: []string{"kubectl", "--context=server-1", "get", "pods"},
					root:      contextLevel{"server-1", true},
					group:     contextLevel{"staging", false},
					command:   contextLevel{"minikube", false},
					effective: "server-1",
				},
				{
					name:      "only the group is provided",
					arguments: []string{"kubectl", "get", "--context=eu-west", "pods"},
					root:      contextLevel{"default", false},
					group:     contextLevel{"eu-west", true},
					command:   contextLevel{"minikube", false},
					effective: "eu-west",
				},
				{
					name:      "only the command is provided",
					arguments: []string{"kubectl", "get", "pods", "--context=running"},
					root:      contextLevel{"default", false},
					group:     contextLevel{"staging", false},
					command:   contextLevel{"running", true},
					effective: "running",
				},
				{
					name:      "no level is provided",
					arguments: []string{"kubectl", "get", "pods"},
					root:      contextLevel{"default", false},
					group:     contextLevel{"staging", false},
					command:   contextLevel{"minikube", false},
					effective: "default",
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var writer, errWriter bytes.Buffer
					console, fixture := scopedFlags(&writer, &errWriter)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != ExitSuccess {
						t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
					}

					if !fixture.executed {
						t.Fatal("the command was not executed")
					}

					levels := []struct {
						name string
						want contextLevel
						got  *contextFixture
					}{
						{"console", testCase.root, fixture.root},
						{"group", testCase.group, fixture.group},
						{"command", testCase.command, fixture.command},
					}

					for _, level := range levels {
						got := contextLevel{level.got.context, level.got.provided()}

						if diff := cmp.Diff(level.want, got, cmp.AllowUnexported(contextLevel{})); diff != "" {
							t.Errorf("unexpected %s level (-want +got):\n%s", level.name, diff)
						}
					}

					if fixture.effective != testCase.effective {
						t.Errorf("unexpected effective context, want %q got %q", testCase.effective, fixture.effective)
					}

					empty(t, "error", errWriter.String())
				})
			}
		})

		t.Run("every level is parsed by its own flag set", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console, fixture := scopedFlags(&writer, &errWriter)

			if exitStatus := console.Run(context.Background(), []string{"kubectl", "get", "pods"}); exitStatus != ExitSuccess {
				t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
			}

			flagSets := []*FlagSet{fixture.root.flagSet, fixture.group.flagSet, fixture.command.flagSet}

			for i, flagSet := range flagSets {
				for _, other := range flagSets[i+1:] {
					if flagSet == other {
						t.Error("the levels should not share a flag set")
					}

					if flagSet.Lookup("context") == other.Lookup("context") {
						t.Error("the levels should not share a flag")
					}
				}
			}
		})

		t.Run("a level only defines the flags of its own scope", func(t *testing.T) {
			var writer, errWriter bytes.Buffer
			console, _ := scopedFlags(&writer, &errWriter)

			// "--unknown" is defined by no level at all.
			if exitStatus := console.Run(context.Background(), []string{"kubectl", "get", "--unknown", "pods"}); exitStatus != ExitUsageError {
				t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
			}

			if !strings.Contains(errWriter.String(), "flag provided but not defined: --unknown") {
				t.Errorf("unexpected error output: %s", errWriter.String())
			}
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

// contextFixture holds one level's "--context" flag, together with the flag
// set which defines it, so that a provided value can be told apart from a
// default one.
type contextFixture struct {
	context string
	flagSet *FlagSet
}

// bind defines the "--context" flag of the level. Every level binds its own
// variable, as a flag writes its default value when it is defined and two
// levels sharing one would overwrite each other.
func (f *contextFixture) bind(flagSet *FlagSet, defaultContext string) {
	f.flagSet = flagSet

	flagSet.StringVar(&f.context, defaultContext, "the context to work against.", Long("context"))
}

// provided reports whether the flag was provided at this level.
func (f *contextFixture) provided() bool {
	return f.flagSet != nil && f.flagSet.Lookup("context").Provided()
}

// contextLevel is the expected state of one level's "--context" flag.
type contextLevel struct {
	context  string
	provided bool
}

// scopedFlagsFixture holds the "--context" flag of every level of the scoped
// flags fixture, and the context the command resolves out of them.
type scopedFlagsFixture struct {
	root    *contextFixture
	group   *contextFixture
	command *contextFixture

	effective string
	executed  bool
}

// scopedFlags builds a console where the console, the "get" group and the
// "pods" command each define their own "--context" flag:
//
//	kubectl --context=server-1 get --context=eu-west pods --context=running
func scopedFlags(writer, errWriter *bytes.Buffer) (*Console, *scopedFlagsFixture) {
	fixture := &scopedFlagsFixture{
		root:    &contextFixture{},
		group:   &contextFixture{},
		command: &contextFixture{},
	}

	pods := NewSpyCommand("pods", "gets the pods.", "kubectl get pods [flags]", 0, func(fs *FlagSet) {
		fixture.command.bind(fs, "minikube")
	})
	pods.runFunc = func(fs *FlagSet) {
		fixture.executed = true

		// the deepest level which was provided wins, the console's value
		// applies when none of the deeper ones was.
		fixture.effective = fixture.root.context

		for _, level := range []*contextFixture{fixture.group, fixture.command} {
			if level.provided() {
				fixture.effective = level.context
			}
		}
	}

	get := NewGroup("get", "gets resources.").
		Flags(func(fs *FlagSet) { fixture.group.bind(fs, "staging") }).
		Register(pods)

	console := NewConsole("kubectl", "controls the cluster manager.", writer, errWriter, provider.Default)
	console.Flags(func(fs *FlagSet) { fixture.root.bind(fs, "default") })
	console.RegisterGroup(get)

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

// SpyService is a command whose service providers are managed around its run.
type SpyService struct {
	*SpyCommand

	providers []provider.Provider
	bootErr   error

	BootCount int
	Container provider.Container
}

var _ Service = &SpyService{}

func NewSpyService(name string, exitStatus int, providers []provider.Provider, bootErr error) *SpyService {
	return &SpyService{
		SpyCommand: NewSpyCommand(name, "a service command.", "test "+name, exitStatus, nil),
		providers:  providers,
		bootErr:    bootErr,
	}
}

func (s *SpyService) Providers() []provider.Provider {
	return s.providers
}

func (s *SpyService) Boot(ctx context.Context, container provider.Container) error {
	s.BootCount++
	s.Container = container

	return s.bootErr
}

// SpyProvider is a service provider which counts the calls of its lifecycle.
type SpyProvider struct {
	registerErr error

	RegisterCount  int
	BootCount      int
	TerminateCount int
}

var _ provider.Provider = &SpyProvider{}

func (p *SpyProvider) Register(ctx context.Context, container provider.Container) error {
	p.RegisterCount++

	return p.registerErr
}

func (p *SpyProvider) Boot(ctx context.Context, container provider.Container) error {
	p.BootCount++

	return nil
}

func (p *SpyProvider) Terminate(ctx context.Context) error {
	p.TerminateCount++

	return nil
}

// newManager returns a manager of its own, so the providers a test registers
// are not shared with the other ones.
func newManager() *provider.Manager {
	return provider.New(danceable.New(container.New()))
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
