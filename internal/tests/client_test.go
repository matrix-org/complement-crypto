// package tests contains sanity checks that any client implementation can run to ensure their concrete implementation will work
// correctly with complement-crypto. Writing code to interact with your concrete client SDK is error-prone. The purpose of these
// tests is to ensure that the code that implements api.Client is correct.
package tests

import (
	"fmt"
	"log"
	"os"
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
	clientFactories []func(t *testing.T, cfg api.ClientCreationOpts) api.Client
)

func Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	ssMutex.Lock()
	defer ssMutex.Unlock()
	if ssDeployment != nil {
		return ssDeployment
	}
	ssDeployment = deploy.RunNewDeployment(t, "", false)
	return ssDeployment
}

func TestMain(m *testing.M) {
	rustClientCreator := func(t *testing.T, cfg api.ClientCreationOpts) api.Client {
		client, err := rust.NewRustClient(t, cfg)
		if err != nil {
			t.Fatalf("NewRustClient: %s", err)
		}
		return client
	}
	jsClientCreator := func(t *testing.T, cfg api.ClientCreationOpts) api.Client {
		client, err := js.NewJSClient(t, cfg)
		if err != nil {
			t.Fatalf("NewJSClient: %s", err)
		}
		return client
	}
	clientFactories = append(clientFactories, rustClientCreator, jsClientCreator)

	rpcBinary := os.Getenv("COMPLEMENT_CRYPTO_RPC_BINARY")
	if rpcBinary != "" {
		clientFactories = append(clientFactories, func(t *testing.T, cfg api.ClientCreationOpts) api.Client {
			remoteBindings, err := deploy.NewRPCLanguageBindings(rpcBinary, api.ClientTypeRust, "")
			if err != nil {
				log.Fatal(err)
			}
			return remoteBindings.MustCreateClient(t, cfg)
		})
	}
	rust.SetupLogs("rust_sdk_logs")
	js.SetupJSLogs("./logs/js_sdk.log")                       // rust sdk logs on its own
	complement.TestMainWithCleanup(m, "clienttests", func() { // always teardown even if panicking
		ssMutex.Lock()
		if ssDeployment != nil {
			ssDeployment.Teardown()
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
		client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventIDs[len(eventIDs)-1])).Waitf(t, 5*time.Second, "client did not see latest event")
		// ensure we have backpaginated if we need to. It is valid for a client to only sync the latest
		// event in the room, so we have to backpaginate here.
		client.MustBackpaginate(t, roomID, len(eventIDs))
		// ensure we see all the events
		for _, eventID := range eventIDs {
			client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID)).Waitf(t, 5*time.Second, "client did not see event %s", eventID)
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
			client.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID)).Waitf(t, 5*time.Second, "client did not see event %s", eventID)
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
		waiter.Waitf(t, 5*time.Second, "client did not see final message")

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
			waiter.Waitf(t, 5*time.Second, "client did not seee event %s", eventID)
			completed.Finish()
		}()
		t.Logf("waiting for event %s", eventID)
		stopSyncing := client.MustStartSyncing(t)
		defer stopSyncing()
		completed.Wait(t, 5*time.Second)
	})
}

func TestSendingEvents(t *testing.T) {
	deployment := Deploy(t)
	ForEachClient(t, "", deployment, func(t *testing.T, client api.Client, csapi *client.CSAPI) {
		must.NotError(t, "Failed to login", client.Login(t, client.Opts()))
		roomID := csapi.MustCreateRoom(t, map[string]interface{}{})
		stopSyncing := client.MustStartSyncing(t)
		defer stopSyncing()
		eventID := client.SendMessage(t, roomID, "Test Message")
		event := client.MustGetEvent(t, roomID, eventID)
		must.Equal(t, event.Text, "Test Message", "event text mismatch")
		eventID2, err := client.TrySendMessage(t, roomID, "Another Test Message")
		must.NotError(t, "TrySendMessage failed", err)
		event2 := client.MustGetEvent(t, roomID, eventID2)
		must.Equal(t, event2.Text, "Another Test Message", "event text mismatch")
		// sending to a bogus room should error but not fail the test
		invalidEventID, err := client.TrySendMessage(t, "!foo:hs1", "This should not work")
		t.Logf("TrySendMessage -> %v", err)
		must.NotEqual(t, err, nil, "TrySendMessage returned no error when it should have")
		must.Equal(t, invalidEventID, "", "TrySendMessage returned an event ID when it should have returned an error")
	})
}

// run a subtest for each client factory
func ForEachClient(t *testing.T, name string, deployment *deploy.SlidingSyncDeployment, fn func(t *testing.T, client api.Client, csapi *client.CSAPI)) {
	for _, createClient := range clientFactories {
		csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "client",
			Password:        "complement-crypto-password",
		})
		opts := api.NewClientCreationOpts(csapiAlice)
		opts.SlidingSyncURL = deployment.SlidingSyncURLForHS(t, "hs1")
		client := createClient(t, opts)
		t.Run(name+" "+string(client.Type()), func(t *testing.T) {
			fn(t, client, csapiAlice)
		})
	}
}
