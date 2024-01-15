package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
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

func ProcessSignal() {
	sigch := make(chan os.Signal, 2)
	signal.Notify(sigch, syscall.SIGUSR2)
	for {
		signalType := <-sigch
		fmt.Println("Received signal SIGUSR2 from channel : ", signalType)
		panic("terminating...")
	}
}

func main() {
	go ProcessSignal()
	time.Sleep(time.Second)
	t := &MockT{}
	cfg := api.ClientCreationOpts{
		BaseURL:           "{{.BaseURL}}",
		UserID:            "{{.UserID}}",
		DeviceID:          "{{.DeviceID}}",
		Password:          "{{.Password}}",
		PersistentStorage: true,
	}
	client, err := rust.NewRustClient(t, cfg, "{{.SSURL}}")
	if err != nil {
		panic(err)
	}
	fmt.Println(time.Now(), "script about to login, expect /keys/upload")
	client.Login(t, cfg)
	fmt.Println("exiting.. you should not see this as it should have been sigkilled by now!")

}
