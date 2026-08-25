package console

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"
)

// level is a type of its own, which flags are defined for by registering how
// it is parsed rather than by writing a value for it.
type level int

const (
	low level = iota
	high
)

func (l level) String() string {
	if l == high {
		return "high"
	}

	return "low"
}

// parseLevel is the parser the level type is registered with.
func parseLevel(argument string) (level, error) {
	switch argument {
	case "low":
		return low, nil
	case "high":
		return high, nil
	default:
		return low, errors.New("unknown level " + strconv.Quote(argument))
	}
}

// registerLevel registers the level type, and gives the registry back the types
// it held once the test is over.
func registerLevel(t *testing.T, flagType Type[level]) {
	t.Helper()

	registeredTypes.mu.Lock()
	registered := maps.Clone(registeredTypes.binders)
	registeredTypes.mu.Unlock()

	t.Cleanup(func() {
		registeredTypes.mu.Lock()
		defer registeredTypes.mu.Unlock()

		registeredTypes.binders = registered
	})

	Register(flagType)
}

func TestRegister(t *testing.T) {
	t.Run("a registered type parses the flags of its variables", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel})

		var (
			alerts level
			logs   level
		)

		flagSet := NewFlagSet("test", io.Discard)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Short("a"), Default(low))
		Var(flagSet, &logs, Long("logs"), "the logging level.", Default(high))

		if err := flagSet.Parse([]string{"-a", "high"}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if alerts != high {
			t.Errorf("unexpected level, want high got %s", alerts)
		}

		// the flag which was not provided keeps the default it was defined with.
		if logs != high {
			t.Errorf("unexpected level, want high got %s", logs)
		}
	})

	t.Run("a registered type parses the fields of a struct", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel})

		type configs struct {
			Alerts level `usage:"the alerting level." long:"alerts"`
			Logs   level `usage:"the logging level." long:"logs" default:"high"`
		}

		configuration := &configs{}

		flagSet := NewFlagSet("test", io.Discard)
		if err := flagSet.Struct(configuration); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if err := flagSet.Parse([]string{"--alerts=high"}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if configuration.Alerts != high {
			t.Errorf("unexpected level, want high got %s", configuration.Alerts)
		}

		// the "default" tag is parsed by the type of the field.
		if configuration.Logs != high {
			t.Errorf("unexpected level, want high got %s", configuration.Logs)
		}
	})

	t.Run("a default value the type cannot parse is reported", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Expects: "a level (low or high)", Parse: parseLevel})

		type configs struct {
			Alerts level `long:"alerts" default:"loud"`
		}

		err := (NewFlagSet("test", io.Discard)).Struct(&configs{})
		if !errors.Is(err, ErrInvalidDefault) {
			t.Fatalf("unexpected error, want %v got %v", ErrInvalidDefault, err)
		}

		if !strings.Contains(err.Error(), "can't be parsed as a level (low or high)") {
			t.Errorf("the error should name the type the value was expected to be, got %q", err)
		}
	})

	t.Run("a type explains the values it rejects", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Expects: "a level (low or high)", Parse: parseLevel})

		var (
			errWriter bytes.Buffer
			alerts    level
		)

		flagSet := NewFlagSet("test", &errWriter)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Default(low))

		if err := flagSet.Parse([]string{"--alerts=loud"}); err == nil {
			t.Fatal("an unknown level should have been rejected")
		}

		want := "invalid value \"loud\" for flag --alerts: can't be parsed as a level (low or high)\n"

		if got := errWriter.String(); got != want {
			t.Errorf("unexpected error, want %q got %q", want, got)
		}

		if alerts != low {
			t.Errorf("a rejected value should leave the variable untouched, got %s", alerts)
		}
	})

	t.Run("a type which expects nothing in particular reports the error of its parser", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel})

		var (
			errWriter bytes.Buffer
			alerts    level
		)

		flagSet := NewFlagSet("test", &errWriter)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Default(low))

		if err := flagSet.Parse([]string{"--alerts=loud"}); err == nil {
			t.Fatal("an unknown level should have been rejected")
		}

		want := "invalid value \"loud\" for flag --alerts: unknown level \"loud\"\n"

		if got := errWriter.String(); got != want {
			t.Errorf("unexpected error, want %q got %q", want, got)
		}
	})

	t.Run("a type which takes no value is provided without one", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel, Implicit: "high"})

		var (
			alerts level
			help   bytes.Buffer
		)

		flagSet := NewFlagSet("test", io.Discard)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Short("a"), Default(low))

		if err := flagSet.Parse([]string{"-a", "list"}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if alerts != high {
			t.Errorf("unexpected level, want high got %s", alerts)
		}

		// the flag takes no value, so the argument which follows it is left alone.
		if !equalArgs(flagSet.Args(), []string{"list"}) {
			t.Errorf("unexpected arguments, got %v", flagSet.Args())
		}

		flagSet.PrintDefaults(&help)

		if strings.Contains(help.String(), "--alerts level") {
			t.Errorf("a flag which takes no value should be presented by its name alone:\n%s", help.String())
		}
	})

	t.Run("the help presents the type and its default", func(t *testing.T) {
		registerLevel(t, Type[level]{
			Name:   "level",
			Parse:  parseLevel,
			Format: func(l level) string { return strings.ToUpper(l.String()) },
		})

		var (
			alerts level
			logs   level
			help   bytes.Buffer
		)

		flagSet := NewFlagSet("test", io.Discard)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Default(high))
		Var(flagSet, &logs, Long("logs"), "the logging level.", Default(low))
		flagSet.PrintDefaults(&help)

		want := "      --alerts level  the alerting level. (default HIGH)\n" +
			"      --logs level    the logging level.\n"

		if got := help.String(); !strings.Contains(got, want) {
			t.Errorf("unexpected help, want\n%s\ngot\n%s", want, got)
		}
	})

	t.Run("a type which names itself with nothing is presented as a value", func(t *testing.T) {
		registerLevel(t, Type[level]{Parse: parseLevel})

		var (
			alerts level
			help   bytes.Buffer
		)

		flagSet := NewFlagSet("test", io.Discard)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Default(low))
		flagSet.PrintDefaults(&help)

		if !strings.Contains(help.String(), "--alerts value") {
			t.Errorf("the flag should be named by a value:\n%s", help.String())
		}
	})

	t.Run("registering a type twice replaces the first registration", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel})
		registerLevel(t, Type[level]{
			Name: "level",
			Parse: func(argument string) (level, error) {
				return level(len(argument)), nil // anything goes.
			},
		})

		var alerts level

		flagSet := NewFlagSet("test", io.Discard)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Default(low))

		if err := flagSet.Parse([]string{"--alerts=loud"}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if alerts != 4 {
			t.Errorf("unexpected level, want 4 got %d", alerts)
		}
	})

	t.Run("a flag keeps the type it was defined with", func(t *testing.T) {
		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel})

		var alerts level

		flagSet := NewFlagSet("test", io.Discard)
		Var(flagSet, &alerts, Long("alerts"), "the alerting level.", Default(low))

		// the flag is bound to its type when it is defined, so a later
		// registration doesn't reach it.
		registerLevel(t, Type[level]{Name: "level", Parse: func(string) (level, error) { return high, nil }})

		if err := flagSet.Parse([]string{"--alerts=low"}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if alerts != low {
			t.Errorf("unexpected level, want low got %s", alerts)
		}
	})

	t.Run("a type without a parser cannot be registered", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("a type without a parser should panic")
			}
		}()

		registerLevel(t, Type[level]{Name: "level"})
	})

	t.Run("a variable of a type no flag can be defined for panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("an unregistered type should panic")
			}
		}()

		var hosts []string

		Var(NewFlagSet("test", io.Discard), &hosts, Long("hosts"), "the hosts.")
	})

	t.Run("a variable which parses itself carries its own parsing", func(t *testing.T) {
		// a value of your own is built with the default it carries, as it is
		// the one which knows how to hold it.
		own := plainValue{value: "default"}

		flagSet := NewFlagSet("test", io.Discard)
		flag := Var(flagSet, &own, Long("custom"), "a value of your own.")

		if flag.DefValue() != "default" {
			t.Errorf("unexpected default value, got %q", flag.DefValue())
		}

		if err := flagSet.Parse([]string{"--custom=set"}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if own.value != "set" {
			t.Errorf("unexpected value, got %q", own.value)
		}
	})

	t.Run("a value of your own takes no default", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("a default value given to a value of your own should panic")
			}
		}()

		var own plainValue

		Var(NewFlagSet("test", io.Discard), &own, Long("custom"), "", Default(plainValue{}))
	})
}

