package tests

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
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
	testNSEReceive(t, 0, 0)
}

// What happens if you get pushed for an event not in the SS response? It should hit /context.
func TestNSEReceiveForOldMessage(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	testNSEReceive(t, 0, 30)
}

// what happens if there's many events and you only get pushed for the last one?
func TestNSEReceiveForMessageWithManyUnread(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	testNSEReceive(t, 30, 0)
}

func testNSEReceive(t *testing.T, numMsgsBefore, numMsgsAfter int) {
	t.Helper()
	tc, roomID := createAndJoinRoom(t)

	// login as Alice (uploads OTKs/device keys) and remember the access token for NSE
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
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
	pushNotifEventID := bobSendsMessage(t, tc, roomID, "push notification", numMsgsBefore, numMsgsAfter)

	// now make the "NSE" process and get bob's message
	opts := tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS, WithPersistentStorage())
	opts.EnableCrossProcessRefreshLockProcessName = api.ProcessNameNSE
	opts.AccessToken = accessToken
	client := tc.MustCreateMultiprocessClient(t, tc.AliceClientType.Lang, opts) // this should login already as we provided an access token
	defer client.Close(t)
	// we don't sync in the NSE process, just call GetNotification
	notif, err := client.GetNotification(t, roomID, pushNotifEventID)
	must.NotError(t, "failed to get notification", err)
	must.Equal(t, notif.Text, "push notification", "failed to decrypt msg body")
	must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
}

// what happens if you receive an NSE event for a non-pre key message (i.e not the first encrypted msg sent by that user)
func TestNSEReceiveForNonPreKeyMessage(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	stopSyncing := alice.MustStartSyncing(t)
	// Bob sends a message to alice
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)
		// Send a message as Bob, this will contain ensure an Olm session is set up already before we do NSE work
		bob.SendMessage(t, roomID, "initial message")
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("initial message")).Waitf(t, 5*time.Second, "alice did not see bob's initial message")
		// Alice goes into the background
		accessToken := alice.Opts().AccessToken
		stopSyncing()
		alice.Close(t)
		// Bob sends another message which the NSE process will get
		eventID := bob.SendMessage(t, roomID, "for nse")
		bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID)).Waitf(t, 5*time.Second, "bob did not see his own message")
		// now make the "NSE" process and get bob's message
		opts := tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS, WithPersistentStorage())
		opts.EnableCrossProcessRefreshLockProcessName = api.ProcessNameNSE
		opts.AccessToken = accessToken
		client := MustCreateClient(t, tc.AliceClientType, opts) // this should login already as we provided an access token
		defer client.Close(t)
		// we don't sync in the NSE process, just call GetNotification
		notif, err := client.GetNotification(t, roomID, eventID)
		must.NotError(t, "failed to get notification", err)
		must.Equal(t, notif.Text, "for nse", "failed to decrypt msg body")
		must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
	})
}

