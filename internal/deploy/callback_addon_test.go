package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
	"github.com/tidwall/gjson"
)

func TestMain(m *testing.M) {
	complement.TestMain(m, "deploy")
}

// Test the functionality of the mitmproxy addon 'callback'.
func TestCallbackAddon(t *testing.T) {
	workingDir, err := os.Getwd()
	must.NotError(t, "failed to get working dir", err)
	mitmProxyAddonsDir := filepath.Join(workingDir, "../../tests/mitmproxy_addons")
	deployment := RunNewDeployment(t, mitmProxyAddonsDir, "")
	defer deployment.Teardown()
	client := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "callback",
	})
	other := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "callback",
	})

	testCases := []struct {
		name                 string
		filter               string
		needsRequestCallback bool
		inner                func(t *testing.T, checker *checker)
	}{
		{
			name:   "works",
			filter: "",
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					Method:       "GET",
					PathContains: "_matrix/client/v3/capabilities",
					AccessToken:  client.AccessToken,
					ResponseCode: 200,
				})
				client.GetCapabilities(t)
				checker.wait()
			},
		},
		{
			name:   "can be filtered by path",
			filter: "~u capabilities",
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					Method:       "GET",
					PathContains: "_matrix/client/v3/capabilities",
					AccessToken:  client.AccessToken,
					ResponseCode: 200,
				})
				client.GetCapabilities(t)
				checker.wait()
				checker.expectNoCallbacks(true)
				client.GetGlobalAccountData(t, "this_does_a_get")
				checker.expectNoCallbacks(false)
			},
		},
		{
			name:   "can be filtered by method",
			filter: "~m GET",
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					Method:       "GET",
					PathContains: "_matrix/client/v3/capabilities",
					AccessToken:  client.AccessToken,
					ResponseCode: 200,
				})
				client.GetCapabilities(t)
				checker.wait()
				checker.expectNoCallbacks(true)
				client.MustSetGlobalAccountData(t, "this_does_a_put", map[string]any{})
				checker.expectNoCallbacks(false)
			},
		},
		{
			name:   "can be filtered by access token",
			filter: "~hq " + client.AccessToken,
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					Method:       "GET",
					PathContains: "_matrix/client/v3/capabilities",
					AccessToken:  client.AccessToken,
					ResponseCode: 200,
				})
				client.GetCapabilities(t)
				checker.wait()
				checker.expectNoCallbacks(true)
				other.GetCapabilities(t)
				checker.expectNoCallbacks(false)
			},
		},
		{
			name:   "can be filtered by combinations of method path and access token",
			filter: "~m GET ~u capabilities ~hq " + client.AccessToken,
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					Method:       "GET",
					PathContains: "_matrix/client/v3/capabilities",
					AccessToken:  client.AccessToken,
					ResponseCode: 200,
				})
				client.GetCapabilities(t)
				checker.wait()
				checker.expectNoCallbacks(true)
				other.GetCapabilities(t)
				checker.expectNoCallbacks(false)
			},
		},
		{
			// ensure that if we tarpit a request it doesn't tarpit unrelated requests
			name:   "processes callbacks concurrently",
			filter: "~hq " + client.AccessToken,
			inner: func(t *testing.T, checker *checker) {
				// signal when to make the unrelated request
				signalSendUnrelatedRequest := make(chan bool)
				signalTestFinished := make(chan bool)
				checker.expect(&callbackRequest{
					OnCallback: func(cd CallbackData) *CallbackResponse {
						if strings.Contains(cd.URL, "capabilities") {
							close(signalSendUnrelatedRequest) // send the signal to make the unrelated request
							time.Sleep(time.Second)           // tarpit this request
							close(signalTestFinished)         // test is done, cleanup
						}
						return nil
					},
				})
				beforeSendingRequests := time.Now()
				// send the tarpit request without waiting
				go func() {
					client.GetCapabilities(t)
				}()
				select {
				case <-signalSendUnrelatedRequest:
					// send the unrelated request
					t.Logf("received signal @ %v", time.Since(beforeSendingRequests))
					client.GetGlobalAccountData(t, "this_does_a_get")
					t.Logf("received unrelated response @ %v", time.Since(beforeSendingRequests))
				case <-time.After(time.Second):
					ct.Errorf(t, "did not receive signal to send unrelated request")
					return
				}
				since := time.Since(beforeSendingRequests)
				if since > time.Second {
					ct.Errorf(t, "unrelated request was tarpitted, took %v", since)
					return
				}

				// now wait for the tarpit
				select {
				case <-signalTestFinished:
				case <-time.After(2 * time.Second):
					ct.Errorf(t, "timed out waiting for tarpit response")
				}
			},
		},
		{
			name:   "can modify response codes without modifying the response body",
			filter: "~hq " + client.AccessToken,
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					OnCallback: func(cd CallbackData) *CallbackResponse {
						return &CallbackResponse{
							RespondStatusCode: 404,
						}
					},
				})
				res := client.Do(t, "GET", []string{"_matrix", "client", "v3", "capabilities"})
				checker.wait()
				must.Equal(t, res.StatusCode, 404, "response code was not altered")
				body, err := io.ReadAll(res.Body)
				must.NotError(t, "failed to read CSAPI response", err)
				must.Equal(t, gjson.ParseBytes(body).Get("capabilities").Exists(), true, "response body was modified")
			},
		},
		{
			name:   "can modify response bodies without modifying the response code",
			filter: "~hq " + client.AccessToken,
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					OnCallback: func(cd CallbackData) *CallbackResponse {
						return &CallbackResponse{
							RespondBody: json.RawMessage(`{
								"foo": "bar"
							}`),
						}
					},
				})
				res := client.Do(t, "GET", []string{"_matrix", "client", "v3", "capabilities"})
				checker.wait()
				must.Equal(t, res.StatusCode, 200, "response code was modified")
				body, err := io.ReadAll(res.Body)
				must.NotError(t, "failed to read CSAPI response", err)
				must.Equal(t, gjson.ParseBytes(body).Get("foo").Str, "bar", "response body was not altered")
			},
		},
		{
			name:   "can modify response codes and bodies",
			filter: "~hq " + client.AccessToken,
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					OnCallback: func(cd CallbackData) *CallbackResponse {
						return &CallbackResponse{
							RespondStatusCode: 403,
							RespondBody: json.RawMessage(`{
								"foo": "bar"
							}`),
						}
					},
				})
				res := client.Do(t, "GET", []string{"_matrix", "client", "v3", "capabilities"})
				checker.wait()
				must.Equal(t, res.StatusCode, 403, "response code was not modified")
				body, err := io.ReadAll(res.Body)
				must.NotError(t, "failed to read CSAPI response", err)
				must.Equal(t, gjson.ParseBytes(body).Get("foo").Str, "bar", "response body was not modified")
			},
		},
		{
			name:                 "can block requests and modify response codes and bodies",
			filter:               "~m PUT",
			needsRequestCallback: true,
			inner: func(t *testing.T, checker *checker) {
				checker.expect(&callbackRequest{
					OnRequestCallback: func(cd CallbackData) *CallbackResponse {
						return &CallbackResponse{
							RespondStatusCode: 200,
							RespondBody:       json.RawMessage(`{"yep": "ok"}`),
						}
					},
				})
				// This is a PUT so will be intercepted
				res := client.MustSetGlobalAccountData(t, "this_wont_go_through", map[string]any{"foo": "bar"})
				checker.wait()
				must.Equal(t, res.StatusCode, 200, "response code was not set")
				body, err := io.ReadAll(res.Body)
				must.NotError(t, "failed to read CSAPI response", err)
				must.Equal(t, gjson.ParseBytes(body).Get("yep").Str, "ok", "response body was not set")

				// now check it didn't go through by doing a GET which isn't intercepted
				res = client.GetGlobalAccountData(t, "this_wont_go_through")
				must.Equal(t, res.StatusCode, 404, "GET returned data when the PUT should have been intercepted")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			checker := &checker{
				t:  t,
				ch: make(chan callbackRequest, 3),
				mu: &sync.Mutex{},
			}
			cbServer, err := NewCallbackServer(
				t, deployment.GetConfig().HostnameRunningComplement,
			)
			callbackURL := cbServer.SetOnResponseCallback(t, func(cd CallbackData) *CallbackResponse {
				return checker.onResponseCallback(cd)
			})
			var reqCallbackURL string
			if tc.needsRequestCallback {
				reqCallbackURL = cbServer.SetOnRequestCallback(t, func(cd CallbackData) *CallbackResponse {
					return checker.onRequestCallback(cd)
				})
			}
			must.NotError(t, "failed to create callback server", err)
			defer cbServer.Close()
			callbackOpts := map[string]any{
				"callback_response_url": callbackURL,
			}
			if tc.filter != "" {
				callbackOpts["filter"] = tc.filter
			}
			if reqCallbackURL != "" {
				callbackOpts["callback_request_url"] = reqCallbackURL
			}

			mitmClient := deployment.MITM()
			lockID := mitmClient.lockOptions(t, map[string]any{
				"callback": callbackOpts,
			})
			tc.inner(t, checker)
			mitmClient.unlockOptions(t, lockID)
		})
	}
}

