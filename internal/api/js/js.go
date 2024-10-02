package js

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js/chrome"
	"github.com/matrix-org/complement/ct"
	"github.com/tidwall/gjson"
)

const (
	indexedDBName       = "complement-crypto"
	indexedDBCryptoName = "complement-crypto:crypto"
)

// For clients which want persistent storage, we need to ensure when the browser
// starts up a 2nd+ time we serve the same URL so the browser uses the same origin
var userDeviceToPort = map[string]int{}

var logFile *os.File

func SetupJSLogs(filename string) {
	var err error
	logFile, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	logFile.Truncate(0)
}

func WriteJSLogs() {
	logFile.Close()
}

func writeToLog(s string, args ...interface{}) {
	str := fmt.Sprintf(s, args...)
	_, err := logFile.WriteString(time.Now().Format("15:04:05.000000Z07:00") + " " + str)
	if err != nil {
		panic(err)
	}
}

type JSClient struct {
	browser               *chrome.Browser
	listeners             map[int32]func(ctrlMsg *ControlMessage)
	listenerID            atomic.Int32
	listenersMu           *sync.RWMutex
	userID                string
	opts                  api.ClientCreationOpts
	verificationChannel   chan api.VerificationStage
	verificationChannelMu *sync.Mutex
}

func NewJSClient(t ct.TestLike, opts api.ClientCreationOpts) (api.Client, error) {
	jsc := &JSClient{
		listeners:             make(map[int32]func(ctrlMsg *ControlMessage)),
		userID:                opts.UserID,
		listenersMu:           &sync.RWMutex{},
		opts:                  opts,
		verificationChannelMu: &sync.Mutex{},
	}
	portKey := opts.UserID + opts.DeviceID
	browser, err := chrome.RunHeadless(func(s string) {
		writeToLog("[%s,%s] console.log %s\n", opts.UserID, opts.DeviceID, s)

		msg := unpackControlMessage(t, s)
		if msg == nil {
			return
		}
		jsc.listenersMu.RLock()
		var listeners []func(ctrlMsg *ControlMessage)
		for _, l := range jsc.listeners {
			listeners = append(listeners, l)
		}
		jsc.listenersMu.RUnlock()
		for _, l := range listeners {
			l(msg)
		}
	}, opts.PersistentStorage, userDeviceToPort[portKey])
	if err != nil {
		return nil, fmt.Errorf("failed to RunHeadless: %s", err)
	}
	jsc.browser = browser

	// now login
	deviceID := "undefined"
	if opts.DeviceID != "" {
		deviceID = `"` + opts.DeviceID + `"`
	}
	store := "undefined"
	cryptoStore := "undefined"
	if opts.PersistentStorage {
		// TODO: Cannot Must this because of a bug in JS SDK
		// "Uncaught (in promise) Error: createUser is undefined, it should be set with setUserCreator()!"
		// https://github.com/matrix-org/matrix-js-sdk/blob/76b9c3950bfdfca922bec7f70502ff2da93bd731/src/store/indexeddb.ts#L143
		chrome.RunAsyncFn[chrome.Void](t, browser.Ctx, fmt.Sprintf(`
		// FIXME: this doesn't seem to work.
		// JS SDK doesn't store this for us, so we need to. Do this before making the stores which can error out.
		// window.__accessToken = window.localStorage.getItem("complement_crypto_access_token") || undefined;
		// console.log("localStorage.getItem(complement_crypto_access_token) => " + window.__accessToken);

		window.__store = new IndexedDBStore({
			indexedDB: window.indexedDB,
			dbName: "%s",
			localStorage: window.localStorage,
		});
		await window.__store.startup();
		`, indexedDBName))
		store = "window.__store"
		//cryptoStore = fmt.Sprintf(`new IndexedDBCryptoStore(indexedDB, "%s")`, indexedDBCryptoName)
		// remember the port for same-origin to remember the store
		u, _ := url.Parse(browser.BaseURL)
		portStr := u.Port()
		port, err := strconv.Atoi(portStr)
		if portStr == "" || err != nil {
			ct.Fatalf(t, "failed to extract port from base url %s", browser.BaseURL)
		}
		userDeviceToPort[portKey] = port
		t.Logf("user=%s device=%s will be served from %s due to persistent storage", opts.UserID, opts.DeviceID, browser.BaseURL)
	}

	chrome.MustRunAsyncFn[chrome.Void](t, browser.Ctx, fmt.Sprintf(`
	window._secretStorageKeys = {};
	window.__client = matrix.createClient({
		baseUrl:                "%s",
		useAuthorizationHeader: %s,
		userId:                 "%s",
		deviceId: %s,
		accessToken: window.__accessToken || undefined,
		store: %s,
		cryptoStore: %s,
		cryptoCallbacks: {
			cacheSecretStorageKey: (keyId, keyInfo, key) => {
				console.log("cacheSecretStorageKey: keyId="+keyId+" keyInfo="+JSON.stringify(keyInfo)+" key.length:"+key.length);
				window._secretStorageKeys[keyId] = {
					keyInfo: keyInfo,
					key: key,
				};
			},
			getSecretStorageKey: (keys, name) => { //
				console.log("getSecretStorageKey: name=" + name + " keys=" + JSON.stringify(keys));
				const result = [];
				for (const keyId of Object.keys(keys.keys)) {
					const ssKey = window._secretStorageKeys[keyId];
					if (ssKey) {
						result.push(keyId);
						result.push(ssKey.key);
						console.log("getSecretStorageKey: found key ID: " + keyId);
					} else {
						console.log("getSecretStorageKey: unknown key ID: " + keyId);
					}
				}
				return Promise.resolve(result);
			},
		}
	});
	await window.__client.initRustCrypto();
	`, opts.BaseURL, "true", opts.UserID, deviceID, store, cryptoStore))
	jsc.Logf(t, "NewJSClient[%s,%s] created client storage=%v", opts.UserID, opts.DeviceID, opts.PersistentStorage)
	return &api.LoggedClient{Client: jsc}, nil
}

