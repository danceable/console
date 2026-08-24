package console

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// The struct tags a field may carry to describe the flag it is bound to.
//
// They are all optional and their order is irrelevant. A field which carries
// none of them is not a flag and is left alone, while a field which carries
// any of them has to be reachable, which means defining at least one of
// "long", "short" and "env":
//
//	type configs struct {
//		AppEnv       string `usage:"the deployment environment." env:"APP_ENV"`
//		PostgresHost string `usage:"the host." env:"POSTGRES_HOST" long:"postgres-host" short:"p"`
//		PostgresPort int    `usage:"the port." long:"postgres-port" default:"5432"`
//		Redis        redis  // nested, so its own fields are flattened in.
//		internal     string // untagged, so not a flag.
//	}
const (
	tagUsage   = "usage"
	tagLong    = "long"
	tagShort   = "short"
	tagEnv     = "env"
	tagDefault = "default"
)

// noDefault is the value of a "default" tag which drops the default of a flag.
// The field is reset to the zero value of its type, whatever the struct holds,
// and the help shows no default for it.
const noDefault = "-"

// flagTags lists every tag the package understands, in the order they are read.
var flagTags = [...]string{tagUsage, tagLong, tagShort, tagEnv, tagDefault}

// The errors a struct which cannot be turned into flags is reported with. They
// are always wrapped in a FieldError, except ErrNotStruct which is about the
// target itself.
var (
	// ErrNotStruct is returned when the target is not a non nil pointer to a struct.
	ErrNotStruct = errors.New("console: the target must be a non nil pointer to a struct")

	// ErrNoName is returned for a tagged field which defines none of the
	// "long", "short" and "env" tags, leaving the flag unreachable.
	ErrNoName = errors.New("a tagged field needs at least one of the long, short and env tags")

	// ErrInvalidName is returned for a "long" or a "short" tag which names a
	// flag the parser could never match.
	ErrInvalidName = errors.New("invalid flag name")

	// ErrDuplicateName is returned when two fields, or a field and a flag which
	// is already defined, claim the same name.
	ErrDuplicateName = errors.New("duplicated flag name")

	// ErrUnexportedField is returned for a tagged field which cannot be addressed.
	ErrUnexportedField = errors.New("an unexported field cannot be bound to a flag")

	// ErrUnsupportedType is returned for a tagged field of a type no flag can
	// be defined for.
	ErrUnsupportedType = errors.New("unsupported field type")

	// ErrInvalidDefault is returned for a "default" tag the type of the field
	// cannot be parsed from.
	ErrInvalidDefault = errors.New("invalid default value")

	// ErrNestedTags is returned for a nested or an embedded struct which
	// carries flag tags of its own. Its fields are flattened into the flag
	// set, so the struct itself is never a flag.
	ErrNestedTags = errors.New("a nested struct is flattened into the flag set and cannot carry flag tags")

	// ErrRecursiveType is returned for a struct which nests itself.
	ErrRecursiveType = errors.New("recursive struct type")
)

// FieldError reports the field of the target struct which cannot be turned
// into a flag, and why. It wraps one of the package's errors, so the reason is
// matched with errors.Is.
type FieldError struct {
	// Field is the name of the offending field.
	Field string

	// Err is the reason the field was rejected.
	Err error
}

func (e *FieldError) Error() string { return fmt.Sprintf("console: field %s: %s", e.Field, e.Err) }
func (e *FieldError) Unwrap() error { return e.Err }

// Struct defines one flag per tagged field of target, which has to be a non
// nil pointer to a struct.
//
// The value a field already holds becomes the default value of its flag, so a
// struct is given its defaults by being populated before it is bound. A
// "default" tag overrides it, and a `default:"-"` tag drops it.
//
// A nested or an embedded struct is flattened into the same flag set, so the
// fields of a struct which groups the settings of a subsystem name their flags
// the way the fields of the outer struct do. A nil pointer to a struct is
// allocated to hold its flags.
//
// Nothing is defined when the struct cannot be bound: the fields are resolved
// and validated before the first flag is defined, so a failure leaves the flag
// set untouched and the struct keeps the values it came with.
func (f *FlagSet) Struct(target any) error {
	plan, err := newFlagPlan(target)
	if err != nil {
		return err
	}

	return plan.bind(f)
}

// StructFlags builds the configure function of a struct, ready to be handed to
// Console.Flags, Group.Flags or a command's Configure method.
//
// It validates the struct before returning, which surfaces a mistake in the
// tags at build time rather than when the console reaches the level the flags
// belong to. The returned function panics when the struct collides with a flag
// the set already defines, as any other duplicated definition does.
func StructFlags(target any) (func(*FlagSet), error) {
	plan, err := newFlagPlan(target)
	if err != nil {
		return nil, err
	}

	return func(flagSet *FlagSet) {
		if err := plan.bind(flagSet); err != nil {
			panic(err.Error())
		}
	}, nil
}

// fieldTags holds the flag tags of a single field.
type fieldTags struct {
	usage string
	long  string
	short string
	env   string

	// defaultValue is the "default" tag, and hasDefault tells an absent tag
	// from one which is present but empty.
	defaultValue string
	hasDefault   bool
}

