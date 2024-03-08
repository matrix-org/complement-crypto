package tests

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	templates "github.com/matrix-org/complement-crypto/tests/go_templates"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
	"github.com/tidwall/gjson"
)

// Test that if a client is unable to call /sendToDevice, it retries.
func TestClientRetriesSendToDevice(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, EncRoomOptions.PresetPublicChat())
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			// lets device keys be exchanged
			time.Sleep(time.Second)

			wantMsgBody := "Hello world!"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))

			var evID string
			var err error
			// now gateway timeout the /sendToDevice endpoint
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"statuscode": map[string]interface{}{
					"return_status": http.StatusGatewayTimeout,
					"filter":        "~u .*\\/sendToDevice.*",
				},
			}, func() {
				evID, err = alice.TrySendMessage(t, roomID, wantMsgBody)
				if err != nil {
					// we allow clients to fail the send if they cannot call /sendToDevice
					t.Logf("TrySendMessage: %s", err)
				}
				if evID != "" {
					t.Logf("TrySendMessage: => %s", evID)
				}
			})

			if err != nil {
				// retry now we have connectivity
				evID = alice.SendMessage(t, roomID, wantMsgBody)
			}

			// Bob receives the message
			t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
			waiter.Wait(t, 5*time.Second)
		})
	})
}

// Regression test for https://github.com/vector-im/element-web/issues/23113
// "If you restart (e.g. upgrade) Element while it's waiting to process a m.room_key, it'll drop it and you'll get UISIs"
//
// - Alice (2 devices) and Bob are in an encrypted room.
// - Bob's client is shut down temporarily.
// - Alice's 2nd device logs out, which will Alice's 1st device to cycle room keys.
// - Start sniffing /sync traffic. Bob's client comes back.
// - When /sync shows a to-device message from Alice (indicating the room key), sleep(1ms) then SIGKILL Bob.
// - Restart Bob's client.
// - Ensure Bob can decrypt new messages sent from Alice.
func TestUnprocessedToDeviceMessagesArentLostOnRestart(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		// prepare for the test: register all 3 clients and create the room
		tc := CreateTestContext(t, clientType, clientType)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, EncRoomOptions.Invite([]string{tc.Bob.UserID}))
		tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
		alice2 := tc.Deployment.Login(t, clientType.HS, tc.Alice, helpers.LoginOpts{
			DeviceID: "ALICE_TWO",
			Password: "complement-crypto-password",
		})
		// the initial setup for rust/js is the same.
		tc.WithAliceSyncing(t, func(alice api.Client) {
			bob := tc.MustLoginClient(t, tc.Bob, tc.BobClientType, WithPersistentStorage())
			// we will close this in the test, no defer
			bobStopSyncing := bob.MustStartSyncing(t)
			tc.WithClientSyncing(t, tc.AliceClientType, alice2, func(alice2 api.Client) { // sync to ensure alice2 has keys uploaded
				// check the room works
				alice.SendMessage(t, roomID, "Hello World!")
				bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("Hello World!")).Wait(t, 2*time.Second)
			})
			// stop bob's client
			bobStopSyncing()
			bob.Logf(t, "Bob is about to be Closed()")
			bob.Close(t)

			// send a lot of to-device messages to bob to increase the window in which to SIGKILL the client.
			// It's unimportant what these are.
			for i := 0; i < 60; i++ {
				alice2.MustSendToDeviceMessages(t, "m.room_key_request", map[string]map[string]map[string]interface{}{
					bob.UserID(): {
						"*": {
							"action":               "request_cancellation",
							"request_id":           fmt.Sprintf("random_%d", i),
							"requesting_device_id": "WHO_KNOWS",
						},
					},
				})
			}
			t.Logf("to-device msgs sent")

			// logout alice 2
			alice2.MustDo(t, "POST", []string{"_matrix", "client", "v3", "logout"})

			// if clients cycle room keys eagerly then the above logout will cause room keys to be sent.
			// We want to wait for that to happen before sending the kick event. This is notable for JS.
			time.Sleep(time.Second)

			// send a message as alice to make a new room key (if we didn't already on the /logout above)
			eventID := alice.SendMessage(t, roomID, "Kick to make a new room key!")

			// client specific impls to handle restarts.
			switch clientType.Lang {
			case api.ClientTypeRust:
				testUnprocessedToDeviceMessagesArentLostOnRestartRust(t, tc, bob.Opts(), roomID, eventID)
			case api.ClientTypeJS:
				testUnprocessedToDeviceMessagesArentLostOnRestartJS(t, tc, bob.Opts(), roomID, eventID)
			default:
				t.Fatalf("unknown lang: %s", clientType.Lang)
			}
		})
	})
}

