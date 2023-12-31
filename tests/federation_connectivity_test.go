package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/helpers"
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
	wantUndecryptableMsgBody := "Bob can't see this because his server is down"
	waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantUndecryptableMsgBody))
	undecryptableEventID := charlie.SendMessage(t, roomID, wantUndecryptableMsgBody)
	t.Logf("alice (%s) waiting for event %s", alice.Type(), undecryptableEventID)
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

	// make sure bob cannot decrypt the msg from when his server was offline
	// TODO: this isn't ideal, see https://github.com/matrix-org/matrix-rust-sdk/issues/2864
	ev := bob.MustGetEvent(t, roomID, undecryptableEventID)
	must.Equal(t, ev.FailedToDecrypt, true, "bob was able to decrypt the undecryptable event")
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
	roomIDbc := csapiCharlie.MustCreateRoom(t, map[string]interface{}{
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
	roomIDab := csapiAlice.MustCreateRoom(t, map[string]interface{}{
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
	csapiBob.MustJoinRoom(t, roomIDab, []string{"hs1"})
	csapiBob.MustJoinRoom(t, roomIDbc, []string{"hs1"})

	ss := deployment.SlidingSyncURL(t)
	alice := MustLoginClient(t, api.ClientType{HS: "hs1", Lang: api.ClientTypeRust}, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
	defer alice.Close(t)
	bob := MustLoginClient(t, api.ClientType{HS: "hs2", Lang: api.ClientTypeJS}, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
	defer bob.Close(t)
	charlie := MustLoginClient(t, api.ClientType{HS: "hs1", Lang: api.ClientTypeRust}, api.FromComplementClient(csapiCharlie, "complement-crypto-password"), ss)
	defer charlie.Close(t)
	aliceStopSyncing := alice.StartSyncing(t)
	defer aliceStopSyncing()
	bobStopSyncing := bob.StartSyncing(t)
	defer bobStopSyncing()
	charlieStopSyncing := charlie.StartSyncing(t)
	defer charlieStopSyncing()

	// let clients sync device keys
	time.Sleep(time.Second)

	// ensure encrypted messaging works in rooms ab,bc
	wantMsgBody := "Hello world"
	waiter := bob.WaitUntilEventInRoom(t, roomIDab, api.CheckEventHasBody(wantMsgBody))
	evID := alice.SendMessage(t, roomIDab, wantMsgBody)
	t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
	waiter.Wait(t, 5*time.Second)
	waiter = bob.WaitUntilEventInRoom(t, roomIDbc, api.CheckEventHasBody(wantMsgBody))
	evID = charlie.SendMessage(t, roomIDbc, wantMsgBody)
	t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
	waiter.Wait(t, 5*time.Second)

	// now bob's HS becomes unreachable
	deployment.PauseServer(t, "hs2")

	// C now joins the room ab
	csapiAlice.MustInviteRoom(t, roomIDab, csapiCharlie.UserID)
	csapiCharlie.MustJoinRoom(t, roomIDab, []string{"hs1"})

	// let charlie sync device keys...
	time.Sleep(time.Second)

	// send a message as C: everyone should be able to decrypt this because Olm sessions
	// are per-device, not per-room.
	wantDecryptableMsgBody := "Bob can see this even though his server is down as we had a session already"
	waiter = alice.WaitUntilEventInRoom(t, roomIDab, api.CheckEventHasBody(wantDecryptableMsgBody))
	decryptableEventID := charlie.SendMessage(t, roomIDab, wantDecryptableMsgBody)
	t.Logf("alice (%s) waiting for event %s", alice.Type(), decryptableEventID)
	waiter.Wait(t, 5*time.Second)

	// now bob's server comes back online
	deployment.UnpauseServer(t, "hs2")

	waiter = bob.WaitUntilEventInRoom(t, roomIDab, api.CheckEventHasBody(wantDecryptableMsgBody))
	waiter.Wait(t, 10*time.Second) // longer time to allow for retries
}