// Get an encrypted room set up with keys exchanged, then concurrently receive messages and see if we end up with a wedged
// session. We should see "Crypto store generation mismatch" log lines in rust SDK.
func TestMultiprocessNSE(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	t.Skipf("TODO: skipped until backup bug is fixed")
	numPreBackgroundMsgs := 1
	numPostNSEMsgs := 300
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing to get an encrypted room set up
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	stopSyncing := alice.MustStartSyncing(t)
	accessToken := alice.Opts().AccessToken
	recoveryKey := alice.MustBackupKeys(t)
	var eventTimeline []string
	// Bob sends a message to alice
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)
		for i := 0; i < numPreBackgroundMsgs; i++ {
			msg := fmt.Sprintf("numPreBackgroundMsgs %d", i)
			bob.SendMessage(t, roomID, msg)
			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not see '%s'", msg)
		}

		stopAliceSyncing := func() {
			if alice == nil {
				t.Fatalf("stopAliceSyncing: alice was already not syncing")
			}
			alice.Close(t)
			stopSyncing()
			alice = nil
		}
		startAliceSyncing := func() {
			if alice != nil {
				t.Fatalf("startAliceSyncing: alice was already syncing")
			}
			alice = MustCreateClient(t, tc.AliceClientType, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
				WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock("main"),
			)) // this should login already as we provided an access token
			stopSyncing = alice.MustStartSyncing(t)
		}
		checkNSECanDecryptEvent := func(nseAlice api.Client, roomID, eventID, msg string) {
			notif, err := nseAlice.GetNotification(t, roomID, eventID)
			must.NotError(t, fmt.Sprintf("failed to get notification for event %s '%s'", eventID, msg), err)
			must.Equal(t, notif.Text, msg, fmt.Sprintf("NSE failed to decrypt event %s '%s' => %+v", eventID, msg, notif))
		}

		// set up the nse process. It doesn't actively keep a sync loop so we don't need to do the close dance with it.
		nseAlice := tc.MustCreateMultiprocessClient(t, tc.AliceClientType.Lang, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
			WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock(api.ProcessNameNSE),
		)) // this should login already as we provided an access token

		randomSource := rand.NewSource(2) // static seed for determinism

		// now bob will send lots of messages
		for i := 0; i < numPostNSEMsgs; i++ {
			if t.Failed() {
				t.Logf("bailing at iteration %d", i)
				break
			}
			// we want to emulate the handover of the lock between NSE and the App process.
			// For this to happen, we need decryption failures to happen on both processes.
			// If we always keep the main App process syncing, we will never see decryption failures on the NSE process.
			// We want to randomise this for maximum effect.
			restartAlice := randomSource.Int63()%2 == 0
			restartNSE := randomSource.Int63()%2 == 0
			nseOpensFirst := randomSource.Int63()%2 == 0
			aliceSendsMsg := randomSource.Int63()%2 == 0
			t.Logf("iteration %d restart app=%v nse=%v nse_open_first=%v alice_sends=%v", i, restartAlice, restartNSE, nseOpensFirst, aliceSendsMsg)
			if restartAlice {
				stopAliceSyncing()
			}
			if restartNSE {
				nseAlice.Close(t)
			}
			msg := fmt.Sprintf("numPostNSEMsgs %d", i)
			eventID := bob.SendMessage(t, roomID, msg)
			eventTimeline = append(eventTimeline, eventID)
			t.Logf("event %s => '%s'", eventID, msg)
			if restartNSE { // a new NSE process is created as a result of bob's message
				nseAlice = tc.MustCreateMultiprocessClient(t, tc.AliceClientType.Lang, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
					WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock(api.ProcessNameNSE),
				))
			} // else we reuse the same NSE process for bob's message

			// both the nse process and the app process should be able to decrypt the event
			if nseOpensFirst {
				checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
			}
			if restartAlice {
				t.Logf("restarting alice")
				startAliceSyncing()
			}
			if aliceSendsMsg { // this will cause the main app to update the crypto store
				sentEventID := alice.SendMessage(t, roomID, "dummy")
				eventTimeline = append(eventTimeline, sentEventID)
			}
			if !nseOpensFirst {
				checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
			}

			alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not decrypt '%s'", msg)
		}

		// let keys be backed up
		time.Sleep(time.Second)
		nseAlice.Close(t)
		stopAliceSyncing()
	})

	// do a new login to alice and use the recovery key
	newDevice := tc.MustRegisterNewDevice(t, tc.Alice, tc.AliceClientType.HS, "RESTORE")
	alice2 := tc.MustLoginClient(t, newDevice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	alice2.MustLoadBackup(t, recoveryKey)
	stopSyncing = alice2.MustStartSyncing(t)
	defer stopSyncing()
	// scrollback all the messages and check we can read them
	alice2.MustBackpaginate(t, roomID, len(eventTimeline))
	time.Sleep(time.Second)
	for _, eventID := range eventTimeline {
		ev := alice2.MustGetEvent(t, roomID, eventID)
		must.Equal(t, ev.FailedToDecrypt, false, fmt.Sprintf("failed to decrypt event ID %s : %+v", eventID, ev))
	}
}

