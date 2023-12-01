package tests

import (
	"fmt"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
)

// Test that if a client is unable to call /sendToDevice, it retries.
func TestClientRetriesSendToDevice(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		t.Logf("checking mitm")
		cmd := exec.Command("curl", "-v", "-X", "POST", "-d", "{}", "-x", tc.Deployment.ControllerURL, "http://mitm.code/options/unlock")
		output, cerr := cmd.CombinedOutput()
		fmt.Println(cerr)
		fmt.Println(string(output))
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "public_chat", nil)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		alice := tc.MustLoginClient(t, tc.Alice, clientTypeA)
		defer alice.Close(t)
		bob := tc.MustLoginClient(t, tc.Bob, clientTypeB)
		defer bob.Close(t)
		aliceStopSyncing := alice.StartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.StartSyncing(t)
		defer bobStopSyncing()
		// lets device keys be exchanged
		time.Sleep(time.Second)

		wantMsgBody := "Hello world!"
		waiter := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsgBody))

		var evID string
		var err error
		// now gateway timeout the /sendToDevice endpoint
		tc.Deployment.WithMITMOptions(t, map[string]interface{}{
			"statuscode": map[string]interface{}{
				"return_status": http.StatusGatewayTimeout,
				"filter":        "~u .*\\/sendToDevice.*",
			},
		}, func() {
			evID, err = alice.TrySendMessage(t, roomID, wantMsgBody)
			if err != nil {
				// we allow clients to fail the send if they cannot call /sendToDevice
				t.Logf("TrySendMessage: %s", err)
			}
			if evID != "" {
				t.Logf("TrySendMessage: => %s", evID)
			}
		})

		if err != nil {
			// retry now we have connectivity
			evID = alice.SendMessage(t, roomID, wantMsgBody)
		}

		// Bob receives the message
		t.Logf("bob (%s) waiting for event %s", bob.Type(), evID)
		waiter.Wait(t, 5*time.Second)
	})
}
