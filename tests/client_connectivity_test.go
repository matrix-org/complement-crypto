package tests

import (
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	templates "github.com/matrix-org/complement-crypto/tests/go_templates"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// Test that if the client is restarted BEFORE getting the /keys/upload response but
// AFTER the server has processed the request, the keys are not regenerated (which would
// cause duplicate key IDs with different keys). Requires persistent storage.
// Regression test for https://github.com/matrix-org/matrix-rust-sdk/issues/1415
func TestSigkillBeforeKeysUploadResponseRust(t *testing.T) {
	clientType := api.ClientType{Lang: api.ClientTypeRust, HS: "hs1"}
	var mu sync.Mutex
	var terminated atomic.Bool
	var terminateClient func()
	seenSecondKeysUploadWaiter := helpers.NewWaiter()
	callbackURL, close := deploy.NewCallbackServer(t, func(cd deploy.CallbackData) {
		if terminated.Load() {
			// make sure the 2nd upload 200 OKs
			if cd.ResponseCode != 200 {
				// TODO: Errorf FIXME
				t.Logf("2nd /keys/upload did not 200 OK => got %v", cd.ResponseCode)
			}
			seenSecondKeysUploadWaiter.Finish()
			return
		}
		// destroy the client
		mu.Lock()
		terminateClient()
		mu.Unlock()
	})
	defer close()

	tc := CreateTestContext(t, clientType, clientType)
	tc.Deployment.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			"filter":       "~u .*\\/keys\\/upload.*",
		},
	}, func() {
		cfg := api.FromComplementClient(tc.Alice, "complement-crypto-password")
		cfg.BaseURL = tc.Deployment.ReverseProxyURLForHS(clientType.HS)
		cfg.PersistentStorage = true
		// run some code in a separate process so we can kill it later
		cmd, close := templates.PrepareGoScript(t, "login_rust_client/login_rust_client.go",
			struct {
				UserID            string
				DeviceID          string
				Password          string
				BaseURL           string
				SSURL             string
				PersistentStorage bool
			}{
				UserID:            cfg.UserID,
				Password:          cfg.Password,
				DeviceID:          cfg.DeviceID,
				BaseURL:           cfg.BaseURL,
				PersistentStorage: cfg.PersistentStorage,
				SSURL:             tc.Deployment.SlidingSyncURL(t),
			})
		cmd.WaitDelay = 3 * time.Second
		defer close()
		waiter := helpers.NewWaiter()
		terminateClient = func() {
			terminated.Store(true)
			t.Logf("got keys/upload: terminating process %v", cmd.Process.Pid)
			if err := cmd.Process.Kill(); err != nil {
				t.Errorf("failed to kill process: %s", err)
				return
			}
			t.Logf("terminated process")
			waiter.Finish()
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Start()
		waiter.Waitf(t, 5*time.Second, "failed to terminate process")
		t.Logf("terminated process, making new client")
		// now make the same client
		alice := MustCreateClient(t, clientType, cfg, tc.Deployment.SlidingSyncURL(t))
		alice.Login(t, cfg) // login should work
		alice.Close(t)
		alice.DeletePersistentStorage(t)
		// ensure we see the 2nd keys/upload
		seenSecondKeysUploadWaiter.Wait(t, 3*time.Second)
	})
}

func xTestSigkillBeforeKeysUploadResponseJS(t *testing.T) {
	clientType := api.ClientType{Lang: api.ClientTypeJS, HS: "hs1"}
	var mu sync.Mutex
	var terminated atomic.Bool
	var terminateClient func()
	seenSecondKeysUploadWaiter := helpers.NewWaiter()
	callbackURL, close := deploy.NewCallbackServer(t, func(cd deploy.CallbackData) {
		if cd.Method == "OPTIONS" {
			return // ignore CORS
		}
		if terminated.Load() {
			// make sure the 2nd upload 200 OKs
			if cd.ResponseCode != 200 {
				// TODO: Errorf FIXME
				t.Logf("2nd /keys/upload did not 200 OK => got %v", cd.ResponseCode)
			}
			seenSecondKeysUploadWaiter.Finish()
			return
		}
		// destroy the client
		mu.Lock()
		terminateClient()
		mu.Unlock()
	})
	defer close()

	tc := CreateTestContext(t, clientType, clientType)
	tc.Deployment.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			"filter":       "~u .*\\/keys\\/upload.*",
		},
	}, func() {
		cfg := api.FromComplementClient(tc.Alice, "complement-crypto-password")
		cfg.BaseURL = tc.Deployment.ReverseProxyURLForHS(clientType.HS)
		cfg.PersistentStorage = true
		clientWhichWillBeKilled := MustCreateClient(t, clientType, cfg, tc.Deployment.SlidingSyncURL(t))
		// attempt to login, this should cause OTKs to be uploaded
		waiter := helpers.NewWaiter()
		terminateClient = func() {
			terminated.Store(true)
			t.Logf("got keys/upload: terminating browser")
			clientWhichWillBeKilled.Close(t)
			t.Logf("terminated browser")
			waiter.Finish()
		}
		go func() {
			must.NotError(t, "failed to login", clientWhichWillBeKilled.Login(t, cfg))
			// need to start syncing to make JS do /keys/upload
			// we don't need to stopSyncing because we'll SIGKILL this.
			clientWhichWillBeKilled.StartSyncing(t)
			t.Logf("clientWhichWillBeKilled.Login returned")
		}()
		waiter.Wait(t, 5*time.Second) // wait for /keys/upload and subsequent SIGKILL
		t.Logf("terminated browser, making new client")
		// now make the same client
		recreatedClient := MustCreateClient(t, clientType, cfg, tc.Deployment.SlidingSyncURL(t))
		recreatedClient.Login(t, cfg)   // login should work
		recreatedClient.StartSyncing(t) // ignore errors, we just need to kick it to /keys/upload
		recreatedClient.DeletePersistentStorage(t)
		recreatedClient.Close(t)
		// ensure we see the 2nd keys/upload
		seenSecondKeysUploadWaiter.Wait(t, 3*time.Second)
	})
}

// Test that if a client is unable to call /sendToDevice, it retries.
func TestClientRetriesSendToDevice(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, "public_chat", nil)
		tc.Bob.MustJoinRoom(t, roomID, []string{clientTypeA.HS})
		alice := tc.MustLoginClient(t, tc.Alice, clientTypeA)
		defer alice.Close(t)
		bob := tc.MustLoginClient(t, tc.Bob, clientTypeB)
		defer bob.Close(t)
		aliceStopSyncing := alice.MustStartSyncing(t)
		defer aliceStopSyncing()
		bobStopSyncing := bob.MustStartSyncing(t)
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
