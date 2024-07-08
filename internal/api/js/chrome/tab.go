package chrome

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

//go:embed dist
var jsSDKDistDirectory embed.FS

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type JSSDKInstanceOpts struct {
	// Required. The prefix to use when constructing base URLs.
	// This is used to namespace storage between tests by prefixing the URL e.g
	// 'foo.localhost:12345' vs 'bar.localhost:12345'. We cannot simply use the
	// port number alone because it is randomly allocated, which means the port
	// number can be reused. If this happens, the 2nd+ test with the same port
	// number will fail with:
	//   'Error: the account in the store doesn't match the account in the constructor'
	// By prefixing, we ensure we are treated as a different origin whilst keeping
	// routing the same.
	HostPrefix string
	// Optional. The specific port this instance should be hosted on.
	// If 0, uses a random high numbered port.
	// This is crucial for persistent storage which relies on a stable port number
	// across restarts.
	Port int
}

// NewJSSDKInstanceOptsFromURL returns SDK options based on a pre-existing base URL. If the
// base URL doesn't exist yet, create the information from the provided user/device ID.
func NewJSSDKInstanceOptsFromURL(baseURL, userID, deviceID string) (*JSSDKInstanceOpts, error) {
	if baseURL == "" {
		return &JSSDKInstanceOpts{
			HostPrefix: nonAlphanumericRegex.ReplaceAllString(userID+deviceID, ""),
		}, nil
	}
	u, _ := url.Parse(baseURL)
	portStr := u.Port()
	port, err := strconv.Atoi(portStr)
	if portStr == "" || err != nil {
		return nil, fmt.Errorf("failed to extract port from base url %s", baseURL)
	}

	return &JSSDKInstanceOpts{
		HostPrefix: strings.Split(u.Hostname(), ".")[0],
		Port:       port,
	}, nil
}

// NewJSSDKWebsite hosts the JS SDK HTML/JS on a random high-numbered port
// and runs a Go web server to serve those files.
func NewJSSDKWebsite(opts *JSSDKInstanceOpts) (baseURL string, close func(), err error) {
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
		baseURL = fmt.Sprintf(
			"http://%s.localhost:%v", opts.HostPrefix, ln.Addr().(*net.TCPAddr).Port,
		)
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
