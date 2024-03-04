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
		PersistentStorage: {{.PersistentStorage}},
	}
	client, err := rust.NewRustClient(t, cfg, "{{.SSURL}}")
	if err != nil {
		panic(err)
	}
	fmt.Println(time.Now(), "script about to login, expect /keys/upload")
	client.Login(t, cfg)
	client.MustStartSyncing(t)
	time.Sleep(2 * time.Second)
	fmt.Println("exiting.. you should not see this as it should have been sigkilled by now!")

}
