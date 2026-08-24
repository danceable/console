package console

import (
	"bytes"
	"errors"
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
			options   []FlagOption
			arguments []string
			env       map[string]string
			want      string
		}{
			{
				name:      "long name",
				options:   []FlagOption{Long("username")},
				arguments: []string{"--username", "admin"},
				want:      "admin",
			},
			{
				name:      "long name with an inline value",
				options:   []FlagOption{Long("username")},
				arguments: []string{"--username=admin"},
				want:      "admin",
			},
			{
				name:      "short name",
				options:   []FlagOption{Short("u")},
				arguments: []string{"-u", "admin"},
				want:      "admin",
			},
			{
				name:      "short name with an inline value",
				options:   []FlagOption{Short("u")},
				arguments: []string{"-u=admin"},
				want:      "admin",
			},
			{
				name:      "short name with an attached value",
				options:   []FlagOption{Short("u")},
				arguments: []string{"-uadmin"},
				want:      "admin",
			},
			{
				name:      "environment variable",
				options:   []FlagOption{Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				env:       map[string]string{"CONSOLE_TEST_USERNAME": "admin"},
				want:      "admin",
			},
			{
				name:      "the command line wins over the environment",
				options:   []FlagOption{Long("username"), Short("u"), Env("CONSOLE_TEST_USERNAME")},
				arguments: []string{"-u", "admin"},
				env:       map[string]string{"CONSOLE_TEST_USERNAME": "root"},
				want:      "admin",
			},
			{
				name:      "the environment fills the missing flag",
				options:   []FlagOption{Long("username"), Short("u"), Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				env:       map[string]string{"CONSOLE_TEST_USERNAME": "root"},
				want:      "root",
			},
			{
				name:      "an empty environment variable keeps the default value",
				options:   []FlagOption{Long("username"), Env("CONSOLE_TEST_USERNAME")},
				arguments: nil,
				env:       map[string]string{"CONSOLE_TEST_USERNAME": ""},
				want:      "default",
			},
			{
				name:      "an unset environment variable keeps the default value",
				options:   []FlagOption{Long("username"), Env("CONSOLE_TEST_USERNAME")},
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
				flag := flagSet.StringVar(&username, "default", "the user to authenticate as.", testCase.options...)

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
			options   []FlagOption
			arguments []string
			want      string
		}{
			{
				name:      "long name of a short-only flag",
				options:   []FlagOption{Short("u")},
				arguments: []string{"--username", "admin"},
				want:      "flag provided but not defined: --username\n",
			},
			{
				name:      "short name of a long-only flag",
				options:   []FlagOption{Long("username")},
				arguments: []string{"-u", "admin"},
				want:      "flag provided but not defined: -u\n",
			},
			{
				name:      "any name of an env-only flag",
				options:   []FlagOption{Env("CONSOLE_TEST_USERNAME")},
				arguments: []string{"--username", "admin"},
				want:      "flag provided but not defined: --username\n",
			},
			{
				name:      "a long name provided with a single dash",
				options:   []FlagOption{Long("username")},
				arguments: []string{"-username", "admin"},
				want:      "flag provided but not defined: -username (did you mean --username?)\n",
			},
			{
				name:      "a long name provided with a single dash and an inline value",
				options:   []FlagOption{Long("username")},
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
				flagSet.StringVar(&username, "default", "the user to authenticate as.", testCase.options...)

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
				flagSet.BoolVar(&all, false, "targets everything.", Long("all"), Short("a"))
				flagSet.BoolVar(&force, false, "does not ask for confirmation.", Long("force"), Short("f"))

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
				flagSet.IntVar(&port, 80, "the port to listen to.", Long("port"), Short("p"))

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
				want:      "invalid value \"http\" for flag --port: parse error\n",
			},
			{
				name:      "an out of range value",
				arguments: []string{"--port", "99999999999999999999"},
				want:      "invalid value \"99999999999999999999\" for flag --port: value out of range\n",
			},
			{
				name: "an invalid value of an environment variable",
				env:  map[string]string{"CONSOLE_TEST_PORT": "http"},
				want: "invalid value \"http\" for environment variable CONSOLE_TEST_PORT: parse error\n",
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
				flagSet.IntVar(&port, 80, "the port to listen to.", Long("port"), Short("p"), Env("CONSOLE_TEST_PORT"))

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
			options   []FlagOption
		}{
			{
				name:      "-h",
				arguments: []string{"-h"},
				options:   []FlagOption{Long("port")},
			},
			{
				name:      "--help",
				arguments: []string{"--help"},
				options:   []FlagOption{Long("port")},
			},
			{
				name:      "-h of a defined short flag is not the help",
				arguments: []string{"-h", "8080"},
				options:   []FlagOption{Long("port"), Short("h")},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var (
					errWriter bytes.Buffer
					port      int
				)

				flagSet := NewFlagSet("test", &errWriter)
				flagSet.IntVar(&port, 80, "the port to listen to.", testCase.options...)

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
		flagSet.StringVar(&stringValue, "", "", Long("string"))
		flagSet.BoolVar(&boolValue, false, "", Long("bool"))
		flagSet.IntVar(&intValue, 0, "", Long("int"))
		flagSet.Int64Var(&int64Value, 0, "", Long("int64"))
		flagSet.UintVar(&uintValue, 0, "", Long("uint"))
		flagSet.Uint64Var(&uint64Value, 0, "", Long("uint64"))
		flagSet.Float64Var(&float64Value, 0, "", Long("float64"))
		flagSet.DurationVar(&durationValue, 0, "", Long("duration"))

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
		flag := flagSet.IntVar(&port, 80, "the port to listen to.", Long("port"), Short("p"), Env("CONSOLE_TEST_PORT"))

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
		flagSet.StringVar(&username, "", "the user to authenticate as.", Long("username"), Short("u"), Env("CONSOLE_TEST_USERNAME"))
		flagSet.BoolVar(&all, false, "targets every namespace.", Long("all"), Short("a"))
		flagSet.IntVar(&port, 80, "the port to listen to.", Long("port"))
		flagSet.Float64Var(&ratio, 1.5, "the sampling ratio.", Short("r"))
		flagSet.DurationVar(&timeout, 10*time.Second, "the request timeout.", Long("timeout"), Short("t"))
		flagSet.UintVar(&retries, 3, "the number of retries.", Long("retries"))
		flagSet.StringVar(&secret, "", "the signing secret.", Env("CONSOLE_TEST_SECRET"))
		flagSet.Int64Var(&threshold, 0, "the alerting threshold.", Long("threshold"))
		flagSet.Uint64Var(&size, 0, "the maximum size.", Long("size"))

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
		flagSet.StringVar(&zone, "", "the zone to target.", Long("zone"))
		flagSet.BoolVar(&all, false, "targets every namespace.", Long("all"), Short("a"))
		flagSet.Float64Var(&ratio, 0, "the sampling ratio.", Short("r"))
		flagSet.StringVar(&secret, "", "the signing secret.", Env("CONSOLE_TEST_SECRET"))
		flagSet.BoolVar(&verbose, false, "logs every step.", Long("verbose"), Short("v"))

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
		zoneFlag := flagSet.StringVar(&zone, "", "the zone to target.", Long("zone"))
		allFlag := flagSet.BoolVar(&all, false, "targets every namespace.", Long("all"))

		flagSet.PrintDefaults(io.Discard)

		if flags := flagSet.Flags(); flags[0] != zoneFlag || flags[1] != allFlag {
			t.Error("the flags should stay in definition order")
		}
	})

	t.Run("invalid definitions panic", func(t *testing.T) {
		testCases := []struct {
			name    string
			options []FlagOption
		}{
			{
				name:    "no name at all",
				options: nil,
			},
			{
				name:    "a long name starting with a dash",
				options: []FlagOption{Long("-port")},
			},
			{
				name:    "a long name containing an equal sign",
				options: []FlagOption{Long("port=80")},
			},
			{
				name:    "a short name longer than a character",
				options: []FlagOption{Short("port")},
			},
			{
				name:    "a short name which is a dash",
				options: []FlagOption{Short("-")},
			},
			{
				name:    "a short name which is an equal sign",
				options: []FlagOption{Short("=")},
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

				NewFlagSet("test", nil).IntVar(&port, 80, "the port to listen to.", testCase.options...)
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
			flagSet.IntVar(&port, 80, "the port to listen to.", Long("port"), Short("p"))
			flagSet.IntVar(&otherPort, 8080, "another port.", Long("other-port"), Short("p"))
		})

		t.Run("a long name defined twice", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("a flag defined twice should panic")
				}
			}()

			var port, otherPort int

			flagSet := NewFlagSet("test", nil)
			flagSet.IntVar(&port, 80, "the port to listen to.", Long("port"), Short("p"))
			flagSet.IntVar(&otherPort, 8080, "another port.", Long("port"), Short("o"))
		})
	})

	t.Run("a value which the type cannot parse is reported", func(t *testing.T) {
		testCases := []struct {
			name    string
			define  func(*FlagSet)
			invalid string
		}{
			{
				name:    "bool",
				define:  func(fs *FlagSet) { var v bool; fs.BoolVar(&v, false, "", Long("flag"), Short("f")) },
				invalid: "maybe",
			},
			{
				name:    "int",
				define:  func(fs *FlagSet) { var v int; fs.IntVar(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "abc",
			},
			{
				name:    "int out of range",
				define:  func(fs *FlagSet) { var v int; fs.IntVar(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "99999999999999999999",
			},
			{
				name:    "int64",
				define:  func(fs *FlagSet) { var v int64; fs.Int64Var(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "abc",
			},
			{
				name:    "uint",
				define:  func(fs *FlagSet) { var v uint; fs.UintVar(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "-1",
			},
			{
				name:    "uint64",
				define:  func(fs *FlagSet) { var v uint64; fs.Uint64Var(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "-1",
			},
			{
				name:    "float64",
				define:  func(fs *FlagSet) { var v float64; fs.Float64Var(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "abc",
			},
			{
				name:    "duration",
				define:  func(fs *FlagSet) { var v time.Duration; fs.DurationVar(&v, 0, "", Long("flag"), Short("f")) },
				invalid: "abc",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				// the long and the short forms report the value separately.
				for _, arguments := range [][]string{
					{"--flag=" + testCase.invalid},
					{"-f=" + testCase.invalid},
				} {
					var errWriter bytes.Buffer

					flagSet := NewFlagSet("test", &errWriter)
					testCase.define(flagSet)

					if err := flagSet.Parse(arguments); err == nil {
						t.Fatalf("%q should have been rejected", arguments)
					}

					if !strings.Contains(errWriter.String(), "invalid value") {
						t.Errorf("unexpected error for %q: %s", arguments, errWriter.String())
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
			flagSet.Var(&value, "a value of your own.", Long("custom"))
			flagSet.PrintDefaults(&help)

			if !strings.Contains(help.String(), "--custom value") {
				t.Errorf("the flag should be named by a value:\n%s", help.String())
			}
		})

		t.Run("is reachable through the flag", func(t *testing.T) {
			var value plainValue

			flagSet := NewFlagSet("test", nil)
			flag := flagSet.Var(&value, "a value of your own.", Long("custom"))

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
			name  string
			value Value
			want  string
		}{
			{"bool", newBoolValue(false, &boolFlag), "bool"},
			{"string", newStringValue("", &stringFlag), "string"},
			{"int", newIntValue(0, &intFlag), "int"},
			{"int64", newInt64Value(0, &int64Flag), "int"},
			{"uint", newUintValue(0, &uintFlag), "uint"},
			{"uint64", newUint64Value(0, &uint64Flag), "uint"},
			{"float64", newFloat64Value(0, &float64Flag), "float"},
			{"duration", newDurationValue(0, &durationFlag), "duration"},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				typed, names := testCase.value.(typer)
				if !names {
					t.Fatalf("%T should name its type", testCase.value)
				}

				if got := typed.Type(); got != testCase.want {
					t.Errorf("unexpected type, want %q got %q", testCase.want, got)
				}
			})
		}
	})

	t.Run("an error which is not a number error is kept as it is", func(t *testing.T) {
		err := errors.New("not a strconv error")

		if got := numError(err); !errors.Is(got, err) {
			t.Errorf("unexpected error, want %v got %v", err, got)
		}
	})
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