func TestMultiprocessNSEBackupKeyMacError(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	t.Skipf("pending bugfix")
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing to get an encrypted room set up
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	stopSyncing := alice.MustStartSyncing(t)
	accessToken := alice.Opts().AccessToken
	recoveryKey := alice.MustBackupKeys(t)
	var eventTimeline []string

	// Bob sends a message to alice
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)

		stopAliceSyncing := func() {
			if alice == nil {
				t.Fatalf("stopAliceSyncing: alice was already not syncing")
			}
			alice.Close(t)
			stopSyncing()
			alice = nil
		}
		startAliceSyncing := func() {
			if alice != nil {
				t.Fatalf("startAliceSyncing: alice was already syncing")
			}
			alice = MustCreateClient(t, tc.AliceClientType, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
				WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock("main"),
			)) // this should login already as we provided an access token
			stopSyncing = alice.MustStartSyncing(t)
		}
		checkNSECanDecryptEvent := func(nseAlice api.Client, roomID, eventID, msg string) {
			notif, err := nseAlice.GetNotification(t, roomID, eventID)
			must.NotError(t, fmt.Sprintf("failed to get notification for event %s '%s'", eventID, msg), err)
			must.Equal(t, notif.Text, msg, fmt.Sprintf("NSE failed to decrypt event %s '%s' => %+v", eventID, msg, notif))
		}

		// set up the nse process. It doesn't actively keep a sync loop so we don't need to do the close dance with it.
		nseAlice := tc.MustCreateMultiprocessClient(t, tc.AliceClientType.Lang, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
			WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock(api.ProcessNameNSE),
		)) // this should login already as we provided an access token

		msg := "first message"
		eventID := bob.SendMessage(t, roomID, msg)
		eventTimeline = append(eventTimeline, eventID)
		t.Logf("first event %s => '%s'", eventID, msg)
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not decrypt '%s'", msg)

		// restart alice but keep nse process around
		stopAliceSyncing()

		// send finaly message
		msg = "final message"
		eventID = bob.SendMessage(t, roomID, msg)
		eventTimeline = append(eventTimeline, eventID)
		t.Logf("final event %s => '%s'", eventID, msg)

		// both the nse process and the app process should be able to decrypt the event
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)

		t.Logf("restarting alice")
		startAliceSyncing()
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not decrypt '%s'", msg)

		// let keys be backed up
		time.Sleep(time.Second)
		nseAlice.Close(t)
		stopAliceSyncing()
	})

	// do a new login to alice and use the recovery key
	newDevice := tc.MustRegisterNewDevice(t, tc.Alice, tc.AliceClientType.HS, "RESTORE")
	alice2 := tc.MustLoginClient(t, newDevice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	alice2.MustLoadBackup(t, recoveryKey)
	stopSyncing = alice2.MustStartSyncing(t)
	defer stopSyncing()
	// scrollback all the messages and check we can read them
	alice2.MustBackpaginate(t, roomID, len(eventTimeline))
	time.Sleep(time.Second)
	for _, eventID := range eventTimeline {
		ev := alice2.MustGetEvent(t, roomID, eventID)
		must.Equal(t, ev.FailedToDecrypt, false, fmt.Sprintf("failed to decrypt event using key from backup event ID %s : %+v", eventID, ev))
	}
}

func TestMultiprocessNSEOlmSessionWedge(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing to get an encrypted room set up
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	stopSyncing := alice.MustStartSyncing(t)
	accessToken := alice.Opts().AccessToken
	// Bob sends a message to alice
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)
		msg := "pre message"
		bob.SendMessage(t, roomID, msg)
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not see '%s'", msg)

		stopAliceSyncing := func() {
			t.Helper()
			if alice == nil {
				t.Fatalf("stopAliceSyncing: alice was already not syncing")
			}
			alice.Close(t)
			stopSyncing()
			alice = nil
		}
		startAliceSyncing := func() {
			t.Helper()
			if alice != nil {
				t.Fatalf("startAliceSyncing: alice was already syncing")
			}
			alice = MustCreateClient(t, tc.AliceClientType, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
				WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock("main"),
			)) // this should login already as we provided an access token
			stopSyncing = alice.MustStartSyncing(t)
		}
		checkNSECanDecryptEvent := func(nseAlice api.Client, roomID, eventID, msg string) {
			t.Helper()
			notif, err := nseAlice.GetNotification(t, roomID, eventID)
			must.NotError(t, fmt.Sprintf("failed to get notification for event %s '%s'", eventID, msg), err)
			must.Equal(t, notif.Text, msg, fmt.Sprintf("NSE failed to decrypt event %s '%s' => %+v", eventID, msg, notif))
			t.Logf("notif %+v", notif)
		}

		// set up the nse process. It doesn't actively keep a sync loop so we don't need to do the close dance with it.
		// Note we do not restart the NSE process in this test. This matches reality where the NSE process is often used
		// to process multiple push notifs one after the other.
		nseAlice := tc.MustCreateMultiprocessClient(t, tc.AliceClientType.Lang, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
			WithPersistentStorage(), WithAccessToken(accessToken), WithCrossProcessLock(api.ProcessNameNSE),
		)) // this should login already as we provided an access token

		stopAliceSyncing()
		msg = fmt.Sprintf("test message %d", 1)
		eventID := bob.SendMessage(t, roomID, msg)
		t.Logf("event %s => '%s'", eventID, msg)

		// both the nse process and the app process should be able to decrypt the event.
		// NSE goes first (as it's the push notif process)
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
		t.Logf("restarting alice")
		nseAlice.Logf(t, "post checkNSECanDecryptEvent")
		startAliceSyncing()
		alice.SendMessage(t, roomID, "dummy")

		// iteration 2
		stopAliceSyncing()
		msg = fmt.Sprintf("test message %d", 2)
		eventID = bob.SendMessage(t, roomID, msg)
		t.Logf("event %s => '%s'", eventID, msg)

		// both the nse process and the app process should be able to decrypt the event.
		// NSE goes first (as it's the push notif process)
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
		t.Logf("restarting alice")
		startAliceSyncing()
		alice.SendMessage(t, roomID, "dummy")

		nseAlice.Close(t)
		stopAliceSyncing()
	})
}

