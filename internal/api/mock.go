package api

import (
	"fmt"
	"os"
)

type MockT struct {
	TestName string
}

func (t *MockT) Helper() {}
func (t *MockT) Logf(f string, args ...any) {
	fmt.Printf(f+"\n", args...)
}
func (t *MockT) Skipf(f string, args ...any) {
	fmt.Printf(f+"\n", args...)
}
func (t *MockT) Errorf(f string, args ...any) {
	fmt.Printf(f+"\n", args...)
}
func (t *MockT) Fatalf(f string, args ...any) {
	fmt.Printf(f+"\n", args...)
	os.Exit(1)
}
func (t *MockT) Error(args ...any) {
	fmt.Printf("Error: %v", args...)
}
func (t *MockT) Name() string {
	if t.TestName != "" {
		return t.TestName
	}
	return "inline_script"
}
func (t *MockT) Failed() bool { return false }