// named reports whether the field defines a name the flag is reachable by.
func (t fieldTags) named() bool { return t.long != "" || t.short != "" || t.env != "" }

// options builds the flag options the tags describe.
func (t fieldTags) options() []FlagOption {
	options := make([]FlagOption, 0, 3)

	if t.long != "" {
		options = append(options, Long(t.long))
	}

	if t.short != "" {
		options = append(options, Short(t.short))
	}

	if t.env != "" {
		options = append(options, Env(t.env))
	}

	return options
}

// readTags reads the flag tags of a field. It reports whether the field
// carries any of them, which is what tells a flag from a field the struct
// merely holds. The tags are looked up by name, so their order is irrelevant.
func readTags(tag reflect.StructTag) (fieldTags, bool) {
	var (
		tags   fieldTags
		tagged bool
	)

	for _, name := range flagTags {
		value, exists := tag.Lookup(name)
		if !exists {
			continue
		}

		tagged = true

		switch name {
		case tagUsage:
			tags.usage = value
		case tagLong:
			tags.long = value
		case tagShort:
			tags.short = value
		case tagEnv:
			tags.env = value
		case tagDefault:
			tags.defaultValue, tags.hasDefault = value, true
		}
	}

	return tags, tagged
}

// plannedFlag is a flag a field describes, resolved but not defined yet.
type plannedFlag struct {
	field   string // the name of the field, to report the errors with.
	value   Value
	usage   string
	long    string
	short   string
	options []FlagOption
}

// flagPlan is the set of flags a struct describes. Resolving the whole struct
// before defining anything is what lets Struct fail without leaving half of
// the flags behind.
type flagPlan struct {
	flags []plannedFlag
}

// newFlagPlan resolves the flags the tagged fields of target describe.
func newFlagPlan(target any) (plan *flagPlan, err error) {
	pointer := reflect.ValueOf(target)
	if pointer.Kind() != reflect.Pointer || pointer.IsNil() || pointer.Elem().Kind() != reflect.Struct {
		return nil, ErrNotStruct
	}

	value := pointer.Elem()

	// planning writes the defaults into the fields, so the struct is given
	// back the values it came with when a later field turns out to be invalid.
	snapshot := reflect.New(value.Type()).Elem()
	snapshot.Set(value)

	defer func() {
		if err != nil {
			value.Set(snapshot)
		}
	}()

	plan = &flagPlan{}
	names := &nameSet{longs: make(map[string]string), shorts: make(map[string]string)}

	if err = plan.walk(value, "", names, make(map[reflect.Type]bool)); err != nil {
		return nil, err
	}

	return plan, nil
}

// nameSet holds the names the fields of a struct have claimed so far, mapped
// to the field which claimed them. It is shared by the whole recursion, so two
// nested structs cannot claim the same name.
type nameSet struct {
	longs  map[string]string
	shorts map[string]string
}

// walk resolves the flags of a struct, recursing into the structs it nests.
//
// The nested structs are flattened into the same flag set: a nested field
// names its flag the way a field of the outer struct does, no name is derived
// from the field it is nested in. The path is the dotted name of the struct
// being walked, used to report the errors with the field they belong to.
func (p *flagPlan) walk(value reflect.Value, path string, names *nameSet, visiting map[reflect.Type]bool) error {
	fields := value.Type()

	// a struct which nests itself, through a pointer, would recurse forever.
	if visiting[fields] {
		return &FieldError{Field: strings.TrimSuffix(path, "."), Err: fmt.Errorf("%w %s", ErrRecursiveType, fields)}
	}

	visiting[fields] = true
	defer delete(visiting, fields)

	for i := 0; i < fields.NumField(); i++ {
		var (
			field = fields.Field(i)
			name  = path + field.Name
		)

		tags, tagged := readTags(field.Tag)

		// a nested or an embedded struct contributes its own fields.
		if nested, nests := nestedStruct(value.Field(i), field); nests {
			if tagged {
				return &FieldError{Field: name, Err: ErrNestedTags}
			}

			if err := p.walk(nested, name+".", names, visiting); err != nil {
				return err
			}

			continue
		}

		if !tagged {
			continue // an untagged field is not a flag.
		}

		if !field.IsExported() {
			return &FieldError{Field: name, Err: ErrUnexportedField}
		}

		if invalid := validate(tags, names); invalid != nil {
			return &FieldError{Field: name, Err: invalid}
		}

		flagValue, err := newFieldValue(value.Field(i), tags)
		if err != nil {
			return &FieldError{Field: name, Err: err}
		}

		if tags.long != "" {
			names.longs[tags.long] = name
		}

		if tags.short != "" {
			names.shorts[tags.short] = name
		}

		p.flags = append(p.flags, plannedFlag{
			field:   name,
			value:   flagValue,
			usage:   tags.usage,
			long:    tags.long,
			short:   tags.short,
			options: tags.options(),
		})
	}

	return nil
}

