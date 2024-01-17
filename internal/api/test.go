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
func (t *MockT) Skipf(f string, args ...any) {
	fmt.Printf(f, args...)
}
func (t *MockT) Errorf(f string, args ...any) {
	fmt.Printf(f, args...)
}
func (t *MockT) Fatalf(f string, args ...any) {
	fmt.Printf(f, args...)
	os.Exit(1)
}
func (t *MockT) Error(args ...any) {
	t.Errorf("Error:", args...)
}
func (t *MockT) Name() string { return "inline_script" }
func (t *MockT) Failed() bool { return false }
