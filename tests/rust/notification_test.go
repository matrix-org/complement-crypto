package rust_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement-crypto/internal/deploy/mitm"
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
	testNSEReceive(t, 0, 0)
}

// What happens if you get pushed for an event not in the SS response? It should hit /context.
func TestNSEReceiveForOldMessage(t *testing.T) {
	testNSEReceive(t, 0, 30)
}

// what happens if there's many events and you only get pushed for the last one?
func TestNSEReceiveForMessageWithManyUnread(t *testing.T) {
	testNSEReceive(t, 30, 0)
}

func testNSEReceive(t *testing.T, numMsgsBefore, numMsgsAfter int) {
	t.Helper()
	tc, roomID := createAndJoinRoom(t)

	// login as Alice (uploads OTKs/device keys) and remember the access token for NSE
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
	alice.Logf(t, "syncing and sending dummy message to ensure e2ee keys are uploaded")
	stopSyncing := alice.MustStartSyncing(t)
	alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasMembership(tc.Bob.UserID, "join")).Waitf(t, 5*time.Second, "did not see bob's join")
	alice.MustSendMessage(t, roomID, "test message to ensure E2EE keys are uploaded")
	accessToken := alice.Opts().AccessToken

	// app is "backgrounded" so we tidy things up
	alice.Logf(t, "stopping syncing and closing client to background the app")
	stopSyncing()
	alice.Close(t)

	// bob sends a message which we will be "pushed" for
	pushNotifEventID := bobSendsMessage(t, tc, roomID, "push notification", numMsgsBefore, numMsgsAfter)

	// now make the "NSE" process and get bob's message
	client := tc.MustCreateClient(t, &cc.ClientCreationRequest{
		User:         tc.Alice,
		Multiprocess: true,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: rust.ProcessNameNSE,
			},
			AccessToken: accessToken,
		},
	}) // this should login already as we provided an access token
	defer client.Close(t)
	// we don't sync in the NSE process, just call GetNotification
	notif, err := client.GetNotification(t, roomID, pushNotifEventID)
	must.NotError(t, "failed to get notification", err)
	must.Equal(t, notif.Text, "push notification", "failed to decrypt msg body")
	must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
}