func testUnprocessedToDeviceMessagesArentLostOnRestartRust(t *testing.T, tc *TestContext, bobOpts api.ClientCreationOpts, roomID, eventID string) {
	// sniff /sync traffic
	waitForRoomKey := helpers.NewWaiter()
	tc.Deployment.WithSniffedEndpoint(t, "/sync", func(cd deploy.CallbackData) {
		// When /sync shows a to-device message from Alice (indicating the room key), then SIGKILL Bob.
		body := gjson.ParseBytes(cd.ResponseBody)
		toDeviceEvents := body.Get("extensions.to_device.events").Array() // Sliding Sync form
		if len(toDeviceEvents) > 0 {
			for _, ev := range toDeviceEvents {
				if ev.Get("type").Str == "m.room.encrypted" {
					t.Logf("detected potential room key")
					waitForRoomKey.Finish()
				}
			}
		}
	}, func() {
		// bob comes back online, and will be killed a short while later.
		t.Logf("recreating bob")
		cmd, close := templates.PrepareGoScript(t, "testUnprocessedToDeviceMessagesArentLostOnRestartRust/test.go",
			struct {
				UserID            string
				DeviceID          string
				Password          string
				BaseURL           string
				SSURL             string
				PersistentStorage bool
			}{
				UserID:            bobOpts.UserID,
				Password:          bobOpts.Password,
				DeviceID:          bobOpts.DeviceID,
				BaseURL:           bobOpts.BaseURL,
				PersistentStorage: bobOpts.PersistentStorage,
				SSURL:             bobOpts.SlidingSyncURL,
			})
		cmd.WaitDelay = 3 * time.Second
		defer close()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Start()
		waitForRoomKey.Wait(t, 10*time.Second)
		time.Sleep(time.Millisecond) // wait a bit to let the client be mid-processing
		t.Logf("killing external process")
		must.NotError(t, "failed to kill process", cmd.Process.Kill())

		// Ensure Bob can decrypt new messages sent from Alice.
		bob := tc.MustLoginClient(t, tc.Bob, tc.BobClientType, WithPersistentStorage())
		defer bob.Close(t)
		bobStopSyncing := bob.MustStartSyncing(t)
		defer bobStopSyncing()
		// we can't rely on MustStartSyncing returning to know that the room key has been received, as
		// in rust we just wait for RoomListLoadingStateLoaded which is a separate connection to the
		// encryption loop.
		time.Sleep(time.Second)
		ev := bob.MustGetEvent(t, roomID, eventID)
		must.Equal(t, ev.FailedToDecrypt, false, "unable to decrypt message")
		must.Equal(t, ev.Text, "Kick to make a new room key!", "event text mismatch")
	})
}

func testUnprocessedToDeviceMessagesArentLostOnRestartJS(t *testing.T, tc *TestContext, bobOpts api.ClientCreationOpts, roomID, eventID string) {
	// sniff /sync traffic
	waitForRoomKey := helpers.NewWaiter()
	tc.Deployment.WithSniffedEndpoint(t, "/sync", func(cd deploy.CallbackData) {
		// When /sync shows a to-device message from Alice (indicating the room key) then SIGKILL Bob.
		body := gjson.ParseBytes(cd.ResponseBody)
		toDeviceEvents := body.Get("to_device.events").Array() // Sync v2 form
		if len(toDeviceEvents) > 0 {
			for _, ev := range toDeviceEvents {
				if ev.Get("type").Str == "m.room.encrypted" {
					t.Logf("detected potential room key")
					waitForRoomKey.Finish()
				}
			}
		}
	}, func() {
		bob := tc.MustLoginClient(t, tc.Bob, tc.BobClientType, WithPersistentStorage()) // no need to login as we have an account in storage already
		// this is time-sensitive: start waiting for waitForRoomKey BEFORE we call MustStartSyncing
		// which itself needs to be in a separate goroutine.
		browserIsClosed := helpers.NewWaiter()
		go func() {
			waitForRoomKey.Wait(t, 10*time.Second)
			t.Logf("killing bob as room key event received")
			bob.Close(t) // close the browser
			browserIsClosed.Finish()
		}()
		time.Sleep(100 * time.Millisecond)
		go func() { // in a goroutine so we don't need this to return before closing the browser
			t.Logf("bob starting to sync, expecting to be killed..")
			bob.StartSyncing(t)
		}()

		browserIsClosed.Wait(t, 10*time.Second)

		// Ensure Bob can decrypt new messages sent from Alice.
		bob = tc.MustLoginClient(t, tc.Bob, tc.BobClientType, WithPersistentStorage())
		defer bob.Close(t)
		bobStopSyncing := bob.MustStartSyncing(t)
		defer bobStopSyncing()
		// include a grace period like rust, no specific reason beyond consistency.
		time.Sleep(time.Second)
		ev := bob.MustGetEvent(t, roomID, eventID)
		must.Equal(t, ev.FailedToDecrypt, false, "unable to decrypt message")
		must.Equal(t, ev.Text, "Kick to make a new room key!", "event text mismatch")
	})
}
