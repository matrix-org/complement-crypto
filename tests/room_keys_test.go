package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

func sniffToDeviceEvent(t *testing.T, d complement.Deployment, ch chan deploy.CallbackData) (callbackURL string, close func()) {
	callbackURL, close = deploy.NewCallbackServer(t, d.GetConfig().HostnameRunningComplement, func(cd deploy.CallbackData) {
		if cd.Method == "OPTIONS" {
			return // ignore CORS
		}
		if strings.Contains(cd.URL, "m.room.encrypted") {
			// we can't decrypt this, but we know that this should most likely be the m.room_key to-device event.
			ch <- cd
		}
	})
	return callbackURL, close
}

// This test ensures we change the m.room_key when a device leaves an E2EE room.
// If the key is not changed, the left device could potentially decrypt the encrypted
// event if they could get access to it.
func TestRoomKeyIsCycledOnDeviceLogout(t *testing.T) {
	Instance().ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := Instance().CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(
			t,
			tc.Alice,
			cc.EncRoomOptions.PresetTrustedPrivateChat(),
			cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
		)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		// Alice, Alice2 and Bob are in a room.
		csapiAlice2 := tc.MustRegisterNewDevice(t, tc.Alice, "OTHER_DEVICE")
		alice2 := tc.MustLoginClient(t, &cc.ClientCreationRequest{
			User: csapiAlice2,
		})
		defer alice2.Close(t)
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			alice2StopSyncing := alice2.MustStartSyncing(t)
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 5*time.Second, "alice did not see own join")
			// check the room works
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			waiter2 := alice2.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.SendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			waiter2.Waitf(t, 5*time.Second, "alice2 did not see alice's message")

			// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
			ch := make(chan deploy.CallbackData, 10)
			callbackURL, close := sniffToDeviceEvent(t, tc.Deployment, ch)
			defer close()

			alice2StopSyncing()
			// we don't know when the new room key will be sent, it could be sent as soon as the device list update
			// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
			// traffic now.
			tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*\\/sendToDevice.*",
				},
			}, func() {
				// now alice2 is going to logout, causing her user ID to appear in device_lists.changed which
				// should cause a /keys/query request, resulting in the client realising the device is gone,
				// which should trigger a new room key to be sent (on message send)
				csapiAlice2.MustDo(t, "POST", []string{"_matrix", "client", "v3", "logout"}, client.WithJSONBody(t, map[string]any{}))

				// we don't know how long it will take for the device list update to be processed, so wait 1s
				time.Sleep(time.Second)

				// now send another message from Alice, who should negotiate a new room key
				wantMsgBody = "Another Test Message"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's new message")
			})

			// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
			// the room key.
			select {
			case <-ch:
			default:
				ct.Fatalf(t, "did not see /sendToDevice when logging out and sending a new message")
			}
		})
	})
}

// The room key is cycled when `rotation_period_msgs` is met (default: 100).
//
// This test ensures we change the m.room_key when we have sent enough messages,
// where "enough" means the value set in the `m.room.encryption` event under the
// `rotation_period_msgs` property.
//
// If the key were not changed, someone who stole the key would have access to
// future messages.
//
// See https://gitlab.matrix.org/matrix-org/olm/blob/master/docs/megolm.md#lack-of-backward-secrecy
func TestRoomKeyIsCycledAfterEnoughMessages(t *testing.T) {
	Instance().ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Given a room containing Alice and Bob, where we rotate keys every 5 messages
		tc := Instance().CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(
			t,
			tc.Alice,
			cc.EncRoomOptions.PresetTrustedPrivateChat(),
			cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
			cc.EncRoomOptions.RotationPeriodMsgs(5),
		)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			// And some messages were sent, but not enough to trigger resending
			for i := 0; i < 4; i++ {
				wantMsgBody := fmt.Sprintf("Before we hit the threshold %d", i)
				waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)
			}

			// Sniff calls to /sendToDevice to ensure we see the new room key being sent.
			ch := make(chan deploy.CallbackData, 10)
			callbackURL, close := sniffToDeviceEvent(t, tc.Deployment, ch)
			defer close()
			tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*\\/sendToDevice.*",
				},
			}, func() {
				// When we send two messages (one to hit the threshold and one to pass it)
				//
				// Note that we deliberately cover two possible valid behaviours
				// of the client here. It's valid for the client to cycle the key:
				// - eagerly as soon as the threshold is reached, or
				// - lazily on the next message that would take the count above the threshold
				// By sending two messages, we ensure that clients using either
				// of these approaches will pass the test.
				wantMsgBody := "This one hits the threshold"
				waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)

				wantMsgBody = "After the threshold"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)
			})

			// Then we did send out new keys
			select {
			case <-ch:
				// Success - keys were sent
			default:
				ct.Fatalf(t, "did not see /sendToDevice after sending rotation_period_msgs messages")
			}
		})
	})
}

