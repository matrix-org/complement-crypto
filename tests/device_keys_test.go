package tests

import (
	"github.com/matrix-org/gomatrixserverlib/spec"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement-crypto/internal/deploy/mitm"
	"github.com/matrix-org/complement/must"
)

// If a client cannot query device keys for a user, it retries.
//
// Block the first few requests to /keys/query so device key download fails.
// Create two users and ensure they can send encrypted messages to each other.
// This proves that device keys download requests get retried.
func TestFailedDeviceKeyDownloadRetries(t *testing.T) {
	Instance().ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := Instance().CreateTestContext(t, clientType, clientType)

		// Track whether we received any requests on /keys/query
		var queryReceived atomic.Bool

		// Given that the first 4 attempts to download device keys will fail
		tc.Deployment.MITM().Configure(t).WithIntercept(mitm.InterceptOpts{
			Filter: mitm.FilterParams{
				PathContains: "/keys/query",
				Method:       "POST",
			},
			RequestCallback: callback.SendError(4, http.StatusGatewayTimeout),
			ResponseCallback: func(data callback.Data) *callback.Response {
				queryReceived.Store(true)
				return nil
			},
		}, func() {
			// And Alice and Bob are in an encrypted room together
			roomID := tc.CreateNewEncryptedRoom(t, tc.Alice, cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}))
			tc.Bob.MustJoinRoom(t, roomID, []spec.ServerName{"hs1"})

			tc.WithAliceAndBobSyncing(t, func(alice, bob api.TestClient) {
				// When Alice sends a message
				alice.MustSendMessage(t, roomID, "checking whether we can send a message")

				// Then Bob should receive it
				bob.WaitUntilEventInRoom(
					t,
					roomID,
					api.CheckEventHasBody("checking whether we can send a message"),
				).Waitf(t, 5*time.Second, "bob did not see alice's decrypted message")

			})
		})

		// Sanity: we did receive some requests (which we initially blocked)
		must.Equal(t, queryReceived.Load(), true, "No request to /keys/query was received!")
	})
}
