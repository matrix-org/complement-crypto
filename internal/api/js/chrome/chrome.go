// package chrome provides helper functions to execute JS in a Chrome browser
//
// This would ordinarily be done via a Chrome struct but Go does not allow
// generic methods, only generic static functions, producing "method must have no type parameters".
package chrome

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/matrix-org/complement/ct"
)

//go:embed dist
var jsSDKDistDirectory embed.FS

// Void is a type which can be used when you want to run an async function without returning anything.
// It can stop large responses causing errors "Object reference chain is too long (-32000)"
// when we don't care about the response.
type Void *runtime.RemoteObject

// Run an anonymous async iffe in the browser. Set the type parameter to a basic data type
// which can be returned as JSON e.g string, map[string]any, []string. If you do not want
// to return anything, use chrome.Void. For example:
//
//	result, err := RunAsyncFn[string](t, ctx, "return await getSomeString()")
//	void, err := RunAsyncFn[chrome.Void](t, ctx, "doSomething(); await doSomethingElse();")
func RunAsyncFn[T any](t ct.TestLike, ctx context.Context, js string) (*T, error) {
	t.Helper()
	out := new(T)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`(async () => {`+js+`})()`, &out, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MustRunAsyncFn is RunAsyncFn but fails the test if an error is returned when executing.
//
// Run an anonymous async iffe in the browser. Set the type parameter to a basic data type
// which can be returned as JSON e.g string, map[string]any, []string. If you do not want
// to return anything, use chrome.Void
func MustRunAsyncFn[T any](t ct.TestLike, ctx context.Context, js string) *T {
	t.Helper()
	result, err := RunAsyncFn[T](t, ctx, js)
	if err != nil {
		ct.Fatalf(t, "MustRunAsyncFn: %s", err)
	}
	return result
}

type Browser struct {
	BaseURL string
	Ctx     context.Context
	Cancel  func()
}

func RunHeadless(onConsoleLog func(s string), requiresPersistance bool, listenPort int) (*Browser, error) {
	ansiRedForeground := "\x1b[31m"
	ansiResetForeground := "\x1b[39m"

	colorifyError := func(format string, args ...any) {
		format = ansiRedForeground + time.Now().Format(time.RFC3339) + " " + format + ansiResetForeground
		fmt.Printf(format, args...)
	}
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if requiresPersistance {
		os.Mkdir("chromedp", os.ModePerm) // ignore errors to allow repeated runs
		wd, _ := os.Getwd()
		userDir := filepath.Join(wd, "chromedp")
		opts = append(opts,
			chromedp.UserDataDir(userDir),
		)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithBrowserOption(
		chromedp.WithBrowserLogf(colorifyError), chromedp.WithBrowserErrorf(colorifyError), //chromedp.WithBrowserDebugf(log.Printf),
	))

	// Listen for console logs for debugging AND to communicate live updates
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				s, err := strconv.Unquote(string(arg.Value))
				if err != nil {
					s = string(arg.Value)
				}
				onConsoleLog(s)
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
	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", listenPort),
		Handler: mux,
	}
	startServer := func() {
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

	// navigate to the page
	err = chromedp.Run(ctx,
		chromedp.Navigate(baseJSURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate to %s: %s", baseJSURL, err)
	}

	return &Browser{
		Ctx: ctx,
		Cancel: func() {
			cancel()
			allocCancel()
			srv.Close()
		},
		BaseURL: baseJSURL,
	}, nil
}