// The room key is cycled when `rotation_period_ms` is exceeded (default: 1 week).
//
// This test ensures we change the m.room_key when enough time has passed,
// where "enough" means the number of milliseconds set in the
// `m.room.encryption` event under the `rotation_period_ms` property.
//
// If the key were not changed, someone who stole the key would have access to
// future messages.
//
// See https://gitlab.matrix.org/matrix-org/olm/blob/master/docs/megolm.md#lack-of-backward-secrecy
func TestRoomKeyIsCycledAfterEnoughTime(t *testing.T) {
	// if this is too high, the test takes needlessly long to complete.
	// if this is too low, it can cause flakey test failures as various assertions in rust SDK
	// around expired sessions fail.
	rotationPeriod := 3 * time.Second
	Instance().ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Disable this test if the sender is on JS.
		// We require a custom Rust build to enable the hidden feature flag
		// `_disable-minimum-rotation-period-ms`, so that we can set the
		// rotation period to a small value. We don't control the version of
		// rust-sdk that is built into the JS, so we can't enable this flag.
		// (For the Rust side, we modify Cargo.toml within `rebuild_js_sdk.sh`.)
		if clientTypeA.Lang == api.ClientTypeJS {
			t.Skipf("Skipping on JS since we require a custom Rust build to allow small rotation_period_ms")
			return
		}

		// Given a room containing Alice and Bob, where we rotate keys every second
		tc := Instance().CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(
			t,
			tc.Alice,
			cc.EncRoomOptions.PresetTrustedPrivateChat(),
			cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
			cc.EncRoomOptions.RotationPeriodMs(int(rotationPeriod.Milliseconds())),
		)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			// Before we start, ensure some keys have already been sent, so we
			// don't get a false positive.
			wantMsgBody := "Before we start"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.SendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "Did not see 'before we start' event in the room")

			// Sniff calls to /sendToDevice to ensure we see the new room key being sent.
			ch := make(chan deploy.CallbackData, 10)
			callbackURL, close := sniffToDeviceEvent(t, tc.Deployment, ch)
			defer close()
			tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*\\/sendToDevice.*",
				},
			}, func() {
				// Send a message to ensure the room is working, and any timer is set up
				wantMsgBody := "Before the time expires"
				waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "Did not see 'before the time expires' event in the room")

				// When we wait 1+period seconds
				time.Sleep(rotationPeriod + time.Second)

				// And send another message
				wantMsgBody = "After the time expires"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "Did not see 'after the time expires' event in the room")
			})

			// Then we sent out new keys because the rotation timer had expired
			select {
			case <-ch:
				// Success - keys were sent
			default:
				ct.Fatalf(t, "did not see /sendToDevice after waiting rotation_period_ms milliseconds")
			}
		})
	})
}

func TestRoomKeyIsCycledOnMemberLeaving(t *testing.T) {
	Instance().ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := Instance().CreateTestContext(t, clientTypeA, clientTypeB, clientTypeB)
		// Alice, Bob and Charlie are in a room.
		tc.WithAliceBobAndCharlieSyncing(t, func(alice, bob, charlie api.Client) {
			// do setup code after all clients are syncing to ensure that if Alice asks for Charlie's keys on receipt of the
			// join event, then Charlie has already uploaded keys.
			roomID := tc.CreateNewEncryptedRoom(
				t,
				tc.Alice,
				cc.EncRoomOptions.PresetTrustedPrivateChat(),
				cc.EncRoomOptions.Invite([]string{tc.Bob.UserID, tc.Charlie.UserID}),
			)
			tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
			tc.Charlie.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Charlie.UserID, "join")).Waitf(t, 5*time.Second, "alice did not see charlie's join")
			// check the room works
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			waiter2 := charlie.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.SendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			waiter2.Waitf(t, 5*time.Second, "charlie did not see alice's message")

			// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
			ch := make(chan deploy.CallbackData, 10)
			callbackURL, close := sniffToDeviceEvent(t, tc.Deployment, ch)
			defer close()

			// we don't know when the new room key will be sent, it could be sent as soon as the device list update
			// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
			// traffic now.
			tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*\\/sendToDevice.*",
				},
			}, func() {
				// now Charlie is going to leave the room, causing her user ID to appear in device_lists.left
				// which should trigger a new room key to be sent (on message send)
				tc.Charlie.MustDo(t, "POST", []string{"_matrix", "client", "v3", "rooms", roomID, "leave"}, client.WithJSONBody(t, map[string]any{}))

				// we don't know how long it will take for the device list update to be processed, so wait 1s
				time.Sleep(time.Second)

				// now send another message from Alice, who should negotiate a new room key
				wantMsgBody = "Another Test Message"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			})

			// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
			// the room key.
			select {
			case <-ch:
			default:
				ct.Fatalf(t, "did not see /sendToDevice when logging out and sending a new message")
			}
		})
	})
}

