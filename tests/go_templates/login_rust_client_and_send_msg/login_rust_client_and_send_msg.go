package main

import (
	"fmt"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
)

func main() {
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
	if err := client.Login(t, cfg); err != nil {
		panic(err)
	}
	client.MustStartSyncing(t)
	defer client.Close(t)
	roomID := "{{.RoomID}}"
	fmt.Println("Client logged in. Sending '{{.Body}}' in room {{.RoomID}}")
	eventID := client.SendMessage(t, "{{.RoomID}}", "{{.Body}}")
	fmt.Println("Sent event " + eventID +" waiting for remote echo")

	waiter := client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID))
	waiter.Wait(t, 5 * time.Second)

	time.Sleep(time.Second)
	fmt.Println("exiting")
}
