package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// Test that backups can be created and stored in secret storage.
// Test that backups can be restored using secret storage and the recovery key.
func TestCanBackupKeys(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		if clientTypeB.Lang == api.ClientTypeJS {
			t.Skipf("key backup restoring is unsupported (js)")
			return
		}
		if clientTypeA.HS != clientTypeB.HS {
			t.Skipf("client A and B must be on the same HS as this is testing key backups so A=backup creator B=backup restorer")
			return
		}
		deployment := Deploy(t)
		csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
			"name":   t.Name(),
			"preset": "public_chat", // shared history visibility
			"invite": []string{},
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

		// SDK testing below
		// -----------------

		backupCreator := LoginClientFromComplementClient(t, deployment, csapiAlice, clientTypeA)
		defer backupCreator.Close(t)
		stopSyncing := backupCreator.StartSyncing(t)
		defer stopSyncing()

		body := "An encrypted message"
		waiter := backupCreator.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(body))
		evID := backupCreator.SendMessage(t, roomID, body)
		t.Logf("backupCreator (%s) waiting for event %s", backupCreator.Type(), evID)
		waiter.Wait(t, 5*time.Second)

		// Now backupCreator backs up his keys. Some clients may automatically do this, but let's be explicit about it.
		recoveryKey := backupCreator.MustBackupKeys(t)
		t.Logf("recovery key -> %s", recoveryKey)

		// Now login on a new device
		csapiAlice2 := deployment.Login(t, clientTypeB.HS, csapiAlice, helpers.LoginOpts{
			DeviceID: "BACKUP_RESTORER",
			Password: "complement-crypto-password",
		})
		backupRestorer := LoginClientFromComplementClient(t, deployment, csapiAlice2, clientTypeB)
		defer backupRestorer.Close(t)

		// load the key backup using the recovery key
		backupRestorer.MustLoadBackup(t, recoveryKey)

		// new device can decrypt the encrypted message
		backupRestorerStopSyncing := backupRestorer.StartSyncing(t)
		defer backupRestorerStopSyncing()
		time.Sleep(time.Second)
		backupRestorer.MustBackpaginate(t, roomID, 5) // get the old message

		ev := backupRestorer.MustGetEvent(t, roomID, evID)
		must.Equal(t, ev.FailedToDecrypt, false, "bob's new device failed to decrypt the event: bad backup?")
		must.Equal(t, ev.Text, body, "bob's new device failed to see the clear text message")
	})
}