func (c *JSClient) Login(t ct.TestLike, opts api.ClientCreationOpts) error {
	deviceID := "undefined"
	if opts.DeviceID != "" {
		deviceID = `"` + opts.DeviceID + `"`
	}
	// cannot use loginWithPassword as this generates a new device ID
	_, err := chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
	await window.__client.login("m.login.password", {
		user: "%s",
		password: "%s",
		device_id: %s,
	});
	// kick off outgoing requests which will upload OTKs and device keys
	await window.__client.getCrypto().outgoingRequestsManager.doProcessOutgoingRequests();
	`, opts.UserID, opts.Password, deviceID))
	if err != nil {
		return err
	}

	// any events need to log the control string so we get notified
	_, err = chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
	window.__client.on("Event.decrypted", function(event) {
		`+EmitControlMessageEventJS("event.getRoomId()", "event.getEffectiveEvent()")+`
	});
	window.__client.on("event", function(event) {
		`+EmitControlMessageEventJS("event.getRoomId()", "event.getEffectiveEvent()")+`
	});`))
	if err != nil {
		return err
	}

	if c.opts.PersistentStorage {
		/* FIXME: this doesn't work. It doesn't seem to remember across restarts.
		chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
			const token = window.__client.getAccessToken();
			if (token) {
				window.localStorage.setItem("complement_crypto_access_token",token);
				console.log("localStorage.setItem(complement_crypto_access_token) => " + token);
			}
		`) */
	}

	return nil
}

