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
		if clientTypeA.HS != clientTypeB.HS {
			t.Skipf("client A and B must be on the same HS as this is testing key backups so A=backup creator B=backup restorer")
			return
		}
		t.Logf("backup creator = %s backup restorer = %s", clientTypeA.Lang, clientTypeB.Lang)
		tc := CreateTestContext(t, clientTypeA)
		roomID := tc.Alice.MustCreateRoom(t, map[string]interface{}{
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
		tc.WithAliceSyncing(t, func(backupCreator api.Client) {
			body := "An encrypted message"
			waiter := backupCreator.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(body))
			evID := backupCreator.SendMessage(t, roomID, body)
			t.Logf("backupCreator (%s) waiting for event %s", backupCreator.Type(), evID)
			waiter.Wait(t, 5*time.Second)

			// Now backupCreator backs up his keys. Some clients may automatically do this, but let's be explicit about it.
			recoveryKey := backupCreator.MustBackupKeys(t)
			t.Logf("recovery key -> %s", recoveryKey)

			// Now login on a new device
			csapiAlice2 := tc.Deployment.Login(t, clientTypeB.HS, tc.Alice, helpers.LoginOpts{
				DeviceID: "BACKUP_RESTORER",
				Password: "complement-crypto-password",
			})
			backupRestorer := tc.MustLoginClient(t, csapiAlice2, clientTypeB)
			defer backupRestorer.Close(t)

			// load the key backup using the recovery key
			backupRestorer.MustLoadBackup(t, recoveryKey)

			// new device can decrypt the encrypted message
			backupRestorerStopSyncing := backupRestorer.MustStartSyncing(t)
			defer backupRestorerStopSyncing()
			time.Sleep(time.Second)
			backupRestorer.MustBackpaginate(t, roomID, 5) // get the old message

			ev := backupRestorer.MustGetEvent(t, roomID, evID)
			must.Equal(t, ev.FailedToDecrypt, false, "bob's new device failed to decrypt the event: bad backup?")
			must.Equal(t, ev.Text, body, "bob's new device failed to see the clear text message")
		})
	})
}

func TestBackupWrongRecoveryKeyFails(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		if clientTypeA.HS != clientTypeB.HS {
			t.Skipf("client A and B must be on the same HS as this is testing key backups so A=backup creator B=backup restorer")
			return
		}
		t.Logf("backup creator = %s backup restorer = %s", clientTypeA.Lang, clientTypeB.Lang)
		tc := CreateTestContext(t, clientTypeA)
		roomID := tc.Alice.MustCreateRoom(t, map[string]interface{}{
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
		tc.WithAliceSyncing(t, func(backupCreator api.Client) {
			body := "An encrypted message"
			waiter := backupCreator.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(body))
			evID := backupCreator.SendMessage(t, roomID, body)
			t.Logf("backupCreator (%s) waiting for event %s", backupCreator.Type(), evID)
			waiter.Wait(t, 5*time.Second)

			// Now backupCreator backs up his keys. Some clients may automatically do this, but let's be explicit about it.
			recoveryKey := backupCreator.MustBackupKeys(t)
			t.Logf("recovery key -> %s", recoveryKey)

			// Now login on a new device
			csapiAlice2 := tc.Deployment.Login(t, clientTypeB.HS, tc.Alice, helpers.LoginOpts{
				DeviceID: "BACKUP_RESTORER",
				Password: "complement-crypto-password",
			})
			backupRestorer := tc.MustLoginClient(t, csapiAlice2, clientTypeB)
			defer backupRestorer.Close(t)

			// load the key backup using a valid but wrong recovery key
			wrongRecoveryKey := "EsU1 591R iFs4 8xe6 kR79 7wKu 8XTG xdmx 9PVW 8pX9 LAnC Pe5r"
			backupRestorer.LoadBackup(t, wrongRecoveryKey)

			// new device cannot decrypt the encrypted message
			backupRestorerStopSyncing := backupRestorer.MustStartSyncing(t)
			defer backupRestorerStopSyncing()
			time.Sleep(time.Second)
			backupRestorer.MustBackpaginate(t, roomID, 5) // get the old message

			ev := backupRestorer.MustGetEvent(t, roomID, evID)
			must.Equal(t, ev.FailedToDecrypt, true, "bob's new device decrypted the event: insecure backup?")
		})
	})
}
