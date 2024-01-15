package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// TODO: move to internal? or addons?!
type CallbackData struct {
	Method       string `json:"method"`
	URL          string `json:"url"`
	AccessToken  string `json:"access_token"`
	ResponseCode int    `json:"response_code"`
}

// TODO: move internally
func RunGoProcess(t *testing.T, templateFilename string, templateData any) (*exec.Cmd, func()) {
	tmpl, err := template.New(templateFilename).ParseFiles("./templates/" + templateFilename)
	if err != nil {
		api.Fatalf(t, "failed to parse template %s : %s", templateFilename, err)
	}
	scriptFile, err := os.CreateTemp("./templates", "script_*.go")
	if err != nil {
		api.Fatalf(t, "failed to open temporary file: %s", err)
	}
	defer scriptFile.Close()
	if err = tmpl.ExecuteTemplate(scriptFile, templateFilename, templateData); err != nil {
		api.Fatalf(t, "failed to execute template to file: %s", err)
	}
	// TODO: should we build output to the random number?
	// e.g go build -o ./templates/script ./templates/script_3523965439.go
	cmd := exec.Command("go", "build", "-o", "./templates/script", scriptFile.Name())
	t.Logf(cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build script %s: %s", scriptFile.Name(), err)
	}
	return exec.Command("./templates/script"), func() {
		os.Remove(scriptFile.Name())
		os.Remove("./templates/script")
	}
}

// Test that if the client is restarted BEFORE getting the /keys/upload response but
// AFTER the server has processed the request, the keys are not regenerated (which would
// cause duplicate key IDs with different keys). Requires persistent storage.
func TestSigkillBeforeKeysUploadResponse(t *testing.T) {
	for _, clientType := range []api.ClientType{{Lang: api.ClientTypeRust, HS: "hs1"}} { // {Lang: api.ClientTypeJS}
		t.Run(string(clientType.Lang), func(t *testing.T) {
			var mu sync.Mutex
			var terminated atomic.Bool
			var terminateClient func()
			// TODO: factor out to helper
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				var data CallbackData
				if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
					t.Logf("error decoding json: %s", err)
					w.WriteHeader(500)
					return
				}
				t.Logf("%v %+v", time.Now(), data)
				if terminated.Load() {
					// make sure the 2nd upload 200 OKs
					if data.ResponseCode != 200 {
						// TODO: Errorf
						t.Logf("2nd /keys/upload did not 200 OK => got %v", data.ResponseCode)
					}
					w.WriteHeader(200)
					return // 2nd /keys/upload should go through
				}
				// destroy the client
				mu.Lock()
				terminateClient()
				mu.Unlock()
				w.WriteHeader(200)
			})
			srv := http.Server{
				Addr:    ":6879",
				Handler: mux,
			}
			defer srv.Close()
			go srv.ListenAndServe()

			tc := CreateTestContext(t, clientType, clientType)
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"callback": map[string]interface{}{
					"callback_url": "http://host.docker.internal:6879",
					"filter":       "~u .*\\/keys\\/upload.*",
				},
			}, func() {
				cfg := api.FromComplementClient(tc.Alice, "complement-crypto-password")
				// run some code in a separate process so we can kill it later
				cmd, close := RunGoProcess(t, "sigkill_before_keys_upload_response.go",
					struct {
						UserID   string
						DeviceID string
						Password string
						BaseURL  string
						SSURL    string
					}{
						UserID:   cfg.UserID,
						Password: cfg.Password,
						DeviceID: cfg.DeviceID,
						BaseURL:  tc.Deployment.ReverseProxyURLForHS(clientType.HS),
						SSURL:    tc.Deployment.SlidingSyncURL(t),
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
				cfg.BaseURL = tc.Deployment.ReverseProxyURLForHS(clientType.HS)
				cfg.PersistentStorage = true
				alice := mustCreateClient(t, clientType, tc, cfg)
				alice.Login(t, cfg) // login should work
				alice.Close(t)
				alice.DeletePersistentStorage(t)
			})
		})
	}
}

func mustCreateClient(t *testing.T, clientType api.ClientType, tc *TestContext, cfg api.ClientCreationOpts) api.Client {
	switch clientType.Lang {
	case api.ClientTypeRust:
		client, err := rust.NewRustClient(t, cfg, tc.Deployment.SlidingSyncURL(t))
		must.NotError(t, "NewRustClient: %s", err)
		return client
	case api.ClientTypeJS:
		client, err := js.NewJSClient(t, cfg)
		must.NotError(t, "NewJSClient: %s", err)
		return client
	default:
		t.Fatalf("unknown client type %v", clientType)
	}
	panic("unreachable")
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
