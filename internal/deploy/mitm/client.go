package mitm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/complement/must"
)

// must match the value in tests/addons/__init__.py
const magicMITMURL = "http://mitm.code"

var (
	boolTrue  = true
	boolFalse = false
)

type Client struct {
	client                    *http.Client
	hostnameRunningComplement string
}

func NewClient(proxyURL *url.URL, hostnameRunningComplement string) *Client {
	return &Client{
		hostnameRunningComplement: hostnameRunningComplement,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		},
	}
}

func (m *Client) Configure(t *testing.T) *Configuration {
	return &Configuration{
		t:        t,
		pathCfgs: make(map[string]*MITMPathConfiguration),
		mu:       &sync.Mutex{},
		client:   m,
	}
}

func (m *Client) lockOptions(t *testing.T, options map[string]any) (lockID []byte) {
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

func (m *Client) unlockOptions(t *testing.T, lockID []byte) {
	t.Logf("unlockOptions")
	req, err := http.NewRequest("POST", magicMITMURL+"/options/unlock", bytes.NewBuffer(lockID))
	must.NotError(t, "failed to prepare request", err)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	must.NotError(t, "failed to do request", err)
	must.Equal(t, res.StatusCode, 200, "controller returned wrong HTTP status")
}
