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
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Setup Code
		// ----------
		deployment := Deploy(t)
		// pre-register alice and bob
		csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
		roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
			"name":   "TestAliceBobEncryptionWorks",
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
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
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
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()
		time.Sleep(time.Second) // TODO: find another way to wait until initial sync is done

		isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted")
		t.Logf("bob room encrypted = %v", isEncrypted)

		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		alice.SendMessage(t, roomID, wantMsgBody)

		// Bob receives the message
		t.Logf("bob (%s) waiting for event", bob.Type())
		waiter.Wait(t, 5*time.Second)
	})
}

// This test checks that Bob can decrypt messages sent before he was joined but after he was invited.
// - Alice creates the room. Alice invites Bob.
// - Alice sends an encrypted message.
// - Bob joins the room and backpaginates.
// - Ensure Bob can see the decrypted content.
func TestCanDecryptMessagesAfterInviteButBeforeJoin(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Setup Code
		// ----------
		deployment := Deploy(t)
		// pre-register alice and bob
		csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
		// Alice invites Bob to the encrypted room
		roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
			"name":   "TestCanDecryptMessagesAfterInviteButBeforeJoin",
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

		// Bob logs in BEFORE Alice. This is important because the act of logging in should cause
		// Bob to upload OTKs which will be needed to send the encrypted event.
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)

		// Alice logs in.
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)

		// Alice and Bob start syncing.
		// FIXME: Bob must sync before Alice otherwise Alice does not seem to get Bob's device in /keys/query. By putting
		// Bob first, we ensure that the _first_ device list sync for the room includes Bob.
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()

		wantMsgBody := "Message sent when bob is invited not joined"

		// Check the room is in fact encrypted
		isEncrypted, err := alice.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted when it should be")

		// Alice sends the message whilst Bob is still invited.
		alice.SendMessage(t, roomID, wantMsgBody)
		// wait for SS proxy to get it. Only needed when testing Rust TODO FIXME
		// Without this, the join will race with sending the msg and you could end up with the
		// message being sent POST join, which breaks the point of this test.
		// kegan: I think this happens because SendMessage on Rust does not block until a 200 OK
		// because it allows for local echo. Can we fix the RustClient?
		time.Sleep(time.Second)

		// Bob joins the room (via Complement, but it shouldn't matter)
		csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})

		isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted")
		t.Logf("bob room encrypted = %v", isEncrypted)

		// send a sentinel message and wait for it to ensure we are joined and syncing.
		// This also checks that subsequent messages are decryptable.
		sentinelBody := "Sentinel"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(sentinelBody))
		alice.SendMessage(t, roomID, sentinelBody)
		waiter.Wait(t, 5*time.Second)

		// Explicitly ask for a pagination, rather than assuming the SDK will return events
		// earlier than the join by default. This is important because:
		// - sync v2 (JS SDK) it depends on the timeline limit, which is 20 by default but we don't want to assume.
		// - sliding sync (FFI) it won't return events before the join by default, relying on clients using the prev_batch token.
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		bob.MustBackpaginate(t, roomID, 5) // number is arbitrary, just needs to be >=2
		waiter.Wait(t, 5*time.Second)
	})
}

/*
// In a public, `shared` history visibility room, a new user Bob cannot decrypt earlier messages prior to his join,
// despite being able to see the events. Subsequent messages are decryptable.
func TestBobCanSeeButNotDecryptHistoryInPublicRoom(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Setup Code
		// ----------
		deployment := Deploy(t)
		// pre-register alice and bob
		csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, "hs1", helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
		roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
			"name":   "TestBobCanSeeButNotDecryptHistoryInPublicRoom",
			"preset": "public_chat",
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
		// TODO csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})
		ss := deployment.SlidingSyncURL(t)

		// SDK testing below
		// -----------------
		// Alice and Bob are present with keys uploaded etc
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)

		// Alice and Bob start syncing
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()
		time.Sleep(time.Second) // TODO: find another way to wait until initial sync is done

		undecryptableBody := "Bob cannot decrypt this"

		// Check the room is in fact encrypted
		isEncrypted, err := alice.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted when it should be")

		// Alice sends a message to herself in this public room
		aliceWaiter := alice.WaitUntilEventInRoom(t, roomID, undecryptableBody)
		alice.SendMessage(t, roomID, undecryptableBody)
		t.Logf("alice (%s) waiting for event", alice.Type())
		aliceWaiter.Wait(t, 5*time.Second)

		// Bob joins the room
		csapiBob.JoinRoom(t, roomID, []string{"hs1"})
		time.Sleep(time.Second) // TODO alice waits until she sees bob's join

		// Alice sends a new message which Bob should be able to decrypt
		decryptableBody := "Bob can decrypt this"
		aliceWaiter = alice.WaitUntilEventInRoom(t, roomID, decryptableBody)
		// Rust SDK listener doesn't seem to always catch this unless we are listening before the message is sent
		bobWaiter := bob.WaitUntilEventInRoom(t, roomID, decryptableBody)
		alice.SendMessage(t, roomID, decryptableBody)
		t.Logf("alice (%s) waiting for event", alice.Type())
		aliceWaiter.Wait(t, 5*time.Second)

		isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted")
		t.Logf("bob room encrypted = %v", isEncrypted)

		// Bob receives the decryptable message
		t.Logf("bob (%s) waiting for event", bob.Type())
		bobWaiter.Wait(t, 5*time.Second)

		// Bob attempts to backpaginate to see the older message
		bob.MustBackpaginate(t, roomID, 5) // arbitrary, must be >2

		// TODO Ensure Bob cannot see the undecrypted content, find the event by event ID to confirm

	})
} */
