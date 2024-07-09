package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement/must"
)

// A and B are in a room, on different servers.
// B's server goes offline.
// C joins the room (on A's server).
// C sends a message. C will not be able to get device keys for B.
// B comes back online.
// B will be unable to decrypt C's message. TODO: see https://github.com/matrix-org/matrix-rust-sdk/issues/2864
// Ensure sending another message from C is decryptable.
func TestNewUserCannotGetKeysForOfflineServer(t *testing.T) {
	Instance().ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := Instance().CreateTestContext(t, api.ClientType{
			Lang: clientType.Lang,
			HS:   "hs1",
		}, api.ClientType{
			Lang: clientType.Lang,
			HS:   "hs2",
		}, api.ClientType{
			Lang: clientType.Lang,
			HS:   "hs1",
		})
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}))
		t.Logf("%s joining room %s", tc.Bob.UserID, roomID)
		tc.Bob.MustJoinRoom(t, roomID, []string{"hs1"})

		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			// let clients sync device keys
			time.Sleep(time.Second)

			// ensure encrypted messaging works
			wantMsgBody := "Hello world"
			waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
			evID := alice.SendMessage(t, roomID, wantMsgBody)
			t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message '%s'", wantMsgBody)

			// now bob's HS becomes unreachable
			tc.Deployment.PauseServer(t, "hs2")

			// C now joins the room
			tc.Alice.MustInviteRoom(t, roomID, tc.Charlie.UserID)
			tc.WithClientSyncing(t, &cc.ClientCreationRequest{
				User: tc.Charlie,
			}, func(charlie api.Client) {
				tc.Charlie.MustJoinRoom(t, roomID, []string{"hs1"})

				// let charlie sync device keys... and fail to get bob's keys!
				time.Sleep(time.Second)

				// send a message: bob won't be able to decrypt this, but alice will.
				wantUndecryptableMsgBody := "Bob can't see this because his server is down"
				waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantUndecryptableMsgBody))
				undecryptableEventID := charlie.SendMessage(t, roomID, wantUndecryptableMsgBody)
				t.Logf("alice (%s) waiting for event %s", alice.Type(), undecryptableEventID)
				waiter.Waitf(t, 5*time.Second, "alice did not see charlie's messages '%s'", wantUndecryptableMsgBody)

				// now bob's server comes back online
				tc.Deployment.UnpauseServer(t, "hs2")

				// now we need to wait a bit for charlie's client to decide to hit /keys/claim again.
				// If the client hits too often, there will be constantly send lag so long as bob's HS is offline.
				// If the client hits too infrequently, there will be multiple undecryptable messages.
				// See https://github.com/matrix-org/matrix-rust-sdk/issues/281 for why we want to backoff.
				// See https://github.com/matrix-org/matrix-rust-sdk/issues/2804 for discussions on what the backoff should be.
				t.Logf("sleeping until client timeout is ready...")
				time.Sleep(20 * time.Second)

				// send another message, bob should be able to decrypt it.
				wantMsgBody = "Bob can see this because his server is now back online"
				waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
				evID = charlie.SendMessage(t, roomID, wantMsgBody)
				t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
				waiter.Waitf(t, 7*time.Second, "bob did not see charlie's message '%s'", wantMsgBody)

				// make sure bob cannot decrypt the msg from when his server was offline
				// TODO: this isn't ideal, see https://github.com/matrix-org/matrix-rust-sdk/issues/2864
				ev := bob.MustGetEvent(t, roomID, undecryptableEventID)
				must.Equal(t, ev.FailedToDecrypt, true, "bob was able to decrypt the undecryptable event")
			})
		})
	})
}

// A and B are in a room, on different servers.
// C and B are in another room, on different servers.
// B's server goes offline.
// C joins the room (on A's server).
// C sends a message. C will not be able to get device keys for B, but should already have them from previous room.
// B comes back online.
// B will be able to decrypt C's message.
// This is ultimately checking that Olm sessions are per-device and not per-room.
func TestExistingSessionCannotGetKeysForOfflineServer(t *testing.T) {
	Instance().ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := Instance().CreateTestContext(t, api.ClientType{
			Lang: clientType.Lang,
			HS:   "hs1",
		}, api.ClientType{
			Lang: clientType.Lang,
			HS:   "hs2",
		}, api.ClientType{
			Lang: clientType.Lang,
			HS:   "hs1",
		})
		roomIDbc := tc.CreateNewEncryptedRoom(t, tc.Charlie, cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}))
		roomIDab := tc.CreateNewEncryptedRoom(t, tc.Alice, cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}))
		t.Logf("%s joining rooms %s and %s", tc.Bob.UserID, roomIDab, roomIDbc)
		tc.Bob.MustJoinRoom(t, roomIDab, []string{"hs1"})
		tc.Bob.MustJoinRoom(t, roomIDbc, []string{"hs1"})

		tc.WithAliceBobAndCharlieSyncing(t, func(alice, bob, charlie api.Client) {
			// let clients sync device keys
			time.Sleep(time.Second)

			// ensure encrypted messaging works in rooms ab,bc
			wantMsgBody := "Hello world"
			waiter := bob.WaitUntilEventInRoom(t, roomIDab, api.CheckEventHasBody(wantMsgBody))
			evID := alice.SendMessage(t, roomIDab, wantMsgBody)
			t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
			waiter.Waitf(t, 5*time.Second, "bob did not see alice's message: '%s'", wantMsgBody)
			waiter = bob.WaitUntilEventInRoom(t, roomIDbc, api.CheckEventHasBody(wantMsgBody))
			evID = charlie.SendMessage(t, roomIDbc, wantMsgBody)
			t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
			waiter.Waitf(t, 5*time.Second, "bob did not see charlie's message: '%s'", wantMsgBody)

			// now bob's HS becomes unreachable
			tc.Deployment.PauseServer(t, "hs2")

			// C now joins the room ab
			tc.Alice.MustInviteRoom(t, roomIDab, tc.Charlie.UserID)
			tc.Charlie.MustJoinRoom(t, roomIDab, []string{"hs1"})

			// let charlie sync device keys...
			time.Sleep(time.Second)

			// send a message as C: everyone should be able to decrypt this because Olm sessions
			// are per-device, not per-room.
			wantDecryptableMsgBody := "Bob can see this even though his server is down as we had a session already"
			waiter = alice.WaitUntilEventInRoom(t, roomIDab, api.CheckEventHasBody(wantDecryptableMsgBody))
			decryptableEventID := charlie.SendMessage(t, roomIDab, wantDecryptableMsgBody)
			t.Logf("alice (%s) waiting for event %s", alice.Type(), decryptableEventID)
			waiter.Waitf(t, 5*time.Second, "alice did not see charlie's message: '%s'", wantDecryptableMsgBody)

			// now bob's server comes back online
			tc.Deployment.UnpauseServer(t, "hs2")

			waiter = bob.WaitUntilEventInRoom(t, roomIDab, api.CheckEventHasBody(wantDecryptableMsgBody))
			waiter.Waitf(t, 10*time.Second, "bob did not see charlie's message: '%s'", wantDecryptableMsgBody) // longer time to allow for retries
		})
	})
}
