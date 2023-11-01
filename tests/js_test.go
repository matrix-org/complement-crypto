package tests

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
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/matrix-org/complement-crypto/chrome"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

//go:embed dist
var jsSDKDistDirectory embed.FS

func setupClient(t *testing.T, ctx context.Context, jsSDKURL string, csapi *client.CSAPI) {
	// bob syncs and joins the room
	createClientOpts := map[string]interface{}{
		"baseUrl":                csapi.BaseURL,
		"useAuthorizationHeader": true,
		"userId":                 csapi.UserID,
		"deviceId":               csapi.DeviceID,
		"accessToken":            csapi.AccessToken,
	}
	createClientOptsJSON, err := json.Marshal(createClientOpts)
	must.NotError(t, "failed to serialise json", err)
	val := fmt.Sprintf("window.__client = matrix.createClient(%s);", string(createClientOptsJSON))
	fmt.Println(val)
	var r *runtime.RemoteObject
	err = chromedp.Run(ctx,
		chromedp.Navigate(jsSDKURL),
		chromedp.Evaluate(val, &r),
	)
	must.NotError(t, "failed to go to new page", err)
	chrome.AwaitExecute(t, ctx, `window.__client.initRustCrypto();`)
	chrome.AwaitExecute(t, ctx, `window.__client.startClient({});`)
	time.Sleep(2 * time.Second)
}