// what happens if you receive an NSE event for a non-pre key message (i.e not the first encrypted msg sent by that user)
func TestNSEReceiveForNonPreKeyMessage(t *testing.T) {
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
	stopSyncing := alice.MustStartSyncing(t)
	// Bob sends a message to alice
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.TestClient) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)
		// Send a message as Bob, this will contain ensure an Olm session is set up already before we do NSE work
		bob.MustSendMessage(t, roomID, "initial message")
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody("initial message")).Waitf(t, 5*time.Second, "alice did not see bob's initial message")
		// Alice goes into the background
		accessToken := alice.Opts().AccessToken
		stopSyncing()
		alice.Close(t)
		// Bob sends another message which the NSE process will get
		eventID := bob.MustSendMessage(t, roomID, "for nse")
		bob.WaitUntilEventInRoom(t, roomID, api.CheckEventHasEventID(eventID)).Waitf(t, 5*time.Second, "bob did not see his own message")
		// now make the "NSE" process and get bob's message
		client := tc.MustCreateClient(t, &cc.ClientCreationRequest{
			User: tc.Alice,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
				ExtraOpts: map[string]any{
					rust.CrossProcessStoreLocksHolderName: rust.ProcessNameNSE,
				},
				AccessToken: accessToken,
			},
		}) // this should login already as we provided an access token
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
	t.Skipf("TODO: skipped until backup bug is fixed")
	numPreBackgroundMsgs := 1
	numPostNSEMsgs := 300
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing to get an encrypted room set up
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
	stopSyncing := alice.MustStartSyncing(t)
	accessToken := alice.Opts().AccessToken
	recoveryKey := alice.MustBackupKeys(t)
	var eventTimeline []string
	// Bob sends a message to alice
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.TestClient) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)
		for i := 0; i < numPreBackgroundMsgs; i++ {
			msg := fmt.Sprintf("numPreBackgroundMsgs %d", i)
			bob.MustSendMessage(t, roomID, msg)
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
			alice = tc.MustCreateClient(t, &cc.ClientCreationRequest{
				User: tc.Alice,
				Opts: api.ClientCreationOpts{
					PersistentStorage: true,
					ExtraOpts: map[string]any{
						rust.CrossProcessStoreLocksHolderName: "main",
					},
					AccessToken: accessToken,
				},
			}) // this should login already as we provided an access token
			stopSyncing = alice.MustStartSyncing(t)
		}
		checkNSECanDecryptEvent := func(nseAlice api.Client, roomID, eventID, msg string) {
			notif, err := nseAlice.GetNotification(t, roomID, eventID)
			must.NotError(t, fmt.Sprintf("failed to get notification for event %s '%s'", eventID, msg), err)
			must.Equal(t, notif.Text, msg, fmt.Sprintf("NSE failed to decrypt event %s '%s' => %+v", eventID, msg, notif))
		}

		// set up the nse process. It doesn't actively keep a sync loop so we don't need to do the close dance with it.
		nseAlice := tc.MustCreateClient(t, &cc.ClientCreationRequest{
			User:         tc.Alice,
			Multiprocess: true,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
				AccessToken:       accessToken,
				ExtraOpts: map[string]any{
					rust.CrossProcessStoreLocksHolderName: "main",
				},
			},
		}) // this should login already as we provided an access token

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
			eventID := bob.MustSendMessage(t, roomID, msg)
			eventTimeline = append(eventTimeline, eventID)
			t.Logf("event %s => '%s'", eventID, msg)
			if restartNSE { // a new NSE process is created as a result of bob's message
				nseAlice = tc.MustCreateClient(t, &cc.ClientCreationRequest{
					User:         tc.Alice,
					Multiprocess: true,
					Opts: api.ClientCreationOpts{
						PersistentStorage: true,
						ExtraOpts: map[string]any{
							rust.CrossProcessStoreLocksHolderName: rust.ProcessNameNSE,
						},
						AccessToken: accessToken,
					},
				})
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
				sentEventID := alice.MustSendMessage(t, roomID, "dummy")
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
	newDevice := tc.MustRegisterNewDevice(t, tc.Alice, "RESTORE")
	alice2 := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: newDevice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
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
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing to get an encrypted room set up
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
	stopSyncing := alice.MustStartSyncing(t)
	accessToken := alice.Opts().AccessToken
	recoveryKey := alice.MustBackupKeys(t)
	var eventTimeline []string

	// Bob sends a message to alice
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.TestClient) {
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
			alice = tc.MustCreateClient(t, &cc.ClientCreationRequest{
				User: tc.Alice,
				Opts: api.ClientCreationOpts{
					PersistentStorage: true,
					AccessToken:       accessToken,
					ExtraOpts: map[string]any{
						rust.CrossProcessStoreLocksHolderName: "main",
					},
				},
			}) // this should login already as we provided an access token
			stopSyncing = alice.MustStartSyncing(t)
		}
		checkNSECanDecryptEvent := func(nseAlice api.Client, roomID, eventID, msg string) {
			notif, err := nseAlice.GetNotification(t, roomID, eventID)
			must.NotError(t, fmt.Sprintf("failed to get notification for event %s '%s'", eventID, msg), err)
			must.Equal(t, notif.Text, msg, fmt.Sprintf("NSE failed to decrypt event %s '%s' => %+v", eventID, msg, notif))
		}

		// set up the nse process. It doesn't actively keep a sync loop so we don't need to do the close dance with it.
		nseAlice := tc.MustCreateClient(t, &cc.ClientCreationRequest{
			User:         tc.Alice,
			Multiprocess: true,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
				ExtraOpts: map[string]any{
					rust.CrossProcessStoreLocksHolderName: rust.ProcessNameNSE,
				},
				AccessToken: accessToken,
			},
		}) // this should login already as we provided an access token

		msg := "first message"
		eventID := bob.MustSendMessage(t, roomID, msg)
		eventTimeline = append(eventTimeline, eventID)
		t.Logf("first event %s => '%s'", eventID, msg)
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not decrypt '%s'", msg)

		// restart alice but keep nse process around
		stopAliceSyncing()

		// send final message
		msg = "final message"
		eventID = bob.MustSendMessage(t, roomID, msg)
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
	newDevice := tc.MustRegisterNewDevice(t, tc.Alice, "RESTORE")
	alice2 := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: newDevice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
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
	tc, roomID := createAndJoinRoom(t)
	// Alice starts syncing to get an encrypted room set up
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
	stopSyncing := alice.MustStartSyncing(t)
	accessToken := alice.Opts().AccessToken
	// Bob sends a message to alice
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.TestClient) {
		// let bob realise alice exists and claims keys
		time.Sleep(time.Second)
		msg := "pre message"
		bob.MustSendMessage(t, roomID, msg)
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
			alice = tc.MustCreateClient(t, &cc.ClientCreationRequest{
				User: tc.Alice,
				Opts: api.ClientCreationOpts{
					PersistentStorage: true,
					ExtraOpts: map[string]any{
						rust.CrossProcessStoreLocksHolderName: "main",
					},
					AccessToken: accessToken,
				},
			}) // this should login already as we provided an access token
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
		nseAlice := tc.MustCreateClient(t, &cc.ClientCreationRequest{
			User:         tc.Alice,
			Multiprocess: true,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
				AccessToken:       accessToken,
				ExtraOpts: map[string]any{
					rust.CrossProcessStoreLocksHolderName: rust.ProcessNameNSE,
				},
			},
		}) // this should login already as we provided an access token

		stopAliceSyncing()
		msg = fmt.Sprintf("test message %d", 1)
		eventID := bob.MustSendMessage(t, roomID, msg)
		t.Logf("event %s => '%s'", eventID, msg)

		// both the nse process and the app process should be able to decrypt the event.
		// NSE goes first (as it's the push notif process)
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
		t.Logf("restarting alice")
		nseAlice.Logf(t, "post checkNSECanDecryptEvent")
		startAliceSyncing()
		alice.MustSendMessage(t, roomID, "dummy")

		// iteration 2
		stopAliceSyncing()
		msg = fmt.Sprintf("test message %d", 2)
		eventID = bob.MustSendMessage(t, roomID, msg)
		t.Logf("event %s => '%s'", eventID, msg)

		// both the nse process and the app process should be able to decrypt the event.
		// NSE goes first (as it's the push notif process)
		checkNSECanDecryptEvent(nseAlice, roomID, eventID, msg)
		t.Logf("restarting alice")
		startAliceSyncing()
		alice.MustSendMessage(t, roomID, "dummy")

		nseAlice.Close(t)
		stopAliceSyncing()
	})
}

