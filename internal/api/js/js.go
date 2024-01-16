package js

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js/chrome"
	"github.com/tidwall/gjson"
)

const CONSOLE_LOG_CONTROL_STRING = "CC:" // for "complement-crypto"

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
	browser    *chrome.Browser
	listeners  map[int32]func(roomID string, ev api.Event)
	listenerID atomic.Int32
	userID     string
}

func NewJSClient(t api.Test, opts api.ClientCreationOpts) (api.Client, error) {
	jsc := &JSClient{
		listeners: make(map[int32]func(roomID string, ev api.Event)),
		userID:    opts.UserID,
	}
	portKey := opts.UserID + opts.DeviceID
	browser, err := chrome.RunHeadless(func(s string) {
		// TODO: debug mode only?
		writeToLog("[%s,%s] console.log %s\n", jsc.browser.BaseURL, opts.UserID, s)

		if strings.HasPrefix(s, CONSOLE_LOG_CONTROL_STRING) {
			val := strings.TrimPrefix(s, CONSOLE_LOG_CONTROL_STRING)
			// for now the format is always 'room_id||{event}'
			segs := strings.Split(val, "||")
			var ev JSEvent
			if err := json.Unmarshal([]byte(segs[1]), &ev); err != nil {
				writeToLog("[%s] failed to unmarshal event '%s' into Go %s\n", opts.UserID, segs[1], err)
				return
			}
			for _, l := range jsc.listeners {
				l(segs[0], jsToEvent(ev))
			}
		}
	}, opts.PersistentStorage, userDeviceToPort[portKey])
	if err != nil {
		return nil, fmt.Errorf("failed to RunHeadless: %s", err)
	}
	jsc.browser = browser
	/*
		chrome.MustRunAsyncFn[chrome.Void](t, browser.Ctx, `
		const databases = await indexedDB.databases();
		console.log("====STARTUP=============== idb " + JSON.stringify(databases));
		console.log("=================== localstorage len", window.localStorage.length);
	`) */

	// now login
	deviceID := "undefined"
	if opts.DeviceID != "" {
		deviceID = `"` + opts.DeviceID + `"`
	}
	store := "undefined"
	cryptoStore := "undefined"
	if opts.PersistentStorage {
		store = fmt.Sprintf(`new IndexedDBStore({
			indexedDB: window.indexedDB,
			dbName: "%s",
			localStorage: window.localStorage,
		})`, indexedDBName)
		cryptoStore = fmt.Sprintf(`new IndexedDBCryptoStore(indexedDB, "%s")`, indexedDBCryptoName)
		// remember the port for same-origin to remember the store
		u, _ := url.Parse(browser.BaseURL)
		portStr := u.Port()
		port, err := strconv.Atoi(portStr)
		if portStr == "" || err != nil {
			api.Fatalf(t, "failed to extract port from base url %s", browser.BaseURL)
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
	});`, opts.BaseURL, "true", opts.UserID, deviceID, store, cryptoStore))

	return &api.LoggedClient{Client: jsc}, nil
}

func (c *JSClient) Login(t api.Test, opts api.ClientCreationOpts) error {
	deviceID := "undefined"
	if opts.DeviceID != "" {
		deviceID = `"` + opts.DeviceID + `"`
	}
	// cannot use loginWithPassword as this generates a new device ID
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
	await window.__client.login("m.login.password", {
		user: "%s",
		password: "%s",
		device_id: %s,
	});
	await window.__client.initRustCrypto();`, opts.UserID, opts.Password, deviceID))

	// any events need to log the control string so we get notified
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
	window.__client.on("Event.decrypted", function(event) {
		console.log("%s"+event.getRoomId()+"||"+JSON.stringify(event.getEffectiveEvent()));
	});
	window.__client.on("event", function(event) {
		console.log("%s"+event.getRoomId()+"||"+JSON.stringify(event.getEffectiveEvent()));
	});`, CONSOLE_LOG_CONTROL_STRING, CONSOLE_LOG_CONTROL_STRING))

	return nil
}

func (c *JSClient) DeletePersistentStorage(t api.Test) {
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

// Close is called to clean up resources.
// Specifically, we need to shut off existing browsers and any FFI bindings.
// If we get callbacks/events after this point, tests may panic if the callbacks
// log messages.
func (c *JSClient) Close(t api.Test) {
	/*
		chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, `
		const databases = await indexedDB.databases();
		console.log("====CLOSE======= idb " + JSON.stringify(databases));
		console.log("=================== localstorage len", window.localStorage.length);
	`) */
	c.browser.Cancel()
	c.listeners = make(map[int32]func(roomID string, ev api.Event))
}

func (c *JSClient) UserID() string {
	return c.userID
}

func (c *JSClient) MustGetEvent(t api.Test, roomID, eventID string) api.Event {
	t.Helper()
	// serialised output (if encrypted):
	// {
	//    encrypted: { event }
	//    decrypted: { event }
	// }
	// else just returns { event }
	evSerialised := chrome.MustRunAsyncFn[string](t, c.browser.Ctx, fmt.Sprintf(`
	return JSON.stringify(window.__client.getRoom("%s")?.getLiveTimeline()?.getEvents().filter((ev, i) => {
		console.log("MustGetEvent["+i+"] => " + ev.getId()+ " " + JSON.stringify(ev.toJSON()));
		return ev.getId() === "%s";
	})[0].toJSON());
	`, roomID, eventID))
	if !gjson.Valid(*evSerialised) {
		api.Fatalf(t, "MustGetEvent(%s, %s) %s (js): invalid event, got %s", roomID, eventID, c.userID, *evSerialised)
	}
	result := gjson.Parse(*evSerialised)
	decryptedEvent := result.Get("decrypted")
	if !decryptedEvent.Exists() {
		decryptedEvent = result
	}
	encryptedEvent := result.Get("encrypted")
	//fmt.Printf("DECRYPTED: %s\nENCRYPTED: %s\n\n", decryptedEvent.Raw, encryptedEvent.Raw)
	ev := api.Event{
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

	return ev
}

func (c *JSClient) MustStartSyncing(t api.Test) (stopSyncing func()) {
	t.Helper()
	stopSyncing, err := c.StartSyncing(t)
	api.MustNotError(t, "StartSyncing", err)
	return stopSyncing
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *JSClient) StartSyncing(t api.Test) (stopSyncing func(), err error) {
	t.Helper()
	_, err = chrome.RunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
		var fn;
		fn = function(state) {
			if (state !== "SYNCING") {
				return;
			}
			console.log("%s"+"sync||{\"type\":\"sync\",\"content\":{}}");
			window.__client.off("sync", fn);
		};
		window.__client.on("sync", fn);`, CONSOLE_LOG_CONTROL_STRING))
	if err != nil {
		return nil, fmt.Errorf("[%s]failed to listen for sync callback: %s", c.userID, err)
	}
	ch := make(chan struct{})
	cancel := c.listenForUpdates(func(roomID string, ev api.Event) {
		if roomID != "sync" {
			return
		}
		close(ch)
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
func (c *JSClient) IsRoomEncrypted(t api.Test, roomID string) (bool, error) {
	t.Helper()
	isEncrypted, err := chrome.RunAsyncFn[bool](
		t, c.browser.Ctx, fmt.Sprintf(`return window.__client.isRoomEncrypted("%s")`, roomID),
	)
	if err != nil {
		return false, err
	}
	return *isEncrypted, nil
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room.
func (c *JSClient) SendMessage(t api.Test, roomID, text string) (eventID string) {
	t.Helper()
	eventID, err := c.TrySendMessage(t, roomID, text)
	api.MustNotError(t, "failed to sendMessage", err)
	return eventID
}

func (c *JSClient) TrySendMessage(t api.Test, roomID, text string) (eventID string, err error) {
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

func (c *JSClient) MustBackpaginate(t api.Test, roomID string, count int) {
	t.Helper()
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(
		`await window.__client.scrollback(window.__client.getRoom("%s"), %d);`, roomID, count,
	))
}

func (c *JSClient) MustBackupKeys(t api.Test) (recoveryKey string) {
	t.Helper()
	key := chrome.MustRunAsyncFn[string](t, c.browser.Ctx, `
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
	// the backup loop which sends keys will wait between 0-10s before uploading keys...
	// See https://github.com/matrix-org/matrix-js-sdk/blob/49624d5d7308e772ebee84322886a39d2e866869/src/rust-crypto/backup.ts#L319
	// Ideally this would be configurable..
	time.Sleep(11 * time.Second)
	return *key
}

func (c *JSClient) MustLoadBackup(t api.Test, recoveryKey string) {
	chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`
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
}

func (c *JSClient) WaitUntilEventInRoom(t api.Test, roomID string, checker func(e api.Event) bool) api.Waiter {
	t.Helper()
	return &jsTimelineWaiter{
		roomID:  roomID,
		checker: checker,
		client:  c,
	}
}

func (c *JSClient) Logf(t api.Test, format string, args ...interface{}) {
	t.Helper()
	formatted := fmt.Sprintf(t.Name()+": "+format, args...)
	if c.browser.Ctx.Err() == nil { // don't log on dead browsers
		chrome.MustRunAsyncFn[chrome.Void](t, c.browser.Ctx, fmt.Sprintf(`console.log("%s");`, formatted))
	}
	t.Logf(format, args...)
}

func (c *JSClient) Type() api.ClientTypeLang {
	return api.ClientTypeJS
}

func (c *JSClient) listenForUpdates(callback func(roomID string, ev api.Event)) (cancel func()) {
	id := c.listenerID.Add(1)
	c.listeners[id] = callback
	return func() {
		delete(c.listeners, id)
	}
}

type jsTimelineWaiter struct {
	roomID  string
	checker func(e api.Event) bool
	client  *JSClient
}

func (w *jsTimelineWaiter) Wait(t api.Test, s time.Duration) {
	t.Helper()
	updates := make(chan bool, 3)
	cancel := w.client.listenForUpdates(func(roomID string, ev api.Event) {
		if w.roomID != roomID {
			return
		}
		if !w.checker(ev) {
			return
		}
		updates <- true
	})
	defer cancel()

	// check if it already exists by echoing the current timeline. This will call the callback above.
	chrome.MustRunAsyncFn[chrome.Void](t, w.client.browser.Ctx, fmt.Sprintf(
		`window.__client.getRoom("%s")?.getLiveTimeline()?.getEvents().forEach((e)=>{
			console.log("%s"+e.getRoomId()+"||"+JSON.stringify(e.getEffectiveEvent()));
		});`, w.roomID, CONSOLE_LOG_CONTROL_STRING,
	))

	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			api.Fatalf(t, "%s (js): Wait[%s]: timed out", w.client.userID, w.roomID)
		}
		select {
		case <-time.After(timeLeft):
			api.Fatalf(t, "%s (js): Wait[%s]: timed out", w.client.userID, w.roomID)
		case <-updates:
			return
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
