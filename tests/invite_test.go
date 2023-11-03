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
// - Bob joins the room.
// - Ensure Bob can see the decrypted content.
func TestCanSeeMessagesAfterInviteButBeforeJoin(t *testing.T) {
	ClientTypeMatrix(t, testCanSeeMessagesAfterInviteButBeforeJoin)
}

func testCanSeeMessagesAfterInviteButBeforeJoin(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
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

	// Bob joins the room (via Complement, but it shouldn't matter)
	csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})

	isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
	must.NotError(t, "failed to check if room is encrypted", err)
	must.Equal(t, isEncrypted, true, "room is not encrypted")
	t.Logf("bob room encrypted = %v", isEncrypted)

	// TODO: this is sensitive to the timeline limit used on the SDK. If 1, then the message won't
	// be here (and will need to be fetched via /messages).
	waiter := bob.WaitUntilEventInRoom(t, roomID, wantMsgBody)
	waiter.Wait(t, 2*time.Second)
}
