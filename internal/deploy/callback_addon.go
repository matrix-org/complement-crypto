package deploy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

var lastTestName string

type CallbackData struct {
	Method       string `json:"method"`
	URL          string `json:"url"`
	AccessToken  string `json:"access_token"`
	ResponseCode int    `json:"response_code"`
}

// NewCallbackServer runs a local HTTP server that can read callbacks from mitmproxy.
// Returns the URL of the callback server for use with WithMITMOptions, along with a close function
// which should be called when the test finishes to shut down the HTTP server.
func NewCallbackServer(t *testing.T, cb func(CallbackData)) (callbackURL string, close func()) {
	if lastTestName != "" {
		t.Logf("WARNING: NewCallbackServer called without closing the last one. Check test '%s'", lastTestName)
	}
	lastTestName = t.Name()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var data CallbackData
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			ct.Errorf(t, "error decoding json: %s", err)
			w.WriteHeader(500)
			return
		}
		t.Logf("CallbackServer: %v %+v", time.Now(), data)
		cb(data)
		w.WriteHeader(200)
	})
	// listen on a random high numbered port
	ln, err := net.Listen("tcp", ":0") //nolint
	must.NotError(t, "failed to listen on a tcp port", err)
	port := ln.Addr().(*net.TCPAddr).Port
	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	go srv.Serve(ln)
	return fmt.Sprintf("http://host.docker.internal:%d", port), func() {
		srv.Close()
		lastTestName = ""
	}
}
