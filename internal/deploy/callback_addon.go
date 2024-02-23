package deploy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

var lastTestName string

type CallbackData struct {
	Method       string          `json:"method"`
	URL          string          `json:"url"`
	AccessToken  string          `json:"access_token"`
	ResponseCode int             `json:"response_code"`
	RequestBody  json.RawMessage `json:"request_body"`
}

func (cd CallbackData) String() string {
	return fmt.Sprintf("%s %s (token=%s) req_len=%d => HTTP %v", cd.Method, cd.URL, cd.AccessToken, len(cd.RequestBody), cd.ResponseCode)
}

// NewCallbackServer runs a local HTTP server that can read callbacks from mitmproxy.
// Returns the URL of the callback server for use with WithMITMOptions, along with a close function
// which should be called when the test finishes to shut down the HTTP server.
func NewCallbackServer(t *testing.T, cb func(CallbackData)) (callbackURL string, close func()) {
	if lastTestName != "" {
		t.Logf("WARNING[%s]: NewCallbackServer called without closing the last one. Check test '%s'", t.Name(), lastTestName)
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
		localpart := ""
		if strings.HasPrefix(data.AccessToken, "syt_") {
			maybeLocalpart, _ := base64.RawStdEncoding.DecodeString(strings.Split(data.AccessToken, "_")[1])
			if maybeLocalpart != nil {
				localpart = string(maybeLocalpart)
			}
		}
		t.Logf("CallbackServer[%s]%s: %v %s", t.Name(), localpart, time.Now(), data)
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
