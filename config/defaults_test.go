package config

// Code from Sonnet-4.6 (edited)

import (
	"errors"
	"testing"
	"time"

	flags "github.com/jessevdk/go-flags"
)

// ---- nested config types (mirror real go-flags usage) ----

type DBConfig struct {
	MaxConns int    `long:"max-conns" default:"10"`
	SSLMode  string `long:"ssl-mode" default:"disable"`
	// base tag: go-flags will parse this as hex; our old hand-rolled code
	// could not handle this at all.
	Flags uint32 `long:"flags" default:"FF" base:"16"`
}

type ServerConfig struct {
	Host    string        `long:"host" default:"localhost"`
	Port    uint16        `long:"port" default:"8080"`
	Timeout time.Duration `long:"timeout" default:"30s"`
}

// CustomType implements flags.Unmarshaler so go-flags (and therefore
// our Defaults) must call UnmarshalFlag instead of strconv.
type CustomType struct{ Val string }

func (c *CustomType) UnmarshalFlag(s string) error {
	c.Val = "custom:" + s
	return nil
}

type AppConfig struct {
	DSN     string     `long:"dsn" default:"postgres://localhost/db" env:"DSN"`
	Debug   bool       `long:"debug"`
	Workers int        `long:"workers" default:"4"`
	Rate    float64    `long:"rate" default:"1.5"`
	Custom  CustomType `long:"custom" default:"hello"`

	// Multiple default tags — only go-flags (not our old code) parses these
	// into []string{...} correctly; a single `default:"1 2 3"` string is the
	// old workaround.
	Tags []string `long:"tags" default:"a" default:"b" default:"c"`
	Nums []int    `long:"nums" default:"1" default:"2" default:"3"`

	DB     DBConfig     `group:"Database" namespace:"db" env-namespace:"DB"`
	Server ServerConfig `group:"Server" namespace:"server" env-namespace:"SERVER"`
}

// ---- tests ----

func TestDefaults_EmptyStruct(t *testing.T) {
	cfg, err := Defaults(AppConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertStr(t, "DSN", cfg.DSN, "postgres://localhost/db")
	assertBool(t, "Debug", cfg.Debug, false)
	assertInt(t, "Workers", int64(cfg.Workers), 4)
	assertFloat(t, "Rate", cfg.Rate, 1.5)

	// Unmarshaler
	assertStr(t, "Custom.Val", cfg.Custom.Val, "custom:hello")

	// Multiple default tags
	assertStrSlice(t, "Tags", cfg.Tags, []string{"a", "b", "c"})
	assertIntSlice(t, "Nums", cfg.Nums, []int{1, 2, 3})

	// Nested group
	assertStr(t, "DB.SSLMode", cfg.DB.SSLMode, "disable")
	assertInt(t, "DB.MaxConns", int64(cfg.DB.MaxConns), 10)
	// base:"16" — go-flags parses "FF" as 255 in hex
	assertUint(t, "DB.Flags", uint64(cfg.DB.Flags), 255)

	assertStr(t, "Server.Host", cfg.Server.Host, "localhost")
	assertUint(t, "Server.Port", uint64(cfg.Server.Port), 8080)
	assertDuration(t, "Server.Timeout", cfg.Server.Timeout, 30*time.Second)
}

func TestDefaults_PreserveExistingValues(t *testing.T) {
	input := AppConfig{
		DSN:     "custom://db",
		Workers: 99,
		DB:      DBConfig{SSLMode: "require"},
	}

	cfg, err := Defaults(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-zero fields must not be overwritten.
	assertStr(t, "DSN", cfg.DSN, "custom://db")
	assertInt(t, "Workers", int64(cfg.Workers), 99)
	assertStr(t, "DB.SSLMode", cfg.DB.SSLMode, "require")

	// Zero sibling fields in the same nested struct still get their default.
	assertInt(t, "DB.MaxConns", int64(cfg.DB.MaxConns), 10)
}

func TestDefaults_DoesNotMutateOriginal(t *testing.T) {
	original := AppConfig{}
	result, err := Defaults(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if original.DSN != "" {
		t.Error("original was mutated")
	}
	if result.DSN == "" {
		t.Error("result should have default DSN")
	}
}

func TestDefaults_MultipleDefaultTags_Slices(t *testing.T) {
	// Explicit test that multiple default tags (not a comma-split workaround)
	// produce the right number of elements.
	type S struct {
		Items []string `long:"items" default:"x" default:"y" default:"z"`
	}
	cfg, err := Defaults(S{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStrSlice(t, "Items", cfg.Items, []string{"x", "y", "z"})
}

func TestDefaults_BaseTag_HexInteger(t *testing.T) {
	type S struct {
		Perm uint32 `long:"perm" default:"1FF" base:"16"`
	}
	cfg, err := Defaults(S{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Perm != 0x1FF {
		t.Errorf("Perm: got %d, want %d", cfg.Perm, 0x1FF)
	}
}

func TestDefaults_UnmarshalerInterface(t *testing.T) {
	type S struct {
		Val CustomType `long:"val" default:"world"`
	}
	cfg, err := Defaults(S{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// UnmarshalFlag prepends "custom:" — proves go-flags' path was taken.
	assertStr(t, "Val.Val", cfg.Val.Val, "custom:world")
}

func TestDefaults_NoDefaultTag(t *testing.T) {
	type S struct {
		Value string `long:"value"`
	}
	cfg, err := Defaults(S{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Value != "" {
		t.Errorf("Value should remain empty, got %q", cfg.Value)
	}
}

func TestDefaults_InvalidDefault_ReturnsError(t *testing.T) {
	type S struct {
		N int `long:"n" default:"not-a-number"`
	}
	_, err := Defaults(S{})
	if err == nil {
		t.Error("expected error for invalid default value, got nil")
	}
	// The error must come from go-flags' own conversion layer (ErrMarshal).
	var flagErr *flags.Error
	if !errors.As(err, &flagErr) || flagErr.Type != flags.ErrMarshal {
		t.Errorf("expected flags.ErrMarshal, got: %v", err)
	}
}

// Verify our Defaults signature is compatible with the flags.Unmarshaler
// interface check at compile time.
var _ flags.Unmarshaler = (*CustomType)(nil)

// ---- helpers ----

func assertStr(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", name, got, want)
	}
}

func assertBool(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", name, got, want)
	}
}

func assertInt(t *testing.T, name string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %d, want %d", name, got, want)
	}
}

func assertUint(t *testing.T, name string, got, want uint64) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %d, want %d", name, got, want)
	}
}

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %f, want %f", name, got, want)
	}
}

func assertDuration(t *testing.T, name string, got, want time.Duration) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", name, got, want)
	}
}

func assertStrSlice(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len %d, want %d (%v vs %v)", name, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: got %q, want %q", name, i, got[i], want[i])
		}
	}
}

func assertIntSlice(t *testing.T, name string, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len %d, want %d (%v vs %v)", name, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: got %d, want %d", name, i, got[i], want[i])
		}
	}
}