// Test that the notification client doesn't cause duplicate OTK uploads.
// Regression test for https://github.com/matrix-org/matrix-rust-sdk/issues/1415
//
// This test creates a normal Alice rust client and lets it upload OTKs. It then:
//   - hooks into /keys/upload requests and artificially delays them by adding 4s of latency
//   - creates a Bob client who sends a message to Alice, consuming 1 OTK in the process
//   - immediately calls GetNotification on Bob's event as soon as it 200 OKs, which creates
//     a NotificationClient inside the same process.
//
// This means there are 2 sync loops: the main Client and the NotificationClient. Both clients
// will see the OTK count being lowered so both may try to upload a new OTK. Because we are
// delaying upload requests by 4s, this increases the chance of both uploads being in-flight at
// the same time. If they do not synchronise this operation, they will both try to upload
// _different keys_ with the _same_ key ID, which causes synapse to HTTP 400 with:
//
//	> One time key signed_curve25519:AAAAAAAAADI already exists
//
// Which will fail the test.
func TestNotificationClientDupeOTKUpload(t *testing.T) {
	tc, roomID := createAndJoinRoom(t)

	// start the "main" app
	alice := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Alice,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
		},
	})
	stopSyncing := alice.MustStartSyncing(t)
	defer stopSyncing()
	aliceAccessToken := alice.Opts().AccessToken

	aliceUploadedNewKeys := false
	// artificially slow down the HTTP responses, such that we will potentially have 2 in-flight /keys/upload requests
	// at once. If the NSE and main apps are talking to each other, they should be using the same key ID + key.
	// If not... well, that's a bug because then the client will forget one of these keys.
	tc.Deployment.MITM().Configure(t).WithIntercept(mitm.InterceptOpts{
		Filter: mitm.FilterParams{
			PathContains: "/keys/upload",
		},
		ResponseCallback: func(cd callback.Data) *callback.Response {
			if cd.AccessToken != aliceAccessToken {
				return nil // let bob upload OTKs
			}
			aliceUploadedNewKeys = true
			if cd.ResponseCode != 200 {
				// we rely on the homeserver checking and rejecting when the same key ID is used with
				// different keys.
				t.Errorf("/keys/upload returned an error, duplicate key upload? %+v => %v", cd, string(cd.ResponseBody))
			}
			// tarpit the response
			t.Logf("tarpitting keys/upload response for 4 seconds")
			time.Sleep(4 * time.Second)
			return nil
		},
	}, func() {
		// Bob appears and sends a message, causing Bob to claim one of Alice's OTKs.
		// The main app will see this in /sync and then try to upload another OTK, which we will tarpit.
		tc.WithClientSyncing(t, &cc.ClientCreationRequest{
			User: tc.Bob,
		}, func(bob api.TestClient) {
			eventID := bob.MustSendMessage(t, roomID, "Hello world!")
			// create a NotificationClient in the same process to fetch this "push notification".
			// It might make the NotificationClient upload a OTK as it would have seen 1 has been used.
			// The NotificationClient and main Client must talk to each other to ensure they use the same key.
			alice.Logf(t, "GetNotification %s, %s", roomID, eventID)
			notif, err := alice.GetNotification(t, roomID, eventID)
			must.NotError(t, "failed to get notification", err)
			must.Equal(t, notif.Text, "Hello world!", "failed to decrypt msg body")
			must.Equal(t, notif.FailedToDecrypt, false, "FailedToDecrypt but we should be able to decrypt")
		})
	})
	if !aliceUploadedNewKeys {
		t.Errorf("Alice did not upload new OTKs")
	}
}

