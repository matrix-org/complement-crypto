package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/must"
)

func TestCanBackupKeys(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		if clientTypeB.Lang == api.ClientTypeJS {
			t.Skipf("key backups unsupported (js)")
			return
		}
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		// shared history visibility
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "public_chat", nil)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})

		// SDK testing below
		// -----------------

		// login both clients first, so OTKs etc are uploaded.
		alice := tc.MustLoginClient(t, tc.Alice, clientTypeA)
		defer alice.Close(t)
		bob := tc.MustLoginClient(t, tc.Bob, clientTypeB)
		defer bob.Close(t)

		// Alice and Bob start syncing
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()

		// Alice sends a message which Bob should be able to decrypt
		body := "An encrypted message"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(body))
		evID := alice.SendMessage(t, roomID, body)
		t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// Now Bob backs up his keys. Some clients may automatically do this, but let's be explicit about it.
		recoveryKey := bob.MustBackupKeys(t)

		// Now Bob logs in on a new device
		_, bob2 := tc.MustLoginDevice(t, tc.Bob, clientTypeB, "NEW_DEVICE")

		// Bob loads the key backup using the recovery key
		bob2.MustLoadBackup(t, recoveryKey)

		// Bob's new device can decrypt the encrypted message
		bob2StopSyncing := bob2.StartSyncing(t)
		defer bob2StopSyncing()
		time.Sleep(time.Second)
		bob2.MustBackpaginate(t, roomID, 5) // get the old message

		ev := bob2.MustGetEvent(t, roomID, evID)
		must.Equal(t, ev.FailedToDecrypt, false, "bob's new device failed to decrypt the event: bad backup?")
		must.Equal(t, ev.Text, body, "bob's new device failed to see the clear text message")
	})
}
