package api

import (
	"fmt"
	"os"
)

type MockT struct{}

func (t *MockT) Helper() {}
func (t *MockT) Logf(f string, args ...any) {
	fmt.Printf(f, args...)
}
func (t *MockT) Errorf(f string, args ...any) {
	fmt.Printf(f, args...)
}
func (t *MockT) Fatalf(f string, args ...any) {
	fmt.Printf(f, args...)
	os.Exit(1)
}
func (t *MockT) Name() string { return "inline_script" }

type Test interface {
	Logf(f string, args ...any)
	Errorf(f string, args ...any)
	Fatalf(f string, args ...any)
	Helper()
	Name() string
}

// TODO move to must package when it accepts an interface

// NotError will ensure `err` is nil else terminate the test with `msg`.
func MustNotError(t Test, msg string, err error) {
	t.Helper()
	if err != nil {
		Fatalf(t, "must.NotError: %s -> %s", msg, err)
	}
}

// NotEqual ensures that got!=want else logs an error.
// The 'msg' is displayed with the error to provide extra context.
func MustNotEqual[V comparable](t Test, got, want V, msg string) {
	t.Helper()
	if got == want {
		Errorf(t, "NotEqual %s: got '%v', want '%v'", msg, got, want)
	}
}

const ansiRedForeground = "\x1b[31m"
const ansiResetForeground = "\x1b[39m"

// Errorf is a wrapper around t.Errorf which prints the failing error message in red.
func Errorf(t Test, format string, args ...any) {
	t.Helper()
	format = ansiRedForeground + format + ansiResetForeground
	t.Errorf(format, args...)
}

// Fatalf is a wrapper around t.Fatalf which prints the failing error message in red.
func Fatalf(t Test, format string, args ...any) {
	t.Helper()
	format = ansiRedForeground + format + ansiResetForeground
	t.Fatalf(format, args...)
}
