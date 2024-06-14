package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
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
			waiter.Waitf(t, 5*time.Second, "bob did not see event with body '%s'", wantMsgBody)
		})
	})
}

// Regression test for https://github.com/vector-im/element-web/issues/23113
// "If you restart (e.g. upgrade) Element while it's waiting to process a m.room_key, it'll drop it and you'll get UISIs"
//
// - Alice and Bob are in an encrypted room with rotation period msgs = 1
// - Bob's client is shut down temporarily.
// - Alice sends a message, this will cause a new room key to be sent.
// - Start sniffing /sync traffic. Bob's client comes back.
// - When /sync shows a to-device message from Alice (indicating the room key), sleep(1ms) then SIGKILL Bob.
// - Restart Bob's client.
// - Ensure Bob can decrypt new messages sent from Alice.
func TestUnprocessedToDeviceMessagesArentLostOnRestart(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		// prepare for the test: register all 3 clients and create the room
		tc := CreateTestContext(t, clientType, clientType)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice,
			EncRoomOptions.Invite([]string{tc.Bob.UserID}), EncRoomOptions.RotationPeriodMsgs(1),
		)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
		// the initial setup for rust/js is the same.
		// login bob first so we have OTKs
		bob := tc.MustLoginClient(t, tc.Bob, tc.BobClientType, WithPersistentStorage())
		tc.WithAliceSyncing(t, func(alice api.Client) {
			// we will close this in the test, no defer
			bobStopSyncing := bob.MustStartSyncing(t)
			// check the room works
			alice.SendMessage(t, roomID, "Hello World!")
			bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("Hello World!")).Waitf(t, 2*time.Second, "bob did not see event with body 'Hello World!'")
			// stop bob's client, but grab the access token first so we can re-use it
			bobOpts := bob.Opts()
			bobStopSyncing()
			bob.Logf(t, "Bob is about to be Closed()")
			bob.Close(t)

			// send a lot of to-device messages to bob to increase the window in which to SIGKILL the client.
			// It's unimportant what these are.
			for i := 0; i < 60; i++ {
				tc.Alice.MustSendToDeviceMessages(t, "m.room_key_request", map[string]map[string]map[string]interface{}{
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

			// send a message as alice to make a new room key
			eventID := alice.SendMessage(t, roomID, "Kick to make a new room key!")

			// client specific impls to handle restarts.
			switch clientType.Lang {
			case api.ClientTypeRust:
				testUnprocessedToDeviceMessagesArentLostOnRestartRust(t, tc, bobOpts, roomID, eventID)
			case api.ClientTypeJS:
				testUnprocessedToDeviceMessagesArentLostOnRestartJS(t, tc, bobOpts, roomID, eventID)
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
		t.Logf("/sync => %v", string(cd.ResponseBody))
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
		// No need to login as we will reuse the session from before.
		// This is critical to ensure we get the room key update as it would have been sent
		// to bob's logged in device, not any new logins.
		remoteClient := tc.MustCreateMultiprocessClient(t, api.ClientTypeRust, bobOpts)

		// start syncing but don't wait, we wait for the to device event
		go func() {
			// we purposefully ignore the error here because we expect the RPC client
			// to return an error when the RPC server is sigkilled.
			remoteClient.StartSyncing(t)
		}()

		waitForRoomKey.Waitf(t, 10*time.Second, "did not see room key")
		t.Logf("killing remote bob client")
		remoteClient.ForceClose(t)

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

// Regression test for https://github.com/element-hq/element-web/issues/24680
//
// It's important that room keys are sent out ASAP, else the encrypted event may arrive
// before the keys, causing a temporary unable-to-decrypt error. Clients SHOULD be batching
// to-device messages, but old implementations batched too low (20 messages per request).
// This test asserts we batch at least 100 per request.
//
// It does this by creating an E2EE room with 100 E2EE users, and forces a key rotation
// by sending a message with rotation_period_msgs=1. It does not ensure that the room key
// is correctly sent to all 100 users as that would entail having 100 users running at
// the same time (think 100 browsers = expensive). Instead, we sequentially spin up 100
// clients and then close them before doing the test, and assert we send 100 events.
//
// In the future, it may be difficult to run this test for 1 user with 100 devices due to
// HS limits on the number of devices and forced cross-signing.
func TestToDeviceMessagesAreBatched(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := CreateTestContext(t, clientType)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, EncRoomOptions.RotationPeriodMsgs(1), EncRoomOptions.PresetPublicChat())
		// create 100 users
		for i := 0; i < 100; i++ {
			cli := tc.Deployment.Register(t, clientType.HS, helpers.RegistrationOpts{
				LocalpartSuffix: fmt.Sprintf("bob-%d", i),
				Password:        "complement-crypto-password",
			})
			cli.MustJoinRoom(t, roomID, []string{clientType.HS})
			// this blocks until it has uploaded OTKs/device keys
			clientUnderTest := tc.MustLoginClient(t, cli, tc.AliceClientType)
			clientUnderTest.Close(t)
		}
		waiter := helpers.NewWaiter()
		tc.WithAliceSyncing(t, func(alice api.Client) {
			// intercept /sendToDevice and check we are sending 100 messages per request
			tc.Deployment.WithSniffedEndpoint(t, "/sendToDevice", func(cd deploy.CallbackData) {
				if cd.Method != "PUT" {
					return
				}
				// format is:
				/*
					{
					  "messages": {
					    "@alice:example.com": {
					      "TLLBEANAAG": {
					        "example_content_key": "value"
					      }
					    }
					  }
					}
				*/
				usersMap := gjson.GetBytes(cd.RequestBody, "messages")
				if !usersMap.Exists() {
					t.Logf("intercepted PUT /sendToDevice but no messages existed")
					return
				}
				if len(usersMap.Map()) != 100 {
					t.Errorf("PUT /sendToDevice did not batch messages, got %d want 100", len(usersMap.Map()))
					t.Logf(usersMap.Raw)
				}
				waiter.Finish()
			}, func() {
				alice.SendMessage(t, roomID, "this should cause to-device msgs to be sent")
				time.Sleep(time.Second)
				waiter.Waitf(t, 5*time.Second, "did not see /sendToDevice")
			})
		})

	})
}

// Regression test for https://github.com/element-hq/element-web/issues/24682
//
// When a to-device msg is received, the SDK may need to check that the device belongs
// to the user in question. To do this, it needs an up-to-date device list. To get this,
// it does a /keys/query request. If this request fails, the entire processing of the
// to-device msg could fail, dropping the msg and the room key it contains.
//
// This test reproduces this by having an existing E2EE room between Alice and Bob, then:
//   - Block /keys/query requests.
//   - Alice logs in on a new device.
//   - Alice sends a message on the new device.
//   - Bob should get that message but may refuse to decrypt it as it cannot verify that the sender_key
//     belongs to Alice.
//   - Unblock /keys/query requests.
//   - Bob should eventually retry and be able to decrypt the event.
func TestToDeviceMessagesArentLostWhenKeysQueryFails(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := CreateTestContext(t, clientType, clientType)
		// get a normal E2EE room set up
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, EncRoomOptions.Invite([]string{tc.Bob.UserID}))
		tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			msg := "hello world"
			msg2 := "new device message from alice"
			alice.SendMessage(t, roomID, msg)
			bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "bob failed to see message from alice")
			// Block /keys/query requests
			waiter := helpers.NewWaiter()
			callbackURL, closeCallbackServer := deploy.NewCallbackServer(t, tc.Deployment, func(cd deploy.CallbackData) {
				t.Logf("%+v", cd)
				waiter.Finish()
			})
			defer closeCallbackServer()
			var eventID string
			bobAccessToken := bob.CurrentAccessToken(t)
			t.Logf("Bob's token => %s", bobAccessToken)
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"statuscode": map[string]interface{}{
					"return_status": http.StatusGatewayTimeout,
					"block_request": true,
					"count":         3,
					"filter":        "~u .*/keys/query.* ~hq " + bobAccessToken,
				},
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*/keys/query.*",
				},
			}, func() {
				// Alice logs in on a new device.
				csapiAlice2 := tc.MustRegisterNewDevice(t, tc.Alice, clientType.HS, "OTHER_DEVICE")
				alice2 := tc.MustLoginClient(t, csapiAlice2, clientType)
				defer alice2.Close(t)
				alice2StopSyncing := alice2.MustStartSyncing(t)
				defer alice2StopSyncing()
				// we don't know how long it will take for the device list update to be processed, so wait 1s
				time.Sleep(time.Second)

				// Alice sends a message on the new device.
				eventID = alice2.SendMessage(t, roomID, msg2)

				waiter.Waitf(t, 3*time.Second, "did not see /keys/query")
				time.Sleep(3 * time.Second) // let Bob retry /keys/query
			})
			// now we aren't blocking /keys/query anymore.
			// Bob should be able to decrypt this message.
			ev := bob.MustGetEvent(t, roomID, eventID)
			must.Equal(t, ev.Text, msg2, "bob failed to decrypt "+eventID)
		})

	})
}

