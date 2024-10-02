package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement-crypto/internal/deploy/mitm"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// Test that clients wait for invites to be processed before sending encrypted messages.
// Consider:
//   - Alice is in a E2EE room and invites Bob, the request has yet to 200 OK.
//   - Alice tries to send a message in the room. This should be queued behind the invite.
//     If it is not, the message will not be encrypted for Bob.
//
// It is valid for SDKs to simply document that you shouldn't call Invite and SendMessage concurrently.
// Therefore, we will not test this.
//
// However, consider:
//   - Alice is in a E2EE room and invites Bob. The request 200 OKs but has yet to come down /sync.
//   - Alice tries to send a message in the room.
//   - Alice should encrypt for Bob.
//
// This is much more realistic, as servers are typically asynchronous internally so /invite can 200 OK
// _before_ it comes down /sync.
func TestDelayedInviteResponse(t *testing.T) {
	Instance().ForEachClientType(t, func(t *testing.T, clientType api.ClientType) {
		tc := Instance().CreateTestContext(t, clientType, clientType)
		roomID := tc.CreateNewEncryptedRoom(t, tc.Alice)
		tc.WithAliceAndBobSyncing(t, func(alice, bob api.TestClient) {
			// we send a message first so clients which lazily call /members can do so now.
			// if we don't do this, the client won't rely on /sync for the member list so won't fail.
			alice.MustSendMessage(t, roomID, "dummy message to make /members call")

			config := tc.Deployment.MITM().Configure(t)
			serverHasInvite := helpers.NewWaiter()
			config.WithIntercept(mitm.InterceptOpts{
				Filter: mitm.FilterParams{
					PathContains: "/sync",
					AccessToken:  alice.CurrentAccessToken(t),
				},
				ResponseCallback: func(cd callback.Data) *callback.Response {
					if strings.Contains(
						strings.ReplaceAll(string(cd.ResponseBody), " ", ""),
						`"membership":"invite"`,
					) {
						t.Logf("/sync => %v", string(cd.ResponseBody))
						delayTime := 3 * time.Second
						t.Logf("intercepted /sync response which has the invite, tarpitting for %v - %v", delayTime, cd)
						serverHasInvite.Finish()
						time.Sleep(delayTime)
					}
					return nil
				},
			}, func() {
				t.Logf("Alice about to /invite Bob")
				if err := alice.InviteUser(t, roomID, bob.UserID()); err != nil {
					ct.Errorf(t, "failed to invite user: %s", err)
				}
				t.Logf("Alice /invited Bob")
				// once the server got the invite, send a message
				serverHasInvite.Waitf(t, 3*time.Second, "did not intercept invite")
				t.Logf("intercepted invite; sending message")
				eventID := alice.MustSendMessage(t, roomID, "hello world!")

				// bob joins, ensure he can decrypt the message.
				tc.Bob.JoinRoom(t, roomID, []string{clientType.HS})
				bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 7*time.Second, "did not see own join")
				bob.MustBackpaginate(t, roomID, 3)

				time.Sleep(time.Second) // let things settle / decrypt

				ev := bob.MustGetEvent(t, roomID, eventID)

				// TODO: FIXME fix this issue in the SDK
				// -
				//
				if ev.FailedToDecrypt || ev.Text != "hello world!" {
					if clientType.Lang == api.ClientTypeRust {
						t.Skipf("known broken: see https://github.com/matrix-org/matrix-rust-sdk/issues/3622")
					}
					if clientType.Lang == api.ClientTypeJS {
						t.Skipf("known broken: see https://github.com/matrix-org/matrix-js-sdk/issues/4291")
					}
				}
				must.Equal(t, ev.FailedToDecrypt, false, "failed to decrypt event")
				must.Equal(t, ev.Text, "hello world!", "failed to decrypt plaintext")
			})
		})
	})
}
