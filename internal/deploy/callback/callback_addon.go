package callback

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/matrix-org/complement/ct"
)

var lastTestName atomic.Value = atomic.Value{}

type CallbackData struct {
	Method       string          `json:"method"`
	URL          string          `json:"url"`
	AccessToken  string          `json:"access_token"`
	ResponseCode int             `json:"response_code"`
	ResponseBody json.RawMessage `json:"response_body"`
	RequestBody  json.RawMessage `json:"request_body"`
}

type CallbackResponse struct {
	// if set, changes the HTTP response status code for this request.
	RespondStatusCode int `json:"respond_status_code,omitempty"`
	// if set, changes the HTTP response body for this request.
	RespondBody json.RawMessage `json:"respond_body,omitempty"`
}

func (cd CallbackData) String() string {
	return fmt.Sprintf("%s %s (token=%s) req_len=%d => HTTP %v", cd.Method, cd.URL, cd.AccessToken, len(cd.RequestBody), cd.ResponseCode)
}

const (
	requestPath  = "/request"
	responsePath = "/response"
)

type CallbackServer struct {
	srv     *http.Server
	mux     *http.ServeMux
	baseURL string

	mu         *sync.Mutex
	onRequest  http.HandlerFunc
	onResponse http.HandlerFunc
}

func (s *CallbackServer) SetOnRequestCallback(t ct.TestLike, cb func(CallbackData) *CallbackResponse) (callbackURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRequest = s.createHandler(t, cb)
	return s.baseURL + requestPath
}
func (s *CallbackServer) SetOnResponseCallback(t ct.TestLike, cb func(CallbackData) *CallbackResponse) (callbackURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onResponse = s.createHandler(t, cb)
	return s.baseURL + responsePath
}

// Shut down the server.
func (s *CallbackServer) Close() {
	s.srv.Close()
	lastTestName.Store("")
}
func (s *CallbackServer) createHandler(t ct.TestLike, cb func(CallbackData) *CallbackResponse) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		cbRes := cb(data)
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		if cbRes == nil {
			w.Write([]byte(`{}`))
			return
		}
		cbResBytes, err := json.Marshal(cbRes)
		if err != nil {
			ct.Errorf(t, "failed to marshal callback response: %s", err)
			return
		}
		fmt.Println(string(cbResBytes))
		w.Write(cbResBytes)
	}
}

// NewCallbackServer runs a local HTTP server that can read callbacks from mitmproxy.
// Automatically listens on a high numbered port. Must be Close()d at the end of the test.
// Register callback handlers via CallbackServer.SetOnRequestCallback and CallbackServer.SetOnResponseCallback
func NewCallbackServer(t ct.TestLike, hostnameRunningComplement string) (*CallbackServer, error) {
	last := lastTestName.Load()
	if last != nil && last.(string) != "" {
		t.Logf("WARNING[%s]: NewCallbackServer called without closing the last one. Check test '%s'", t.Name(), last.(string))
	}
	lastTestName.Store(t.Name())
	mux := http.NewServeMux()

	// listen on a random high numbered port
	ln, err := net.Listen("tcp", ":0") //nolint
	if err != nil {
		return nil, fmt.Errorf("failed to listen on a tcp port: %s", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	go srv.Serve(ln)

	callbackServer := &CallbackServer{
		mux:     mux,
		srv:     srv,
		mu:      &sync.Mutex{},
		baseURL: fmt.Sprintf("http://%s:%d", hostnameRunningComplement, port),
	}
	mux.HandleFunc(requestPath, func(w http.ResponseWriter, r *http.Request) {
		callbackServer.mu.Lock()
		h := callbackServer.onRequest
		callbackServer.mu.Unlock()
		if h == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no request handler registered"}`))
			return
		}
		h(w, r)
	})
	mux.HandleFunc(responsePath, func(w http.ResponseWriter, r *http.Request) {
		callbackServer.mu.Lock()
		h := callbackServer.onResponse
		callbackServer.mu.Unlock()
		if h == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no response handler registered"}`))
			return
		}
		h(w, r)
	})

	return callbackServer, nil
}
