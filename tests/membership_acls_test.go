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

		// login both clients first, so OTKs etc are uploaded.
		// We sign in Bob first to try to encourage Alice to get a device list
		// update with bob's device keys, which will be important when Alice
		// sends the event.
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)
		time.Sleep(500 * time.Millisecond)
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)

		// Alice starts syncing
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()

		wantMsgBody := "Hello world"

		// Check the room is in fact encrypted
		isEncrypted, err := alice.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted when it should be")

		// Bob starts syncing
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()

		isEncrypted, err = bob.IsRoomEncrypted(t, roomID)
		must.NotError(t, "failed to check if room is encrypted", err)
		must.Equal(t, isEncrypted, true, "room is not encrypted")
		t.Logf("bob room encrypted = %v", isEncrypted)

		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))
		evID := alice.SendMessage(t, roomID, wantMsgBody)

		// Bob receives the message
		t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
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
			"preset": "public_chat", // shared history visibility
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

		// login both clients first, so OTKs etc are uploaded.
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)

		// Alice and Bob start syncing
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()

		// Alice sends a message which Bob should not be able to decrypt
		beforeJoinBody := "Before Bob joins"
		waiter := alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(beforeJoinBody))
		evID := alice.SendMessage(t, roomID, beforeJoinBody)
		t.Logf("alice (%s) waiting for event %s", alice.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// now bob joins the room
		csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})
		time.Sleep(time.Second) // wait for it to appear on the client else rust crashes if it cannot find the room FIXME
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(bob.UserID(), "join"))
		waiter.Wait(t, 5*time.Second)

		// bob hits scrollback and should see but not be able to decrypt the message
		bob.MustBackpaginate(t, roomID, 5)
		ev := bob.MustGetEvent(t, roomID, evID)
		must.NotEqual(t, ev.Text, beforeJoinBody, "bob was able to decrypt a message from before he was joined")
		must.Equal(t, ev.FailedToDecrypt, true, "message not marked as failed to decrypt")
	})
}

// Bob leaves the room. Some messages are sent. Bob rejoins and cannot decrypt the messages sent whilst he was gone (ensuring we cycle keys).
func TestOnRejoinBobCanSeeButNotDecryptHistoryInPublicRoom(t *testing.T) {
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
			"name":   "TestOnRejoinBobCanSeeButNotDecryptHistoryInPublicRoom",
			"preset": "public_chat", // shared history visibility
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

		// login both clients first, so OTKs etc are uploaded.
		// Similarly to TestAliceBobEncryptionWorks, log Bob in first.
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)
		time.Sleep(500 * time.Millisecond)
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)

		// Alice and Bob start syncing. Both are in the same room
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()

		// Alice sends a message which Bob should be able to decrypt.
		bothJoinedBody := "Alice and Bob in a room"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(bothJoinedBody))
		evID := alice.SendMessage(t, roomID, bothJoinedBody)
		t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// now bob leaves the room, wait for alice to see it
		waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(bob.UserID(), "leave"))
		csapiBob.MustLeaveRoom(t, roomID)
		waiter.Wait(t, 5*time.Second)

		// now alice sends another message, which should use a key that bob does not have. Wait for the remote echo to come back.
		onlyAliceBody := "Only me on my lonesome"
		waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(onlyAliceBody))
		evID = alice.SendMessage(t, roomID, onlyAliceBody)
		t.Logf("alice (%s) waiting for event %s", alice.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// now bob rejoins the room, wait until he sees it.
		csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(bob.UserID(), "join"))
		waiter.Wait(t, 5*time.Second)
		// this is required for some reason else tests fail
		time.Sleep(time.Second)

		// bob hits scrollback and should see but not be able to decrypt the message
		bob.MustBackpaginate(t, roomID, 5)
		ev := bob.MustGetEvent(t, roomID, evID)
		must.NotEqual(t, ev.Text, onlyAliceBody, "bob was able to decrypt a message from before he was joined")
		must.Equal(t, ev.FailedToDecrypt, true, "message not marked as failed to decrypt")

		/* TODO: needs client changes
		time.Sleep(time.Second) // let alice realise bob is back in the room
		// bob should be able to decrypt subsequent messages
		bothJoinedBody = "Alice and Bob in a room again"
		evID = alice.SendMessage(t, roomID, bothJoinedBody)
		time.Sleep(time.Second) // TODO: use a Waiter; currently this is broken as it seems like listeners get detached on leave?
		ev = bob.MustGetEvent(t, roomID, evID)
		must.Equal(t, ev.Text, bothJoinedBody, "event was not decrypted correctly") */
	})
}
