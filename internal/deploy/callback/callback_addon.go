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

// Fn represents the callback function to invoke
type Fn func(Data) *Response

type Data struct {
	Method       string          `json:"method"`
	URL          string          `json:"url"`
	AccessToken  string          `json:"access_token"`
	ResponseCode int             `json:"response_code"`
	ResponseBody json.RawMessage `json:"response_body"`
	RequestBody  json.RawMessage `json:"request_body"`
}

type Response struct {
	// if set, changes the HTTP response status code for this request.
	RespondStatusCode int `json:"respond_status_code,omitempty"`
	// if set, changes the HTTP response body for this request.
	RespondBody json.RawMessage `json:"respond_body,omitempty"`
}

func (cd Data) String() string {
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

func (s *CallbackServer) SetOnRequestCallback(t ct.TestLike, cb Fn) (callbackURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRequest = s.createHandler(t, cb)
	return s.baseURL + requestPath
}
func (s *CallbackServer) SetOnResponseCallback(t ct.TestLike, cb Fn) (callbackURL string) {
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
func (s *CallbackServer) createHandler(t ct.TestLike, cb Fn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var data Data
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

// SendError returns a callback.Fn which returns the provided statusCode
// along with a JSON error $count times, after which it lets the response
// pass through. This is useful for testing retries.
func SendError(count uint32, statusCode int) Fn {
	var seen atomic.Uint32
	return func(d Data) *Response {
		next := seen.Add(1)
		if next > count {
			return nil
		}
		return &Response{
			RespondStatusCode: statusCode,
			RespondBody:       json.RawMessage(`{"error":"callback.SendError"}`),
		}
	}
}

// TODO: helpers for "wait for this conditional to pass then execute this code whilst blocking"
//  - for tarpitting
//  - for sigkilling

type PassiveChannel struct {
	recvCh  chan *Data
	timeout time.Duration
	closed  *atomic.Bool
}

func (c *PassiveChannel) Close() {
	if c.closed.CompareAndSwap(false, true) {
		close(c.recvCh)
	}
}

// Callback returns the callback implementation used to send data to this channel.
func (c *PassiveChannel) Callback() Fn {
	return func(d Data) *Response {
		if c.closed.Load() {
			return nil // test has ended, don't send on a closed channel else we panic
		}
		c.recvCh <- &d
		return nil // don't modify the response
	}
}

// Block until this channel receives a callback.
func (c *PassiveChannel) Recv(t ct.TestLike, msg string, args ...any) *Data {
	t.Helper()
	select {
	case d := <-c.recvCh:
		return d
	case <-time.After(c.timeout):
		ct.Fatalf(t, msg, args...)
	}
	panic("unreachable")
}

// Try to receive from the channel. Does not block. Returns nil if there is nothing
// waiting in the channel. Useful for detecting absence of a callback.
func (c *PassiveChannel) TryRecv(t ct.TestLike) *Data {
	t.Helper()
	select {
	case d := <-c.recvCh:
		return d
	default:
		return nil
	}
}

// Chan returns a consume-only channel.
//
// This exists as an escape hatch for when Recv/TryRecv are insufficient, and
// this channel needs to be used as part of a larger `select` block.
func (c *PassiveChannel) Chan() <-chan *Data {
	return c.recvCh
}

type ActiveChannel struct {
	*PassiveChannel
	sendCh chan *Response
}

func (c *ActiveChannel) Close() {
	if c.PassiveChannel.closed.CompareAndSwap(false, true) {
		close(c.recvCh)
		close(c.sendCh)
	}
}

// Callback returns the callback implementation used to send data to this channel.
func (c *ActiveChannel) Callback() Fn {
	return func(d Data) *Response {
		if c.closed.Load() {
			return nil // test has ended, don't send on a closed channel else we panic
		}
		c.recvCh <- &d
		// wait for the response from the test
		return <-c.sendCh
	}
}

// Send a callback response to this channel.
// Fails the test if the response cannot be put
// into the channel for $timeout time.
func (c *ActiveChannel) Send(t ct.TestLike, res *Response) {
	t.Helper()
	select {
	case c.sendCh <- res:
		return
	case <-time.After(c.timeout):
		ct.Fatalf(t, "Channel.Send timed out sending the response")
	}
}

// NewPassiveChannel returns a channel which can receive callback responses,
// but cannot modify them. This is useful for sniffing network traffic. The
// timeout controls how long Recv() can wait until there is callback data before
// failing. If blocking is true, callbacks will not return until Recv() is called.
// This can be useful for synchronising actions when a callback is invoked.
//
// Channels are useful when tests want to manipulate callbacks from within an `inner`
// function.
func NewPassiveChannel(timeout time.Duration, blocking bool) *PassiveChannel {
	// passive channels can be non-blocking as there is nothing to pair up,
	// so let there be at most 10 callbacks in-flight at any one time.
	// If this is too low then concurrent callbacks will block each other.
	// If this is too high then we consume needless amounts of memory.
	buffer := 10
	if blocking {
		buffer = 0
	}
	return &PassiveChannel{
		timeout: timeout,
		recvCh:  make(chan *Data, buffer),
		closed:  &atomic.Bool{},
	}
}

// NewActiveChannel returns a channel which can receive and modify callback responses. The
// timeout controls how long Recv() and Send() can wait until there is callback data before
// failing.
//
// Channels are useful when tests want to manipulate callbacks from within an `inner`
// function.
func NewActiveChannel(timeout time.Duration) *ActiveChannel {
	// An active channel is a passive channel with bits on top
	passive := NewPassiveChannel(timeout, true)
	return &ActiveChannel{
		PassiveChannel: passive,
		// active channels need to be blocking to pair up requests/responses,
		// so set the buffer size to 0 (blocking).
		sendCh: make(chan *Response),
	}
}
