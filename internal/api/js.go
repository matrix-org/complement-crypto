package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
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
)

const CONSOLE_LOG_CONTROL_STRING = "CC:" // for "complement-crypto"

//go:embed dist
var jsSDKDistDirectory embed.FS

type JSClient struct {
	ctx        context.Context
	cancel     func()
	baseJSURL  string
	listeners  map[int32]func(roomID, text string)
	listenerID atomic.Int32
	userID     string
}

func NewJSClient(t *testing.T, opts ClientCreationOpts) (Client, error) {
	// start a headless chrome
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithBrowserOption(
		chromedp.WithBrowserLogf(log.Printf), chromedp.WithBrowserErrorf(log.Printf), //chromedp.WithBrowserDebugf(log.Printf),
	))
	jsc := &JSClient{
		listeners: make(map[int32]func(roomID, text string)),
		userID:    opts.UserID,
	}
	// Listen for console logs for debugging AND to communicate live updates
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				s, err := strconv.Unquote(string(arg.Value))
				if err != nil {
					log.Println(err)
					continue
				}
				// TODO: debug mode only?
				colorify("[%s] console.log %s\n", opts.UserID, s)

				if strings.HasPrefix(s, CONSOLE_LOG_CONTROL_STRING) {
					val := strings.TrimPrefix(s, CONSOLE_LOG_CONTROL_STRING)
					// for now the format is always 'room_id||text'
					segs := strings.Split(val, "||")
					for _, l := range jsc.listeners {
						l(segs[0], segs[1])
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
		if (event.getType() !== "m.room.message") {
			return; // only use messages
		}
		console.log("%s"+event.getRoomId()+"||"+event.getEffectiveEvent().content.body);
	});`, CONSOLE_LOG_CONTROL_STRING))

	jsc.ctx = ctx
	jsc.cancel = cancel
	jsc.baseJSURL = baseJSURL
	return jsc, nil
}

// Close is called to clean up resources.
// Specifically, we need to shut off existing browsers and any FFI bindings.
// If we get callbacks/events after this point, tests may panic if the callbacks
// log messages.
func (c *JSClient) Close(t *testing.T) {
	c.cancel()
	c.listeners = make(map[int32]func(roomID string, text string))
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *JSClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	chrome.AwaitExecute(t, c.ctx, `window.__client.startClient({});`)
	return func() {
		chrome.AwaitExecute(t, c.ctx, `window.__client.stopClient();`)
	}
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *JSClient) IsRoomEncrypted(t *testing.T, roomID string) (bool, error) {
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
func (c *JSClient) SendMessage(t *testing.T, roomID, text string) {
	err := chrome.AwaitExecute(t, c.ctx, fmt.Sprintf(`window.__client.sendMessage("%s", {
		"msgtype": "m.text",
		"body": "%s"
	});`, roomID, text))
	must.NotError(t, "failed to sendMessage", err)
}

func (c *JSClient) MustBackpaginate(t *testing.T, roomID string, count int) {
	chrome.MustAwaitExecute(t, c.ctx, fmt.Sprintf(
		`window.__client.scrollback(window.__client.getRoom("%s"), %d);`, roomID, count,
	))
}

func (c *JSClient) WaitUntilEventInRoom(t *testing.T, roomID, wantBody string) Waiter {
	exists := chrome.MustExecuteInto[bool](t, c.ctx, fmt.Sprintf(
		`window.__client.getRoom("%s").getLiveTimeline().getEvents().map((e)=>{return e.getContent().body}).includes("%s");`, roomID, wantBody,
	))

	return &jsTimelineWaiter{
		roomID:   roomID,
		wantBody: wantBody,
		client:   c,
		exists:   exists,
	}
}

func (c *JSClient) Type() ClientType {
	return ClientTypeJS
}

func (c *JSClient) listenForUpdates(callback func(roomID, gotText string)) (cancel func()) {
	id := c.listenerID.Add(1)
	c.listeners[id] = callback
	return func() {
		delete(c.listeners, id)
	}
}

type jsTimelineWaiter struct {
	roomID   string
	wantBody string
	client   *JSClient
	exists   bool
}

func (w *jsTimelineWaiter) Wait(t *testing.T, s time.Duration) {
	if w.exists {
		return
	}
	updates := make(chan bool, 3)
	cancel := w.client.listenForUpdates(func(roomID, gotText string) {
		if w.roomID != roomID {
			return
		}
		if w.wantBody != gotText {
			return
		}
		updates <- true
	})
	defer cancel()

	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			t.Fatalf("%s: Wait[%s]: timed out", w.client.userID, w.roomID)
		}
		select {
		case <-time.After(timeLeft):
			t.Fatalf("%s: Wait[%s]: timed out", w.client.userID, w.roomID)
		case <-updates:
			return
		}
	}
}

const ansiYellowForeground = "\x1b[33m"
const ansiResetForeground = "\x1b[39m"

func colorify(format string, args ...any) {
	format = ansiYellowForeground + format + ansiResetForeground
	fmt.Printf(format, args...)
}