func TestRoomKeyIsNotCycled(t *testing.T) {
	Instance().ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := Instance().CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(
			t,
			tc.Alice,
			cc.EncRoomOptions.PresetTrustedPrivateChat(),
			cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
		)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		// Alice, Bob are in a room.
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			// check the room works
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.SendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

			// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
			ch := make(chan deploy.CallbackData, 10)
			callbackURL, closeCallbackServer := sniffToDeviceEvent(t, tc.Deployment, ch)
			defer closeCallbackServer()

			t.Run("on display name change", func(t *testing.T) {
				// we don't know when the new room key will be sent, it could be sent as soon as the device list update
				// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
				// traffic now.
				tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
					"callback": map[string]interface{}{
						"callback_url": callbackURL,
						"filter":       "~u .*\\/sendToDevice.*",
					},
				}, func() {
					// now Bob is going to change their display name
					// which should NOT trigger a new room key to be sent (on message send)
					tc.Bob.MustDo(t, "PUT", []string{"_matrix", "client", "v3", "profile", tc.Bob.UserID, "displayname"}, client.WithJSONBody(t, map[string]any{
						"displayname": "Little Bobby Tables",
					}))

					// we don't know how long it will take for the device list update to be processed, so wait 1s
					time.Sleep(time.Second)

					// now send another message from Alice, who should negotiate a new room key
					wantMsgBody = "Another Test Message"
					waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
					alice.SendMessage(t, roomID, wantMsgBody)
					waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
				})

				// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
				// the room key.
				select {
				case <-ch:
					ct.Fatalf(t, "saw /sendToDevice when changing display name and sending a new message")
				default:
				}
			})
			t.Run("on new device login", func(t *testing.T) {
				if clientTypeA.HS == "hs2" || clientTypeB.HS == "hs2" {
					// we sniff /sendToDevice and assume that the access_token is for HS1.
					t.Skipf("federation unsupported for this test")
				}
				// we don't know when the new room key will be sent, it could be sent as soon as the device list update
				// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
				// traffic now.
				tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
					"callback": map[string]interface{}{
						"callback_url": callbackURL,
						"filter":       "~u .*\\/sendToDevice.*",
					},
				}, func() {
					// now Bob is going to login on a new device
					// which should NOT trigger a new room key to be sent (on message send)
					csapiBob2 := tc.MustRegisterNewDevice(t, tc.Bob, "OTHER_DEVICE")
					bob2 := tc.MustLoginClient(t, &cc.ClientCreationRequest{
						User: csapiBob2,
					})
					defer bob2.Close(t)
					bob2StopSyncing := bob2.MustStartSyncing(t)
					defer bob2StopSyncing()

					// we don't know how long it will take for the device list update to be processed, so wait 1s
					time.Sleep(time.Second)

					// now send another message from Alice, who should negotiate a new room key
					wantMsgBody = "Yet Another Test Message"
					waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
					alice.SendMessage(t, roomID, wantMsgBody)
					waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
				})

				// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
				// the room key.

			Consume:
				for { // consume all items in the channel
					// the logic here is a bit weird because we DO expect some /sendToDevice calls as Alice and Bob
					// share the room key with Bob2. However, Alice, who is sending the message, should NOT be sending
					// to-device msgs to Bob, as that would indicate a new exchange of room keys. To do this, we use
					// the access token to see who the sender is, and check the request body to see who the receiver is,
					// and make sure it's all what we expect.
					select {
					case sendToDevice := <-ch:
						cli := tc.Deployment.Deployment.UnauthenticatedClient(t, "hs1")
						cli.AccessToken = sendToDevice.AccessToken
						whoami := cli.MustDo(t, "GET", []string{"_matrix", "client", "v3", "account", "whoami"})
						sender := must.ParseJSON(t, whoami.Body).Get("user_id").Str
						reqBody := struct {
							Messages map[string]map[string]any
						}{}
						must.NotError(t, "failed to unmarshal intercepted request body", json.Unmarshal(sendToDevice.RequestBody, &reqBody))

						for target := range reqBody.Messages {
							for targetDeviceID := range reqBody.Messages[target] {
								t.Logf("%s /sendToDevice to %v (%v)", sender, target, targetDeviceID)
								if sender == alice.UserID() && target == bob.UserID() && targetDeviceID != "OTHER_DEVICE" {
									ct.Fatalf(t, "saw Alice /sendToDevice to Bob for old device, implying room keys were refreshed")
								}
							}
						}

					default:
						break Consume
					}
				}
			})
		})
	})
}

