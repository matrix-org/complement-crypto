package tests

import (
	"encoding/json"
	"fmt"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement-crypto/internal/deploy/mitm"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func sniffToDeviceEvent(t *testing.T, tc *cc.TestContext, inner func(pc *callback.PassiveChannel)) {
	passiveChannel := callback.NewPassiveChannel(5*time.Second, false)
	tc.Deployment.MITM().Configure(t).WithIntercept(mitm.InterceptOpts{
		Filter: mitm.FilterParams{
			PathContains: "/sendToDevice",
			Method:       "PUT",
		},
		ResponseCallback: func(cd callback.Data) *callback.Response {
			// only invoke the callback for encrypted to-device events
			// we can't decrypt this, but we know that this should most likely be the m.room_key to-device event.
			if strings.Contains(cd.URL, "m.room.encrypted") {
				return passiveChannel.Callback()(cd)
			}
			return nil
		},
	}, func() {
		inner(passiveChannel)
		passiveChannel.Close()
	})
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
		tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientTypeA.HS})

		// Alice, Alice2 and Bob are in a room.
		csapiAlice2 := tc.MustRegisterNewDevice(t, tc.Alice, "OTHER_DEVICE")
		alice2 := tc.MustLoginClient(t, &cc.ClientCreationRequest{
			User: csapiAlice2,
		})
		defer alice2.Close(t)
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.TestClient) {
			alice2StopSyncing := alice2.MustStartSyncing(t)
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 5*time.Second, "alice did not see own join")
			// check the room works
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			waiter2 := alice2.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.MustSendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			waiter2.Waitf(t, 5*time.Second, "alice2 did not see alice's message")
			alice2StopSyncing()

			// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
			sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
				// now alice2 is going to logout, causing her user ID to appear in device_lists.changed which
				// should cause a /keys/query request, resulting in the client realising the device is gone,
				// which should trigger a new room key to be sent (on message send)
				csapiAlice2.MustDo(t, "POST", []string{"_matrix", "client", "v3", "logout"}, client.WithJSONBody(t, map[string]any{}))

				// we don't know how long it will take for the device list update to be processed, so wait 1s
				time.Sleep(time.Second)

				// now send another message from Alice, who should negotiate a new room key
				wantMsgBody = "Another Test Message"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's new message")

				// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
				// the room key. We can't actually inspect the event itself, so just the fact we see the
				// encrypted to-device event is enough. Recv will timeout if we don't see it.
				pc.Recv(t, "did not see /sendToDevice event")
			})
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
		tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientTypeA.HS})

		tc.WithAliceAndBobSyncing(t, func(alice, bob api.TestClient) {
			// And some messages were sent, but not enough to trigger resending
			for i := 0; i < 4; i++ {
				wantMsgBody := fmt.Sprintf("Before we hit the threshold %d", i)
				waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)
			}

			// Sniff calls to /sendToDevice to ensure we see the new room key being sent.
			sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
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
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)

				wantMsgBody = "After the threshold"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)

				// Then we did send out new keys
				pc.Recv(t, "did not see /sendToDevice after sending rotation_period_msgs messages")
			})
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
		tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientTypeA.HS})

		tc.WithAliceAndBobSyncing(t, func(alice, bob api.TestClient) {
			// Before we start, ensure some keys have already been sent, so we
			// don't get a false positive.
			wantMsgBody := "Before we start"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.MustSendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "Did not see 'before we start' event in the room")

			// Sniff calls to /sendToDevice to ensure we see the new room key being sent.
			sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
				// Send a message to ensure the room is working, and any timer is set up
				wantMsgBody := "Before the time expires"
				waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "Did not see 'before the time expires' event in the room")

				// When we wait 1+period seconds
				time.Sleep(rotationPeriod + time.Second)

				// And send another message
				wantMsgBody = "After the time expires"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "Did not see 'after the time expires' event in the room")

				pc.Recv(t, "did not see /sendToDevice after waiting rotation_period_ms milliseconds")
			})
		})
	})
}

