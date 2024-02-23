package tests

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	templates "github.com/matrix-org/complement-crypto/tests/go_templates"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

func sniffToDeviceEvent(t *testing.T, ch chan deploy.CallbackData) (callbackURL string, close func()) {
	callbackURL, close = deploy.NewCallbackServer(t, func(cd deploy.CallbackData) {
		if cd.Method == "OPTIONS" {
			return // ignore CORS
		}
		t.Logf("%+v", cd)
		if strings.Contains(cd.URL, "m.room.encrypted") {
			// we can't decrypt this, but we know that this should most likely be the m.room_key to-device event.
			ch <- cd
		}
	})
	return callbackURL, close
}

// This test ensure we change the m.room_key when a device leaves an E2EE room.
// If the key is not changed, the left device could potentially decrypt the encrypted
// event if they could get access to it.
func TestRoomKeyIsCycledOnDeviceLogout(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "trusted_private_chat", []string{tc.Bob.UserID})
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		// Alice, Alice2 and Bob are in a room.
		alice := tc.MustLoginClient(t, tc.Alice, clientTypeA)
		defer alice.Close(t)
		csapiAlice2 := tc.MustRegisterNewDevice(t, tc.Alice, clientTypeA.HS, "OTHER_DEVICE")
		alice2 := tc.MustLoginClient(t, csapiAlice2, clientTypeA)
		defer alice2.Close(t)
		bob := tc.MustLoginClient(t, tc.Bob, clientTypeB)
		defer bob.Close(t)
		aliceStopSyncing := alice.MustStartSyncing(t)
		defer aliceStopSyncing()
		alice2StopSyncing := alice2.MustStartSyncing(t)
		defer alice2StopSyncing()
		bobStopSyncing := bob.MustStartSyncing(t)
		defer bobStopSyncing()

		// check the room works
		wantMsgBody := "Test Message"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		waiter2 := alice2.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.SendMessage(t, roomID, wantMsgBody)
		waiter.Wait(t, 5*time.Second)
		waiter2.Wait(t, 5*time.Second)

		// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
		ch := make(chan deploy.CallbackData, 10)
		callbackURL, close := sniffToDeviceEvent(t, ch)
		defer close()

		// we don't know when the new room key will be sent, it could be sent as soon as the device list update
		// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
		// traffic now.
		tc.Deployment.WithMITMOptions(t, map[string]interface{}{
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
			waiter.Wait(t, 5*time.Second)
		})

		// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
		// the room key.
		select {
		case <-ch:
		default:
			ct.Fatalf(t, "did not see /sendToDevice when logging out and sending a new message")
		}
	})
}

func TestRoomKeyIsCycledOnMemberLeaving(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "trusted_private_chat", []string{tc.Bob.UserID, tc.Charlie.UserID})
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		tc.Charlie.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		// Alice, Bob and Charlie are in a room.
		alice := tc.MustLoginClient(t, tc.Alice, clientTypeA)
		defer alice.Close(t)
		bob := tc.MustLoginClient(t, tc.Bob, clientTypeB)
		defer bob.Close(t)
		charlie := tc.MustLoginClient(t, tc.Charlie, clientTypeB)
		defer charlie.Close(t)
		aliceStopSyncing := alice.MustStartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.MustStartSyncing(t)
		defer bobStopSyncing()
		charlieStopSyncing := charlie.MustStartSyncing(t)
		defer charlieStopSyncing()

		// check the room works
		wantMsgBody := "Test Message"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		waiter2 := charlie.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.SendMessage(t, roomID, wantMsgBody)
		waiter.Wait(t, 5*time.Second)
		waiter2.Wait(t, 5*time.Second)

		// we're going to sniff calls to /sendToDevice to ensure we see the new room key being sent.
		ch := make(chan deploy.CallbackData, 10)
		callbackURL, close := sniffToDeviceEvent(t, ch)
		defer close()

		// we don't know when the new room key will be sent, it could be sent as soon as the device list update
		// is sent, or it could be delayed until message send. We want to handle both cases so we start sniffing
		// traffic now.
		tc.Deployment.WithMITMOptions(t, map[string]interface{}{
			"callback": map[string]interface{}{
				"callback_url": callbackURL,
				"filter":       "~u .*\\/sendToDevice.*",
			},
		}, func() {
			// now Charlie is going to leave the room, causing her user ID to appear in device_lists.left
			// which should trigger a new room key to be sent (on message send)
			tc.Charlie.MustDo(t, "POST", []string{"_matrix", "client", "v3", "logout"}, client.WithJSONBody(t, map[string]any{}))

			// we don't know how long it will take for the device list update to be processed, so wait 1s
			time.Sleep(time.Second)

			// now send another message from Alice, who should negotiate a new room key
			wantMsgBody = "Another Test Message"
			waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			alice.SendMessage(t, roomID, wantMsgBody)
			waiter.Wait(t, 5*time.Second)
		})

		// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
		// the room key.
		select {
		case <-ch:
		default:
			ct.Fatalf(t, "did not see /sendToDevice when logging out and sending a new message")
		}
	})
}

