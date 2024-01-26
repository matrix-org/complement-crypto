package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/must"
)

func TestRoomKeyIsCycledOnDeviceLeaving(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "trusted_private_chat", []string{tc.Bob.UserID})
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		// Alice, Alice2 and Bob are in a room.
		alice := tc.MustLoginClient(t, tc.Alice, clientTypeA)
		defer alice.Close(t)
		csapiAlice2, alice2 := tc.MustLoginDevice(t, tc.Alice, clientTypeA, "OTHER_DEVICE")
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
		seenToDeviceEventSent := false
		callbackURL, close := deploy.NewCallbackServer(t, func(cd deploy.CallbackData) {
			if cd.Method == "OPTIONS" {
				return // ignore CORS
			}
			t.Logf("%+v", cd)
			if strings.Contains(cd.URL, "m.room.encrypted") {
				// we can't decrypt this, but we know that this should most likely be the m.room_key to-device event.
				seenToDeviceEventSent = true
			}
		})
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
		must.Equal(t, seenToDeviceEventSent, true, "did not see /sendToDevice when logging out and sending a new message")
	})
}