// nestedStruct reports whether a field is a struct whose own fields are
// flattened into the flag set, and returns the struct to walk.
//
// A nil pointer to a struct is allocated, as its fields could not be bound
// otherwise, and a type which parses itself is a flag rather than a struct to
// recurse into.
//
// An unexported field cannot be walked, but an embedded one can: a struct
// embedded by an unexported type is itself unexported, while the fields it
// promotes stay settable.
func nestedStruct(value reflect.Value, field reflect.StructField) (reflect.Value, bool) {
	if !field.IsExported() && !field.Anonymous {
		return reflect.Value{}, false
	}

	// the check needs to read the field, which an embedded unexported struct
	// does not allow. Such a struct is walked rather than parsed.
	if value.CanInterface() {
		if _, parses := value.Addr().Interface().(Value); parses {
			return reflect.Value{}, false
		}
	}

	switch {
	case value.Kind() == reflect.Struct:
		return value, true
	case value.Kind() == reflect.Pointer && value.Type().Elem().Kind() == reflect.Struct:
		if value.IsNil() {
			if !value.CanSet() {
				return reflect.Value{}, false
			}

			value.Set(reflect.New(value.Type().Elem()))
		}

		return value.Elem(), true
	}

	return reflect.Value{}, false
}

// validate checks the names a field claims, mirroring the rules FlagSet.define
// panics on so that a struct reports them as an error instead.
func validate(tags fieldTags, names *nameSet) error {
	switch {
	case !tags.named():
		return ErrNoName
	case strings.HasPrefix(tags.long, "-") || strings.Contains(tags.long, "="):
		return fmt.Errorf("%w: long name %q", ErrInvalidName, tags.long)
	case tags.short != "" && len(tags.short) != 1:
		return fmt.Errorf("%w: short name %q must be a single character", ErrInvalidName, tags.short)
	case tags.short == "-" || tags.short == "=":
		return fmt.Errorf("%w: short name %q", ErrInvalidName, tags.short)
	}

	if owner, exists := names.longs[tags.long]; exists {
		return fmt.Errorf("%w: --%s is already defined by the field %s", ErrDuplicateName, tags.long, owner)
	}

	if owner, exists := names.shorts[tags.short]; exists {
		return fmt.Errorf("%w: -%s is already defined by the field %s", ErrDuplicateName, tags.short, owner)
	}

	return nil
}

// newFieldValue builds the flag value a field is stored in, and settles what
// the flag defaults to.
//
// Without a "default" tag the field keeps the value the struct came with, even
// when it is the zero value of its type. A "default" tag replaces it with the
// value it names, and the "-" tag drops it, resetting the field to its zero
// value. The field is what FlagSet.Var reads the default value from, so the
// default is applied before the flag is defined.
func newFieldValue(field reflect.Value, tags fieldTags) (Value, error) {
	if tags.hasDefault && tags.defaultValue == noDefault {
		field.SetZero()
	}

	value, err := newValue(field)
	if err != nil {
		return nil, err
	}

	if tags.hasDefault && tags.defaultValue != noDefault {
		if err := value.Set(tags.defaultValue); err != nil {
			return nil, fmt.Errorf("%w %q for %s: %s", ErrInvalidDefault, tags.defaultValue, field.Type(), err)
		}
	}

	return value, nil
}

// newValue wraps a field in the flag value which parses into it.
func newValue(field reflect.Value) (Value, error) {
	switch pointer := field.Addr().Interface().(type) {
	case *time.Duration:
		// a defined type of its own, so it is matched before the integers.
		return newDurationValue(*pointer, pointer), nil
	case *string:
		return newStringValue(*pointer, pointer), nil
	case *bool:
		return newBoolValue(*pointer, pointer), nil
	case *int:
		return newIntValue(*pointer, pointer), nil
	case *int64:
		return newInt64Value(*pointer, pointer), nil
	case *uint:
		return newUintValue(*pointer, pointer), nil
	case *uint64:
		return newUint64Value(*pointer, pointer), nil
	case *float64:
		return newFloat64Value(*pointer, pointer), nil
	case Value:
		// a field whose pointer implements Value carries its own parsing,
		// which is the struct equivalent of FlagSet.Var.
		return pointer, nil
	default:
		return nil, fmt.Errorf("%w %s", ErrUnsupportedType, field.Type())
	}
}

// bind defines the planned flags on the flag set. The names are checked
// against the ones the set already holds first, as define panics on a
// duplicate and a struct reports it as an error.
func (p *flagPlan) bind(f *FlagSet) error {
	for _, planned := range p.flags {
		if _, exists := f.long[planned.long]; exists && planned.long != "" {
			return &FieldError{
				Field: planned.field,
				Err:   fmt.Errorf("%w: --%s is already defined on %q", ErrDuplicateName, planned.long, f.name),
			}
		}

		if _, exists := f.short[planned.short]; exists && planned.short != "" {
			return &FieldError{
				Field: planned.field,
				Err:   fmt.Errorf("%w: -%s is already defined on %q", ErrDuplicateName, planned.short, f.name),
			}
		}
	}

	for _, planned := range p.flags {
		f.Var(planned.value, planned.usage, planned.options...)
	}

	return nil
}