func TestJS(t *testing.T) {
	deployment := Deploy(t)
	// pre-register alice and bob
	csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "alice",
		Password:        "testfromjsdk",
	})
	csapiBob := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "bob",
		Password:        "testfromrustsdk",
	})
	roomID := csapiAlice.MustCreateRoom(t, map[string]interface{}{
		"name":   "JS SDK Test",
		"preset": "trusted_private_chat",
		"invite": []string{csapiBob.UserID},
		"initial_state": []map[string]interface{}{
			{
				"type":      "m.room.encryption",
				"state_key": "",
				"content": map[string]interface{}{
					"algorithm": "m.megolm.v1.aes-sha2",
				},
			},
		},
	})
	csapiBob.MustJoinRoom(t, roomID, []string{"hs1"})

	// start a headless chrome
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithBrowserOption(
		chromedp.WithBrowserLogf(log.Printf), chromedp.WithBrowserErrorf(log.Printf), //chromedp.WithBrowserDebugf(log.Printf),
	))
	// log console.log and listen for callbacks
	preparedWaiter := helpers.NewWaiter()
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				s, err := strconv.Unquote(string(arg.Value))
				if err != nil {
					log.Println(err)
					continue
				}
				fmt.Printf("%s\n", s)
				if strings.HasPrefix(s, "JS_SDK_TEST:") {
					val := strings.TrimPrefix(s, "JS_SDK_TEST:")
					fmt.Println(">>>>>>> ", val)
					if val == "SYNCING" {
						preparedWaiter.Finish()
					}
				}
			}
		}
	})
	defer cancel()

	// run js-sdk (need to run this as a web server to avoid CORS errors you'd otherwise get with file: URLs)
	var wg sync.WaitGroup
	wg.Add(2)
	baseJSURL := ""
	baseJSURL2 := ""
	c, err := fs.Sub(jsSDKDistDirectory, "dist")
	if err != nil {
		panic(err)
	}
	http.Handle("/", http.FileServer(http.FS(c)))
	startServer := func(fn func(u string)) {
		srv := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.DefaultServeMux,
		}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			panic(err)
		}

		u := "http://" + ln.Addr().String()
		fmt.Println("listening on", u)
		fn(u)
		wg.Done()
		srv.Serve(ln)
		fmt.Println("closing webserver")
	}
	go startServer(func(u string) {
		baseJSURL = u
	})
	go startServer(func(u string) {
		baseJSURL2 = u
	})
	wg.Wait()

	setupClient(t, ctx, baseJSURL2, csapiBob)

	fmt.Println("hs", csapiAlice.BaseURL)

	// run task list
	createClientOpts := map[string]interface{}{
		"baseUrl":                csapiAlice.BaseURL,
		"useAuthorizationHeader": true,
		"userId":                 csapiAlice.UserID,
		"deviceId":               csapiAlice.DeviceID,
		"accessToken":            csapiAlice.AccessToken,
	}
	createClientOptsJSON, err := json.Marshal(createClientOpts)
	must.NotError(t, "failed to serialise json", err)
	val := fmt.Sprintf("window._test_client = matrix.createClient(%s);", string(createClientOptsJSON))
	fmt.Println(val)
	//time.Sleep(time.Hour)
	var r *runtime.RemoteObject
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseJSURL),
		chromedp.Evaluate(val, &r),
	)
	must.NotError(t, "failed to createClient", err)
	chrome.AwaitExecute(t, ctx, `window._test_client.initRustCrypto();`)

	// add listener for the room to appear
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window._test_client.on("sync", function(state){
				console.log("JS_SDK_TEST:" + state);
			});`, &r),
	)
	must.NotError(t, "failed to listen for prepared callback", err)

	// start syncing
	chrome.AwaitExecute(t, ctx, `window._test_client.startClient({});`)

	// wait for the room to appear
	preparedWaiter.Wait(t, 5*time.Second)

	// check room is encrypted
	must.Equal(t, chrome.MustExecuteInto[bool](
		t, ctx, fmt.Sprintf(`window._test_client.isRoomEncrypted("%s")`, roomID),
	), true, "room is not encrypted")

	// send an encrypted message
	chrome.AwaitExecute(t, ctx, fmt.Sprintf(`window._test_client.sendMessage("%s", {
		"msgtype": "m.text",
		"body": "Hello World!"
	});`, roomID))
	must.NotError(t, "failed to send message", err)

	time.Sleep(2 * time.Second) // wait for keys/changes

	// bob syncs and joins the room
	createClientOpts = map[string]interface{}{
		"baseUrl":                csapiBob.BaseURL,
		"useAuthorizationHeader": true,
		"userId":                 csapiBob.UserID,
		"deviceId":               csapiBob.DeviceID,
		"accessToken":            csapiBob.AccessToken,
	}
	createClientOptsJSON, err = json.Marshal(createClientOpts)
	must.NotError(t, "failed to serialise json", err)
	val = fmt.Sprintf("window._test_bob = matrix.createClient(%s);", string(createClientOptsJSON))
	fmt.Println(val)
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseJSURL2),
		chromedp.Evaluate(val, &r),
	)
	must.NotError(t, "failed to go to new page", err)
	chrome.AwaitExecute(t, ctx, `window._test_bob.initRustCrypto();`)
	chrome.AwaitExecute(t, ctx, `window._test_bob.startClient({});`)

	// ensure bob sees the decrypted event
	time.Sleep(time.Second)

	execute(t, ctx, `window._bob_room = window._test_bob.getRoom("%s")`, roomID)
	execute(t, ctx, `window._bob_tl = window._bob_room.getLiveTimeline().getEvents()`)
	//var tl map[string]interface{}
	//executeInto(t, ctx, &tl, `window._bob_tl[window._bob_tl.length-1].event`)
	tl := chrome.MustExecuteInto[map[string]interface{}](t, ctx, `window._bob_tl[window._bob_tl.length-1].event`)
	t.Logf("%+v", tl)
	tl = chrome.MustExecuteInto[map[string]interface{}](t, ctx, `window._bob_tl[window._bob_tl.length-1].getEffectiveEvent()`)
	t.Logf("%+v", tl)
	isEncrypted := chrome.MustExecuteInto[bool](t, ctx, `window._bob_tl[window._bob_tl.length-1].isEncrypted()`)
	t.Logf("%v", isEncrypted)
	must.Equal(t, isEncrypted, true, "room is not encrypted")
}

func execute(t *testing.T, ctx context.Context, cmd string, args ...interface{}) {
	t.Helper()
	var r *runtime.RemoteObject // stop large responses causing errors "Object reference chain is too long (-32000)"
	js := fmt.Sprintf(cmd, args...)
	t.Log(js)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(js, &r),
	)
	must.NotError(t, js, err)
}

func executeInto(t *testing.T, ctx context.Context, res interface{}, cmd string, args ...interface{}) {
	t.Helper()
	js := fmt.Sprintf(cmd, args...)
	t.Log(js)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(js, &res),
	)
	must.NotError(t, js, err)
}