func TestMultiprocessDupeOTKUpload(t *testing.T) {
	if !ShouldTest(api.ClientTypeRust) {
		t.Skipf("rust only")
		return
	}
	t.Skipf("WIP")
	tc, roomID := createAndJoinRoom(t)

	// start the "main" app
	alice := tc.MustLoginClient(t, tc.Alice, tc.AliceClientType, WithPersistentStorage(), WithCrossProcessLock("main"))
	aliceAccessToken := alice.Opts().AccessToken

	// let OTKs be uploaded
	time.Sleep(time.Second)

	// prep nse process
	nseAlice := tc.MustCreateMultiprocessClient(t, tc.AliceClientType.Lang, tc.ClientCreationOpts(t, tc.Alice, tc.AliceClientType.HS,
		WithPersistentStorage(), WithAccessToken(aliceAccessToken), WithCrossProcessLock(api.ProcessNameNSE),
	))

	aliceUploadedNewKeys := false
	// artificially slow down the HTTP responses, such that we will have 2 in-flight /keys/upload requests
	// at once. If the NSE and main apps are talking to each other, they should be using the same key ID + key.
	// If not... well, that's a bug because then the client will forget one of these keys.
	tc.Deployment.WithSniffedEndpoint(t, "/keys/upload", func(cd deploy.CallbackData) {
		if cd.AccessToken != aliceAccessToken {
			return // let bob upload OTKs
		}
		aliceUploadedNewKeys = true
		if cd.ResponseCode != 200 {
			// we rely on the homeserver checking and rejecting when the same key ID is used with
			// different keys.
			t.Errorf("/keys/upload returned an error, duplicate key upload? %+v", cd)
		}
		// tarpit the response
		t.Logf("tarpitting keys/upload response for 4 seconds")
		time.Sleep(4 * time.Second)
	}, func() {
		var eventID string
		// Bob appears and sends a message, causing Bob to claim one of Alice's OTKs.
		// The main app will see this in /sync and then try to upload another OTK, which we will tarpit.
		tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
			eventID = bob.SendMessage(t, roomID, "Hello world!")
		})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { // nse process
			defer wg.Done()
			// wake up NSE process as if it got a push notification. Calling this function
			// should cause the NSE process to upload a OTK as it would have seen 1 has been used.
			// The NSE and main app must talk to each other to ensure they use the same key.
			nseAlice.Logf(t, "GetNotification %s, %s", roomID, eventID)
			notif, err := nseAlice.GetNotification(t, roomID, eventID)
			must.NotError(t, "failed to get notification", err)
			must.Equal(t, notif.Text, "Hello world!", "failed to decrypt msg body")
			must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
		}()
		go func() { // app process
			defer wg.Done()
			stopSyncing := alice.MustStartSyncing(t)
			// let alice upload new OTK
			time.Sleep(5 * time.Second)
			stopSyncing()
		}()
		wg.Wait()
	})
	if !aliceUploadedNewKeys {
		t.Errorf("Alice did not upload new OTKs")
	}
}

func createAndJoinRoom(t *testing.T) (tc *TestContext, roomID string) {
	t.Helper()
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
		// purposefully low rotation period to force room keys to be updated more frequently.
		// Wedged olm sessions can only happen when we send olm messages, which only happens
		// when we send new room keys!
		EncRoomOptions.RotationPeriodMsgs(1),
	)
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
	return
}

func bobSendsMessage(t *testing.T, tc *TestContext, roomID, text string, msgsBefore, msgsAfter int) (eventID string) {
	t.Helper()
	pushNotifEventID := ""
	tc.WithClientSyncing(t, tc.BobClientType, tc.Bob, func(bob api.Client) {
		for i := 0; i < msgsBefore; i++ {
			bob.SendMessage(t, roomID, fmt.Sprintf("msg before %d", i))
		}
		bob.Logf(t, "sending push notification message as bob")
		pushNotifEventID = bob.SendMessage(t, roomID, text)
		bob.Logf(t, "sent push notification message as bob => %s", pushNotifEventID)
		for i := 0; i < msgsAfter; i++ {
			bob.SendMessage(t, roomID, fmt.Sprintf("msg after %d", i))
		}
	})
	return pushNotifEventID
}
