package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// This test checks that Bob can decrypt messages sent before he was joined but after he was invited.
// - Alice creates the room. Alice invites Bob.
// - Alice sends an encrypted message.
// - Bob joins the room and backpaginates.
// - Ensure Bob can see the decrypted content.
func TestCanDecryptMessagesAfterInviteButBeforeJoin(t *testing.T) {
	ClientTypeMatrix(t, testCanDecryptMessagesAfterInviteButBeforeJoin)
}

func testCanDecryptMessagesAfterInviteButBeforeJoin(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
	if clientTypeA == api.ClientTypeRust && clientTypeB == api.ClientTypeRust {
		t.Skip("Skipping rust/rust as SS proxy sends invite/join in timeline, omitting the invite msg")
	}
	// Setup Code
	// ----------
	deployment := Deploy(t)
	// pre-register alice and bob
	csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "alice",
		Password:        "testfromrustsdk",
	})
	csapiBob := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "bob",
		Password:        "testfromrustsdk",
	})
	// Alice invites Bob to the encrypted room
	roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
		"name":   "JS SDK Test",
		"preset": "trusted_private_chat",
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
	ss := deployment.SlidingSyncURL(t)

	// SDK testing below
	// -----------------

	// Alice logs in.
	alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "testfromrustsdk"), ss)
	defer alice.Close(t)

	// Bob logs in BEFORE Alice starts syncing. This is important because the act of logging in should cause
	// Bob to upload OTKs which will be needed to send the encrypted event.
	bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "testfromrustsdk"), ss)
	defer bob.Close(t)

	// Alice and Bob start syncing
	aliceStopSyncing := alice.StartSyncing(t)
	defer aliceStopSyncing()
	bobStopSyncing := bob.StartSyncing(t)
	defer bobStopSyncing()

	time.Sleep(time.Second) // TODO: find another way to wait until initial sync is done

	wantMsgBody := "Hello world"

	// Check the room is in fact encrypted
	isEncrypted, err := alice.IsRoomEncrypted(t, roomID)
	must.NotError(t, "failed to check if room is encrypted", err)
	must.Equal(t, isEncrypted, true, "room is not encrypted when it should be")

	// Alice sends the message whilst Bob is still invited.
	alice.SendMessage(t, roomID, wantMsgBody)
	// wait for SS proxy to get it. Only needed when testing Rust TODO FIXME
	// Without this, the join will race with sending the msg and you could end up with the
	// message being sent POST join, which breaks the point of this test.
	time.Sleep(time.Second)

	// Bob joins the room (via Complement, but it shouldn't matter)
	csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})

	isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
	must.NotError(t, "failed to check if room is encrypted", err)
	must.Equal(t, isEncrypted, true, "room is not encrypted")
	t.Logf("bob room encrypted = %v", isEncrypted)

	// send a sentinel message and wait for it to ensure we are joined and syncing
	sentinelBody := "Sentinel"
	waiter := bob.WaitUntilEventInRoom(t, roomID, sentinelBody)
	alice.SendMessage(t, roomID, sentinelBody)
	waiter.Wait(t, 5*time.Second)

	// Explicitly ask for a pagination, rather than assuming the SDK will return events
	// earlier than the join by default. This is important because:
	// - sync v2 (JS SDK) it depends on the timeline limit, which is 20 by default but we don't want to assume.
	// - sliding sync (FFI) it won't return events before the join by default, relying on clients using the prev_batch token.
	waiter = bob.WaitUntilEventInRoom(t, roomID, wantMsgBody)
	bob.MustBackpaginate(t, roomID, 5) // number is arbitrary, just needs to be >=2
	waiter.Wait(t, 5*time.Second)
	// time.Sleep(time.Hour)
}
