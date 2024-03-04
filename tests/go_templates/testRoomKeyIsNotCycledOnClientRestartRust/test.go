package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
)

func main() {
	rust.SetupLogs("rust_sdk_inline_script")
	t := &api.MockT{}
	cfg := api.ClientCreationOpts{
		BaseURL:           "{{.BaseURL}}",
		UserID:            "{{.UserID}}",
		DeviceID:          "{{.DeviceID}}",
		Password:          "{{.Password}}",
		SlidingSyncURL:    "{{.SSURL}}",
		PersistentStorage: strings.EqualFold("{{.PersistentStorage}}", "true"),
	}
	client, err := rust.NewRustClient(t, cfg)
	if err != nil {
		panic(err)
	}
	if err := client.Login(t, cfg); err != nil {
		panic(err)
	}
	stopSyncing := client.MustStartSyncing(t)
	defer client.Close(t)
	defer stopSyncing()
	roomID := "{{.RoomID}}"
	fmt.Println("Client logged in. Sending '{{.Body}}' in room {{.RoomID}}")
	eventID := client.SendMessage(t, "{{.RoomID}}", "{{.Body}}")
	fmt.Println("Sent event " + eventID + " waiting for remote echo")

	waiter := client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID))
	waiter.Wait(t, 5*time.Second)

	time.Sleep(time.Second)
	fmt.Println("exiting")
}