func TestRoomKeyIsCycledOnMemberLeaving(t *testing.T) {
	Instance().ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := Instance().CreateTestContext(t, clientTypeA, clientTypeB, clientTypeB)
		// Alice, Bob and Charlie are in a room.
		tc.WithAliceBobAndCharlieSyncing(t, func(alice, bob, charlie api.TestClient) {
			// do setup code after all clients are syncing to ensure that if Alice asks for Charlie's keys on receipt of the
			// join event, then Charlie has already uploaded keys.
			roomID := tc.CreateNewEncryptedRoom(
				t,
				tc.Alice,
				cc.EncRoomOptions.PresetTrustedPrivateChat(),
				cc.EncRoomOptions.Invite([]string{tc.Bob.UserID, tc.Charlie.UserID}),
			)
			tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientTypeA.HS})
			tc.Charlie.MustJoinRoom(t, roomID, []spec.ServerName{clientTypeA.HS})
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Charlie.UserID, "join")).Waitf(t, 5*time.Second, "alice did not see charlie's join")
			// check the room works
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			waiter2 := charlie.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.MustSendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			waiter2.Waitf(t, 5*time.Second, "charlie did not see alice's message")

			// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
			sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
				// now Charlie is going to leave the room, causing his user ID to appear in device_lists.left
				// which should trigger a new room key to be sent (on message send)
				tc.Charlie.MustDo(t, "POST", []string{"_matrix", "client", "v3", "rooms", roomID, "leave"}, client.WithJSONBody(t, map[string]any{}))

				// we don't know how long it will take for the device list update to be processed, so wait 1s
				time.Sleep(time.Second)

				// now send another message from Alice, who should negotiate a new room key
				wantMsgBody = "Another Test Message"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

				// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
				// the room key.
				pc.Recv(t, "did not see /sendToDevice when logging out and sending a new message")
			})
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
		tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientTypeA.HS})

		// Alice, Bob are in a room.
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.TestClient) {
			// check the room works
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.MustSendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			t.Run("on display name change", func(t *testing.T) {
				// we don't know when the new room key will be sent, it could be sent as soon as the device list update
				// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
				// traffic now.
				sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
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
					alice.MustSendMessage(t, roomID, wantMsgBody)
					waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

					got := pc.TryRecv(t)
					if got != nil {
						ct.Fatalf(t, "saw /sendToDevice when changing display name and sending a new message")
					}
				})
			})
			t.Run("on new device login", func(t *testing.T) {
				if clientTypeA.HS == "hs2" || clientTypeB.HS == "hs2" {
					// we sniff /sendToDevice and assume that the access_token is for HS1.
					t.Skipf("federation unsupported for this test")
				}
				// we don't know when the new room key will be sent, it could be sent as soon as the device list update
				// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
				// traffic now.
				sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
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
					alice.MustSendMessage(t, roomID, wantMsgBody)
					waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

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
						case sendToDevice := <-pc.Chan():
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
	tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientType.HS})

	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.TestClient) {
		wantMsgBody := "test from another process"
		// send a message as Alice in a different process
		tc.WithClientSyncing(t, &cc.ClientCreationRequest{
			User: tc.Alice,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
			},
			Multiprocess: true,
		}, func(remoteAlice api.TestClient) {
			eventID := remoteAlice.MustSendMessage(t, roomID, wantMsgBody)
			waiter := remoteAlice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID))
			waiter.Waitf(t, 5*time.Second, "client did not see event %s", eventID)
		})

		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		waiter.Waitf(t, 8*time.Second, "bob did not see alice's message")

		// Now recreate the same client and make sure we don't send new room keys.

		// we're going to sniff calls to /sendToDevice to ensure we do NOT see a new room key being sent.
		sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
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
			alice.MustSendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

			// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
			// the room key.
			got := pc.TryRecv(t)
			if got != nil {
				ct.Fatalf(t, "saw /sendToDevice when restarting the client and sending a new message")
			}
		})
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
	tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{clientType.HS})

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
	}, func(bob api.TestClient) {
		// check the room works
		wantMsgBody := "Test Message"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.MustSendMessage(t, roomID, wantMsgBody)
		waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")

		// we're going to sniff calls to /sendToDevice to ensure we do NOT see a new room key being sent.
		sniffToDeviceEvent(t, tc, func(pc *callback.PassiveChannel) {
			// now alice is going to restart her client
			aliceStopSyncing()
			alice.Close(t)

			tc.WithClientSyncing(t, &cc.ClientCreationRequest{
				User: tc.Alice,
				Opts: alice.Opts(),
			}, func(alice api.TestClient) {
				// now send another message from Alice, who should NOT send another new room key
				wantMsgBody = "Another Test Message"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			})

			// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
			// the room key.
			got := pc.TryRecv(t)
			if got != nil {
				ct.Fatalf(t, "saw /sendToDevice when restarting the client and sending a new message")
			}
		})
	})
}