func (c *JSClient) DeletePersistentStorage(t ct.TestLike) {
	t.Helper()
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
	window.localStorage.clear();
	window.sessionStorage.clear();
	const dbName = "%s";
	await new Promise((resolve, reject) => {
		const req = window.indexedDB.deleteDatabase(dbName);
		req.onerror = (event) => {
			reject("failed to delete " + dbName + ": " + event);
		};
		req.onsuccess = (event) => {
			console.log(dbName + " deleted successfully");
		    resolve();
		};
	});
	const cryptoDBName = "%s";
	await new Promise((resolve, reject) => {
		const req = window.indexedDB.deleteDatabase(cryptoDBName);
		req.onerror = (event) => {
			reject("failed to delete " + cryptoDBName + ": " + event);
		};
		req.onsuccess = (event) => {
			console.log(cryptoDBName + " deleted successfully");
		    resolve();
		};
	});
	`, indexedDBName, indexedDBCryptoName))
}

func (c *JSClient) CurrentAccessToken(t ct.TestLike) string {
	token := chrome.MustRunAsyncFn[string](t, c.browser.Ctx, `
		return window.__client.getAccessToken();`)
	return *token
}

func (c *JSClient) GetNotification(t ct.TestLike, roomID, eventID string) (*api.Notification, error) {
	return nil, fmt.Errorf("not implemented yet") // TODO
}

func (c *JSClient) bootstrapCrossSigning(t ct.TestLike) {
	// when MSC3967 is everywhere, we can drop the auth dict
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
	await window.__client.getCrypto().bootstrapCrossSigning({
		authUploadDeviceSigningKeys: async function (makeRequest) {
			return await makeRequest({
				  "type": "m.login.password",
				  "identifier": {
					  "type": "m.id.user",
					  "user": "%s",
				  },
				  "password": "%s",
		  });
		},
	  });
	  `, c.opts.UserID, c.opts.Password))
}

