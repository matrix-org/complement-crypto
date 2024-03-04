package tests

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	templates "github.com/matrix-org/complement-crypto/tests/go_templates"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

func TestSigkillBeforeKeysUploadResponse(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, a api.ClientType) {
		switch a.Lang {
		case api.ClientTypeRust:
			testSigkillBeforeKeysUploadResponseRust(t, a)
		case api.ClientTypeJS:
			testSigkillBeforeKeysUploadResponseJS(t, a)
		default:
			t.Fatalf("unknown lang: %s", a.Lang)
		}
	})
}

// Test that if the client is restarted BEFORE getting the /keys/upload response but
// AFTER the server has processed the request, the keys are not regenerated (which would
// cause duplicate key IDs with different keys). Requires persistent storage.
// Regression test for https://github.com/matrix-org/matrix-rust-sdk/issues/1415
func testSigkillBeforeKeysUploadResponseRust(t *testing.T, clientType api.ClientType) {
	var mu sync.Mutex
	var terminated atomic.Bool
	var terminateClient func()
	seenSecondKeysUploadWaiter := helpers.NewWaiter()
	tc := CreateTestContext(t, clientType, clientType)
	callbackURL, close := deploy.NewCallbackServer(t, tc.Deployment, func(cd deploy.CallbackData) {
		if terminated.Load() {
			// make sure the 2nd upload 200 OKs
			if cd.ResponseCode != 200 {
				// TODO: Errorf FIXME
				t.Logf("2nd /keys/upload did not 200 OK => got %v", cd.ResponseCode)
			}
			t.Logf("recv 2nd /keys/upload => HTTP %d", cd.ResponseCode)
			seenSecondKeysUploadWaiter.Finish()
			return
		}
		// destroy the client
		mu.Lock()
		terminateClient()
		mu.Unlock()
	})
	defer close()

	tc.Deployment.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			"filter":       "~u .*\\/keys\\/upload.*",
		},
	}, func() {
		cfg := api.NewClientCreationOpts(tc.Alice)
		cfg.PersistentStorage = true
		cfg.SlidingSyncURL = tc.Deployment.SlidingSyncURLForHS(t, clientType.HS)
		// run some code in a separate process so we can kill it later
		cmd, close := templates.PrepareGoScript(t, "testSigkillBeforeKeysUploadResponseRust/test.go",
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
				SSURL:             cfg.SlidingSyncURL,
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
		alice := MustCreateClient(t, clientType, cfg)
		alice.Login(t, cfg) // login should work
		stopSyncing := alice.MustStartSyncing(t)
		// ensure we see the 2nd keys/upload
		seenSecondKeysUploadWaiter.Wait(t, 5*time.Second)
		stopSyncing()
		alice.Close(t)
	})
}

func testSigkillBeforeKeysUploadResponseJS(t *testing.T, clientType api.ClientType) {
	var mu sync.Mutex
	var terminated atomic.Bool
	var terminateClient func()
	seenSecondKeysUploadWaiter := helpers.NewWaiter()
	tc := CreateTestContext(t, clientType, clientType)
	callbackURL, close := deploy.NewCallbackServer(t, tc.Deployment, func(cd deploy.CallbackData) {
		if cd.Method == "OPTIONS" {
			return // ignore CORS
		}
		if terminated.Load() {
			// make sure the 2nd upload 200 OKs
			if cd.ResponseCode != 200 {
				ct.Errorf(t, "2nd /keys/upload did not 200 OK => got %v", cd.ResponseCode)
			}
			seenSecondKeysUploadWaiter.Finish()
			return
		}
		// destroy the client
		mu.Lock()
		if terminateClient != nil {
			terminateClient()
		} else {
			ct.Errorf(t, "terminateClient is nil. Did WithMITMOptions lock?")
		}
		mu.Unlock()
	})
	defer close()

	tc.Deployment.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			"filter":       "~u .*\\/keys\\/upload.*",
		},
	}, func() {
		clientWhichWillBeKilled := tc.MustCreateClient(t, tc.Alice, clientType, WithPersistentStorage())
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
			must.NotError(t, "failed to login", clientWhichWillBeKilled.Login(t, clientWhichWillBeKilled.Opts()))
			// need to start syncing to make JS do /keys/upload
			// we don't need to stopSyncing because we'll SIGKILL this.
			clientWhichWillBeKilled.StartSyncing(t)
			t.Logf("clientWhichWillBeKilled.Login returned")
		}()
		waiter.Wait(t, 5*time.Second) // wait for /keys/upload and subsequent SIGKILL
		t.Logf("terminated browser, making new client")
		// now make the same client
		recreatedClient := tc.MustCreateClient(t, tc.Alice, clientType, WithPersistentStorage())
		recreatedClient.Login(t, recreatedClient.Opts()) // login should work
		recreatedClient.StartSyncing(t)                  // ignore errors, we just need to kick it to /keys/upload

		recreatedClient.DeletePersistentStorage(t)
		recreatedClient.Close(t)
		// ensure we see the 2nd keys/upload
		seenSecondKeysUploadWaiter.Wait(t, 3*time.Second)
	})
}