// An attacker working in cahoots with a homeserver admin cannot spoof the `sender` of an event to make it look like
// someone else's.
func TestSpoofedEventSenderHandling(t *testing.T) {
	runTest := func(t *testing.T, clientType api.ClientType, spoofAsMXID bool, expectUTD bool) {
		tc := Instance().CreateTestContext(t, clientType, clientType, clientType)
		// Alice, Bob and Charlie are in a room.
		tc.WithAliceBobAndCharlieSyncing(t, func(alice, bob, charlie api.TestClient) {
			roomID := tc.CreateNewEncryptedRoom(
				t,
				tc.Alice,
				cc.EncRoomOptions.PresetTrustedPrivateChat(),
				cc.EncRoomOptions.Invite([]string{tc.Bob.UserID, tc.Charlie.UserID}),
			)
			tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{})
			tc.Charlie.MustJoinRoom(t, roomID, []spec.ServerName{})
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Charlie.UserID, "join")).Waitf(t, 5*time.Second, "alice did not see charlie's join")

			// Alice sends a message to the room; Bob and Charlie should both receive it.
			wantMsgBody := "Test Message"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			waiter2 := charlie.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.MustSendMessage(t, roomID, wantMsgBody)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message")
			waiter2.Waitf(t, 5*time.Second, "charlie did not see alice's message")

			// Intercept Bob's /sync requests, and rewrite any events from Alice to have a sender of Charlie.
			var spoofedUserId string
			if spoofAsMXID {
				spoofedUserId = tc.Charlie.UserID
			} else {
				spoofedUserId = "charlie"
			}
			withSpoofSender(t, tc, tc.Alice.UserID, bob.CurrentAccessToken(t), spoofedUserId, func() {
				// Alice sends another message. Wait for Charlie to see it so we know it has got through.
				wantMsgBody = "Another Test Message"
				waiter = charlie.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				spoofedEventID := alice.MustSendMessage(t, roomID, wantMsgBody)
				waiter.Waitf(t, 5*time.Second, "Charlie did not see Alice's message")

				// Decryption happens asynchronously, so give a chance for it to happen.
				time.Sleep(1 * time.Second)

				if expectUTD {
					ev := bob.MustGetEvent(t, roomID, spoofedEventID)
					must.Equal(t, ev.FailedToDecrypt, true, fmt.Sprintf("Bob was able to decrypt the spoofed event: %v", ev))
				} else {
					// Bob should see a red shield.
					shield, err := bob.GetEventShield(t, roomID, spoofedEventID)
					must.NotError(t, "Could not get shield for Bob's view of spoofed message", err)
					if shield == nil {
						t.Errorf("Bob did not get a shield for the spoofed message")
					} else {
						must.Equal(t, shield.Colour, api.EventShieldColourRed, "Colour of shield")
						must.Equal(t, shield.Code, api.EventShieldCodeUnknownDevice, "Shield code")
					}
				}
			})
		})
	}

	Instance().ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		t.Run("SpoofedMXIDSenderGivesRedShield", func(t *testing.T) {
			runTest(t, clientType, true, false)
		})

		if clientType.Lang != api.ClientTypeRust {
			// The Rust SDK refuses to deserialize /sync responses that have non-MXID senders, so we don't bother
			// with this test.
			t.Run("SpoofedPlaintextSenderGivesUTD", func(t *testing.T) {
				runTest(t, clientType, false, true)
			})
		}
	})
}

// withSpoofSender sets up a MITM intercept which rewrites responses to `/sync` requests from the device with
// access token `targetUserAccessToken`, so that all events sent by `attackerUserID` appear to have been sent by
// `spoofedUserID`.
//
// The `inner` function is called with the intercept in place, and the configuration is reverted when `inner` completes.
func withSpoofSender(t *testing.T, tc *cc.TestContext, attackerUserID string, targetUserAccessToken string, spoofedUserID string, inner func()) {
	// Take the given event timeline from a `/sync` response, and rewrite any matching events in the list.
	//
	// Returns the modified JSON.
	patchTimeline := func(eventArray gjson.Result) string {
		eventArrayRaw := eventArray.Raw
		eventArray.ForEach(func(idx, event gjson.Result) bool {
			if event.Get("type").String() == "m.room.encrypted" && event.Get("sender").String() == attackerUserID {
				t.Logf("Rewriting event %s from %s to have sender of %s", event.Get("event_id").String(), event.Get("sender").String(), spoofedUserID)
				var err error
				if eventArrayRaw, err = sjson.Set(eventArrayRaw, fmt.Sprintf("%d.sender", idx.Int()), spoofedUserID); err != nil {
					t.Fatalf("Couldn't patch event array: %s", err)
				}
			}
			return true
		})
		return eventArrayRaw
	}

	tc.Deployment.MITM().Configure(t).WithIntercept(mitm.InterceptOpts{
		Filter: mitm.FilterParams{
			PathContains: "/sync",
			AccessToken:  targetUserAccessToken,
		},
		ResponseCallback: func(cd callback.Data) *callback.Response {
			var roomListJSONPath, timelineJSONPath string

			if strings.Contains(cd.URL, "/_matrix/client/v3/sync") {
				roomListJSONPath = "rooms.join"
				timelineJSONPath = "timeline.events"
			} else if strings.Contains(cd.URL, "org.matrix.simplified_msc3575/sync") {
				roomListJSONPath = "rooms"
				timelineJSONPath = "timeline"
			} else {
				t.Fatalf("Unknown sync endpoint: %s", cd.URL)
			}

			rawBody := string(cd.ResponseBody)
			// t.Logf("%s => %s", cd.URL, rawBody)
			joinedRooms := gjson.Parse(rawBody).Get(roomListJSONPath)
			joinedRooms.ForEach(func(roomID, room gjson.Result) bool {
				patchedTimeline := patchTimeline(room.Get(timelineJSONPath))

				jsonPath := fmt.Sprintf("%s.%s.%s", roomListJSONPath, gjson.Escape(roomID.String()), timelineJSONPath)
				var err error
				if rawBody, err = sjson.SetRaw(rawBody, jsonPath, patchedTimeline); err != nil {
					t.Fatalf("Couldn't patch response json: %s", err)
				}
				return true
			})

			return &callback.Response{
				RespondBody: json.RawMessage(rawBody),
			}
		},
	}, inner)
}
