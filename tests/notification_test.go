package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/must"
)

// https://github.com/element-hq/element-x-ios/blob/9bc24e2038f0381869237c6c6f7e5b68be9d3134/NSE/Sources/NotificationServiceExtension.swift#L21
// The lifecycle of the NSE looks something like the following:
//  1)  App receives notification
//  2)  System creates an instance of the extension class
//      and calls `didReceive` in the background
//  3)  Extension processes messages / displays whatever
//      notifications it needs to
//  4)  Extension notifies its work is complete by calling
//      the contentHandler
//  5)  If the extension takes too long to perform its work
//      (more than 30s), it will be notified and immediately
//      terminated
//
// Note that the NSE does *not* always spawn a new process to
// handle a new notification and will also try and process notifications
// in parallel. `didReceive` could be called twice for the same process,
// but it will always be called on different threads. It may or may not be
// called on the same instance of `NotificationService` as a previous
// notification.
//
// We keep a global `environment` singleton to ensure that our app context,
// database, logging, etc. are only ever setup once per *process*

// These tests try to trip up this logic by providing multiple notifications to a single process, etc.

func TestNSEReceive(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	tc, roomID := createAndJoinRoom(t)

	// login as Alice (uploads OTKs/device keys) and remember the access token for NSE
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage())
	alice.Logf(t, "syncing and sending dummy message to ensure e2ee keys are uploaded")
	stopSyncing := alice.MustStartSyncing(t)
	alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 5*time.Second, "did not see bob's join")
	alice.SendMessage(t, roomID, "test message to ensure E2EE keys are uploaded")
	accessToken := alice.Opts().AccessToken

	// app is "backgrounded" so we tidy things up
	alice.Logf(t, "stopping syncing and closing client to background the app")
	stopSyncing()
	alice.Close(t)

	// bob sends a message which we will be "pushed" for
	pushNotifEventID := ""
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		bob.Logf(t, "sending push notification message as bob")
		pushNotifEventID = bob.SendMessage(t, roomID, "push notification")
		bob.Logf(t, "sent push notification message as bob => %s", pushNotifEventID)
	})

	// now make the "NSE" process and get bob's message
	opts := tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS, WithPersistentStorage())
	opts.EnableCrossProcessRefreshLockProcessName = api.ProcessNameNSE
	opts.AccessToken = accessToken
	client := MustCreateClient(t, tc.AliceClientType, opts) // this should login already as we provided an access token
	// we don't sync in the NSE process, just call GetNotification
	notif, err := client.GetNotification(t, roomID, pushNotifEventID)
	must.NotError(t, "failed to get notification", err)
	must.Equal(t, notif.Text, "push notification", "failed to decrypt msg body")
	must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
}

// What happens if you get pushed for an event not in the SS repsonse?
func TestNSEReceiveForOldMessage(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	tc, roomID := createAndJoinRoom(t)

	// login as Alice (uploads OTKs/device keys) and remember the access token for NSE
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage())
	alice.Logf(t, "syncing and sending dummy message to ensure e2ee keys are uploaded")
	stopSyncing := alice.MustStartSyncing(t)
	alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 5*time.Second, "did not see bob's join")
	alice.SendMessage(t, roomID, "test message to ensure E2EE keys are uploaded")
	accessToken := alice.Opts().AccessToken

	// app is "backgrounded" so we tidy things up
	alice.Logf(t, "stopping syncing and closing client to background the app")
	stopSyncing()
	alice.Close(t)

	// bob sends a message which we will be "pushed" for
	pushNotifEventID := ""
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		bob.Logf(t, "sending push notification message as bob")
		pushNotifEventID = bob.SendMessage(t, roomID, "push notification")
		bob.Logf(t, "sent push notification message as bob => %s", pushNotifEventID)
		for i := 0; i < 30; i++ {
			// now send a bunch of other messages so sliding sync does not return  the event
			bob.SendMessage(t, roomID, fmt.Sprintf("msg %d", i))
		}
	})

	// now make the "NSE" process and get bob's OLD message
	opts := tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS, WithPersistentStorage())
	opts.EnableCrossProcessRefreshLockProcessName = api.ProcessNameNSE
	opts.AccessToken = accessToken
	client := MustCreateClient(t, tc.AliceClientType, opts) // this should login already as we provided an access token
	// we don't sync in the NSE process, just call GetNotification
	notif, err := client.GetNotification(t, roomID, pushNotifEventID)
	must.NotError(t, "failed to get notification", err)
	must.Equal(t, notif.Text, "push notification", "failed to decrypt msg body")
	must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
}

func createAndJoinRoom(t *testing.T) (tc *TestContext, roomID string) {
	clientType := api.ClientType{
		Lang: api.ClientTypeRust,
		HS:   "hs1",
	}
	tc = CreateTestContext(t, clientType, clientType)
	roomID = tc.CreateNewEncryptedRoom(
		t,
		tc.Alice,
		EncRoomOptions.PresetTrustedPrivateChat(),
		EncRoomOptions.Invite([]string{tc.Bob.UserID}),
	)
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
	return
}
