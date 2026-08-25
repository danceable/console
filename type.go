package console

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// Parser turns the string form of a flag's value into a T, and reports the
// values it cannot parse. Parsers compose with the standard library, whose
// parsing functions (strconv.ParseBool, time.ParseDuration, net.ParseMAC and
// their like) are already of that shape.
type Parser[T any] func(string) (T, error)

// Formatter turns a T back into the string form it is shown as in the help.
type Formatter[T any] func(T) string

// Type describes how the flags of a Go type are parsed and presented. It is
// registered once, with [Register], and every flag and struct field of that
// type is bound to it:
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
// Only Parse is required. A type which needs no more than that is presented as
// a "value" in the help, formats itself the way the fmt package does, requires
// a value on the command line and reports the errors of its parser as they are.
type Type[T any] struct {
	// Name names the type in the help output, as the "int" of "--port int".
	// A type which names itself with nothing is presented as a "value".
	Name string

	// Expects describes what the type accepts, as "an integer" or "a duration".
	// It is what a rejected value is explained with:
	//
	//	invalid value "http" for flag --port: can't be parsed as an integer
	//
	// Every value the parser rejects is explained the same way, except the ones
	// it rejects as out of range, so the message stays about the type rather
	// than about the parser. A type which leaves it empty reports the error of
	// its parser instead.
	Expects string

	// Parse turns the string form of a value into a T. It is the only field a
	// type has to define, and a type which defines no parser cannot be
	// registered.
	Parse Parser[T]

	// Format turns a T back into its string form, which the help shows the
	// default value as. It defaults to the string form the fmt package gives
	// the value.
	Format Formatter[T]

	// Implicit is the value a flag of this type takes when it is provided
	// without one, the way a boolean is provided as "--verbose" rather than
	// "--verbose=true". Such a flag never consumes the argument which follows
	// it, and is presented by its name alone in the help.
	//
	// A type which leaves it empty requires a value.
	Implicit string
}

// format returns the string form of a value.
func (t Type[T]) format(value T) string {
	if t.Format != nil {
		return t.Format(value)
	}

	return fmt.Sprint(value)
}

// explain turns the error a parser returned into the reason the value was
// rejected, named after the type the value was expected to be. A type which
// describes what it expects speaks for its parser, whose own message would
// name the standard library rather than the flag.
func (t Type[T]) explain(err error) error {
	switch {
	case t.Expects == "":
		return err
	case errors.Is(err, strconv.ErrRange):
		return outOfRange(t.Expects)
	default:
		return cantParse(t.Expects)
	}
}

// binder wraps a pointer to a variable in the flag value which parses into it.
// It is the type erased form of a [Type], which is what lets the flags of every
// type share a single registry.
type binder func(pointer any) Value

// registry holds the registered types, keyed by the Go type they parse.
type registry struct {
	mu      sync.RWMutex
	binders map[reflect.Type]binder
}

// registeredTypes is the registry every flag and every struct field is bound
// through.
var registeredTypes = &registry{binders: make(map[reflect.Type]binder)}

func (r *registry) register(goType reflect.Type, bind binder) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.binders[goType] = bind
}

func (r *registry) lookup(goType reflect.Type) (binder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bind, registered := r.binders[goType]

	return bind, registered
}

// Register registers the flag type of T, which every flag and every struct
// field of that type is then parsed by. The Go types are registered by the
// package itself, so registering is what a type of your own needs to be
// usable as a flag.
//
// Registering a type twice replaces the first registration, which is how the
// parsing of a Go type is given rules of your own. A type is normally
// registered from an init function, before any flag is defined: a flag is
// bound to its type when it is defined, so a flag which is already defined
// keeps the parsing it was defined with.
//
// It panics when the type defines no parser, as a definition error is a
// programming mistake rather than user input.
func Register[T any](flagType Type[T]) {
	goType := reflect.TypeFor[T]()

	if flagType.Parse == nil {
		panic(fmt.Sprintf("console: the flag type %s needs a parser", goType))
	}

	registeredTypes.register(goType, func(pointer any) Value {
		return &value[T]{pointer: pointer.(*T), flagType: flagType}
	})
}

// Default defines the value a flag defaults to, which is otherwise the value
// its variable already holds:
//
//	console.Var(flagSet, &port, console.Long("port"), "the port to listen to.", console.Default(80))
//
// The default value is written into the variable when the flag is defined, so
// the variable holds it until the flags are parsed, and the help presents it
// unless it is the zero value of its type.
//
// A default value reaches a flag of another type the way an untyped constant
// reaches a variable, so Default(0) is the zero of whatever number the flag
// holds and Default(80) is a uint16 as readily as an int.
//
// It panics when the default value cannot be the value of the flag, as a
// definition error is a programming mistake rather than user input. A flag
// which holds a value of your own takes no default: it keeps the one the value
// was built with.
func Default[T any](value T) FlagOption {
	return flagOption(func(f *Flag) {
		target, takesOne := f.value.(defaulter)
		if !takesOne {
			panic(fmt.Sprintf("console: a flag of type %s takes no default value", reflect.TypeOf(f.value)))
		}

		// the value is read through a pointer, so that a T which is an
		// interface is still the type the default was given as.
		given := reflect.ValueOf(&value).Elem()

		converted, fits := convert(given, target.goType())
		if !fits {
			panic(fmt.Sprintf(
				"console: a default value of type %s cannot be given to a flag of type %s",
				given.Type(), target.goType(),
			))
		}

		target.setDefault(converted)
	})
}

