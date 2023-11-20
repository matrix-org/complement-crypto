package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/helpers"
)

// A and B are in a room, on different servers.
// B's server goes offline.
// C joins the room (on A's server).
// C sends a message. C will not be able to get device keys for B.
// B comes back online.
// B will be unable to decrypt C's message. TODO: how to fix?
// Ensure sending another message from C is decryptable.
func TestNewUserCannotGetKeysForOfflineServer(t *testing.T) {
	deployment := Deploy(t)
	csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "alice",
		Password:        "complement-crypto-password",
	})
	csapiBob := deployment.Register(t, "hs2", helpers.RegistrationOpts{
		LocalpartSuffix: "bob",
		Password:        "complement-crypto-password",
	})
	csapiCharlie := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "charlie",
		Password:        "complement-crypto-password",
	})
	roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
		"preset": "private_chat",
		"invite": []string{csapiBob.UserID},
		"initial_state": []map[string]interface{}{
			{
				"type":      "m.room.encryption",
				"state_key": "",
				"content": map[string]interface{}{
					"algorithm": "m.megolm.v1.aes-sha2",
				},
			},
		},
	})

	csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})

	ss := deployment.SlidingSyncURL(t)
	alice := MustLoginClient(t, api.ClientType{HS: "hs1", Lang: api.ClientTypeRust}, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
	defer alice.Close(t)
	bob := MustLoginClient(t, api.ClientType{HS: "hs2", Lang: api.ClientTypeJS}, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
	defer bob.Close(t)
	aliceStopSyncing := alice.StartSyncing(t)
	defer aliceStopSyncing()
	bobStopSyncing := bob.StartSyncing(t)
	defer bobStopSyncing()

	// let clients sync device keys
	time.Sleep(time.Second)

	// ensure encrypted messaging works
	wantMsgBody := "Hello world"
	waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
	evID := alice.SendMessage(t, roomID, wantMsgBody)
	t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
	waiter.Wait(t, 5*time.Second)

	// now bob's HS becomes unreachable
	deployment.PauseServer(t, "hs2")

	// C now joins the room
	csapiAlice.MustInviteRoom(t, roomID, csapiCharlie.UserID)
	charlie := MustLoginClient(t, api.ClientType{HS: "hs1", Lang: api.ClientTypeRust}, api.FromComplementClient(csapiCharlie, "complement-crypto-password"), ss)
	defer charlie.Close(t)
	charlieStopSyncing := charlie.StartSyncing(t)
	defer charlieStopSyncing()
	csapiCharlie.MustJoinRoom(t, roomID, []string{"hs1"})

	// let charlie sync device keys... and fail to get bob's keys!
	time.Sleep(time.Second)

	// send a message: bob won't be able to decrypt this, but alice will.
	wantMsgBody = "Bob can't see this because his server is down"
	waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
	evID = charlie.SendMessage(t, roomID, wantMsgBody)
	t.Logf("alice (%s) waiting for event %s", alice.Type(), evID)
	waiter.Wait(t, 5*time.Second)

	// now bob's server comes back online
	deployment.UnpauseServer(t, "hs2")

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
	waiter.Wait(t, 5*time.Second)
}
