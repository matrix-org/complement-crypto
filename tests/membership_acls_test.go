package tests

import (
	"fmt"
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
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
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
		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		ss := deployment.SlidingSyncURL(t)

		// SDK testing below
		// -----------------

		// login both clients first, so OTKs etc are uploaded.
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)

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
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
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
		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

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
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
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
		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		time.Sleep(time.Second) // wait for it to appear on the client else rust crashes if it cannot find the room FIXME
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(bob.UserID(), "join"))
		waiter.Wait(t, 5*time.Second)

		// bob hits scrollback and should see but not be able to decrypt the message
		bob.MustBackpaginate(t, roomID, 5)
		// jJ runs need this, else the event will exist but not yet be marked as failed to decrypt. Unsure why fed slows it down.
		time.Sleep(500 * time.Millisecond)
		ev := bob.MustGetEvent(t, roomID, evID)
		must.NotEqual(t, ev.Text, beforeJoinBody, "bob was able to decrypt a message from before he was joined")
		must.Equal(t, ev.FailedToDecrypt, true, fmt.Sprintf("message not marked as failed to decrypt: %+v", ev))
	})
}

// Bob leaves the room. Some messages are sent. Bob rejoins and cannot decrypt the messages sent whilst he was gone (ensuring we cycle keys).
func TestOnRejoinBobCanSeeButNotDecryptHistoryInPublicRoom(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Setup Code
		// ----------
		deployment := Deploy(t)
		// pre-register alice and bob
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
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
		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
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
		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(bob.UserID(), "join"))
		waiter.Wait(t, 5*time.Second)
		// this is required for some reason else tests fail
		time.Sleep(time.Second)

		// bob hits scrollback and should see but not be able to decrypt the message
		bob.MustBackpaginate(t, roomID, 5)
		// TODO: jJ runs fail as the timeline omits the event e.g it has leave,join and not leave,msg,join.
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

// Test that Bob's devices are treated as separate members wrt encryption. Therefore, if the device does not exist (not in the room)
// then messages aren't decryptable. Likewise, if the device DID exist but no longer does (due to /logout), ensure messages sent whilst
// logged out are not decryptable.
func TestOnNewDeviceBobCanSeeButNotDecryptHistoryInPublicRoom(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Setup Code
		// ----------
		deployment := Deploy(t)
		// pre-register alice and bob
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
		roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
			"name":   "TestOnNewDeviceBobCanSeeButNotDecryptHistoryInPublicRoom",
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
		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		ss := deployment.SlidingSyncURL(t)

		// SDK testing below
		// -----------------

		// login both clients first, so OTKs etc are uploaded.
		// Similarly to TestAliceBobEncryptionWorks, log Bob in first.
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)

		// Alice and Bob start syncing. Both are in the same room
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()

		// Alice sends a message which Bob should be able to decrypt.
		onlyFirstDeviceBody := "Alice and Bob in a room"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(onlyFirstDeviceBody))
		evID := alice.SendMessage(t, roomID, onlyFirstDeviceBody)
		t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// now bob logs in on a new device. He should NOT be able to decrypt this event (though can see it due to history visibility)
		csapiBob2 := deployment.Login(t, clientTypeB.HS, csapiBob, helpers.LoginOpts{
			DeviceID: "NEW_DEVICE",
			Password: "complement-crypto-password",
		})
		bob2 := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob2, "complement-crypto-password"), ss)
		bob2StopSyncing := bob2.StartSyncing(t)
		bob2StoppedSyncing := false
		defer func() {
			if bob2StoppedSyncing {
				return
			}
			bob2StopSyncing()
		}()
		time.Sleep(time.Second)             // let device keys propagate to alice
		bob2.MustBackpaginate(t, roomID, 5) // ensure the older event is there
		time.Sleep(time.Second)
		undecryptableEvent := bob2.MustGetEvent(t, roomID, evID)
		must.Equal(t, undecryptableEvent.FailedToDecrypt, true, "bob's new device was able to decrypt a message sent before he logged in")

		// now alice sends another message, which bob's new device should be able to decrypt.
		decryptableBody := "Bob's new device can decrypt this"
		waiter = bob2.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(decryptableBody))
		evID = alice.SendMessage(t, roomID, decryptableBody)
		t.Logf("bob2 (%s) waiting for event %s", bob2.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// now bob logs out
		bob2StopSyncing()
		bob2StoppedSyncing = true
		csapiBob2.MustDo(t, "POST", []string{"_matrix", "client", "v3", "logout"})
		bob2.Close(t)

		time.Sleep(time.Second) // let device keys propagate to alice

		// alice sends another message which should not be decryptable due to key cycling. The message should be decryptable
		// by bob's other logged in device though.
		undecryptableBody := "Bob's logged out device won't be able to decrypt this"
		waiter = bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(undecryptableBody))
		evID = alice.SendMessage(t, roomID, undecryptableBody)
		t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// now bob logs in again
		bob2 = MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob2, "complement-crypto-password"), ss)
		bob2StopSyncingAgain := bob2.StartSyncing(t)
		defer bob2StopSyncingAgain()

		time.Sleep(time.Second) // let device keys propagate to alice

		undecryptableEvent = bob2.MustGetEvent(t, roomID, evID)
		must.Equal(t, undecryptableEvent.FailedToDecrypt, true, "bob's new device was able to decrypt a message sent after he had logged out")
	})
}

/* TODO: unclear when Alice should send msg, need clarification 21/11/2023
// Alice invites Bob, Bob changes their device, then Bob joins. Bob should be able to see Alice's message.
func TestChangingDeviceAfterInviteReEncrypts(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		// Setup Code
		// ----------
		deployment := Deploy(t)
		// pre-register alice and bob
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
		roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
			"name":   "TestChangingDeviceAfterInviteReEncrypts",
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
		// Similarly to TestAliceBobEncryptionWorks, log Bob in first.
		bob := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob, "complement-crypto-password"), ss)
		defer bob.Close(t)
		alice := MustLoginClient(t, clientTypeA, api.FromComplementClient(csapiAlice, "complement-crypto-password"), ss)
		defer alice.Close(t)

		// Alice and Bob start syncing. Alice is in her own room.
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()

		// Alice invites Bob and then she sends an event
		csapiAlice.MustInviteRoom(t, roomID, csapiBob.UserID)
		time.Sleep(time.Second) // let device keys propagate
		body := "Alice should re-encrypt this message for bob's new device"
		evID := alice.SendMessage(t, roomID, body)

		// now Bob logs in on a different device and accepts the invite. The different device should be able to decrypt the message.
		csapiBob2 := deployment.Login(t, clientTypeB.HS, csapiBob, helpers.LoginOpts{
			DeviceID: "NEW_DEVICE",
			Password: "complement-crypto-password",
		})
		bob2 := MustLoginClient(t, clientTypeB, api.FromComplementClient(csapiBob2, "complement-crypto-password"), ss)
		bob2StopSyncing := bob2.StartSyncing(t)
		defer bob2StopSyncing()

		time.Sleep(time.Second) // let device keys propagate

		csapiBob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		time.Sleep(time.Second) // let the client load the events
		bob2.MustBackpaginate(t, roomID, 5)
		event := bob2.MustGetEvent(t, roomID, evID)
		must.Equal(t, event.FailedToDecrypt, false, "bob2 was not able to decrypt the message")
		must.Equal(t, event.Text, body, "bob2 failed to decrypt body")
	})
}
*/
