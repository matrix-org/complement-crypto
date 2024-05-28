package tests

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/b"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/match"
	"github.com/matrix-org/complement/must"
	"github.com/tidwall/gjson"
)

func mustClaimFallbackKey(t *testing.T, claimer *client.CSAPI, target *client.CSAPI) (fallbackKeyID string, keyJSON gjson.Result) {
	t.Helper()
	res := claimer.MustDo(t, "POST", []string{
		"_matrix", "client", "v3", "keys", "claim",
	}, client.WithJSONBody(t, map[string]any{
		"one_time_keys": map[string]any{
			target.UserID: map[string]any{
				target.DeviceID: "signed_curve25519",
			},
		},
	}))
	defer res.Body.Close()
	result := must.ParseJSON(t, res.Body)
	otks := result.Get(fmt.Sprintf(
		"one_time_keys.%s.%s", client.GjsonEscape(target.UserID), client.GjsonEscape(target.DeviceID),
	))
	if !otks.Exists() {
		ct.Fatalf(t, "failed to claim a OTK for %s|%s: no entry exists in the response to /keys/claim, got %v", target.UserID, target.DeviceID, result.Raw)
	}
	fallbackKey := otks.Get("signed_curve25519*")
	// check it's the fallback key
	must.MatchGJSON(t, fallbackKey, match.JSONKeyEqual("fallback", true))
	for keyID := range otks.Map() {
		fallbackKeyID = keyID
	}
	return fallbackKeyID, fallbackKey
}

func mustClaimOTKs(t *testing.T, claimer *client.CSAPI, target *client.CSAPI, otkCount int) {
	t.Helper()
	for i := 0; i < otkCount; i++ {
		res := claimer.MustDo(t, "POST", []string{
			"_matrix", "client", "v3", "keys", "claim",
		}, client.WithJSONBody(t, map[string]any{
			"one_time_keys": map[string]any{
				target.UserID: map[string]any{
					target.DeviceID: "signed_curve25519",
				},
			},
		}))
		// check each key is not the fallback key
		must.MatchResponse(t, res, match.HTTPResponse{
			StatusCode: 200,
			JSON: []match.JSON{
				match.JSONKeyMissing(
					fmt.Sprintf(
						"one_time_keys.%s.%s.signed_curve25519*.fallback", client.GjsonEscape(target.UserID), client.GjsonEscape(target.DeviceID),
					),
				),
				match.JSONKeyPresent(fmt.Sprintf(
					"one_time_keys.%s.%s.signed_curve25519*", client.GjsonEscape(target.UserID), client.GjsonEscape(target.DeviceID),
				)),
			},
		})
	}
}

// - Alice logs in, uploads OTKs AND A FALLBACK KEY (which is what this is trying to test!)
// - Block all /keys/upload
// - Manually claim all OTKs in the test.
// - Claim the fallback key.
// - Bob logs in, tries to talk to Alice, will have to claim fallback key. Ensure session works.
// - Charlie logs in, tries to talk to Alice, will have to claim _the same fallback key_. Ensure session works.
func TestFallbackKeyIsUsedIfOneTimeKeysRunOut(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, keyProviderClientType, keyConsumerClientType api.ClientType) {
		tc := CreateTestContext(t, keyProviderClientType, keyConsumerClientType, keyConsumerClientType)
		otkGobbler := tc.Deployment.Register(t, keyConsumerClientType.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "eater_of_keys",
			Password:        "complement-crypto-password",
		})

		// SDK testing below
		// =================

		// Upload OTKs and a fallback
		tc.WithAliceBobAndCharlieSyncing(t, func(alice, bob, charlie api.Client) {
			// we need to send _something_ to cause /sync v2 to return a long poll response, as fallback
			// keys don't wake up /sync v2. If we don't do this, rust SDK fails to realise it needs to upload a fallback
			// key because SS doesn't tell it, because Synapse doesn't tell SS that the fallback key was used.
			tc.Alice.MustCreateRoom(t, map[string]interface{}{})

			// Query OTK count so we know how many to consume
			res, _ := tc.Alice.MustSync(t, client.SyncReq{})
			otkCount := res.Get("device_one_time_keys_count.signed_curve25519").Int()
			t.Logf("uploaded otk count => %d", otkCount)

			var roomID string
			var waiter api.Waiter
			// Block all /keys/upload requests for Alice
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"statuscode": map[string]interface{}{
					"return_status": http.StatusGatewayTimeout,
					"block_request": true,
					"filter":        "~u .*/keys/upload.* ~hq " + alice.CurrentAccessToken(t),
				},
			}, func() {
				// claim all OTKs
				mustClaimOTKs(t, otkGobbler, tc.Alice, int(otkCount))

				// now claim the fallback key
				fallbackKeyID, fallbackKey := mustClaimFallbackKey(t, otkGobbler, tc.Alice)
				t.Logf("claimed fallback key %s => %s", fallbackKeyID, fallbackKey.Raw)

				// now bob & charlie try to talk to alice, the fallback key should be used
				roomID = tc.CreateNewEncryptedRoom(
					t,
					tc.Bob,
					EncRoomOptions.PresetPublicChat(),
					EncRoomOptions.Invite([]string{tc.Alice.UserID, tc.Charlie.UserID}),
				)
				tc.Charlie.MustJoinRoom(t, roomID, []string{keyConsumerClientType.HS})
				tc.Alice.MustJoinRoom(t, roomID, []string{keyConsumerClientType.HS})
				charlie.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(alice.UserID(), "join")).Waitf(t, 5*time.Second, "charlie did not see alice's join")
				bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(alice.UserID(), "join")).Waitf(t, 5*time.Second, "bob did not see alice's join")
				alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(alice.UserID(), "join")).Waitf(t, 5*time.Second, "alice did not see own join")
				bob.SendMessage(t, roomID, "Hello world!")
				charlie.SendMessage(t, roomID, "Goodbye world!")
				waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("Hello world!"))
				// ensure that /keys/upload is actually blocked (OTK count should be 0)
				res, _ := tc.Alice.MustSync(t, client.SyncReq{})
				otkCount := res.Get("device_one_time_keys_count.signed_curve25519").Int()
				must.Equal(t, otkCount, 0, "OTKs were uploaded when they should have been blocked by mitmproxy")
			})
			// rust sdk needs /keys/upload to 200 OK before it will decrypt the hello world msg,
			// so only wait _after_ we have unblocked the endpoint.
			waiter.Waitf(t, 5*time.Second, "alice did not see bob's message")
			// check charlie's message is also here
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("Goodbye world!")).Waitf(t, 5*time.Second, "alice did not see charlie's message")

			// We do not check if the fallback key is cycled because some clients don't trust the server to tell them.
			// see https://github.com/matrix-org/matrix-rust-sdk/pull/3151
		})
	})
}

