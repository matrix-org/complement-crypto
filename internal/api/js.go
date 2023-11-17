package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/matrix-org/complement-crypto/internal/chrome"
	"github.com/matrix-org/complement/must"
	"github.com/tidwall/gjson"
)

const CONSOLE_LOG_CONTROL_STRING = "CC:" // for "complement-crypto"

//go:embed dist
var jsSDKDistDirectory embed.FS

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
	ctx        context.Context
	cancel     func()
	baseJSURL  string
	listeners  map[int32]func(roomID string, ev Event)
	listenerID atomic.Int32
	userID     string
}

func NewJSClient(t *testing.T, opts ClientCreationOpts) (Client, error) {
	// start a headless chrome
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithBrowserOption(
		chromedp.WithBrowserLogf(colorifyError), chromedp.WithBrowserErrorf(colorifyError), //chromedp.WithBrowserDebugf(log.Printf),
	))
	jsc := &JSClient{
		listeners: make(map[int32]func(roomID string, ev Event)),
		userID:    opts.UserID,
	}
	// Listen for console logs for debugging AND to communicate live updates
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				s, err := strconv.Unquote(string(arg.Value))
				if err != nil {
					s = string(arg.Value)
				}
				// TODO: debug mode only?
				writeToLog("[%s] console.log %s\n", opts.UserID, s)

				if strings.HasPrefix(s, CONSOLE_LOG_CONTROL_STRING) {
					val := strings.TrimPrefix(s, CONSOLE_LOG_CONTROL_STRING)
					// for now the format is always 'room_id||{event}'
					segs := strings.Split(val, "||")
					var ev JSEvent
					if err := json.Unmarshal([]byte(segs[1]), &ev); err != nil {
						writeToLog("[%s] failed to unmarshal event '%s' into Go %s\n", opts.UserID, segs[1], err)
						continue
					}
					for _, l := range jsc.listeners {
						l(segs[0], jsToEvent(ev))
					}
				}
			}
		}
	})

	// strip /dist so /index.html loads correctly as does /assets/xxx.js
	c, err := fs.Sub(jsSDKDistDirectory, "dist")
	if err != nil {
		return nil, fmt.Errorf("failed to strip /dist off JS SDK files: %s", err)
	}

	baseJSURL := ""
	// run js-sdk (need to run this as a web server to avoid CORS errors you'd otherwise get with file: URLs)
	var wg sync.WaitGroup
	wg.Add(1)
	mux := &http.ServeMux{}
	mux.Handle("/", http.FileServer(http.FS(c)))
	startServer := func() {
		srv := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: mux,
		}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			panic(err)
		}
		baseJSURL = "http://" + ln.Addr().String()
		fmt.Println("JS SDK listening on", baseJSURL)
		wg.Done()
		srv.Serve(ln)
		fmt.Println("JS SDK closing webserver")
	}
	go startServer()
	wg.Wait()

	// now login
	createClientOpts := map[string]interface{}{
		"baseUrl":                opts.BaseURL,
		"useAuthorizationHeader": true,
		"userId":                 opts.UserID,
	}
	if opts.DeviceID != "" {
		createClientOpts["deviceId"] = opts.DeviceID
	}
	createClientOptsJSON, err := json.Marshal(createClientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to serialise login info: %s", err)
	}
	val := fmt.Sprintf("window.__client = matrix.createClient(%s);", string(createClientOptsJSON))
	fmt.Println(val)
	// TODO: move to chrome package
	var r *runtime.RemoteObject
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseJSURL),
		chromedp.Evaluate(val, &r),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to go to %s and createClient: %s", baseJSURL, err)
	}
	// cannot use loginWithPassword as this generates a new device ID
	chrome.AwaitExecute(t, ctx, fmt.Sprintf(`window.__client.login("m.login.password", {
		user: "%s",
		password: "%s",
		device_id: "%s",
	});`, opts.UserID, opts.Password, opts.DeviceID))
	chrome.AwaitExecute(t, ctx, `window.__client.initRustCrypto();`)

	// any events need to log the control string so we get notified
	chrome.MustExecute(t, ctx, fmt.Sprintf(`window.__client.on("Event.decrypted", function(event) {
		console.log("%s"+event.getRoomId()+"||"+JSON.stringify(event.getEffectiveEvent()));
	});`, CONSOLE_LOG_CONTROL_STRING))
	chrome.MustExecute(t, ctx, fmt.Sprintf(`window.__client.on("event", function(event) {
		console.log("%s"+event.getRoomId()+"||"+JSON.stringify(event.getEffectiveEvent()));
	});`, CONSOLE_LOG_CONTROL_STRING))

	jsc.ctx = ctx
	jsc.cancel = cancel
	jsc.baseJSURL = baseJSURL
	return &LoggedClient{Client: jsc}, nil
}

// Close is called to clean up resources.
// Specifically, we need to shut off existing browsers and any FFI bindings.
// If we get callbacks/events after this point, tests may panic if the callbacks
// log messages.
func (c *JSClient) Close(t *testing.T) {
	c.cancel()
	c.listeners = make(map[int32]func(roomID string, ev Event))
}

func (c *JSClient) UserID() string {
	return c.userID
}

