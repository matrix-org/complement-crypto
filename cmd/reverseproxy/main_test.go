package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"sync"
	"testing"
	"time"
)

type mockHandler struct {
	serveHTTP func(w http.ResponseWriter, req *http.Request)
}

func (h *mockHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.serveHTTP(w, req)
}

type controllerHandler struct {
	t            *testing.T
	onHSRequest  func(r *http.Request)
	onHSResponse func(res *http.Response) *http.Response
}

func (h *controllerHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.t.Errorf("ReadAll: %s", err)
	}
	h.t.Logf("controller recv %v : %v", req.URL.Path, string(body))
	if req.URL.Path == "/request" {
		proxyReq, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(body)))
		if err != nil {
			h.t.Errorf("ReadRequest: %s", err)
		}
		h.onHSRequest(proxyReq)
		// echo back the body
		w.Write(body)
	} else if req.URL.Path == "/response" {
		proxyRes, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(body)), nil)
		if err != nil {
			h.t.Errorf("ReadResponse: %s", err)
		}
		modifiedRes := h.onHSResponse(proxyRes)
		dump, err := httputil.DumpResponse(modifiedRes, true)
		if err != nil {
			h.t.Errorf("DumpResponse: %v", err)
		}
		w.Write(dump)
	} else {
		h.t.Errorf("controller got unknown path: %s", req.URL.Path)
	}
}

func listenAndServe(addr string, h http.Handler) (close func()) {
	srv := http.Server{
		Addr:    addr,
		Handler: h,
	}
	go srv.ListenAndServe()
	return func() {
		srv.Close()
	}
}