// Test that the m.room_key is NOT cycled when the client is restarted, but there is no change in devices
// in the room. This is important to ensure that we don't cycle m.room_keys too frequently, which increases
// the chances of seeing undecryptable events.
func TestRoomKeyIsNotCycledOnClientRestart(t *testing.T) {
	Instance().ForEachClientType(t, func(t *testing.T, a api.ClientType) {
		switch a.Lang {
		case api.ClientTypeRust:
			testRoomKeyIsNotCycledOnClientRestartRust(t, a)
		case api.ClientTypeJS:
			testRoomKeyIsNotCycledOnClientRestartJS(t, a)
		default:
			t.Fatalf("unknown lang: %s", a.Lang)
		}
	})
}

func testRoomKeyIsNotCycledOnClientRestartRust(t *testing.T, clientType api.ClientType) {
	tc := Instance().CreateTestContext(t, clientType, clientType)
	roomID := tc.CreateNewEncryptedRoom(
		t,
		tc.Alice,
		cc.EncRoomOptions.PresetTrustedPrivateChat(),
		cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
	)
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})

	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.Client) {
		wantMsgBody := "test from another process"
		// send a message as Alice in a different process
		tc.WithClientSyncing(t, &cc.ClientCreationRequest{
			User: tc.Alice,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
			},
			Multiprocess: true,
		}, func(remoteAlice api.Client) {
			eventID := remoteAlice.SendMessage(t, roomID, wantMsgBody)
			waiter := remoteAlice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID))
			waiter.Waitf(t, 5*time.Second, "client did not see event %s", eventID)
		})

		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		waiter.Waitf(t, 8*time.Second, "bob did not see alice's message")

		// Now recreate the same client and make sure we don't send new room keys.

		// we're going to sniff calls to /sendToDevice to ensure we do NOT see a new room key being sent.
		ch := make(chan deploy.CallbackData, 10)
		callbackURL, close := sniffToDeviceEvent(t, tc.Deployment, ch)
		defer close()

		tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
			"callback": map[string]interface{}{
				"callback_url": callbackURL,
				"filter":       "~u .*\\/sendToDevice.*",
			},
		}, func() {
			// login as alice
			alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
				User: tc.Alice,
				Opts: api.ClientCreationOpts{
					PersistentStorage: true,
				},
			})
			defer alice.Close(t)
			aliceStopSyncing := alice.MustStartSyncing(t)
			defer aliceStopSyncing()

			// we don't know how long it will take for the device list update to be processed, so wait 1s
			time.Sleep(time.Second)

			// now send another message from Alice, who should NOT negotiate a new room key
			wantMsgBody = "Another Test Message"
			waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.SendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
		})

		// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
		// the room key.
		select {
		case <-ch:
			ct.Fatalf(t, "saw /sendToDevice when restarting the client and sending a new message")
		default:
		}
	})
}

func testRoomKeyIsNotCycledOnClientRestartJS(t *testing.T, clientType api.ClientType) {
	tc := Instance().CreateTestContext(t, clientType, clientType)
	roomID := tc.CreateNewEncryptedRoom(
		t,
		tc.Alice,
		cc.EncRoomOptions.PresetTrustedPrivateChat(),
		cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
	)
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})

	// Alice and Bob are in a room.
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
		},
	})
	aliceStopSyncing := alice.MustStartSyncing(t)
	// no alice.close here as we'll close it in the test mid-way
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.Client) {
		// check the room works
		wantMsgBody := "Test Message"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.SendMessage(t, roomID, wantMsgBody)
		waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

		// we're going to sniff calls to /sendToDevice to ensure we do NOT see a new room key being sent.
		ch := make(chan deploy.CallbackData, 10)
		callbackURL, close := sniffToDeviceEvent(t, tc.Deployment, ch)
		defer close()

		// we want to start sniffing for the to-device event just before we restart the client.
		tc.Deployment.MITM().WithMITMOptions(t, map[string]interface{}{
			"callback": map[string]interface{}{
				"callback_url": callbackURL,
				"filter":       "~u .*\\/sendToDevice.*",
			},
		}, func() {
			// now alice is going to restart her client
			aliceStopSyncing()
			alice.Close(t)

			tc.WithClientSyncing(t, &cc.ClientCreationRequest{
				User: tc.Alice,
				Opts: alice.Opts(),
			}, func(alice api.Client) {
				// now send another message from Alice, who should NOT send another new room key
				wantMsgBody = "Another Test Message"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.SendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			})
		})

		// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
		// the room key.
		select {
		case <-ch:
			ct.Fatalf(t, "saw /sendToDevice when restarting the client and sending a new message")
		default:
		}
	})
}