func (c *JSClient) MustGetEvent(t *testing.T, roomID, eventID string) Event {
	t.Helper()
	// serialised output (if encrypted):
	// {
	//    encrypted: { event }
	//    decrypted: { event }
	// }
	// else just returns { event }
	evSerialised := chrome.MustExecuteInto[string](t, c.ctx, fmt.Sprintf(`
	JSON.stringify(window.__client.getRoom("%s")?.getLiveTimeline()?.getEvents().filter((ev, i) => {
		console.log("MustGetEvent["+i+"] => " + ev.getId()+ " " + JSON.stringify(ev.toJSON()));
		return ev.getId() === "%s";
	})[0].toJSON());
	`, roomID, eventID))
	if !gjson.Valid(evSerialised) {
		fatalf(t, "MustGetEvent(%s, %s) %s (js): invalid event, got %s", roomID, eventID, c.userID, evSerialised)
	}
	result := gjson.Parse(evSerialised)
	decryptedEvent := result.Get("decrypted")
	if !decryptedEvent.Exists() {
		decryptedEvent = result
	}
	encryptedEvent := result.Get("encrypted")
	//fmt.Printf("DECRYPTED: %s\nENCRYPTED: %s\n\n", decryptedEvent.Raw, encryptedEvent.Raw)
	ev := Event{
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

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *JSClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	t.Helper()
	t.Logf("%s is starting to sync", c.userID)
	chrome.MustExecute(t, c.ctx, fmt.Sprintf(`
		var fn;
		fn = function(state) {
			if (state !== "SYNCING") {
				return;
			}
			console.log("%s"+"sync||{\"type\":\"sync\",\"content\":{}}");
			window.__client.off("sync", fn);
		};
		window.__client.on("sync", fn);`, CONSOLE_LOG_CONTROL_STRING))
	ch := make(chan struct{})
	cancel := c.listenForUpdates(func(roomID string, ev Event) {
		if roomID != "sync" {
			return
		}
		close(ch)
	})
	chrome.AwaitExecute(t, c.ctx, `window.__client.startClient({});`)
	select {
	case <-time.After(5 * time.Second):
		fatalf(t, "[%s](js) took >5s to StartSyncing", c.userID)
	case <-ch:
	}
	cancel()
	// we need to wait for rust crypto's outgoing request loop to finish.
	// There's no callbacks for that yet, so sleep and pray.
	// See https://github.com/matrix-org/matrix-js-sdk/blob/v29.1.0/src/rust-crypto/rust-crypto.ts#L1483
	time.Sleep(500 * time.Millisecond)
	t.Logf("%s is now syncing", c.userID)
	return func() {
		chrome.AwaitExecute(t, c.ctx, `window.__client.stopClient();`)
	}
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *JSClient) IsRoomEncrypted(t *testing.T, roomID string) (bool, error) {
	t.Helper()
	isEncrypted, err := chrome.ExecuteInto[bool](
		t, c.ctx, fmt.Sprintf(`window.__client.isRoomEncrypted("%s")`, roomID),
	)
	if err != nil {
		return false, err
	}
	return *isEncrypted, nil
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room.
func (c *JSClient) SendMessage(t *testing.T, roomID, text string) (eventID string) {
	t.Helper()
	res, err := chrome.AwaitExecuteInto[map[string]interface{}](t, c.ctx, fmt.Sprintf(`window.__client.sendMessage("%s", {
		"msgtype": "m.text",
		"body": "%s"
	});`, roomID, text))
	must.NotError(t, "failed to sendMessage", err)
	return (*res)["event_id"].(string)
}

func (c *JSClient) MustBackpaginate(t *testing.T, roomID string, count int) {
	t.Helper()
	chrome.MustAwaitExecute(t, c.ctx, fmt.Sprintf(
		`window.__client.scrollback(window.__client.getRoom("%s"), %d);`, roomID, count,
	))
}

func (c *JSClient) WaitUntilEventInRoom(t *testing.T, roomID string, checker func(e Event) bool) Waiter {
	t.Helper()
	return &jsTimelineWaiter{
		roomID:  roomID,
		checker: checker,
		client:  c,
	}
}

func (c *JSClient) Logf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	formatted := fmt.Sprintf(t.Name()+": "+format, args...)
	chrome.MustExecute(t, c.ctx, fmt.Sprintf(`console.log("%s");`, formatted))
	t.Logf(format, args...)
}

func (c *JSClient) Type() ClientType {
	return ClientTypeJS
}

func (c *JSClient) listenForUpdates(callback func(roomID string, ev Event)) (cancel func()) {
	id := c.listenerID.Add(1)
	c.listeners[id] = callback
	return func() {
		delete(c.listeners, id)
	}
}

type jsTimelineWaiter struct {
	roomID  string
	checker func(e Event) bool
	client  *JSClient
}

func (w *jsTimelineWaiter) Wait(t *testing.T, s time.Duration) {
	t.Helper()
	updates := make(chan bool, 3)
	cancel := w.client.listenForUpdates(func(roomID string, ev Event) {
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
	chrome.MustExecute(t, w.client.ctx, fmt.Sprintf(
		`window.__client.getRoom("%s")?.getLiveTimeline()?.getEvents().forEach((e)=>{
			console.log("%s"+e.getRoomId()+"||"+JSON.stringify(e.getEffectiveEvent()));
		});`, w.roomID, CONSOLE_LOG_CONTROL_STRING,
	))

	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			fatalf(t, "%s (js): Wait[%s]: timed out", w.client.userID, w.roomID)
		}
		select {
		case <-time.After(timeLeft):
			fatalf(t, "%s (js): Wait[%s]: timed out", w.client.userID, w.roomID)
		case <-updates:
			return
		}
	}
}

func colorifyError(format string, args ...any) {
	format = ansiRedForeground + time.Now().Format(time.RFC3339) + " " + format + ansiResetForeground
	fmt.Printf(format, args...)
}

type JSEvent struct {
	Type     string                 `json:"type"`
	Sender   string                 `json:"sender,omitempty"`
	StateKey *string                `json:"state_key,omitempty"`
	Content  map[string]interface{} `json:"content"`
	ID       string                 `json:"event_id"`
}

func jsToEvent(j JSEvent) Event {
	var ev Event
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