// defaulter is implemented by the values a default value can be written into.
type defaulter interface {
	// goType is the Go type the value parses, which the default value has to
	// be convertible to.
	goType() reflect.Type

	// setDefault writes the default value into the variable of the flag.
	setDefault(reflect.Value)
}

// convert adapts a default value to the type of the flag it is given to, which
// it reaches when both are of the same class of types and the value survives
// the conversion.
func convert(given reflect.Value, target reflect.Type) (reflect.Value, bool) {
	source := given.Type()

	if source == target {
		return given, true
	}

	sourceClass, targetClass := classOf(source.Kind()), classOf(target.Kind())

	switch {
	case sourceClass == otherClass || sourceClass != targetClass:
		return reflect.Value{}, false
	case !source.ConvertibleTo(target):
		return reflect.Value{}, false
	case targetClass == numberClass && !fits(given, target):
		return reflect.Value{}, false
	}

	return given.Convert(target), true
}

// The classes a default value is converted within. A number reaches every
// number the flag could hold, and nothing else: an integer converted to a
// string would silently be read as a code point rather than as a number.
type valueClass int

const (
	otherClass valueClass = iota
	numberClass
	stringClass
	boolClass
)

func classOf(kind reflect.Kind) valueClass {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return numberClass
	case reflect.String:
		return stringClass
	case reflect.Bool:
		return boolClass
	default:
		return otherClass
	}
}

// fits reports whether a number is still the same number once it is given to a
// flag of another type, which one that overflows it, that loses its sign or
// that is truncated to reach it is not.
//
// A float given to a float is the exception: a float32 cannot hold every
// float64 to begin with, so it only has to stay in range.
func fits(given reflect.Value, target reflect.Type) bool {
	number := reflect.New(target).Elem()

	switch {
	case given.CanInt():
		integer := given.Int()

		switch {
		case number.CanInt():
			return !number.OverflowInt(integer)
		case number.CanUint():
			return integer >= 0 && !number.OverflowUint(uint64(integer))
		default:
			return !number.OverflowFloat(float64(integer))
		}
	case given.CanUint():
		integer := given.Uint()

		switch {
		case number.CanInt():
			return integer <= math.MaxInt64 && !number.OverflowInt(int64(integer))
		case number.CanUint():
			return !number.OverflowUint(integer)
		default:
			return !number.OverflowFloat(float64(integer))
		}
	default:
		return number.CanFloat() && !number.OverflowFloat(given.Float())
	}
}

// value binds a variable to the type which parses into it. It is the [Value]
// every flag of a registered type is defined with.
type value[T any] struct {
	pointer  *T
	flagType Type[T]
}

// Set parses an argument into the variable the flag was defined with. The
// variable is left untouched when the argument is rejected, so a flag provided
// with an invalid value keeps its default.
func (v *value[T]) Set(argument string) error {
	parsed, err := v.flagType.Parse(argument)
	if err != nil {
		return v.flagType.explain(err)
	}

	*v.pointer = parsed

	return nil
}

func (v *value[T]) String() string { return v.flagType.format(*v.pointer) }
func (v *value[T]) Type() string   { return v.flagType.Name }

// setDefault writes the value the flag defaults to into its variable. The
// value is of the type the flag parses, as convert made sure of it.
func (v *value[T]) setDefault(value reflect.Value) { *v.pointer = value.Interface().(T) }

// goType returns the Go type the value parses, which a default value has to be
// of as well.
func (v *value[T]) goType() reflect.Type { return reflect.TypeFor[T]() }

// Zero returns the string form of the zero value of the type, which is the
// default value the help doesn't bother mentioning.
func (v *value[T]) Zero() string { return v.flagType.format(*new(T)) }

// ImplicitValue returns the value the flag takes when it is provided without
// one, and reports whether it takes one at all.
func (v *value[T]) ImplicitValue() (string, bool) {
	return v.flagType.Implicit, v.flagType.Implicit != ""
}

// ErrUnsupportedType is returned for a variable of a type no flag can be
// defined for, which is a type that is neither registered nor parses itself.
var ErrUnsupportedType = errors.New("unsupported type")