func (c *JSClient) ensureListeningForVerificationRequests(t ct.TestLike) chan api.VerificationStage {
	c.verificationChannelMu.Lock()
	defer c.verificationChannelMu.Unlock()
	if c.verificationChannel == nil {
		// we need x-signing keys in order to do verification requests
		c.bootstrapCrossSigning(t)
		// we need to support multiple transition stages firing at once
		c.verificationChannel = make(chan api.VerificationStage, 4)
		chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
	window.__client.on(CryptoEvent.VerificationRequestReceived, function(request) {
		console.log("CryptoEvent.VerificationRequestReceived fired: request.initiatedByMe " + request.initiatedByMe);
		request.on("change", () => {
			console.log("RequestOwnUserVerification got phase " + request.phase);
			switch(request.phase) {
				case VerificationPhase.Unsent:
					console.log("Unsent");
					break;
				case VerificationPhase.Requested: // An m.key.verification.request event has been sent or received
					`+EmitControlMessageVerificationJS(
			`"Requested"`,
			"request.transactionId",
			"request.otherUserId",
			"request.otherDeviceId",
			"{}",
		)+`
					break;
				case VerificationPhase.Ready: // An m.key.verification.ready event has been sent or received, indicating the verification request is accepted.
					`+EmitControlMessageVerificationJS(
			`"Ready"`,
			"request.transactionId",
			"request.otherUserId",
			"request.otherDeviceId",
			"{}",
		)+`
					break;
				case VerificationPhase.Started: // In-flight verification. This means that an m.key.verification.start event has been sent or received, choosing a verification method; however the verification has not yet completed or been cancelled.
					`+EmitControlMessageVerificationJS(
			`"Started"`,
			"request.transactionId",
			"request.otherUserId",
			"request.otherDeviceId",
			"{}",
		)+`
					break;
				case VerificationPhase.Cancelled: // An m.key.verification.cancel event has been sent or received at any time before the 'done' event, cancelling the verification request
					`+EmitControlMessageVerificationJS(
			`"Cancelled"`,
			"request.transactionId",
			"request.otherUserId",
			"request.otherDeviceId",
			"{}",
		)+`
					break;
				case VerificationPhase.Done: // Normally this means that m.key.verification.done events have been sent and received.
					`+EmitControlMessageVerificationJS(
			`"Done"`,
			"request.transactionId",
			"request.otherUserId",
			"request.otherDeviceId",
			"{}",
		)+`
					break;
			}
		});
		if (!window.__pendingVerificationByTxnID) {
			window.__pendingVerificationByTxnID = {};
		}
		window.__pendingVerificationByTxnID[request.transactionId] = request;
		// we may have requested this! Let's check.
		const stage = request.initiatedByMe ? "Requested" : "VerificationRequestReceived";
		`+EmitControlMessageVerificationJS(
			`stage`,
			"request.transactionId",
			"request.otherUserId",
			"request.otherDeviceId",
			"{}",
		)+`
	});`)
	}
	return c.verificationChannel
}

func (c *JSClient) ListenForVerificationRequests(t ct.TestLike) chan api.VerificationStage {
	ch := c.ensureListeningForVerificationRequests(t)
	txnIDsStarted := make(map[string]bool)
	c.listenForUpdates(func(ctrlMsg *ControlMessage) {
		msg := ctrlMsg.AsControlMessageVerification()
		if msg == nil {
			return
		}
		container := &api.VerificationContainer{
			Mutex: &sync.Mutex{},
			VReq: api.VerificationRequest{
				SenderUserID:     msg.UserID,
				SenderDeviceID:   msg.DeviceID,
				TxnID:            msg.TxnID,
				ReceiverUserID:   c.userID,
				ReceiverDeviceID: c.opts.DeviceID,
			},
			SendReady: func() {
				chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
						await window.__pendingVerificationByTxnID["`+msg.TxnID+`"].accept();
					`)
			},
			SendStart: func(method string) {
				chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
						const verifier = await window.__pendingVerificationByTxnID["`+msg.TxnID+`"].startVerification("`+method+`");
					`)
			},
			SendTransition: func() {
				chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
					const request = window.__pendingVerificationByTxnID["`+msg.TxnID+`"];
					const verifier = request.verifier;
					verifier.on(VerifierEvent.ShowSas, (sas) => {
						`+EmitControlMessageVerificationJS(
					`"TransitionSAS"`,
					"request.transactionId",
					"request.otherUserId",
					"request.otherDeviceId",
					"sas.sas",
				)+`
					});
					// don't await on this as it blocks until the verification has completed/cancelled.
					verifier.verify();
				`)
			},
			SendCancel: func() {
				chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
						await window.__pendingVerificationByTxnID["`+msg.TxnID+`"].cancel();
					`)
			},
			SendApprove: func() {
				chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
						const verifier = window.__pendingVerificationByTxnID["`+msg.TxnID+`"].verifier;
						await verifier.getShowSasCallbacks().confirm()
					`)
			},
			SendDecline: func() {
				chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
					const verifier = window.__pendingVerificationByTxnID["`+msg.TxnID+`"].verifier;
					await verifier.getShowSasCallbacks().mismatch()
				`)
			},
		}
		switch msg.Stage {
		case "VerificationRequestReceived":
			ch <- api.NewVerificationStageRequestedReceiver(container)
		case "Requested":
			ch <- api.NewVerificationStageRequested(container)
		case "Ready":
			ch <- api.NewVerificationStageReady(container)
		case "Started":
			// we will get many "VerificationPhase.Started" calls as we do SAS events. We don't want
			// to call SendTransition many times, so only emit this once.
			if txnIDsStarted[msg.TxnID] {
				return
			}
			txnIDsStarted[msg.TxnID] = true
			ch <- api.NewVerificationStageStart(container)
		case "TransitionSAS":
			verificationData := struct {
				Decimal []uint16    `json:"decimal"`
				Emoji   [][2]string `json:"emoji"` // tuple `[emoji, name]`
			}{}
			if err := json.Unmarshal([]byte(msg.Data), &verificationData); err != nil {
				ct.Errorf(t, "failed to unmarshal verification data: %s", err)
			}
			if len(verificationData.Decimal) > 0 || len(verificationData.Emoji) > 0 {
				var emoji []string
				for _, e := range verificationData.Emoji {
					emoji = append(emoji, e[0])
				}
				container.VData = api.VerificationData{
					Decimals: verificationData.Decimal,
					Emojis:   emoji,
				}
				ch <- api.NewVerificationStageTransitioned(container)
			} else {
				t.Logf("WARN: Got TransitionSAS but no emoji/decimal")
			}
		case "Cancelled":
			ch <- api.NewVerificationStageCancelled(container)
		case "Done":
			ch <- api.NewVerificationStageDone(container)
		}
	})
	return ch
}

func (c *JSClient) RequestOwnUserVerification(t ct.TestLike) chan api.VerificationStage {
	// When we request key verification, we will /sendToDevice with * devices, which
	// rather bizarrely will send the to-device event back to ourselves. This will then
	// be picked up as a VerificationRequestReceived. The code that listens for
	// VerificationRequestReceived will figure out it's a request from itself and adjust
	// the state accordingly. Because of this, we only need to call requestOwnUserVerification
	// and do nothing else.
	ch := c.ListenForVerificationRequests(t)
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
	const request = await window.__client.getCrypto().requestOwnUserVerification();
	`)
	return ch
}