// Regression test for https://github.com/element-hq/element-web/issues/25723
//
// This test doesn't ensure that the messages are processed in-order (as we cannot
// introspect that in a platform agnostic way) but it does cause 100s of to-device
// messages to be sent to the client in one go. If clients process these 100s of
// messages out of order, it will cause decryption errors, hence it serves as a
// canary that something is wrong.
//
// This test does this by:
//   - Alice in a public encrypted room on her own with rotation_period_msgs set to 1.
//   - Block Alice's /sync
//   - Create 4 new users and join them to the encrypted room.
//   - Send 30 messages as each user.
//   - This will cause 40x3=120 to-device messages due to the low rotation period msgs value.
//   - Unblock Alice's /sync
//   - Ensure Alice can decrypt every single event.
//
// Both Sliding Sync and Sync v2 return to-device msgs in batches of 100, so going much above
// 100 here isn't going to do much. We do a good chunk above it (120) just in case the client
// is /syncing before processing the last response, but we also don't want to send too much
// data as it makes this test take a long time to complete.
//
// This is quite a complex stress test so it's possible for this test to fail for reasons
// unrelated to processing out-of-order e.g it will cause fallback keys for alice to be used.
func TestToDeviceMessagesAreProcessedInOrder(t *testing.T) {
	numClients := 4
	numMsgsPerClient := 30
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		if clientType.Lang == api.ClientTypeRust {
			t.Skipf("flakey")
		}
		tc := CreateTestContext(t, clientType)
		roomID := tc.CreateNewEncryptedRoom(
			t, tc.Alice, EncRoomOptions.RotationPeriodMsgs(1), EncRoomOptions.PresetPublicChat(),
		)
		// intercept /sync just so we can observe the number of to-device msgs coming down.
		// We also synchronise on this to know when the client has received the to-device msgs
		callbackURL, close := deploy.NewCallbackServer(t, tc.Deployment, func(cd deploy.CallbackData) {
			// try v2 sync then SS
			toDeviceEvents := gjson.ParseBytes(cd.ResponseBody).Get("to_device.events").Array()
			if len(toDeviceEvents) == 0 {
				toDeviceEvents = gjson.ParseBytes(cd.ResponseBody).Get("extensions.to_device.events").Array()
			}
			if len(toDeviceEvents) > 0 {
				t.Logf("sniffed %d to_device events down /sync", len(toDeviceEvents))
			}
		})
		defer close()
		var timelineEvents = []struct {
			ID   string
			Body string
		}{}
		tc.WithAliceSyncing(t, func(alice api.Client) {
			// Block Alice's /sync
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"statuscode": map[string]interface{}{
					"return_status": http.StatusGatewayTimeout,
					"block_request": true,
					"filter":        "~u .*/sync.* ~hq " + alice.CurrentAccessToken(t),
				},
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*/sync.* ~hq " + alice.CurrentAccessToken(t),
				},
			}, func() {
				// create 10 users and join the room
				baseClients := make([]BaseClient, numClients)
				for i := range baseClients {
					baseClients[i] = BaseClient{
						CSAPI: tc.Deployment.Register(t, clientType.HS, helpers.RegistrationOpts{
							LocalpartSuffix: "ilikebots",
							Password:        "complement-crypto-password",
						}),
						ClientType: clientType,
					}
					baseClients[i].MustJoinRoom(t, roomID, []string{clientType.HS})
				}
				// send 30 messages as each user (interleaved)
				tc.WithClientsSyncing(t, baseClients, func(clients []api.Client) {
					for i := 0; i < numMsgsPerClient; i++ {
						for _, c := range clients {
							body := fmt.Sprintf("Message %d", i+1)
							eventID := c.SendMessage(t, roomID, body)
							timelineEvents = append(timelineEvents, struct {
								ID   string
								Body string
							}{
								ID:   eventID,
								Body: body,
							})
						}
					}
				})
				t.Logf("sent %d timeline events", len(timelineEvents))
			})
			// Alice's /sync is unblocked, wait until we see the last event.
			// Re-add the callback server TODO: allow composing see https://github.com/matrix-org/complement-crypto/issues/68
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*/sync.* ~hq " + alice.CurrentAccessToken(t),
				},
			}, func() {
				lastTimelineEvent := timelineEvents[len(timelineEvents)-1]
				alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(lastTimelineEvent.ID)).Waitf(
					// wait a while here as we need to wait for both /sync to retry and a large response
					// to be processed.
					t, 20*time.Second, "did not see latest timeline event %s", lastTimelineEvent.ID,
				)
				// now verify we can decrypt all the events
				time.Sleep(10 * time.Second)
				// backpaginate 10 times. We don't do a single huge backpagination call because
				// this can cause failures on JS "Promise was collected".
				for i := 0; i < 10; i++ {
					alice.MustBackpaginate(t, roomID, len(timelineEvents)/10)
				}
				for i := len(timelineEvents) - 1; i >= 0; i-- {
					nextTimelineEvent := timelineEvents[i]
					ev := alice.MustGetEvent(t, roomID, nextTimelineEvent.ID)
					must.Equal(t, ev.FailedToDecrypt, false, "failed to decrypt event ID "+nextTimelineEvent.ID)
					must.Equal(t, ev.Text, nextTimelineEvent.Body, "failed to decrypt body of event "+nextTimelineEvent.ID)
				}
			})
		})
	})
}
