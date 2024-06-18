package deploy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/matrix-org/complement/must"
)

// must match the value in tests/addons/__init__.py
const magicMITMURL = "http://mitm.code"

type MITMClient struct {
	client                    *http.Client
	hostnameRunningComplement string
}

func NewMITMClient(proxyURL *url.URL, hostnameRunningComplement string) *MITMClient {
	return &MITMClient{
		hostnameRunningComplement: hostnameRunningComplement,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		},
	}
}

func (m *MITMClient) WithSniffedEndpoint(t *testing.T, partialPath string, onSniff func(CallbackData), inner func()) {
	t.Helper()
	callbackURL, closeCallbackServer := NewCallbackServer(t, m.hostnameRunningComplement, onSniff)
	defer closeCallbackServer()
	m.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			// the filter is a python regexp
			// "Regexes are Python-style" - https://docs.mitmproxy.org/stable/concepts-filters/
			// re.escape() escapes very little:
			// "Changed in version 3.7: Only characters that can have special meaning in a regular expression are escaped.
			// As a result, '!', '"', '%', "'", ',', '/', ':', ';', '<', '=', '>', '@', and "`" are no longer escaped."
			// https://docs.python.org/3/library/re.html#re.escape
			//
			// The majority of HTTP paths are just /foo/bar with % for path-encoding e.g @foo:bar=>%40foo%3Abar,
			// so on balance we can probably just use the path directly.
			"filter": "~u .*" + partialPath + ".*",
		},
	}, func() {
		inner()
	})
}

// WithMITMOptions changes the options of mitmproxy and executes inner() whilst those options are in effect.
// As the options on mitmproxy are a shared resource, this function has transaction-like semantics, ensuring
// the lock is released when inner() returns. This is similar to the `with` keyword in python.
func (m *MITMClient) WithMITMOptions(t *testing.T, options map[string]interface{}, inner func()) {
	t.Helper()
	lockID := m.lockOptions(t, options)
	defer m.unlockOptions(t, lockID)
	inner()
}

func (m *MITMClient) lockOptions(t *testing.T, options map[string]interface{}) (lockID []byte) {
	jsonBody, err := json.Marshal(map[string]interface{}{
		"options": options,
	})
	t.Logf("lockOptions: %v", string(jsonBody))
	must.NotError(t, "failed to marshal options", err)
	u := magicMITMURL + "/options/lock"
	req, err := http.NewRequest("POST", u, bytes.NewBuffer(jsonBody))
	must.NotError(t, "failed to prepare request", err)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	must.NotError(t, "failed to POST "+u, err)
	must.Equal(t, res.StatusCode, 200, "controller returned wrong HTTP status")
	lockID, err = io.ReadAll(res.Body)
	must.NotError(t, "failed to read response", err)
	return lockID
}

func (m *MITMClient) unlockOptions(t *testing.T, lockID []byte) {
	t.Logf("unlockOptions")
	req, err := http.NewRequest("POST", magicMITMURL+"/options/unlock", bytes.NewBuffer(lockID))
	must.NotError(t, "failed to prepare request", err)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	must.NotError(t, "failed to do request", err)
	must.Equal(t, res.StatusCode, 200, "controller returned wrong HTTP status")
}
