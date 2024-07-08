package chrome

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"
)

//go:embed dist
var jsSDKDistDirectory embed.FS

type JSSDKInstanceOpts struct {
	// The specific port this instance should be hosted on.
	// This is crucial for persistent storage which relies on a stable port number
	// across restarts.
	Port int
}

// NewJSSDKWebsite hosts the JS SDK HTML/JS on a random high-numbered port
// and runs a Go web server to serve those files.
func NewJSSDKWebsite(opts JSSDKInstanceOpts) (baseURL string, close func(), err error) {
	// strip /dist so /index.html loads correctly as does /assets/xxx.js
	c, err := fs.Sub(jsSDKDistDirectory, "dist")
	if err != nil {
		return "", nil, fmt.Errorf("failed to strip /dist off JS SDK files: %s", err)
	}
	// run js-sdk (need to run this as a web server to avoid CORS errors you'd otherwise get with file: URLs)
	var wg sync.WaitGroup
	wg.Add(1)
	mux := &http.ServeMux{}
	mux.Handle("/", http.FileServer(http.FS(c)))
	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", opts.Port),
		Handler: mux,
	}
	startServer := func() {
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			panic(err)
		}
		baseURL = "http://" + ln.Addr().String()
		fmt.Println("JS SDK listening on", baseURL)
		wg.Done()
		srv.Serve(ln)
		fmt.Println("JS SDK closing webserver")
	}
	go startServer()
	wg.Wait()
	return baseURL, func() {
		srv.Close()
	}, nil
}

// Tab represents an open JS SDK instance tab
type Tab struct {
	BaseURL     string
	Ctx         context.Context // tab context
	browser     *Browser        // a ref to the browser which made this tab
	closeServer func()
	cancel      func() // closes the tab
}

func (t *Tab) Close() {
	t.cancel()
	if t.closeServer != nil {
		t.closeServer()
	}
}

func (t *Tab) SetCloseServer(close func()) {
	t.closeServer = close
}