func TestDefault(t *testing.T) {
	t.Run("a flag defaults to the value its variable already holds", func(t *testing.T) {
		port := 8080

		flagSet := NewFlagSet("test", io.Discard)
		flag := Var(flagSet, &port, Long("port"), "the port to listen to.")

		if flag.DefValue() != "8080" {
			t.Errorf("unexpected default value, got %q", flag.DefValue())
		}

		if err := flagSet.Parse(nil); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if port != 8080 {
			t.Errorf("the variable should have kept its value, got %d", port)
		}
	})

	t.Run("a default value is written into the variable of the flag", func(t *testing.T) {
		var port int

		flagSet := NewFlagSet("test", io.Discard)
		flag := Var(flagSet, &port, Long("port"), "the port to listen to.", Default(80))

		if port != 80 {
			t.Errorf("the variable should hold the default value, got %d", port)
		}

		if flag.DefValue() != "80" {
			t.Errorf("unexpected default value, got %q", flag.DefValue())
		}
	})

	t.Run("a default value reaches the type of the flag", func(t *testing.T) {
		var (
			size    int64
			retries uint16
			ratio   float32
			timeout time.Duration
			alerts  level
		)

		registerLevel(t, Type[level]{Name: "level", Parse: parseLevel})

		flagSet := NewFlagSet("test", io.Discard)

		// an untyped constant reaches the type of its flag the way it reaches
		// the type of a variable it is assigned to.
		Var(flagSet, &size, Long("size"), "", Default(1024))
		Var(flagSet, &retries, Long("retries"), "", Default(3))
		Var(flagSet, &ratio, Long("ratio"), "", Default(1.5))
		Var(flagSet, &timeout, Long("timeout"), "", Default(10*time.Second))
		Var(flagSet, &alerts, Long("alerts"), "", Default(1))

		switch {
		case size != 1024:
			t.Errorf("unexpected size, got %d", size)
		case retries != 3:
			t.Errorf("unexpected retries, got %d", retries)
		case ratio != 1.5:
			t.Errorf("unexpected ratio, got %v", ratio)
		case timeout != 10*time.Second:
			t.Errorf("unexpected timeout, got %s", timeout)
		case alerts != high:
			t.Errorf("unexpected level, got %s", alerts)
		}
	})

	t.Run("a default value the flag cannot hold panics", func(t *testing.T) {
		testCases := []struct {
			name   string
			define func(*FlagSet)
		}{
			{
				name:   "another class of type",
				define: func(fs *FlagSet) { var v int; Var(fs, &v, Long("flag"), "", Default("80")) },
			},
			{
				name:   "a number given to a string",
				define: func(fs *FlagSet) { var v string; Var(fs, &v, Long("flag"), "", Default(80)) },
			},
			{
				name:   "a number which overflows the type",
				define: func(fs *FlagSet) { var v int8; Var(fs, &v, Long("flag"), "", Default(300)) },
			},
			{
				name:   "a negative number given to an unsigned type",
				define: func(fs *FlagSet) { var v uint; Var(fs, &v, Long("flag"), "", Default(-1)) },
			},
			{
				name:   "a truncated number",
				define: func(fs *FlagSet) { var v int; Var(fs, &v, Long("flag"), "", Default(1.5)) },
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Error("a default value the flag cannot hold should panic")
					}
				}()

				testCase.define(NewFlagSet("test", io.Discard))
			})
		}
	})
}