// Regression test for https://github.com/matrix-org/matrix-rust-sdk/issues/3959
//
// In SS, doing an initial (pos-less) 'e2ee' connection did not cause device list updates
// to be dropped. However, in SSS they do. This test ensures that device list updates
// are not dropped in SSS, meaning that the SDK isn't doing initial (pos-less) syncs in
// the push process. It does this by doing the following:
//   - Alice[1] and Bob are in a room and Bob sends a message to Alice
//     (ensuring Bob has an up-to-date device list for Alice)
//   - Bob stops syncing.
//   - Alice[2] logs in on a new device.
//   - Alice[1] sends a message.
//   - Bob's push process receives Alice[1]'s message.
//   - At this point, the old code would do a pos-less sync, clearing device list updates
//     and forgetting that Alice[2] exists!
//   - Bob sends a message.
//   - Ensure Alice[2] can read it.
func TestMultiprocessInitialE2EESyncDoesntDropDeviceListUpdates(t *testing.T) {
	tc, roomID := createAndJoinRoom(t)
	bob := tc.MustLoginClient(t, &cc.ClientCreationRequest{
		User: tc.Bob,
		Opts: api.ClientCreationOpts{
			PersistentStorage: true,
			ExtraOpts: map[string]any{
				rust.CrossProcessStoreLocksHolderName: "main",
			},
		},
	})
	stopSyncing := bob.MustStartSyncing(t)
	accessToken := bob.Opts().AccessToken
	// Bob sends a message to Alice
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Alice,
	}, func(alice api.TestClient) {
		// ensure bob has queried keys from alice by sending a message.
		msg := "pre message"
		bob.MustSendMessage(t, roomID, msg)
		alice.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(msg)).Waitf(t, 5*time.Second, "alice did not see '%s'", msg)

		stopBobSyncing := func() {
			t.Helper()
			if bob == nil {
				t.Fatalf("stopBobSyncing: bob was already not syncing")
			}
			bob.Close(t)
			stopSyncing()
			bob = nil
		}
		startBobSyncing := func() {
			t.Helper()
			if bob != nil {
				t.Fatalf("startBobSyncing: bob was already syncing")
			}
			bob = tc.MustCreateClient(t, &cc.ClientCreationRequest{
				User: tc.Bob,
				Opts: api.ClientCreationOpts{
					PersistentStorage: true,
					ExtraOpts: map[string]any{
						rust.CrossProcessStoreLocksHolderName: "main",
					},
					AccessToken: accessToken,
				},
			}) // this should login already as we provided an access token
			stopSyncing = bob.MustStartSyncing(t)
		}
		nseBob := tc.MustCreateClient(t, &cc.ClientCreationRequest{
			User:         tc.Bob,
			Multiprocess: true,
			Opts: api.ClientCreationOpts{
				PersistentStorage: true,
				AccessToken:       accessToken,
				ExtraOpts: map[string]any{
					rust.CrossProcessStoreLocksHolderName: rust.ProcessNameNSE,
				},
			},
		}) // this should login already as we provided an access token

		// ensure any outstanding key requests have time to complete before we shut it down
		time.Sleep(time.Second)
		stopBobSyncing()

		// alice logs in on a new device
		csapiAlice2 := tc.MustRegisterNewDevice(t, tc.Alice, "NEW_DEVICE")
		tc.WithClientSyncing(t, &cc.ClientCreationRequest{
			User: csapiAlice2,
		}, func(alice2 api.TestClient) {
			// wait for device keys to sync up
			time.Sleep(time.Second)
			// alice[1] sends a message, this is unimportant other than to grab the event ID for the push process
			pushEventID := alice.MustSendMessage(t, roomID, "pre message 2")
			// Bob's push process receives Alice[1]'s message.
			// This /should/ make Bob aware of Alice[2].
			notif, err := nseBob.GetNotification(t, roomID, pushEventID)
			must.NotError(t, "failed to GetNotification", err)
			must.Equal(t, notif.FailedToDecrypt, false, "failed to decrypt push event")
			// grace period to let bob realise alice[2] exists
			time.Sleep(time.Second)
			// Bob opens the app, with another grace period because we're feeling nice.
			startBobSyncing()
			defer stopBobSyncing()
			time.Sleep(time.Second)
			// Bob sends a message.
			wantMsg := "can alice's new device decrypt this?"
			bob.MustSendMessage(t, roomID, wantMsg)
			alice2.WaitUntilEventInRoom(t, roomID, api.CheckEventHasBody(wantMsg)).Waitf(t, 5*time.Second, "alice[2] did not see '%s'", wantMsg)
		})
	})
}

