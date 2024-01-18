package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
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
// - Claim the fallback key. Remember it.
// - Bob logs in, tries to talk to Alice, will have to claim fallback key. Ensure session works.
// - Unblock /keys/upload
// - Ensure fallback key is cycled by re-claiming all OTKs and the fallback key, ensure it isn't the same as the first fallback key.
// - Expected fail on SS versions <0.99.14
func TestFallbackKeyIsUsedIfOneTimeKeysRunOut(t *testing.T) {
	ClientTypeMatrix(t, func(t *testing.T, clientTypeA, clientTypeB api.ClientType) {
		tc := CreateTestContext(t, clientTypeA, clientTypeB)
		otkGobbler := tc.Deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
			LocalpartSuffix: "eater_of_keys",
			Password:        "complement-crypto-password",
		})

		// SDK testing below
		// =================

		// Upload OTKs and a fallback
		alice := LoginClientFromComplementClient(t, tc.Deployment, tc.Alice, clientTypeA)
		defer alice.Close(t)
		aliceStopSyncing := alice.MustStartSyncing(t)
		defer aliceStopSyncing()

		// we need to send _something_ to cause /sync v2 to return a long poll response, as fallback
		// keys don't wake up /sync v2. If we don't do this, rust SDK fails to realise it needs to upload a fallback
		// key because SS doesn't tell it, because Synapse doesn't tell SS that the fallback key was used.
		tc.Alice.MustCreateRoom(t, map[string]interface{}{})

		// also let bob upload OTKs before we block the upload endpoint!
		bob := LoginClientFromComplementClient(t, tc.Deployment, tc.Bob, clientTypeB)
		defer bob.Close(t)
		bobStopSyncing := bob.MustStartSyncing(t)
		defer bobStopSyncing()

		// Query OTK count so we know how many to consume
		res, _ := tc.Alice.MustSync(t, client.SyncReq{})
		otkCount := res.Get("device_one_time_keys_count.signed_curve25519").Int()
		t.Logf("uploaded otk count => %d", otkCount)

		var roomID string
		var fallbackKeyID string
		var fallbackKey gjson.Result
		var waiter api.Waiter
		// Block all /keys/upload requests
		tc.Deployment.WithMITMOptions(t, map[string]interface{}{
			"statuscode": map[string]interface{}{
				"return_status": http.StatusGatewayTimeout,
				"block_request": true,
				"filter":        "~u .*\\/keys\\/upload.*",
			},
		}, func() {
			// claim all OTKs
			mustClaimOTKs(t, otkGobbler, tc.Alice, int(otkCount))

			// now claim the fallback key
			fallbackKeyID, fallbackKey = mustClaimFallbackKey(t, otkGobbler, tc.Alice)

			// now bob tries to talk to alice, the fallback key should be used
			roomID = tc.CreateNewEncryptedRoom(t, tc.Bob, "public_chat", []string{tc.Alice.UserID})
			tc.Alice.MustJoinRoom(t, roomID, []string{clientTypeB.HS})
			w := bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(alice.UserID(), "join"))
			w.Wait(t, 5*time.Second)
			w = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(alice.UserID(), "join"))
			w.Wait(t, 5*time.Second)
			bob.SendMessage(t, roomID, "Hello world!")
			waiter = alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("Hello world!"))
			// ensure that /keys/upload is actually blocked (OTK count should be 0)
			res, _ := tc.Alice.MustSync(t, client.SyncReq{})
			otkCount := res.Get("device_one_time_keys_count.signed_curve25519").Int()
			must.Equal(t, otkCount, 0, "OTKs were uploaded when they should have been blocked by mitmproxy")
		})
		// rust sdk needs /keys/upload to 200 OK before it will decrypt the hello world msg
		waiter.Wait(t, 5*time.Second)

		// now /keys/upload is unblocked, make sure we upload new keys
		alice.SendMessage(t, roomID, "Kick the client to upload OTKs... hopefully")
		t.Logf("first fallback key %s => %s", fallbackKeyID, fallbackKey.Get("key").Str)

		tc.Alice.MustSyncUntil(t, client.SyncReq{}, func(clientUserID string, topLevelSyncJSON gjson.Result) error {
			otkCount = topLevelSyncJSON.Get("device_one_time_keys_count.signed_curve25519").Int()
			t.Logf("Alice otk count = %d", otkCount)
			if otkCount == 0 {
				return fmt.Errorf("alice hasn't re-uploaded OTKs yet")
			}
			return nil
		})

		// now re-block /keys/upload, re-claim all otks, and check that the fallback key this time around is different
		// to the first
		tc.Deployment.WithMITMOptions(t, map[string]interface{}{
			"statuscode": map[string]interface{}{
				"return_status": http.StatusGatewayTimeout,
				"block_request": true,
				"filter":        "~u .*\\/keys\\/upload.*",
			},
		}, func() {
			// claim all OTKs
			mustClaimOTKs(t, otkGobbler, tc.Alice, int(otkCount))

			// now claim the fallback key
			secondFallbackKeyID, secondFallbackKey := mustClaimFallbackKey(t, otkGobbler, tc.Alice)
			t.Logf("second fallback key %s => %s", secondFallbackKeyID, secondFallbackKey.Get("key").Str)
			must.NotEqual(t, secondFallbackKeyID, fallbackKeyID, "fallback key id same as before, not cycled?")
			must.NotEqual(t, fallbackKey.Get("key").Str, secondFallbackKey.Get("key").Str, "fallback key data same as before, not cycled?")
		})

	})
}
