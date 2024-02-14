// package tests contains sanity checks that any client implementation can run to ensure their concrete implementation will work
// correctly with complement-crypto. Writing code to interact with your concrete client SDK is error-prone. The purpose of these
// tests is to ensure that the code that implements api.Client is correct.
package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/b"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

var (
	ssMutex      *sync.Mutex = &sync.Mutex{}
	ssDeployment *deploy.SlidingSyncDeployment
	// aka functions which make clients, and we don't care about the language.
	// Tests just loop through this array for each client impl.
	clientFactories []func(t *testing.T, cfg api.ClientCreationOpts, deployment *deploy.SlidingSyncDeployment) api.Client
)

func Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	ssMutex.Lock()
	defer ssMutex.Unlock()
	if ssDeployment != nil {
		return ssDeployment
	}
	ssDeployment = deploy.RunNewDeployment(t, false)
	return ssDeployment
}

func TestMain(m *testing.M) {
	rustClientCreator := func(t *testing.T, cfg api.ClientCreationOpts, deployment *deploy.SlidingSyncDeployment) api.Client {
		client, err := rust.NewRustClient(t, cfg, deployment.SlidingSyncURL(t))
		if err != nil {
			t.Fatalf("NewRustClient: %s", err)
		}
		return client
	}
	jsClientCreator := func(t *testing.T, cfg api.ClientCreationOpts, deployment *deploy.SlidingSyncDeployment) api.Client {
		client, err := js.NewJSClient(t, cfg)
		if err != nil {
			t.Fatalf("NewJSClient: %s", err)
		}
		return client
	}
	clientFactories = append(clientFactories, rustClientCreator, jsClientCreator)
	js.SetupJSLogs("./logs/js_sdk.log")                       // rust sdk logs on its own
	complement.TestMainWithCleanup(m, "clienttests", func() { // always teardown even if panicking
		ssMutex.Lock()
		if ssDeployment != nil {
			ssDeployment.Teardown(false)
		}
		ssMutex.Unlock()
		js.WriteJSLogs()
	})
}

// Test that the client can receive live messages as well as return events that have already been received.
func TestReceiveTimeline(t *testing.T) {
	deployment := Deploy(t)

	createAndSendEvents := func(t *testing.T, csapi *client.CSAPI) (roomID string, eventIDs []string) {
		roomID = csapi.MustCreateRoom(t, map[string]interface{}{})
		for i := 0; i < 10; i++ {
			eventIDs = append(eventIDs, csapi.SendEventSynced(t, roomID, b.Event{
				Type: "m.room.message",
				Content: map[string]interface{}{
					"msgtype": "m.text",
					"body":    fmt.Sprintf("Test Message %d", i+1),
				},
			}))
		}
		return
	}

	// test that if we start syncing with a room full of events, we see those events.
	ForEachClient(t, "existing_events", deployment, func(t *testing.T, client api.Client, csapi *client.CSAPI) {
		must.NotError(t, "Failed to login", client.Login(t, client.Opts()))
		roomID, eventIDs := createAndSendEvents(t, csapi)
		time.Sleep(time.Second) // give time for everything to settle server-side e.g sliding sync proxy
		stopSyncing := client.MustStartSyncing(t)
		defer stopSyncing()
		// wait until we see the latest event
		client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventIDs[len(eventIDs)-1])).Wait(t, 5*time.Second)
		// ensure we have backpaginated if we need to. It is valid for a client to only sync the latest
		// event in the room, so we have to backpaginate here.
		client.MustBackpaginate(t, roomID, len(eventIDs))
		// ensure we see all the events
		for _, eventID := range eventIDs {
			client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID)).Wait(t, 5*time.Second)
		}
		// check event content is correct
		for i, eventID := range eventIDs {
			ev := client.MustGetEvent(t, roomID, eventID)
			must.Equal(t, ev.FailedToDecrypt, false, "FailedToDecrypt")
			must.Equal(t, ev.ID, eventID, "ID")
			must.Equal(t, ev.Membership, "", "Membership")
			must.Equal(t, ev.Sender, csapi.UserID, "Sender")
			must.Equal(t, ev.Target, "", "Target")
			must.Equal(t, ev.Text, fmt.Sprintf("Test Message %d", i+1), "Text")
		}
	})

	// test that if we are already syncing and then see a room live stream full of events, we see those events.
	ForEachClient(t, "live_events", deployment, func(t *testing.T, client api.Client, csapi *client.CSAPI) {
		must.NotError(t, "Failed to login", client.Login(t, client.Opts()))
		stopSyncing := client.MustStartSyncing(t)
		defer stopSyncing()
		time.Sleep(time.Second) // give time for syncing to be well established.
		// send the messages whilst syncing.
		roomID, eventIDs := createAndSendEvents(t, csapi)
		// ensure we see all the events
		for i, eventID := range eventIDs {
			t.Logf("waiting for event %d : %s", i, eventID)
			client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID)).Wait(t, 5*time.Second)
		}
		// now send another live event and ensure we see it. This ensure we can still wait for events after having
		// previously waited for events.
		waiter := client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("Final"))
		csapi.SendEventSynced(t, roomID, b.Event{
			Type: "m.room.message",
			Content: map[string]interface{}{
				"msgtype": "m.text",
				"body":    "Final",
			},
		})
		waiter.Wait(t, 5*time.Second)

		// check event content is correct
		for i, eventID := range eventIDs {
			ev := client.MustGetEvent(t, roomID, eventID)
			must.Equal(t, ev.FailedToDecrypt, false, "FailedToDecrypt")
			must.Equal(t, ev.ID, eventID, "ID")
			must.Equal(t, ev.Membership, "", "Membership")
			must.Equal(t, ev.Sender, csapi.UserID, "Sender")
			must.Equal(t, ev.Target, "", "Target")
			must.Equal(t, ev.Text, fmt.Sprintf("Test Message %d", i+1), "Text")
		}
	})
}

func TestCanWaitUntilEventInRoomBeforeRoomIsKnown(t *testing.T) {
	deployment := Deploy(t)
	ForEachClient(t, "", deployment, func(t *testing.T, client api.Client, csapi *client.CSAPI) {
		roomID := csapi.MustCreateRoom(t, map[string]interface{}{})
		eventID := csapi.SendEventSynced(t, roomID, b.Event{
			Type: "m.room.message",
			Content: map[string]interface{}{
				"msgtype": "m.text",
				"body":    "Test Message",
			},
		})
		must.NotError(t, "Failed to login", client.Login(t, client.Opts()))
		completed := helpers.NewWaiter()
		waiter := client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID))
		go func() {
			waiter.Wait(t, 5*time.Second)
			completed.Finish()
		}()
		t.Logf("waiting for event %s", eventID)
		stopSyncing := client.MustStartSyncing(t)
		defer stopSyncing()
		completed.Wait(t, 5*time.Second)
	})
}

// run a subtest for each client factory
func ForEachClient(t *testing.T, name string, deployment *deploy.SlidingSyncDeployment, fn func(t *testing.T, client api.Client, csapi *client.CSAPI)) {
	for _, createClient := range clientFactories {
		csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "client",
			Password:        "complement-crypto-password",
		})
		client := createClient(t, api.NewClientCreationOpts(csapiAlice), deployment)
		t.Run(name+" "+string(client.Type()), func(t *testing.T) {
			fn(t, client, csapiAlice)
		})
	}
}
