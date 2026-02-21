package config

// Code from Sonnet-4.6

import (
	"errors"
	"reflect"
	"time"

	flags "github.com/jessevdk/go-flags"
)

// Defaults returns a copy of src with all zero-value fields filled in from
// their `default:"..."` struct tags, using go-flags' own parsing and
// type-conversion machinery.
//
// It delegates entirely to go-flags' internal setDefaults() by calling
// ParseArgs([]string{}), which means every go-flags feature works correctly:
//   - Multiple `default` tags on one field (slices/maps).
//   - Custom types implementing flags.Unmarshaler.
//   - The `base` tag for non-decimal integers.
//   - Nested structs with `group`/`namespace`/`env-namespace` tags.
//   - time.Duration strings like "30s".
//
// Non-zero fields in src always take priority over any default.
// ErrRequired errors are silently ignored — Defaults only fills gaps, it
// does not enforce field presence.
func Defaults[T any](src T) (T, error) {
	// Step 1 — let go-flags apply every `default` tag to a fresh zero struct.
	// ParseArgs([]string{}) triggers the exact same internal setDefaults() path
	// that go-flags uses during normal argument parsing, so all conversion
	// rules (Unmarshaler, base, multi-default, duration strings, …) are
	// handled identically.
	var withDefaults T
	p := flags.NewParser(&withDefaults, flags.None)
	if _, err := p.ParseArgs([]string{}); err != nil {
		var flagErr *flags.Error
		// ErrRequired fires when a `required:"true"` field has no value.
		// That is expected here — we are filling defaults, not enforcing
		// presence — so we ignore it and return what setDefaults populated.
		if !errors.As(err, &flagErr) ||
			(flagErr.Type != flags.ErrRequired &&
				flagErr.Type != flags.ErrCommandRequired) {
			var zero T
			return zero, err
		}
	}

	// Step 2 — override defaults with whatever the caller already set in src.
	// Any non-zero field in src wins over the default we just computed.
	result := withDefaults
	overrideNonZero(reflect.ValueOf(&result).Elem(), reflect.ValueOf(src))
	return result, nil
}

// overrideNonZero copies non-zero fields from src on top of dst so that
// explicit caller values always win over defaults.
// It recurses into struct fields so nested groups are handled transparently.
func overrideNonZero(dst, src reflect.Value) {
	t := src.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := src.Field(i)
		df := dst.Field(i)

		if !df.CanSet() {
			continue
		}

		switch sf.Kind() {
		case reflect.Struct:
			// time.Time and similar leaf structs must be copied whole, not
			// recursed into field-by-field.
			if sf.Type() == reflect.TypeOf(time.Time{}) {
				if !sf.IsZero() {
					df.Set(sf)
				}
			} else {
				overrideNonZero(df, sf)
			}
		case reflect.Ptr:
			if !sf.IsNil() {
				df.Set(sf)
			}
		default:
			if !sf.IsZero() {
				df.Set(sf)
			}
		}
	}
}