func (c *JSClient) ForceClose(t ct.TestLike) {
	t.Helper()
	t.Logf("force closing a JS client is the same as a normal close (closing browser)")
	c.Close(t)
}

// Close is called to clean up resources.
// Specifically, we need to shut off existing browsers and any FFI bindings.
// If we get callbacks/events after this point, tests may panic if the callbacks
// log messages.
func (c *JSClient) Close(t ct.TestLike) {
	t.Helper()
	c.browser.Cancel()
	c.listenersMu.Lock()
	c.listeners = make(map[int32]func(ctrlMsg *ControlMessage))
	c.listenersMu.Unlock()
}

func (c *JSClient) UserID() string {
	return c.userID
}

func (c *JSClient) Opts() api.ClientCreationOpts {
	return c.opts
}

func (c *JSClient) InviteUser(t ct.TestLike, roomID, userID string) error {
	_, err := chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprint(`
		await window.__client.invite("`, roomID, `","`, userID, `");
	`))
	return err
}

func (c *JSClient) GetEvent(t ct.TestLike, roomID, eventID string) (*api.Event, error) {
	t.Helper()
	// serialised output (if encrypted):
	// {
	//    encrypted: { event }
	//    decrypted: { event }
	// }
	// else just returns { event }
	evSerialised, err := chrome.RunAsyncFn[string](t, c.browser.Ctx, fmt.Sprintf(`
	return JSON.stringify(window.__client.getRoom("%s")?.getLiveTimeline()?.getEvents().filter((ev, i) => {
		console.log("MustGetEvent["+i+"] => " + ev.getId()+ " " + JSON.stringify(ev.toJSON()));
		return ev.getId() === "%s";
	})[0].toJSON());
	`, roomID, eventID))
	if err != nil {
		return nil, fmt.Errorf("failed to get event %s: %s", eventID, err)
	}
	if !gjson.Valid(*evSerialised) {
		return nil, fmt.Errorf("invalid event %s, got %s", eventID, *evSerialised)
	}
	result := gjson.Parse(*evSerialised)
	decryptedEvent := result.Get("decrypted")
	if !decryptedEvent.Exists() {
		decryptedEvent = result
	}
	encryptedEvent := result.Get("encrypted")
	//fmt.Printf("DECRYPTED: %s\nENCRYPTED: %s\n\n", decryptedEvent.Raw, encryptedEvent.Raw)
	ev := &api.Event{
		ID:     decryptedEvent.Get("event_id").Str,
		Text:   decryptedEvent.Get("content.body").Str,
		Sender: decryptedEvent.Get("sender").Str,
	}
	if decryptedEvent.Get("type").Str == "m.room.member" {
		ev.Membership = decryptedEvent.Get("content.membership").Str
		ev.Target = decryptedEvent.Get("state_key").Str
	}
	if encryptedEvent.Exists() && decryptedEvent.Get("content.msgtype").Str == "m.bad.encrypted" {
		ev.FailedToDecrypt = true
	}

	return ev, nil
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *JSClient) StartSyncing(t ct.TestLike) (stopSyncing func(), err error) {
	t.Helper()
	_, err = chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
		var fn;
		fn = function(state) {
			if (state !== "SYNCING") {
				return;
			}
			`+EmitControlMessageSyncJS()+`

			window.__client.off("sync", fn);
		};
		window.__client.on("sync", fn);`))
	if err != nil {
		return nil, fmt.Errorf("[%s]failed to listen for sync callback: %s", c.userID, err)
	}
	ch := make(chan struct{})
	cancel := c.listenForUpdates(func(ctrlMsg *ControlMessage) {
		if msg := ctrlMsg.AsControlMessageSync(); msg != nil {
			close(ch)
		}
	})
	chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, `await window.__client.startClient({});`)
	select {
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("[%s](js) took >5s to StartSyncing", c.userID)
	case <-ch:
	}
	cancel()
	// we need to wait for rust crypto's outgoing request loop to finish.
	// There's no callbacks for that yet, so sleep and pray.
	// See https://github.com/matrix-org/matrix-js-sdk/blob/v29.1.0/src/rust-crypto/rust-crypto.ts#L1483
	time.Sleep(500 * time.Millisecond)
	return func() {
		chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, `await window.__client.stopClient();`)
	}, nil
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *JSClient) IsRoomEncrypted(t ct.TestLike, roomID string) (bool, error) {
	t.Helper()
	isEncrypted, err := chrome.RunAsyncFn[bool](
		t, c.browser.Ctx, fmt.Sprintf(`return window.__client.isRoomEncrypted("%s")`, roomID),
	)
	if err != nil {
		return false, err
	}
	return *isEncrypted, nil
}

func (c *JSClient) SendMessage(t ct.TestLike, roomID, text string) (eventID string, err error) {
	t.Helper()
	res, err := chrome.RunAsyncFn[map[string]interface{}](t, c.browser.Ctx, fmt.Sprintf(`
	return await window.__client.sendMessage("%s", {
		"msgtype": "m.text",
		"body": "%s"
	});`, roomID, text))
	if err != nil {
		return "", err
	}
	return (*res)["event_id"].(string), nil
}

func (c *JSClient) Backpaginate(t ct.TestLike, roomID string, count int) error {
	t.Helper()
	_, err := chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(
		`await window.__client.scrollback(window.__client.getRoom("%s"), %d);`, roomID, count,
	))
	return err
}

func (c *JSClient) BackupKeys(t ct.TestLike) (recoveryKey string, err error) {
	t.Helper()
	key, err := chrome.RunAsyncFn[string](t, c.browser.Ctx, `
		// we need to ensure that we have a recovery key first, though we don't actually care about it..?
		const recoveryKey = await window.__client.getCrypto().createRecoveryKeyFromPassphrase();
		// now use said key to make backups
		await window.__client.getCrypto().bootstrapSecretStorage({
			createSecretStorageKey: async() => { return recoveryKey; },
			setupNewKeyBackup: true,
			setupNewSecretStorage: true,
		});
		// now we can enable key backups
		await window.__client.getCrypto().checkKeyBackupAndEnable();
		return recoveryKey.encodedPrivateKey;`)
	if err != nil {
		return "", fmt.Errorf("error enabling key backup: %s", err)
	}
	// the backup loop which sends keys will wait between 0-10s before uploading keys...
	// See https://github.com/matrix-org/matrix-js-sdk/blob/49624d5d7308e772ebee84322886a39d2e866869/src/rust-crypto/backup.ts#L319
	// Ideally this would be configurable..
	time.Sleep(11 * time.Second)
	return *key, nil
}

func (c *JSClient) LoadBackup(t ct.TestLike, recoveryKey string) error {
	_, err := chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
		// we assume the recovery key is the private key for the default key id so
		// figure out what that key id is.
		const keyId = await window.__client.secretStorage.getDefaultKeyId();
		// now add this to the in-memory cache. We don't actually ever return key info so we just pass in {} here.
		window._secretStorageKeys[keyId] = {
			keyInfo: {},
			key: window.decodeRecoveryKey("%s"),
		}
		console.log("will return recovery key for default key id " + keyId);
		const keyBackupCheck = await window.__client.getCrypto().checkKeyBackupAndEnable();
		console.log("key backup: ", JSON.stringify(keyBackupCheck));
		await window.__client.restoreKeyBackupWithSecretStorage(keyBackupCheck ? keyBackupCheck.backupInfo : null, undefined, undefined);`,
		recoveryKey))
	return err
}

