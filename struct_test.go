package console

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFlagSetStruct(t *testing.T) {
	t.Run("defines a flag per tagged field", func(t *testing.T) {
		type configs struct {
			AppEnv       string        `usage:"the deployment environment." env:"APP_ENV"`
			PostgresHost string        `usage:"the host." env:"POSTGRES_HOST" long:"postgres-host" short:"H"`
			PostgresPort int           `usage:"the port." long:"postgres-port"`
			SSLMode      bool          `usage:"the ssl mode." short:"s"`
			Timeout      time.Duration `usage:"the timeout." long:"timeout"`
		}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(&configs{}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		testCases := []struct {
			name  string
			long  string
			short string
			env   string
		}{
			{name: "APP_ENV", env: "APP_ENV"},
			{name: "postgres-host", long: "postgres-host", short: "H", env: "POSTGRES_HOST"},
			{name: "postgres-port", long: "postgres-port"},
			{name: "s", short: "s"},
			{name: "timeout", long: "timeout"},
		}

		if got := len(flagSet.Flags()); got != len(testCases) {
			t.Fatalf("unexpected number of flags, want %d got %d", len(testCases), got)
		}

		for i, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				flag := flagSet.Flags()[i]

				if flag.Long() != testCase.long {
					t.Errorf("unexpected long name, want %q got %q", testCase.long, flag.Long())
				}

				if flag.Short() != testCase.short {
					t.Errorf("unexpected short name, want %q got %q", testCase.short, flag.Short())
				}

				if flag.Env() != testCase.env {
					t.Errorf("unexpected env name, want %q got %q", testCase.env, flag.Env())
				}
			})
		}
	})

	t.Run("the order of the tags is irrelevant", func(t *testing.T) {
		type configs struct {
			Host string `short:"H" env:"HOST" usage:"the host." long:"host"`
		}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(&configs{}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		flag := flagSet.Lookup("host")
		if flag == nil {
			t.Fatal("the flag was not defined")
		}

		if flag.Short() != "H" || flag.Env() != "HOST" || flag.Usage() != "the host." {
			t.Errorf("unexpected flag: short=%q env=%q usage=%q", flag.Short(), flag.Env(), flag.Usage())
		}
	})

	t.Run("an untagged field is not a flag", func(t *testing.T) {
		type configs struct {
			Host     string `long:"host"`
			Computed string
			hidden   string
		}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(&configs{}); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if got := len(flagSet.Flags()); got != 1 {
			t.Errorf("unexpected number of flags, want 1 got %d", got)
		}
	})

	t.Run("the value a field holds becomes the flag's default", func(t *testing.T) {
		type configs struct {
			Host    string        `long:"host"`
			Port    int           `long:"port"`
			Timeout time.Duration `long:"timeout"`
		}

		configuration := &configs{Host: "localhost", Port: 5432, Timeout: 10 * time.Second}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(configuration); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		for _, testCase := range []struct{ name, defValue string }{
			{"host", "localhost"},
			{"port", "5432"},
			{"timeout", "10s"},
		} {
			if got := flagSet.Lookup(testCase.name).DefValue(); got != testCase.defValue {
				t.Errorf("unexpected default of --%s, want %q got %q", testCase.name, testCase.defValue, got)
			}
		}
	})

	t.Run("the fields are parsed into the struct", func(t *testing.T) {
		type configs struct {
			Host    string        `long:"host" short:"H"`
			Port    int           `long:"port"`
			SSLMode bool          `long:"ssl"`
			Timeout time.Duration `long:"timeout"`
			Token   string        `env:"APP_STRUCT_TOKEN"`
		}

		t.Setenv("APP_STRUCT_TOKEN", "s3cr3t")

		configuration := &configs{}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(configuration); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		arguments := []string{"-H", "db.internal", "--port=6543", "--ssl", "--timeout=1m", "migrate"}

		if err := flagSet.Parse(arguments); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		want := configs{Host: "db.internal", Port: 6543, SSLMode: true, Timeout: time.Minute, Token: "s3cr3t"}

		if *configuration != want {
			t.Errorf("unexpected configuration, want %+v got %+v", want, *configuration)
		}

		if got := flagSet.Args(); len(got) != 1 || got[0] != "migrate" {
			t.Errorf("unexpected arguments, want [migrate] got %q", got)
		}
	})

	t.Run("errors", func(t *testing.T) {
		type unsupported struct {
			Hosts []string `long:"hosts"`
		}

		type unexported struct {
			host string `long:"host"`
		}

		type nameless struct {
			Host string `usage:"the host."`
		}

		type duplicatedLong struct {
			Host  string `long:"host"`
			Other string `long:"host"`
		}

		type duplicatedShort struct {
			Host  string `short:"H"`
			Other string `short:"H"`
		}

		type invalidLong struct {
			Host string `long:"--host"`
		}

		type invalidShort struct {
			Host string `long:"host" short:"ph"`
		}

		type dashShort struct {
			Host string `long:"host" short:"-"`
		}

		type equalShort struct {
			Host string `long:"host" short:"="`
		}

		testCases := []struct {
			name   string
			target any
			err    error
			field  string
		}{
			{name: "not a pointer", target: struct{}{}, err: ErrNotStruct},
			{name: "nil pointer", target: (*nameless)(nil), err: ErrNotStruct},
			{name: "not a struct", target: new(int), err: ErrNotStruct},
			{name: "unsupported type", target: &unsupported{}, err: ErrUnsupportedType, field: "Hosts"},
			{name: "unexported field", target: &unexported{}, err: ErrUnexportedField, field: "host"},
			{name: "no name", target: &nameless{}, err: ErrNoName, field: "Host"},
			{name: "duplicated long name", target: &duplicatedLong{}, err: ErrDuplicateName, field: "Other"},
			{name: "duplicated short name", target: &duplicatedShort{}, err: ErrDuplicateName, field: "Other"},
			{name: "invalid long name", target: &invalidLong{}, err: ErrInvalidName, field: "Host"},
			{name: "multi character short name", target: &invalidShort{}, err: ErrInvalidName, field: "Host"},
			{name: "a dash as a short name", target: &dashShort{}, err: ErrInvalidName, field: "Host"},
			{name: "an equal sign as a short name", target: &equalShort{}, err: ErrInvalidName, field: "Host"},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				flagSet := NewFlagSet("app", &bytes.Buffer{})

				err := flagSet.Struct(testCase.target)
				if !errors.Is(err, testCase.err) {
					t.Fatalf("unexpected error, want %v got %v", testCase.err, err)
				}

				if testCase.field == "" {
					return
				}

				var fieldError *FieldError
				if !errors.As(err, &fieldError) {
					t.Fatalf("the error should be a *FieldError, got %T", err)
				}

				if fieldError.Field != testCase.field {
					t.Errorf("unexpected field, want %q got %q", testCase.field, fieldError.Field)
				}
			})
		}
	})

	t.Run("a rejected struct defines no flag at all", func(t *testing.T) {
		type configs struct {
			Host  string   `long:"host"`
			Port  int      `long:"port"`
			Hosts []string `long:"hosts"`
		}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(&configs{}); err == nil {
			t.Fatal("the struct should have been rejected")
		}

		if got := len(flagSet.Flags()); got != 0 {
			t.Errorf("no flag should have been defined, got %d", got)
		}
	})

	t.Run("a field which claims no name builds none", func(t *testing.T) {
		// such a field is rejected with ErrNoName before its names are built,
		// which still leaves nothing to define a flag by rather than panicking.
		name, options := fieldTags{}.names()

		if name != nil || options != nil {
			t.Errorf("unexpected names, got %v and %v", name, options)
		}
	})

	t.Run("a name already defined on the flag set is reported", func(t *testing.T) {
		type configs struct {
			Host string `long:"host"`
		}

		var host string

		flagSet := NewFlagSet("app", &bytes.Buffer{})
		Var(flagSet, &host, Long("host"), "the host.")

		err := flagSet.Struct(&configs{})
		if !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("unexpected error, want %v got %v", ErrDuplicateName, err)
		}

		if got := len(flagSet.Flags()); got != 1 {
			t.Errorf("no flag should have been added, got %d", got)
		}
	})
	t.Run("a short name already defined on the flag set is reported", func(t *testing.T) {
		type configs struct {
			Host string `short:"H"`
		}

		var host string

		flagSet := NewFlagSet("app", &bytes.Buffer{})
		Var(flagSet, &host, Short("H"), "the host.")

		err := flagSet.Struct(&configs{})
		if !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("unexpected error, want %v got %v", ErrDuplicateName, err)
		}

		if got := len(flagSet.Flags()); got != 1 {
			t.Errorf("no flag should have been added, got %d", got)
		}
	})

	t.Run("every supported type is bound", func(t *testing.T) {
		type configs struct {
			String   string        `long:"string"`
			Bool     bool          `long:"bool"`
			Int      int           `long:"int"`
			Int64    int64         `long:"int64"`
			Uint     uint          `long:"uint"`
			Uint64   uint64        `long:"uint64"`
			Float64  float64       `long:"float64"`
			Duration time.Duration `long:"duration"`
		}

		configuration := &configs{}

		flagSet := NewFlagSet("app", &bytes.Buffer{})

		if err := flagSet.Struct(configuration); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		arguments := []string{
			"--string=value", "--bool", "--int=-1", "--int64=-2",
			"--uint=3", "--uint64=4", "--float64=5.5", "--duration=6s",
		}

		if err := flagSet.Parse(arguments); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		want := configs{
			String: "value", Bool: true, Int: -1, Int64: -2,
			Uint: 3, Uint64: 4, Float64: 5.5, Duration: 6 * time.Second,
		}

		if *configuration != want {
			t.Errorf("unexpected configuration, want %+v got %+v", want, *configuration)
		}
	})

	t.Run("the default tag", func(t *testing.T) {
		t.Run("replaces the value the struct holds", func(t *testing.T) {
			type configs struct {
				Host    string        `long:"host" default:"db.internal"`
				Port    int           `long:"port" default:"10"`
				SSLMode bool          `long:"ssl" default:"true"`
				Ratio   float64       `long:"ratio" default:"1.5"`
				Timeout time.Duration `long:"timeout" default:"1m"`
			}

			// the struct holds values the tags are expected to override.
			configuration := &configs{Host: "localhost", Port: 5432, Timeout: time.Second}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			want := configs{Host: "db.internal", Port: 10, SSLMode: true, Ratio: 1.5, Timeout: time.Minute}
			if *configuration != want {
				t.Errorf("unexpected configuration, want %+v got %+v", want, *configuration)
			}

			for _, testCase := range []struct{ name, defValue string }{
				{"host", "db.internal"},
				{"port", "10"},
				{"ssl", "true"},
				{"ratio", "1.5"},
				{"timeout", "1m0s"},
			} {
				if got := flagSet.Lookup(testCase.name).DefValue(); got != testCase.defValue {
					t.Errorf("unexpected default of --%s, want %q got %q", testCase.name, testCase.defValue, got)
				}
			}
		})

		t.Run("drops the default when it is a dash", func(t *testing.T) {
			type configs struct {
				Host    string        `long:"host" default:"-"`
				Port    int           `long:"port" default:"-"`
				SSLMode bool          `long:"ssl" default:"-"`
				Timeout time.Duration `long:"timeout" default:"-"`
			}

			// every field holds a value the dash is expected to drop.
			configuration := &configs{Host: "localhost", Port: 5432, SSLMode: true, Timeout: time.Second}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if *configuration != (configs{}) {
				t.Errorf("the fields should have been reset, got %+v", *configuration)
			}

			for _, testCase := range []struct{ name, defValue string }{
				{"host", ""},
				{"port", "0"},
				{"ssl", "false"},
				{"timeout", "0s"},
			} {
				if got := flagSet.Lookup(testCase.name).DefValue(); got != testCase.defValue {
					t.Errorf("unexpected default of --%s, want %q got %q", testCase.name, testCase.defValue, got)
				}
			}

			// a dropped default is a zero value, which the help leaves out.
			var help bytes.Buffer
			flagSet.PrintDefaults(&help)

			if strings.Contains(help.String(), "(default") {
				t.Errorf("no default should be shown:\n%s", help.String())
			}
		})

		t.Run("keeps the value of the struct when it is absent, even a zero value", func(t *testing.T) {
			type configs struct {
				Host string `long:"host"`
				Port int    `long:"port"`
			}

			// Host is left at its zero value on purpose.
			configuration := &configs{Port: 5432}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got := flagSet.Lookup("host").DefValue(); got != "" {
				t.Errorf("unexpected default of --host, want %q got %q", "", got)
			}

			if got := flagSet.Lookup("port").DefValue(); got != "5432" {
				t.Errorf("unexpected default of --port, want %q got %q", "5432", got)
			}
		})

		t.Run("is overridden by the command line", func(t *testing.T) {
			type configs struct {
				Port int `long:"port" default:"10"`
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if err := flagSet.Parse([]string{"--port=6543"}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if configuration.Port != 6543 {
				t.Errorf("unexpected port, want 6543 got %d", configuration.Port)
			}
		})

		t.Run("is reported when the type cannot parse it", func(t *testing.T) {
			type configs struct {
				Port int `long:"port" default:"not-a-number"`
			}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			err := flagSet.Struct(&configs{})
			if !errors.Is(err, ErrInvalidDefault) {
				t.Fatalf("unexpected error, want %v got %v", ErrInvalidDefault, err)
			}

			var fieldError *FieldError
			if !errors.As(err, &fieldError) || fieldError.Field != "Port" {
				t.Errorf("unexpected error: %v", err)
			}
		})

		t.Run("needs a name like any other tag", func(t *testing.T) {
			type configs struct {
				Port int `default:"10"`
			}

			if err := (NewFlagSet("app", &bytes.Buffer{})).Struct(&configs{}); !errors.Is(err, ErrNoName) {
				t.Errorf("unexpected error, want %v got %v", ErrNoName, err)
			}
		})

		t.Run("a rejected struct keeps the values it came with", func(t *testing.T) {
			type configs struct {
				Port  int      `long:"port" default:"10"`
				Hosts []string `long:"hosts"`
			}

			configuration := &configs{Port: 5432}

			if err := (NewFlagSet("app", &bytes.Buffer{})).Struct(configuration); !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("unexpected error, want %v got %v", ErrUnsupportedType, err)
			}

			if configuration.Port != 5432 {
				t.Errorf("the field should have been restored, want 5432 got %d", configuration.Port)
			}
		})
	})
	t.Run("nested structs", func(t *testing.T) {
		t.Run("a nested struct is flattened into the flag set", func(t *testing.T) {
			type postgres struct {
				Host string `usage:"the host." long:"postgres-host" short:"H" env:"POSTGRES_HOST"`
				Port int    `usage:"the port." long:"postgres-port" default:"5432"`
			}

			type configs struct {
				AppEnv   string `usage:"the environment." env:"APP_ENV"`
				Postgres postgres
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got := len(flagSet.Flags()); got != 3 {
				t.Fatalf("unexpected number of flags, want 3 got %d", got)
			}

			if flagSet.Lookup("postgres-host") == nil || flagSet.Lookup("postgres-port") == nil {
				t.Fatal("the nested flags were not defined")
			}

			if err := flagSet.Parse([]string{"--postgres-host=db.internal"}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if configuration.Postgres.Host != "db.internal" {
				t.Errorf("unexpected host, want %q got %q", "db.internal", configuration.Postgres.Host)
			}

			if configuration.Postgres.Port != 5432 {
				t.Errorf("the nested default should apply, want 5432 got %d", configuration.Postgres.Port)
			}
		})

		t.Run("an embedded struct is flattened into the flag set", func(t *testing.T) {
			// embedded through an unexported type, the stricter case.
			type common struct {
				Verbose bool `usage:"verbose output." long:"verbose" short:"v"`
			}

			// and through an exported one.
			type Shared struct {
				Quiet bool `usage:"quiet output." long:"quiet" short:"q"`
			}

			type configs struct {
				common
				Shared
				Host string `long:"host"`
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if err := flagSet.Parse([]string{"-v", "-q"}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !configuration.Verbose {
				t.Error("the flag embedded through an unexported type was not parsed")
			}

			if !configuration.Quiet {
				t.Error("the flag embedded through an exported type was not parsed")
			}
		})

		t.Run("the nesting has no depth limit", func(t *testing.T) {
			type third struct {
				Deep string `long:"deep"`
			}

			type second struct {
				Third third
			}

			type first struct {
				Second second
			}

			type configs struct {
				First first
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if err := flagSet.Parse([]string{"--deep=value"}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if configuration.First.Second.Third.Deep != "value" {
				t.Errorf("unexpected value, got %q", configuration.First.Second.Third.Deep)
			}
		})

		t.Run("a nil pointer to a struct is allocated", func(t *testing.T) {
			type postgres struct {
				Host string `long:"postgres-host" default:"localhost"`
			}

			type configs struct {
				Postgres *postgres
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if configuration.Postgres == nil {
				t.Fatal("the pointer should have been allocated")
			}

			if configuration.Postgres.Host != "localhost" {
				t.Errorf("unexpected host, want %q got %q", "localhost", configuration.Postgres.Host)
			}
		})

		t.Run("a pointer to a struct keeps the values it holds", func(t *testing.T) {
			type postgres struct {
				Host string `long:"postgres-host"`
			}

			type configs struct {
				Postgres *postgres
			}

			configuration := &configs{Postgres: &postgres{Host: "held"}}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got := flagSet.Lookup("postgres-host").DefValue(); got != "held" {
				t.Errorf("unexpected default, want %q got %q", "held", got)
			}
		})

		t.Run("a field which parses itself is a flag, not a struct to recurse into", func(t *testing.T) {
			type configs struct {
				Hosts hostList `usage:"the hosts." long:"hosts"`
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if err := flagSet.Parse([]string{"--hosts=a,b,c"}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got := strings.Join(configuration.Hosts.values, "|"); got != "a|b|c" {
				t.Errorf("unexpected hosts, got %q", got)
			}
		})

		t.Run("a nil pointer embedded through an unexported type is left alone", func(t *testing.T) {
			type hidden struct {
				Ignored string `long:"ignored"`
			}

			type configs struct {
				*hidden
				Host string `long:"host"`
			}

			configuration := &configs{}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(configuration); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			// the embedded pointer cannot be set, so it is neither allocated
			// nor walked.
			if configuration.hidden != nil {
				t.Error("the pointer should not have been allocated")
			}

			if got := len(flagSet.Flags()); got != 1 {
				t.Errorf("unexpected number of flags, want 1 got %d", got)
			}
		})

		t.Run("an unexported nested struct is left alone", func(t *testing.T) {
			type configs struct {
				Host     string `long:"host"`
				internal struct{ Ignored string }
			}

			flagSet := NewFlagSet("app", &bytes.Buffer{})

			if err := flagSet.Struct(&configs{}); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got := len(flagSet.Flags()); got != 1 {
				t.Errorf("unexpected number of flags, want 1 got %d", got)
			}
		})

		t.Run("errors", func(t *testing.T) {
			type leaf struct {
				Host string `long:"host"`
			}

			type duplicatedAcrossStructs struct {
				First  leaf
				Second leaf
			}

			type taggedNested struct {
				Postgres leaf `long:"postgres"`
			}

			type recursive struct {
				Host string `long:"host"`
				Next *recursive
			}

			type invalidNested struct {
				Nested struct {
					Port int `long:"port" default:"not-a-number"`
				}
			}

			testCases := []struct {
				name   string
				target any
				err    error
				field  string
			}{
				{
					name:   "a name duplicated across two nested structs",
					target: &duplicatedAcrossStructs{},
					err:    ErrDuplicateName,
					field:  "Second.Host",
				},
				{
					name:   "a nested struct carrying flag tags",
					target: &taggedNested{},
					err:    ErrNestedTags,
					field:  "Postgres",
				},
				{
					name:   "a struct which nests itself",
					target: &recursive{},
					err:    ErrRecursiveType,
					field:  "Next",
				},
				{
					name:   "an invalid field of a nested struct",
					target: &invalidNested{},
					err:    ErrInvalidDefault,
					field:  "Nested.Port",
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					flagSet := NewFlagSet("app", &bytes.Buffer{})

					err := flagSet.Struct(testCase.target)
					if !errors.Is(err, testCase.err) {
						t.Fatalf("unexpected error, want %v got %v", testCase.err, err)
					}

					var fieldError *FieldError
					if !errors.As(err, &fieldError) {
						t.Fatalf("the error should be a *FieldError, got %T", err)
					}

					if fieldError.Field != testCase.field {
						t.Errorf("unexpected field, want %q got %q", testCase.field, fieldError.Field)
					}

					if got := len(flagSet.Flags()); got != 0 {
						t.Errorf("no flag should have been defined, got %d", got)
					}
				})
			}
		})
	})
}

// hostList is a struct which parses itself, so a field of that type is a flag
// rather than a nested struct.
type hostList struct {
	values []string
}

func (h *hostList) String() string { return strings.Join(h.values, ",") }

func (h *hostList) Set(value string) error {
	h.values = strings.Split(value, ",")

	return nil
}

func TestStructFlags(t *testing.T) {
	t.Run("builds the configure function of a console", func(t *testing.T) {
		type configs struct {
			Username string `usage:"the user." long:"username" short:"u" env:"APP_STRUCT_USERNAME"`
			Limit    int    `usage:"the limit." long:"limit"`
		}

		configuration := &configs{Limit: 10}

		configure, err := StructFlags(configuration)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		var writer, errWriter bytes.Buffer

		command := NewSpyCommand("list", "lists.", "app list", 0, nil)

		console := New("app", "an app.", &writer, &errWriter)
		console.Flags(configure)
		console.Register(command)

		if exitStatus := console.Run(context.Background(), []string{"app", "--username=admin", "list"}); exitStatus != ExitSuccess {
			t.Errorf("unexpected exit code, want %d got %d", ExitSuccess, exitStatus)
		}

		if command.RunCount != 1 {
			t.Errorf("the command should have run once, got %d", command.RunCount)
		}

		want := configs{Username: "admin", Limit: 10}
		if *configuration != want {
			t.Errorf("unexpected configuration, want %+v got %+v", want, *configuration)
		}
	})

	t.Run("validates the struct before returning", func(t *testing.T) {
		type configs struct {
			Host string `usage:"the host."`
		}

		configure, err := StructFlags(&configs{})
		if !errors.Is(err, ErrNoName) {
			t.Fatalf("unexpected error, want %v got %v", ErrNoName, err)
		}

		if configure != nil {
			t.Error("no configure function should be returned")
		}
	})

	t.Run("panics when the struct collides with the flag set", func(t *testing.T) {
		type configs struct {
			Host string `long:"host"`
		}

		defer func() {
			recovered := recover()
			if recovered == nil {
				t.Fatal("a colliding name should panic")
			}

			if message, ok := recovered.(string); !ok || !strings.Contains(message, "--host") {
				t.Errorf("unexpected panic: %v", recovered)
			}
		}()

		configure, err := StructFlags(&configs{})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		var host string

		flagSet := NewFlagSet("app", &bytes.Buffer{})
		Var(flagSet, &host, Long("host"), "the host.")

		configure(flagSet)
	})
}