func TestFailedOneTimeKeyUploadRetries(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := CreateTestContext(t, clientType, clientType)
		// make a room so we can kick clients
		roomID := tc.Alice.MustCreateRoom(t, map[string]interface{}{"preset": "public_chat"})
		// block /keys/upload and make a client
		tc.Deployment.WithMITMOptions(t, map[string]interface{}{
			"statuscode": map[string]interface{}{
				"return_status": http.StatusGatewayTimeout,
				"block_request": true,
				"count":         2, // block it twice.
				"filter":        "~u .*\\/keys\\/upload.* ~m POST",
			},
		}, func() {
			tc.WithAliceSyncing(t, func(alice api.Client) {
				tc.Bob.MustDo(t, "POST", []string{
					"_matrix", "client", "v3", "keys", "claim",
				}, client.WithJSONBody(t, map[string]any{
					"one_time_keys": map[string]any{
						tc.Alice.UserID: map[string]any{
							tc.Alice.DeviceID: "signed_curve25519",
						},
					},
				}), client.WithRetryUntil(10*time.Second, func(res *http.Response) bool {
					jsonBody := must.ParseJSON(t, res.Body)
					res.Body.Close()
					err := match.JSONKeyPresent(
						fmt.Sprintf("one_time_keys.%s.%s.signed_curve25519*", tc.Alice.UserID, tc.Alice.DeviceID),
					)(jsonBody)
					if err == nil {
						return true
					}
					t.Logf("failed to claim otk: /keys/claim => %v", jsonBody.Raw)
					// try kicking the client by sending some data down /sync
					// Specifically, JS SDK needs this. Rust has its own backoff independent to /sync
					tc.Alice.SendEventSynced(t, roomID, b.Event{
						Type: "m.room.message",
						Content: map[string]interface{}{
							"msgtype": "m.text",
							"body":    "this is a kick to try to get clients to retry /keys/upload",
						},
					})
					return false
				}))
			})
		})
	})
}

func TestFailedKeysClaimRetries(t *testing.T) {
	ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := CreateTestContext(t, clientType, clientType)
		// both clients start syncing to upload OTKs
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.Client) {
			var stopPoking atomic.Bool
			waiter := helpers.NewWaiter()
			callbackURL, close := deploy.NewCallbackServer(t, tc.Deployment, func(cd deploy.CallbackData) {
				t.Logf("%+v", cd)
				if cd.ResponseCode == 200 {
					waiter.Finish()
					stopPoking.Store(true)
				}
			})
			defer close()

			// make a room which will link the 2 users together when
			roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, EncRoomOptions.PresetPublicChat())
			// block /keys/claim and join the room, causing the Olm session to be created
			tc.Deployment.WithMITMOptions(t, map[string]interface{}{
				"statuscode": map[string]interface{}{
					"return_status": http.StatusGatewayTimeout,
					"block_request": true,
					"count":         2, // block it twice.
					"filter":        "~u .*\\/keys\\/claim.* ~m POST",
				},
				"callback": map[string]interface{}{
					"callback_url": callbackURL,
					"filter":       "~u .*\\/keys\\/claim.* ~m POST",
				},
			}, func() {
				// join the room. This should cause an Olm session to be made but it will fail as we cannot
				// call /keys/claim. We should retry though.
				tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
				time.Sleep(time.Second) // FIXME using WaitUntilEventInRoom panics on rust because the room isn't there yet
				bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 5*time.Second, "bob did not see own join event")

				// Now send a message. On Rust, just sending 1 msg is enough to kick retry schedule.
				// JS SDK won't retry the /keys/claim automatically. Try sending another event to kick it.
				counter := 0
				for !stopPoking.Load() && counter < 10 {
					bob.TrySendMessage(t, roomID, "poke msg")
					counter++
					time.Sleep(100 * time.Millisecond * time.Duration(counter+1))
				}
			})
			waiter.Wait(t, 10*time.Second)
		})
	})
}