// Test that the m.room_key is NOT cycled when the client is restarted, but there is no change in devices
// in the room. This is important to ensure that we don't cycle m.room_keys too frequently, which increases
// the chances of seeing undecryptable events.
func TestRoomKeyIsNotCycledOnClientRestart(t *testing.T) {
	ForEachClientType(t, func(tt *testing.T, a api.ClientType) {
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
	tc := CreateTestContext(t, clientType, clientType)
	roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "trusted_private_chat", []string{tc.Bob.UserID})
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})

	bob := tc.MustLoginClient(t, tc.Bob, clientType)
	defer bob.Close(t)
	bobStopSyncing := bob.MustStartSyncing(t)
	defer bobStopSyncing()

	wantMsgBody := "test from the script"

	// run a script which will login as alice and then send an event in the room.
	// We will wait on that event as Bob to know when the script got to that point.
	cmd, close := templates.PrepareGoScript(t, "login_rust_client_and_send_msg/login_rust_client_and_send_msg.go",
		struct {
			UserID            string
			DeviceID          string
			Password          string
			BaseURL           string
			SSURL             string
			PersistentStorage bool
			Body              string
			RoomID            string
		}{
			UserID:            tc.Alice.UserID,
			Password:          tc.Alice.Password,
			DeviceID:          tc.Alice.DeviceID,
			BaseURL:           tc.Alice.BaseURL,
			PersistentStorage: true,
			SSURL:             tc.Deployment.SlidingSyncURL(t),
			Body:              wantMsgBody,
			RoomID:            roomID,
		})
	cmd.WaitDelay = 3 * time.Second
	defer close()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must.NotError(t, "failed to run script", cmd.Run())

	waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
	waiter.Wait(t, 8*time.Second)

	// the script sent the msg and exited cleanly.
	// Now recreate the same client and make sure we don't send new room keys.

	// we're going to sniff calls to /sendToDevice to ensure we do NOT see a new room key being sent.
	ch := make(chan deploy.CallbackData, 10)
	callbackURL, close := sniffToDeviceEvent(t, ch)
	defer close()

	tc.Deployment.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			"filter":       "~u .*\\/sendToDevice.*",
		},
	}, func() {
		// login as alice
		alice := tc.MustLoginClient(t, tc.Alice, clientType, WithPersistentStorage())
		defer alice.Close(t)
		aliceStopSyncing := alice.MustStartSyncing(t)
		defer aliceStopSyncing()

		// we don't know how long it will take for the device list update to be processed, so wait 1s
		time.Sleep(time.Second)

		// now send another message from Alice, who should NOT negotiate a new room key
		wantMsgBody = "Another Test Message"
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.SendMessage(t, roomID, wantMsgBody)
		waiter.Wait(t, 5*time.Second)
	})

	// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
	// the room key.
	select {
	case <-ch:
		ct.Fatalf(t, "saw /sendToDevice when restarting the client and sending a new message")
	default:
	}
}

func testRoomKeyIsNotCycledOnClientRestartJS(t *testing.T, clientType api.ClientType) {
	tc := CreateTestContext(t, clientType, clientType)
	roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "trusted_private_chat", []string{tc.Bob.UserID})
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})

	// Alice and Bob are in a room.
	alice := tc.MustLoginClient(t, tc.Alice, clientType, WithPersistentStorage())
	aliceStopSyncing := alice.MustStartSyncing(t)
	// no close here as we'll close it in the test mid-way
	bob := tc.MustLoginClient(t, tc.Bob, clientType)
	defer bob.Close(t)
	bobStopSyncing := bob.MustStartSyncing(t)
	defer bobStopSyncing()

	// check the room works
	wantMsgBody := "Test Message"
	waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
	alice.SendMessage(t, roomID, wantMsgBody)
	waiter.Wait(t, 5*time.Second)

	// we're going to sniff calls to /sendToDevice to ensure we do NOT see a new room key being sent.
	ch := make(chan deploy.CallbackData, 10)
	callbackURL, close := sniffToDeviceEvent(t, ch)
	defer close()

	// we want to start sniffing for the to-device event just before we restart the client.
	tc.Deployment.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			"filter":       "~u .*\\/sendToDevice.*",
		},
	}, func() {
		// now alice is going to restart her client
		aliceStopSyncing()
		alice.Close(t)

		alice = tc.MustCreateClient(t, tc.Alice, clientType, WithPersistentStorage())
		defer alice.Close(t)
		alice.Login(t, alice.Opts()) // login should work
		alice2StopSyncing, _ := alice.StartSyncing(t)
		defer alice2StopSyncing()

		// now send another message from Alice, who should NOT send another new room key
		wantMsgBody = "Another Test Message"
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.SendMessage(t, roomID, wantMsgBody)
		waiter.Wait(t, 5*time.Second)
	})

	// we should have seen a /sendToDevice call by now. If we didn't, this implies we didn't cycle
	// the room key.
	select {
	case <-ch:
		ct.Fatalf(t, "saw /sendToDevice when restarting the client and sending a new message")
	default:
	}
}