func (c *JSClient) WaitUntilEventInRoom(t ct.TestLike, roomID string, checker func(e api.Event) bool) api.Waiter {
	t.Helper()
	return &jsTimelineWaiter{
		roomID:  roomID,
		checker: checker,
		client:  c,
	}
}

func (c *JSClient) Logf(t ct.TestLike, format string, args ...interface{}) {
	t.Helper()
	formatted := fmt.Sprintf(t.Name()+": "+format, args...)
	firstLine := strings.Split(formatted, "\n")[0]
	if c.browser.Ctx.Err() == nil { // don't log on dead browsers
		chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`console.log("%s");`, strings.Replace(firstLine, `"`, `\"`, -1)))
		t.Logf(format, args...)
	}
}

func (c *JSClient) Type() api.ClientTypeLang {
	return api.ClientTypeJS
}

func (c *JSClient) listenForUpdates(callback func(ctrlMsg *ControlMessage)) (cancel func()) {
	id := c.listenerID.Add(1)
	c.listenersMu.Lock()
	c.listeners[id] = callback
	c.listenersMu.Unlock()
	return func() {
		c.listenersMu.Lock()
		delete(c.listeners, id)
		c.listenersMu.Unlock()
	}
}

type jsTimelineWaiter struct {
	roomID  string
	checker func(e api.Event) bool
	client  *JSClient
}

