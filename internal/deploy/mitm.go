package deploy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

// must match the value in tests/addons/__init__.py
const magicMITMURL = "http://mitm.code"

var (
	boolTrue  = true
	boolFalse = false
)

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

func (m *MITMClient) Configure(t *testing.T) *MITMConfiguration {
	return &MITMConfiguration{
		t:        t,
		pathCfgs: make(map[string]*MITMPathConfiguration),
		mu:       &sync.Mutex{},
		client:   m,
	}
}

func (m *MITMClient) lockOptions(t *testing.T, options map[string]any) (lockID []byte) {
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

// MITMConfiguration represent a single mitmproxy configuration, with all options specified.
//
// Tests will typically build up this configuration by calling `Intercept` with the paths
// they are interested in.
type MITMConfiguration struct {
	t        *testing.T
	pathCfgs map[string]*MITMPathConfiguration
	mu       *sync.Mutex
	client   *MITMClient
}

func (c *MITMConfiguration) ForPath(partialPath string) *MITMPathConfiguration {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pathCfgs[partialPath]
	if ok {
		return p
	}
	p = &MITMPathConfiguration{
		t:    c.t,
		path: partialPath,
	}
	c.pathCfgs[partialPath] = p
	return p
}

// Execute a mitm proxy configuration for the duration of `inner`.
func (c *MITMConfiguration) Execute(inner func()) {
	// The HTTP request to mitmproxy needs to look like:
	//   {
	//     $addon_name: {
	//       $addon_values...
	//     }
	//   }
	//
	// The API shape of the add-ons are located inside the python files in tests/mitmproxy_addons
	if len(c.pathCfgs) > 1 {
		c.t.Fatalf(">1 path config currently unsupported") // TODO
	}
	c.mu.Lock()
	callbackAddon := map[string]any{}
	for _, pathConfig := range c.pathCfgs {
		if pathConfig.filter() != "" {
			callbackAddon["filter"] = pathConfig.filter()
		}
		cbServer, err := callback.NewCallbackServer(c.t, c.client.hostnameRunningComplement)
		must.NotError(c.t, "failed to start callback server", err)
		defer cbServer.Close()

		if pathConfig.listener != nil {
			responseCallbackURL := cbServer.SetOnResponseCallback(c.t, pathConfig.listener)
			callbackAddon["callback_response_url"] = responseCallbackURL
		}
		if pathConfig.blockRequest != nil && *pathConfig.blockRequest {
			// reimplement statuscode plugin logic in Go
			// TODO: refactor this
			var count atomic.Uint32
			requestCallbackURL := cbServer.SetOnRequestCallback(c.t, func(cd callback.CallbackData) *callback.CallbackResponse {
				newCount := count.Add(1)
				if pathConfig.blockCount > 0 && newCount > uint32(pathConfig.blockCount) {
					return nil // don't block
				}
				// block this request by sending back a fake response
				return &callback.CallbackResponse{
					RespondStatusCode: pathConfig.blockStatusCode,
					RespondBody:       json.RawMessage(`{"error":"complement-crypto says no"}`),
				}
			})
			callbackAddon["callback_request_url"] = requestCallbackURL
		}
	}
	c.mu.Unlock()

	lockID := c.client.lockOptions(c.t, map[string]any{
		"callback": callbackAddon,
	})
	defer c.client.unlockOptions(c.t, lockID)
	inner()

}

type MITMPathConfiguration struct {
	t           *testing.T
	path        string
	accessToken string
	method      string
	listener    func(cd callback.CallbackData) *callback.CallbackResponse

	blockCount      int
	blockStatusCode int
	blockRequest    *bool // nil means don't block
}

func (p *MITMPathConfiguration) filter() string {
	// the filter is a python regexp
	// "Regexes are Python-style" - https://docs.mitmproxy.org/stable/concepts-filters/
	// re.escape() escapes very little:
	// "Changed in version 3.7: Only characters that can have special meaning in a regular expression are escaped.
	// As a result, '!', '"', '%', "'", ',', '/', ':', ';', '<', '=', '>', '@', and "`" are no longer escaped."
	// https://docs.python.org/3/library/re.html#re.escape
	//
	// The majority of HTTP paths are just /foo/bar with % for path-encoding e.g @foo:bar=>%40foo%3Abar,
	// so on balance we can probably just use the path directly.
	var s strings.Builder
	s.WriteString("~u .*" + p.path + ".*")
	if p.method != "" {
		s.WriteString(" ~m " + strings.ToUpper(p.method))
	}
	if p.accessToken != "" {
		s.WriteString(" ~hq " + p.accessToken)
	}
	return s.String()
}

func (p *MITMPathConfiguration) Listen(cb func(cd callback.CallbackData) *callback.CallbackResponse) *MITMPathConfiguration {
	p.listener = cb
	return p
}

func (p *MITMPathConfiguration) AccessToken(accessToken string) *MITMPathConfiguration {
	p.accessToken = accessToken
	return p
}

func (p *MITMPathConfiguration) Method(method string) *MITMPathConfiguration {
	p.method = method
	return p
}

func (p *MITMPathConfiguration) BlockRequest(count, returnStatusCode int) *MITMPathConfiguration {
	if p.blockRequest != nil {
		// we can't express blocking requests and responses separately, it doesn't make sense.
		ct.Fatalf(p.t, "BlockRequest or BlockResponse cannot be called multiple times for the same path")
	}
	p.blockCount = count
	p.blockRequest = &boolTrue
	p.blockStatusCode = returnStatusCode
	return p
}

func (p *MITMPathConfiguration) BlockResponse(count, returnStatusCode int) *MITMPathConfiguration {
	if p.blockRequest != nil {
		ct.Fatalf(p.t, "BlockRequest or BlockResponse cannot be called multiple times for the same path")
	}
	p.blockCount = count
	p.blockRequest = &boolFalse
	p.blockStatusCode = returnStatusCode
	return p
}