type callbackRequest struct {
	Method            string
	PathContains      string
	AccessToken       string
	ResponseCode      int
	OnRequestCallback func(cd CallbackData) *CallbackResponse
	OnCallback        func(cd CallbackData) *CallbackResponse
}

type checker struct {
	t           *testing.T
	ch          chan callbackRequest
	mu          *sync.Mutex
	want        *callbackRequest
	noCallbacks bool
}

func (c *checker) onResponseCallback(cd CallbackData) *CallbackResponse {
	c.mu.Lock()
	if c.noCallbacks {
		ct.Errorf(c.t, "wanted no callbacks but got %+v", cd)
	}
	if c.want == nil {
		c.mu.Unlock()
		return nil
	}
	if c.want.AccessToken != "" {
		must.Equal(c.t, cd.AccessToken, c.want.AccessToken, "access token mismatch")
	}
	if c.want.Method != "" {
		must.Equal(c.t, cd.Method, c.want.Method, "HTTP method mismatch")
	}
	if c.want.PathContains != "" {
		must.Equal(c.t, strings.Contains(cd.URL, c.want.PathContains), true,
			fmt.Sprintf("path mismatch, got %v want partial %v", cd.URL, c.want.PathContains),
		)
	}
	if c.want.ResponseCode != 0 {
		must.Equal(c.t, cd.ResponseCode, c.want.ResponseCode, "response code mismatch")
	}

	customCallback := c.want.OnCallback
	// unlock early so we don't block other requests, as custom callbacks are generally
	// used for testing tarpitting.
	c.mu.Unlock()
	var callbackResponse *CallbackResponse
	if customCallback != nil {
		callbackResponse = customCallback(cd)
	}
	// signal that we processed the callback
	c.ch <- *c.want
	return callbackResponse
}

func (c *checker) onRequestCallback(cd CallbackData) *CallbackResponse {
	c.mu.Lock()
	cb := c.want.OnRequestCallback
	c.mu.Unlock()
	if cb != nil {
		return cb(cd)
	}
	return nil
}

func (c *checker) expect(want *callbackRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.want = want
}

func (c *checker) expectNoCallbacks(noCallbacks bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.noCallbacks = noCallbacks
}

func (c *checker) wait() {
	c.t.Helper()
	select {
	case got := <-c.ch:
		// we can't sanity check if there are callbacks involved, as we can't easily
		// pair responses up.
		if c.want.OnCallback == nil && c.want.OnRequestCallback == nil && !reflect.DeepEqual(got, *c.want) {
			ct.Fatalf(c.t, "checker: got success from a different request: did you forget to wait?"+
				" Received %+v but expected +%v", got, c.want)
		}
		return
	case <-time.After(time.Second):
		ct.Fatalf(c.t, "timed out waiting for %+v", c.want)
	}
}