// valueOf wraps a pointer to a variable in the flag value which parses into it.
//
// A variable of a registered type is bound to the type it was registered with,
// and a pointer which implements [Value] carries its own parsing. Anything else
// is of a type no flag can be defined for.
func valueOf(pointer any) (Value, error) {
	goType := reflect.TypeOf(pointer).Elem()

	if bind, registered := registeredTypes.lookup(goType); registered {
		return bind(pointer), nil
	}

	if own, parses := pointer.(Value); parses {
		return own, nil
	}

	return nil, fmt.Errorf("%w %s", ErrUnsupportedType, goType)
}

// The reasons a value rejects what it was given. They are never printed as
// they are, a parseError explains them with the type the value expects.
var (
	errParse = errors.New("parse error")
	errRange = errors.New("value out of range")
)

// parseError explains why a value was rejected, naming the type the value was
// expected to be. It wraps errParse or errRange, so the reason stays matchable
// while the message reads as a sentence once the flag set has prefixed it with
// the value and the flag it belongs to:
//
//	invalid value "disabled" for environment variable POSTGRES_SSL_MODE: can't be parsed as a boolean (true or false)
type parseError struct {
	reason  error // errParse or errRange.
	message string
}

func (e *parseError) Error() string { return e.message }
func (e *parseError) Unwrap() error { return e.reason }

// cantParse reports a value which doesn't fit the type of its flag at all.
func cantParse(expected string) error {
	return &parseError{reason: errParse, message: "can't be parsed as " + expected}
}

// outOfRange reports a value which fits the type of its flag but overflows it.
func outOfRange(expected string) error {
	return &parseError{reason: errRange, message: "out of range for " + expected}
}

// What the Go number types accept, which is about the type rather than about
// its width: an int8 and an int64 are both integers, they just don't hold the
// same ones.
const (
	expectsInteger  = "an integer"
	expectsUnsigned = "an unsigned integer (0 or greater)"
	expectsFloat    = "a floating point number"
)

// The Go types every flag set parses out of the box. They are registered the
// way a type of your own is, so [Register] replaces any of them.
func init() {
	Register(Type[string]{
		Name:  "string",
		Parse: func(argument string) (string, error) { return argument, nil },
	})

	Register(Type[bool]{
		Name:     "bool",
		Expects:  "a boolean (true or false)",
		Parse:    strconv.ParseBool,
		Implicit: "true",
	})

	Register(Type[int]{Name: "int", Expects: expectsInteger, Parse: signed[int](strconv.IntSize)})
	Register(Type[int8]{Name: "int", Expects: expectsInteger, Parse: signed[int8](8)})
	Register(Type[int16]{Name: "int", Expects: expectsInteger, Parse: signed[int16](16)})
	Register(Type[int32]{Name: "int", Expects: expectsInteger, Parse: signed[int32](32)})
	Register(Type[int64]{Name: "int", Expects: expectsInteger, Parse: signed[int64](64)})

	Register(Type[uint]{Name: "uint", Expects: expectsUnsigned, Parse: unsigned[uint](strconv.IntSize)})
	Register(Type[uint8]{Name: "uint", Expects: expectsUnsigned, Parse: unsigned[uint8](8)})
	Register(Type[uint16]{Name: "uint", Expects: expectsUnsigned, Parse: unsigned[uint16](16)})
	Register(Type[uint32]{Name: "uint", Expects: expectsUnsigned, Parse: unsigned[uint32](32)})
	Register(Type[uint64]{Name: "uint", Expects: expectsUnsigned, Parse: unsigned[uint64](64)})

	Register(Type[float32]{Name: "float", Expects: expectsFloat, Parse: floating[float32](32)})
	Register(Type[float64]{Name: "float", Expects: expectsFloat, Parse: floating[float64](64)})

	Register(Type[time.Duration]{
		Name:    "duration",
		Expects: `a duration (such as "300ms", "1.5h" or "2h45m")`,
		Parse:   time.ParseDuration,
	})
}

// signed parses the signed integers which fit in the given number of bits, in
// the base their notation implies: "42", "0x2a", "0b101010" and "0o52" all
// read as the same number.
func signed[T ~int | ~int8 | ~int16 | ~int32 | ~int64](bits int) Parser[T] {
	return func(argument string) (T, error) {
		parsed, err := strconv.ParseInt(argument, 0, bits)

		return T(parsed), err
	}
}

// unsigned parses the unsigned integers which fit in the given number of bits.
func unsigned[T ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](bits int) Parser[T] {
	return func(argument string) (T, error) {
		parsed, err := strconv.ParseUint(argument, 0, bits)

		return T(parsed), err
	}
}

// floating parses the floating point numbers which fit in the given number of
// bits.
func floating[T ~float32 | ~float64](bits int) Parser[T] {
	return func(argument string) (T, error) {
		parsed, err := strconv.ParseFloat(argument, bits)

		return T(parsed), err
	}
}