func TestBuiltinTypes(t *testing.T) {
	t.Run("every Go type a flag can be defined for is registered", func(t *testing.T) {
		var (
			stringFlag   string
			boolFlag     bool
			intFlag      int
			int8Flag     int8
			int16Flag    int16
			int32Flag    int32
			int64Flag    int64
			uintFlag     uint
			uint8Flag    uint8
			uint16Flag   uint16
			uint32Flag   uint32
			uint64Flag   uint64
			float32Flag  float32
			float64Flag  float64
			durationFlag = time.Duration(0)
		)

		testCases := []struct {
			name     string
			pointer  any
			argument string
			want     string // the string form of the parsed value.
		}{
			{"string", &stringFlag, "text", "text"},
			{"bool", &boolFlag, "true", "true"},
			{"int", &intFlag, "-42", "-42"},
			{"int8", &int8Flag, "-42", "-42"},
			{"int16", &int16Flag, "-42", "-42"},
			{"int32", &int32Flag, "-42", "-42"},
			{"int64", &int64Flag, "-42", "-42"},
			{"uint", &uintFlag, "42", "42"},
			{"uint8", &uint8Flag, "42", "42"},
			{"uint16", &uint16Flag, "42", "42"},
			{"uint32", &uint32Flag, "42", "42"},
			{"uint64", &uint64Flag, "42", "42"},
			{"float32", &float32Flag, "1.5", "1.5"},
			{"float64", &float64Flag, "1.5", "1.5"},
			{"duration", &durationFlag, "1.5h", "1h30m0s"},
			{"hexadecimal", &intFlag, "0x2a", "42"},
			{"binary", &uintFlag, "0b101010", "42"},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				value, err := valueOf(testCase.pointer)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				if err := value.Set(testCase.argument); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				if got := value.String(); got != testCase.want {
					t.Errorf("unexpected value, want %q got %q", testCase.want, got)
				}
			})
		}
	})

	t.Run("a value which doesn't fit the width of its type is out of range", func(t *testing.T) {
		var (
			int8Flag  int8
			uint8Flag uint8
		)

		testCases := []struct {
			name     string
			pointer  any
			argument string
			want     string
		}{
			{"int8", &int8Flag, "128", "out of range for an integer"},
			{"uint8", &uint8Flag, "256", "out of range for an unsigned integer (0 or greater)"},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				value, err := valueOf(testCase.pointer)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				err = value.Set(testCase.argument)
				if err == nil {
					t.Fatalf("%q should have been rejected", testCase.argument)
				}

				if err.Error() != testCase.want {
					t.Errorf("unexpected error, want %q got %q", testCase.want, err)
				}

				if !errors.Is(err, errRange) {
					t.Errorf("%v should be a range error", err)
				}
			})
		}
	})

	t.Run("a type no flag can be defined for is reported", func(t *testing.T) {
		var hosts []string

		if _, err := valueOf(&hosts); !errors.Is(err, ErrUnsupportedType) {
			t.Errorf("unexpected error, want %v got %v", ErrUnsupportedType, err)
		}
	})
}