func (w *jsTimelineWaiter) Waitf(t ct.TestLike, s time.Duration, format string, args ...any) {
	t.Helper()
	err := w.TryWaitf(t, s, format, args...)
	if err != nil {
		ct.Fatalf(t, err.Error())
	}
}

func (w *jsTimelineWaiter) TryWaitf(t ct.TestLike, s time.Duration, format string, args ...any) error {
	t.Helper()
	updates := make(chan bool, 3)
	cancel := w.client.listenForUpdates(func(ctrlMsg *ControlMessage) {
		msg := ctrlMsg.AsControlMessageEvent()
		if msg == nil {
			return
		}
		if w.roomID != msg.RoomID {
			return
		}
		if !w.checker(jsToEvent(msg.Event)) {
			return
		}
		updates <- true
	})
	defer cancel()

	// check if it already exists by echoing the current timeline. This will call the callback above.
	chrome.MustRunAsyncFn[chrome.Void](t, w.client.browser.Ctx, fmt.Sprintf(
		`window.__client.getRoom("%s")?.getLiveTimeline()?.getEvents().forEach((e)=>{
			`+EmitControlMessageEventJS("e.getRoomId()", "e.getEffectiveEvent()")+`
		});`, w.roomID,
	))

	msg := fmt.Sprintf(format, args...)
	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			return fmt.Errorf("%s (js): Wait[%s]: timed out: %s", w.client.userID, w.roomID, msg)
		}
		select {
		case <-time.After(timeLeft):
			return fmt.Errorf("%s (js): Wait[%s]: timed out: %s", w.client.userID, w.roomID, msg)
		case <-updates:
			return nil // event exists
		}
	}
}

type JSEvent struct {
	Type     string                 `json:"type"`
	Sender   string                 `json:"sender,omitempty"`
	StateKey *string                `json:"state_key,omitempty"`
	Content  map[string]interface{} `json:"content"`
	ID       string                 `json:"event_id"`
}

func jsToEvent(j JSEvent) api.Event {
	var ev api.Event
	ev.Sender = j.Sender
	ev.ID = j.ID
	switch j.Type {
	case "m.room.member":
		ev.Target = *j.StateKey
		ev.Membership = j.Content["membership"].(string)
	case "m.room.message":
		ev.Text = j.Content["body"].(string)
	}
	return ev
}
