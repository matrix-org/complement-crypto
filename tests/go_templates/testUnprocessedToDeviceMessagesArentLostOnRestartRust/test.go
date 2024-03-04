package main

import (
	"fmt"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
)

func main() {
	rust.SetupLogs("rust_sdk_inline_script")
	time.Sleep(time.Second)
	t := &api.MockT{}
	cfg := api.ClientCreationOpts{
		BaseURL:           "{{.BaseURL}}",
		UserID:            "{{.UserID}}",
		DeviceID:          "{{.DeviceID}}",
		Password:          "{{.Password}}",
		SlidingSyncURL:    "{{.SSURL}}",
		PersistentStorage: {{.PersistentStorage}},
	}
	client, err := rust.NewRustClient(t, cfg)
	if err != nil {
		panic(err)
	}
	client.Login(t, cfg)
	fmt.Println(time.Now(), "script about to /sync, expecting to be killed when the right to-device event arrives...")
	client.MustStartSyncing(t)
	time.Sleep(10 * time.Second)
	fmt.Println("exiting.. you should not see this as it should have been sigkilled by now!")

}