func createAndJoinRoom(t *testing.T) (tc *cc.TestContext, roomID string) {
	t.Helper()
	clientType := api.ClientType{
		Lang: api.ClientTypeRust,
		HS:   "hs1",
	}
	tc = Instance().CreateTestContext(t, clientType, clientType)
	roomID = tc.CreateNewEncryptedRoom(
		t,
		tc.Alice,
		cc.EncRoomOptions.PresetTrustedPrivateChat(),
		cc.EncRoomOptions.Invite([]string{tc.Bob.UserID}),
		// purposefully low rotation period to force room keys to be updated more frequently.
		// Wedged olm sessions can only happen when we send olm messages, which only happens
		// when we send new room keys!
		cc.EncRoomOptions.RotationPeriodMsgs(1),
	)
	tc.Bob.MustJoinRoom(t, roomID, []string{clientType.HS})
	return
}

func bobSendsMessage(t *testing.T, tc *cc.TestContext, roomID, text string, msgsBefore, msgsAfter int) (eventID string) {
	t.Helper()
	pushNotifEventID := ""
	tc.WithClientSyncing(t, &cc.ClientCreationRequest{
		User: tc.Bob,
	}, func(bob api.TestClient) {
		for i := 0; i < msgsBefore; i++ {
			bob.MustSendMessage(t, roomID, fmt.Sprintf("msg before %d", i))
		}
		bob.Logf(t, "sending push notification message as bob")
		pushNotifEventID = bob.MustSendMessage(t, roomID, text)
		bob.Logf(t, "sent push notification message as bob => %s", pushNotifEventID)
		for i := 0; i < msgsAfter; i++ {
			bob.MustSendMessage(t, roomID, fmt.Sprintf("msg after %d", i))
		}
	})
	return pushNotifEventID
}