func TestReverseProxy(t *testing.T) {
	// the controller is something tests make.
	controllerPort := 9050
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)
	controller := &controllerHandler{
		t:           t,
		onHSRequest: func(r *http.Request) {},
		// onHSResponse will be set in tests
	}
	closeController := listenAndServe(fmt.Sprintf("127.0.0.1:%d", controllerPort), controller)
	defer closeController()

	// the mock hs is a synapse which produces responses
	mockHSPort := 9051
	var mu sync.Mutex
	mockHSServeHTTP := func(w http.ResponseWriter, req *http.Request) {} // replaced in tests
	mockHS := &mockHandler{
		serveHTTP: func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			mockHSServeHTTP(w, req)
		},
	}
	closeHS := listenAndServe(fmt.Sprintf("127.0.0.1:%d", mockHSPort), mockHS)
	defer closeHS()

	// the reverse proxy is the thing clients hit, which will hit the controller then the hs
	reverseProxyListenPort := 9052
	hsURL := fmt.Sprintf("http://localhost:%d,%d", mockHSPort, reverseProxyListenPort)
	crp, port := newComplementProxy(controllerURL, hsURL)
	if port != reverseProxyListenPort {
		t.Fatalf("newComplementProxy: got port %d want %d", port, reverseProxyListenPort)
	}
	closeReverseProxy := listenAndServe(fmt.Sprintf("127.0.0.1:%d", port), crp)
	defer closeReverseProxy()
	reverseProxyURL := fmt.Sprintf("http://localhost:%d", reverseProxyListenPort)

	// wait for things to be listening
	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
		name string
		// client request
		inMethod string
		inPath   string
		inBody   string
		// mock hs response
		hsReturnBody   string
		hsReturnStatus int
		// proxy transforms
		transformResBody    func([]byte) []byte
		transformStatusCode func(int) int
		// assertions
		wantStatusCode int
		wantBody       string
	}{
		{
			name: "no transformations",

			inMethod: "GET",
			inPath:   "/foo/bar",
			inBody:   "",

			hsReturnBody:   "hello world",
			hsReturnStatus: 200,

			wantStatusCode: 200,
			wantBody:       "hello world",
		},
		{
			name: "status code transformation",

			inMethod: "GET",
			inPath:   "/foo/bar",
			inBody:   "",

			hsReturnBody:   "hello world",
			hsReturnStatus: 200,

			transformStatusCode: func(i int) int {
				return 201
			},

			wantStatusCode: 201,
			wantBody:       "hello world",
		},
		{
			name: "response body transformations",

			inMethod: "GET",
			inPath:   "/foo/bar",
			inBody:   "",

			hsReturnBody:   "hello world",
			hsReturnStatus: 200,

			transformResBody: func(s []byte) []byte {
				return []byte("goodbye world")
			},

			wantStatusCode: 200,
			wantBody:       "goodbye world",
		},
		{
			name: "status and body transformations",

			inMethod: "GET",
			inPath:   "/foo/bar",
			inBody:   "",

			hsReturnBody:   "hello world",
			hsReturnStatus: 200,

			transformStatusCode: func(i int) int {
				return 400
			},
			transformResBody: func(s []byte) []byte {
				return []byte(`{"error":"oh no!"}`)
			},

			wantStatusCode: 400,
			wantBody:       `{"error":"oh no!"}`,
		},
		{
			name: "POST request",

			inMethod: "POST",
			inPath:   "/createRoom",
			inBody:   "{}",

			hsReturnBody:   "this is the way",
			hsReturnStatus: 200,

			wantStatusCode: 200,
			wantBody:       `this is the way`,
		},

		{
			name: "POST request transform",

			inMethod: "POST",
			inPath:   "/createRoom2",
			inBody:   "[1,23,4]",

			hsReturnBody:   "this is still the way",
			hsReturnStatus: 200,

			transformResBody: func(s []byte) []byte {
				return []byte("this is not the way")
			},
			transformStatusCode: func(i int) int {
				return 201
			},

			wantStatusCode: 201,
			wantBody:       `this is not the way`,
		},
	}
	for _, tc := range testCases {
		var inBody io.Reader
		if tc.inBody != "" {
			inBody = bytes.NewBufferString(tc.inBody)
		}
		inReq, err := http.NewRequest(tc.inMethod, reverseProxyURL+tc.inPath, inBody)
		if err != nil {
			t.Fatalf("NewRequest: %s", err)
		}
		mockHSServeHTTP = func(w http.ResponseWriter, req *http.Request) {
			// make sure we proxied the request correctly
			if req.URL.Path != tc.inPath {
				t.Errorf("HS received unexpected path: got %v want %v", req.URL.Path, tc.inPath)
			}
			if req.Method != tc.inMethod {
				t.Errorf("HS received unexpected method: got %v want %v", req.Method, tc.inMethod)
			}
			defer req.Body.Close()
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("HS cannot read body: %s", err)
			}
			if !bytes.Equal(body, []byte(tc.inBody)) {
				t.Errorf("HS received unexpected body: got '%v' want '%v'", string(body), tc.inBody)
			}
			// return the response we're told to in this test
			w.WriteHeader(tc.hsReturnStatus)
			w.Write([]byte(tc.hsReturnBody))
		}
		controller.onHSResponse = func(res *http.Response) *http.Response {
			if tc.transformStatusCode != nil {
				res.StatusCode = tc.transformStatusCode(res.StatusCode)
			}
			if tc.transformResBody != nil {
				hsBody, err := io.ReadAll(res.Body)
				if err != nil {
					t.Errorf("ReadAll: %s", err)
					return res
				}
				newBody := tc.transformResBody(hsBody)
				res.Body = io.NopCloser(bytes.NewBuffer(newBody))
				res.ContentLength = int64(len(newBody))
			}
			return res
		}
		gotRes, err := http.DefaultClient.Do(inReq)
		if err != nil {
			t.Fatalf("Do: %s", err)
		}
		if gotRes.StatusCode != tc.wantStatusCode {
			t.Errorf("%s: got status %d want %d", tc.name, gotRes.StatusCode, tc.wantStatusCode)
		}
		var gotBody []byte
		if gotRes.Body != nil {
			gotBody, err = io.ReadAll(gotRes.Body)
			gotRes.Body.Close()
			if err != nil {
				t.Fatalf("ReadAll: %s", err)
			}
		}
		if string(gotBody) != tc.wantBody {
			t.Errorf("%s: got body '%s' want '%s'", tc.name, string(gotBody), tc.wantBody)
		}
	}
}
