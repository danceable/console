package console

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestFlagSet(t *testing.T) {
	t.Run("a flag is provided by the names it defines", func(t *testing.T) {
		testCases := []struct {
			name      string
			names     []FlagName
			arguments []string
			env       map[string]string
			want      string
		}{
			{
				name:      "long name",
				names:     []FlagName{Long("username")},
				arguments: []string{"--username", "admin"},
				want:      "admin",
			},
			{
				name:      "long name with an inline value",
				names:     []FlagName{Long("username")},
				arguments: []string{"--username=admin"},
				want:      "admin",
			},
			{
				name:      "short name",
				names:     []FlagName{Short("u")},
				arguments: []string{"-u", "admin"},
				want:      "admin",
			},
			{
				name:      "short name with an inline value",
				names:     []FlagName{Short("u")},
				arguments: []string{"-u=admin"},
				want:      "admin",
			},
			{
				name:      "short name with an attached value",
				names:     []FlagName{Short("u")},
				arguments: []string{"-uadmin"},
				want:      "admin",
			},
			{
				name:      "environment variable",
				names:     []FlagName{Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				env:       map[string]string{"CONSOLE_TEST_USERNAME": "admin"},
				want:      "admin",
			},
			{
				name:      "the command line wins over the environment",
				names:     []FlagName{Long("username"), Short("u"), Env("CONSOLE_TEST_USERNAME")},
				arguments: []string{"-u", "admin"},
				env:       map[string]string{"CONSOLE_TEST_USERNAME": "root"},
				want:      "admin",
			},
			{
				name:      "the environment fills the missing flag",
				names:     []FlagName{Long("username"), Short("u"), Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				env:       map[string]string{"CONSOLE_TEST_USERNAME": "root"},
				want:      "root",
			},
			{
				name:      "an empty environment variable keeps the default value",
				names:     []FlagName{Long("username"), Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				env:       map[string]string{"CONSOLE_TEST_USERNAME": ""},
				want:      "default",
			},
			{
				name:      "an unset environment variable keeps the default value",
				names:     []FlagName{Long("username"), Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				want:      "default",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				for name, value := range testCase.env {
					t.Setenv(name, value)
				}

				var (
					errWriter bytes.Buffer
					username  string
				)

				flagSet := NewFlagSet("test", &errWriter)
				flag := defineFlag(flagSet, &username, "default", "the user to authenticate as.", testCase.names...)

				if err := flagSet.Parse(testCase.arguments); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				if username != testCase.want {
					t.Errorf("unexpected value, want %q got %q", testCase.want, username)
				}

				if provided := len(testCase.arguments) > 0; flag.Provided() != provided {
					t.Errorf("unexpected provided state, want %t got %t", provided, flag.Provided())
				}

				if want := username != "default" && len(testCase.arguments) == 0; flag.FromEnv() != want {
					t.Errorf("unexpected environment state, want %t got %t", want, flag.FromEnv())
				}

				if errWriter.Len() > 0 {
					t.Errorf("unexpected output: %s", errWriter.String())
				}
			})
		}
	})

	t.Run("a name which is not defined is not enabled", func(t *testing.T) {
		testCases := []struct {
			name      string
			names     []FlagName
			arguments []string
			want      string
		}{
			{
				name:      "long name of a short-only flag",
				names:     []FlagName{Short("u")},
				arguments: []string{"--username", "admin"},
				want:      "flag provided but not defined: --username\n",
			},
			{
				name:      "short name of a long-only flag",
				names:     []FlagName{Long("username")},
				arguments: []string{"-u", "admin"},
				want:      "flag provided but not defined: -u\n",
			},
			{
				name:      "any name of an env-only flag",
				names:     []FlagName{Env("CONSOLE_TEST_USERNAME")},
				arguments: []string{"--username", "admin"},
				want:      "flag provided but not defined: --username\n",
			},
			{
				name:      "a long name provided with a single dash",
				names:     []FlagName{Long("username")},
				arguments: []string{"-username", "admin"},
				want:      "flag provided but not defined: -username (did you mean --username?)\n",
			},
			{
				name:      "a long name provided with a single dash and an inline value",
				names:     []FlagName{Long("username")},
				arguments: []string{"-username=admin"},
				want:      "flag provided but not defined: -username (did you mean --username?)\n",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var (
					errWriter bytes.Buffer
					username  string
				)

				flagSet := NewFlagSet("test", &errWriter)
				defineFlag(flagSet, &username, "default", "the user to authenticate as.", testCase.names...)

				if err := flagSet.Parse(testCase.arguments); err == nil {
					t.Fatal("an error was expected")
				}

				if diff := cmp.Diff(testCase.want, errWriter.String()); diff != "" {
					t.Errorf("error output mismatch (-want +got):\n%s", diff)
				}

				if username != "default" {
					t.Errorf("unexpected value, want %q got %q", "default", username)
				}
			})
		}
	})

	t.Run("boolean flags", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			wantAll   bool
			wantForce bool
			wantArgs  []string
		}{
			{
				name:      "without a value",
				arguments: []string{"--all"},
				wantAll:   true,
			},
			{
				name:      "with an inline value",
				arguments: []string{"--all=false"},
				wantAll:   false,
			},
			{
				name:      "combined short names",
				arguments: []string{"-af"},
				wantAll:   true,
				wantForce: true,
			},
			{
				name:      "the next argument is not consumed as a value",
				arguments: []string{"--all", "list"},
				wantAll:   true,
				wantArgs:  []string{"list"},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var (
					errWriter  bytes.Buffer
					all, force bool
				)

				flagSet := NewFlagSet("test", &errWriter)
				Var(flagSet, &all, Long("all"), "targets everything.", Short("a"))
				Var(flagSet, &force, Long("force"), "does not ask for confirmation.", Short("f"))

				if err := flagSet.Parse(testCase.arguments); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				if all != testCase.wantAll {
					t.Errorf("unexpected value, want %t got %t", testCase.wantAll, all)
				}

				if force != testCase.wantForce {
					t.Errorf("unexpected value, want %t got %t", testCase.wantForce, force)
				}

				if diff := cmp.Diff(testCase.wantArgs, flagSet.Args(), cmp.Comparer(equalArgs)); diff != "" {
					t.Errorf("arguments mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("parsing stops before the arguments", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			wantPort  int
			wantArgs  []string
		}{
			{
				name:      "the first non-flag argument stops the parsing",
				arguments: []string{"--port", "8080", "list", "--limit", "5"},
				wantPort:  8080,
				wantArgs:  []string{"list", "--limit", "5"},
			},
			{
				name:      "a double dash stops the parsing",
				arguments: []string{"--port", "8080", "--", "--limit", "5"},
				wantPort:  8080,
				wantArgs:  []string{"--limit", "5"},
			},
			{
				name:      "a single dash is an argument",
				arguments: []string{"-"},
				wantPort:  80,
				wantArgs:  []string{"-"},
			},
			{
				name:      "no argument",
				arguments: nil,
				wantPort:  80,
				wantArgs:  nil,
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var (
					errWriter bytes.Buffer
					port      int
				)

				flagSet := NewFlagSet("test", &errWriter)
				Var(flagSet, &port, Long("port"), "the port to listen to.", Short("p"), Default(80))

				if err := flagSet.Parse(testCase.arguments); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				if port != testCase.wantPort {
					t.Errorf("unexpected value, want %d got %d", testCase.wantPort, port)
				}

				if diff := cmp.Diff(testCase.wantArgs, flagSet.Args(), cmp.Comparer(equalArgs)); diff != "" {
					t.Errorf("arguments mismatch (-want +got):\n%s", diff)
				}

				if want, got := len(testCase.wantArgs), flagSet.NArg(); want != got {
					t.Errorf("unexpected arguments count, want %d got %d", want, got)
				}

				if len(testCase.wantArgs) > 0 && flagSet.Arg(0) != testCase.wantArgs[0] {
					t.Errorf("unexpected argument, want %q got %q", testCase.wantArgs[0], flagSet.Arg(0))
				}

				if flagSet.Arg(len(testCase.wantArgs)) != "" {
					t.Error("an out of range argument should be empty")
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			env       map[string]string
			want      string
		}{
			{
				name:      "a missing value",
				arguments: []string{"--port"},
				want:      "flag needs an argument: --port\n",
			},
			{
				name:      "a missing value of a short name",
				arguments: []string{"-p"},
				want:      "flag needs an argument: -p\n",
			},
			{
				name:      "an invalid value",
				arguments: []string{"--port", "http"},
				want:      "invalid value \"http\" for flag --port: can't be parsed as an integer\n",
			},
			{
				name:      "an out of range value",
				arguments: []string{"--port", "99999999999999999999"},
				want:      "invalid value \"99999999999999999999\" for flag --port: out of range for an integer\n",
			},
			{
				name: "an invalid value of an environment variable",
				env:  map[string]string{"CONSOLE_TEST_PORT": "http"},
				want: "invalid value \"http\" for environment variable CONSOLE_TEST_PORT: can't be parsed as an integer\n",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				for name, value := range testCase.env {
					t.Setenv(name, value)
				}

				var (
					errWriter bytes.Buffer
					port      int
					usages    int
				)

				flagSet := NewFlagSet("test", &errWriter)
				flagSet.Usage = func() { usages++ }
				Var(flagSet, &port, Long("port"), "the port to listen to.", Short("p"), Env("CONSOLE_TEST_PORT"), Default(80))

				if err := flagSet.Parse(testCase.arguments); err == nil {
					t.Fatal("an error was expected")
				}

				if diff := cmp.Diff(testCase.want, errWriter.String()); diff != "" {
					t.Errorf("error output mismatch (-want +got):\n%s", diff)
				}

				if usages != 1 {
					t.Errorf("the usage should be printed once, got %d", usages)
				}

				if port != 80 {
					t.Errorf("unexpected value, want %d got %d", 80, port)
				}
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			names     []FlagName
		}{
			{
				name:      "-h",
				arguments: []string{"-h"},
				names:     []FlagName{Long("port")},
			},
			{
				name:      "--help",
				arguments: []string{"--help"},
				names:     []FlagName{Long("port")},
			},
			{
				name:      "-h of a defined short flag is not the help",
				arguments: []string{"-h", "8080"},
				names:     []FlagName{Long("port"), Short("h")},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var (
					errWriter bytes.Buffer
					port      int
				)

				flagSet := NewFlagSet("test", &errWriter)
				defineFlag(flagSet, &port, 80, "the port to listen to.", testCase.names...)

				err := flagSet.Parse(testCase.arguments)

				if wantHelp := len(testCase.arguments) == 1; errors.Is(err, ErrHelp) != wantHelp {
					t.Fatalf("unexpected error: %v", err)
				}

				if errWriter.Len() > 0 {
					t.Errorf("the help should not be written by the flag set, got: %s", errWriter.String())
				}
			})
		}
	})

	t.Run("value types", func(t *testing.T) {
		var (
			errWriter bytes.Buffer

			stringValue   string
			boolValue     bool
			intValue      int
			int64Value    int64
			uintValue     uint
			uint64Value   uint64
			float64Value  float64
			durationValue time.Duration
		)

		flagSet := NewFlagSet("test", &errWriter)
		Var(flagSet, &stringValue, Long("string"), "")
		Var(flagSet, &boolValue, Long("bool"), "")
		Var(flagSet, &intValue, Long("int"), "")
		Var(flagSet, &int64Value, Long("int64"), "")
		Var(flagSet, &uintValue, Long("uint"), "")
		Var(flagSet, &uint64Value, Long("uint64"), "")
		Var(flagSet, &float64Value, Long("float64"), "")
		Var(flagSet, &durationValue, Long("duration"), "")

		arguments := []string{
			"--string", "value",
			"--bool",
			"--int", "-1",
			"--int64", "-2",
			"--uint", "3",
			"--uint64", "4",
			"--float64", "5.5",
			"--duration", "6s",
		}

		if err := flagSet.Parse(arguments); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if stringValue != "value" || !boolValue || intValue != -1 || int64Value != -2 ||
			uintValue != 3 || uint64Value != 4 || float64Value != 5.5 || durationValue != 6*time.Second {
			t.Errorf(
				"unexpected values: %q %t %d %d %d %d %f %s",
				stringValue, boolValue, intValue, int64Value, uintValue, uint64Value, float64Value, durationValue,
			)
		}

		for _, name := range []string{"string", "int", "int64", "uint", "uint64", "float64", "duration"} {
			flag := flagSet.Lookup(name)
			if flag == nil {
				t.Fatalf("flag %q is not defined", name)
			}

			if !flag.Provided() {
				t.Errorf("flag %q should be marked as provided", name)
			}
		}
	})

	t.Run("lookup", func(t *testing.T) {
		var (
			errWriter bytes.Buffer
			port      int
		)

		flagSet := NewFlagSet("test", &errWriter)
		flag := Var(flagSet, &port, Long("port"), "the port to listen to.", Short("p"), Env("CONSOLE_TEST_PORT"), Default(80))

		for _, name := range []string{"port", "p", "CONSOLE_TEST_PORT"} {
			if got := flagSet.Lookup(name); got != flag {
				t.Errorf("flag %q should be found", name)
			}
		}

		if flagSet.Lookup("unknown") != nil {
			t.Error("an undefined flag should not be found")
		}

		if flag.Long() != "port" || flag.Short() != "p" || flag.Env() != "CONSOLE_TEST_PORT" {
			t.Errorf("unexpected names: %q %q %q", flag.Long(), flag.Short(), flag.Env())
		}

		if flag.Usage() != "the port to listen to." || flag.DefValue() != "80" {
			t.Errorf("unexpected usage %q or default value %q", flag.Usage(), flag.DefValue())
		}

		if want, got := 1, len(flagSet.Flags()); want != got {
			t.Errorf("unexpected flags count, want %d got %d", want, got)
		}

		if flagSet.Name() != "test" {
			t.Errorf("unexpected name, want %q got %q", "test", flagSet.Name())
		}

		if flagSet.ErrWriter() != &errWriter {
			t.Error("unexpected error writer")
		}

		var other bytes.Buffer

		flagSet.SetErrWriter(&other)

		if flagSet.ErrWriter() != &other {
			t.Error("the error writer should have been replaced")
		}
	})

	t.Run("print defaults", func(t *testing.T) {
		var (
			errWriter bytes.Buffer

			username  string
			all       bool
			port      int
			ratio     float64
			timeout   time.Duration
			retries   uint
			secret    string
			threshold int64
			size      uint64
		)

		flagSet := NewFlagSet("test", &errWriter)
		Var(flagSet, &username, Long("username"), "the user to authenticate as.", Short("u"), Env("CONSOLE_TEST_USERNAME"))
		Var(flagSet, &all, Long("all"), "targets every namespace.", Short("a"))
		Var(flagSet, &port, Long("port"), "the port to listen to.", Default(80))
		Var(flagSet, &ratio, Short("r"), "the sampling ratio.", Default(1.5))
		Var(flagSet, &timeout, Long("timeout"), "the request timeout.", Short("t"), Default(10*time.Second))
		Var(flagSet, &retries, Long("retries"), "the number of retries.", Default(3))
		Var(flagSet, &secret, Env("CONSOLE_TEST_SECRET"), "the signing secret.")
		Var(flagSet, &threshold, Long("threshold"), "the alerting threshold.")
		Var(flagSet, &size, Long("size"), "the maximum size.")

		var b bytes.Buffer
		flagSet.PrintDefaults(&b)

		golden(t, "flag-defaults.txt", b.String())
	})

	t.Run("the flags are printed in alphabetical order", func(t *testing.T) {
		var (
			zone    string
			all     bool
			ratio   float64
			secret  string
			verbose bool
		)

		flagSet := NewFlagSet("test", nil)
		Var(flagSet, &zone, Long("zone"), "the zone to target.")
		Var(flagSet, &all, Long("all"), "targets every namespace.", Short("a"))
		Var(flagSet, &ratio, Short("r"), "the sampling ratio.")
		Var(flagSet, &secret, Env("CONSOLE_TEST_SECRET"), "the signing secret.")
		Var(flagSet, &verbose, Long("verbose"), "logs every step.", Short("v"))

		var b bytes.Buffer
		flagSet.PrintDefaults(&b)

		// an env-only flag sorts by its environment variable name, and the help
		// flag always closes the list.
		want := []string{"-a, --all", "CONSOLE_TEST_SECRET", "-r float", "-v, --verbose", "--zone string", "-h, --help"}

		got := b.String()
		previous := 0

		for _, name := range want {
			at := strings.Index(got, name)

			switch {
			case at < 0:
				t.Fatalf("%q is missing from the help:\n%s", name, got)
			case at < previous:
				t.Errorf("%q is out of order, the flags should be sorted:\n%s", name, got)
			}

			previous = at
		}
	})

	t.Run("printing the flags leaves the definition order untouched", func(t *testing.T) {
		var (
			zone string
			all  bool
		)

		flagSet := NewFlagSet("test", nil)
		zoneFlag := Var(flagSet, &zone, Long("zone"), "the zone to target.")
		allFlag := Var(flagSet, &all, Long("all"), "targets every namespace.")

		flagSet.PrintDefaults(io.Discard)

		if flags := flagSet.Flags(); flags[0] != zoneFlag || flags[1] != allFlag {
			t.Error("the flags should stay in definition order")
		}
	})

	t.Run("invalid definitions panic", func(t *testing.T) {
		testCases := []struct {
			name  string
			names []FlagName
		}{
			{
				name:  "no name at all",
				names: nil,
			},
			{
				name:  "a long name starting with a dash",
				names: []FlagName{Long("-port")},
			},
			{
				name:  "a long name containing an equal sign",
				names: []FlagName{Long("port=80")},
			},
			{
				name:  "a short name longer than a character",
				names: []FlagName{Short("port")},
			},
			{
				name:  "a short name which is a dash",
				names: []FlagName{Short("-")},
			},
			{
				name:  "a short name which is an equal sign",
				names: []FlagName{Short("=")},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Error("an invalid flag definition should panic")
					}
				}()

				var port int

				defineFlag(NewFlagSet("test", nil), &port, 80, "the port to listen to.", testCase.names...)
			})
		}

		t.Run("a short name defined twice", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("a flag defined twice should panic")
				}
			}()

			var port, otherPort int

			flagSet := NewFlagSet("test", nil)
			Var(flagSet, &port, Long("port"), "the port to listen to.", Short("p"), Default(80))
			Var(flagSet, &otherPort, Long("other-port"), "another port.", Short("p"), Default(8080))
		})

		t.Run("a long name defined twice", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("a flag defined twice should panic")
				}
			}()

			var port, otherPort int

			flagSet := NewFlagSet("test", nil)
			Var(flagSet, &port, Long("port"), "the port to listen to.", Short("p"), Default(80))
			Var(flagSet, &otherPort, Long("port"), "another port.", Short("o"), Default(8080))
		})
	})

	t.Run("a value which the type cannot parse is reported", func(t *testing.T) {
		testCases := []struct {
			name    string
			define  func(*FlagSet)
			invalid string
			want    string // the reason the value is rejected with.
		}{
			{
				name:    "bool",
				define:  func(fs *FlagSet) { var v bool; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "maybe",
				want:    "can't be parsed as a boolean (true or false)",
			},
			{
				name:    "int",
				define:  func(fs *FlagSet) { var v int; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "abc",
				want:    "can't be parsed as an integer",
			},
			{
				name:    "int out of range",
				define:  func(fs *FlagSet) { var v int; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "99999999999999999999",
				want:    "out of range for an integer",
			},
			{
				name:    "int64",
				define:  func(fs *FlagSet) { var v int64; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "abc",
				want:    "can't be parsed as an integer",
			},
			{
				name:    "uint",
				define:  func(fs *FlagSet) { var v uint; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "-1",
				want:    "can't be parsed as an unsigned integer (0 or greater)",
			},
			{
				name:    "uint64",
				define:  func(fs *FlagSet) { var v uint64; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "-1",
				want:    "can't be parsed as an unsigned integer (0 or greater)",
			},
			{
				name:    "float64",
				define:  func(fs *FlagSet) { var v float64; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "abc",
				want:    "can't be parsed as a floating point number",
			},
			{
				name:    "duration",
				define:  func(fs *FlagSet) { var v time.Duration; Var(fs, &v, Long("flag"), "", Short("f")) },
				invalid: "abc",
				want:    `can't be parsed as a duration (such as "300ms", "1.5h" or "2h45m")`,
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				// the long and the short forms report the value separately,
				// and the reason names the type the value was expected to be.
				for name, argument := range map[string]string{
					"--flag": "--flag=" + testCase.invalid,
					"-f":     "-f=" + testCase.invalid,
				} {
					var errWriter bytes.Buffer

					flagSet := NewFlagSet("test", &errWriter)
					testCase.define(flagSet)

					if err := flagSet.Parse([]string{argument}); err == nil {
						t.Fatalf("%q should have been rejected", argument)
					}

					want := fmt.Sprintf("invalid value %q for flag %s: %s\n", testCase.invalid, name, testCase.want)

					if got := errWriter.String(); got != want {
						t.Errorf("unexpected error for %q, want %q got %q", argument, want, got)
					}
				}
			})
		}
	})

	t.Run("a value of your own", func(t *testing.T) {
		t.Run("names itself a value when it has no type", func(t *testing.T) {
			var (
				help  bytes.Buffer
				value plainValue
			)

			flagSet := NewFlagSet("test", nil)
			flagSet.Var(&value, Long("custom"), "a value of your own.")
			flagSet.PrintDefaults(&help)

			if !strings.Contains(help.String(), "--custom value") {
				t.Errorf("the flag should be named by a value:\n%s", help.String())
			}
		})

		t.Run("is reachable through the flag", func(t *testing.T) {
			var value plainValue

			flagSet := NewFlagSet("test", nil)
			flag := flagSet.Var(&value, Long("custom"), "a value of your own.")

			if flag.Value() != &value {
				t.Error("the flag should hold the value it was defined with")
			}

			if err := flagSet.Parse([]string{"--custom=set"}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if flagSet.Lookup("custom").Value().String() != "set" {
				t.Errorf("unexpected value, got %q", flagSet.Lookup("custom").Value().String())
			}
		})
	})

	t.Run("the types of the values", func(t *testing.T) {
		var (
			boolFlag     bool
			stringFlag   string
			intFlag      int
			int64Flag    int64
			uintFlag     uint
			uint64Flag   uint64
			float64Flag  float64
			durationFlag time.Duration
		)

		testCases := []struct {
			name    string
			pointer any
			want    string
		}{
			{"bool", &boolFlag, "bool"},
			{"string", &stringFlag, "string"},
			{"int", &intFlag, "int"},
			{"int64", &int64Flag, "int"},
			{"uint", &uintFlag, "uint"},
			{"uint64", &uint64Flag, "uint"},
			{"float64", &float64Flag, "float"},
			{"duration", &durationFlag, "duration"},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				value, err := valueOf(testCase.pointer)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				typed, names := value.(typer)
				if !names {
					t.Fatalf("%T should name its type", value)
				}

				if got := typed.Type(); got != testCase.want {
					t.Errorf("unexpected type, want %q got %q", testCase.want, got)
				}
			})
		}
	})

	t.Run("the reason a value is rejected stays matchable", func(t *testing.T) {
		if err := cantParse("an integer"); !errors.Is(err, errParse) {
			t.Errorf("%v should be a parse error", err)
		}

		if err := outOfRange("an integer"); !errors.Is(err, errRange) {
			t.Errorf("%v should be a range error", err)
		}
	})

	t.Run("a type which expects nothing in particular keeps the error of its parser", func(t *testing.T) {
		var (
			flagType = Type[string]{Parse: func(string) (string, error) { return "", errors.New("nope") }}
			flag     string
		)

		flagSet := NewFlagSet("test", io.Discard)
		flagSet.Var(&value[string]{pointer: &flag, flagType: flagType}, Long("flag"), "")

		want := `invalid value "anything" for flag --flag: nope`

		if err := flagSet.Parse([]string{"--flag=anything"}); err == nil || err.Error() != want {
			t.Errorf("unexpected error, want %q got %v", want, err)
		}
	})
}

// defineFlag defines a flag from a list of names, the first of which is the one
// it is defined by, and from the value it defaults to. A list which is empty
// leaves the flag nameless, which is what the definition it panics on looks
// like.
func defineFlag[T any](flagSet *FlagSet, p *T, value T, usage string, names ...FlagName) *Flag {
	var (
		name    FlagName
		options = []FlagOption{Default(value)}
	)

	if len(names) > 0 {
		name = names[0]

		for _, other := range names[1:] {
			options = append(options, other)
		}
	}

	return Var(flagSet, p, name, usage, options...)
}

// plainValue is a flag value which names no type of its own, so the help falls
// back to calling it a value.
type plainValue struct {
	value string
}

func (p *plainValue) String() string { return p.value }

func (p *plainValue) Set(value string) error {
	p.value = value

	return nil
}

// equalArgs compares two argument lists, treating a nil and an empty list as equal.
func equalArgs(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}

	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}

	return true
}
