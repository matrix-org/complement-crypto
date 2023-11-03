package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// The simplest test case.
// Alice creates the room. Bob joins.
// Alice sends an encrypted message.
// Ensure Bob can see the decrypted content.
//
// Caveats: because this exercises the high level API, we do not explicitly
// say "send an encrypted event". The only indication that encrypted events are
// being sent is the m.room.encryption state event on /createRoom, coupled with
// asserting that isEncrypted() returns true. This test may be expanded in the
// future to assert things like "there is a ciphertext".
func TestAliceBobEncryptionWorks(t *testing.T) {
	// TODO: factor out so we can just call "matrix subtests"
	t.Run("Rust x Rust", func(t *testing.T) {
		testAliceBobEncryptionWorks(t, api.ClientTypeRust, api.ClientTypeRust)
	})
	t.Run("JS x JS", func(t *testing.T) {
		testAliceBobEncryptionWorks(t, api.ClientTypeJS, api.ClientTypeJS)
	})
	t.Run("Rust x JS", func(t *testing.T) {
		testAliceBobEncryptionWorks(t, api.ClientTypeRust, api.ClientTypeJS)
	})
	t.Run("JS x Rust", func(t *testing.T) {
		testAliceBobEncryptionWorks(t, api.ClientTypeJS, api.ClientTypeRust)
	})
}

func testAliceBobEncryptionWorks(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
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
	csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})
	ss := deployment.SlidingSyncURL(t)

	// SDK testing below
	// -----------------
	alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "testfromrustsdk"), ss)
	defer alice.Close(t)

	// Alice starts syncing
	aliceStopSyncing := alice.StartSyncing(t)
	defer aliceStopSyncing()
	time.Sleep(time.Second) // TODO: find another way to wait until initial sync is done

	wantMsgBody := "Hello world"

	// Check the room is in fact encrypted
	isEncrypted, err := alice.IsRoomEncrypted(t, roomID)
	must.NotError(t, "failed to check if room is encrypted", err)
	must.Equal(t, isEncrypted, true, "room is not encrypted when it should be")

	// Bob starts syncing
	bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "testfromrustsdk"), ss)
	defer bob.Close(t)
	bobStopSyncing := bob.StartSyncing(t)
	defer bobStopSyncing()
	time.Sleep(time.Second) // TODO: find another way to wait until initial sync is done

	isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
	must.NotError(t, "failed to check if room is encrypted", err)
	must.Equal(t, isEncrypted, true, "room is not encrypted")
	t.Logf("bob room encrypted = %v", isEncrypted)

	waiter := bob.WaitUntilEventInRoom(t, roomID, wantMsgBody)
	alice.SendMessage(t, roomID, wantMsgBody)

	// Bob receives the message
	t.Logf("bob (%s) waiting for event", bob.Type())
	waiter.Wait(t, 5*time.Second)
}
