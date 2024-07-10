package mitm

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

// Configuration represent a single mitmproxy configuration, with all options specified.
//
// Tests will typically build up this configuration by calling `Intercept` with the paths
// they are interested in.
type Configuration struct {
	t        *testing.T
	pathCfgs map[string]*MITMPathConfiguration
	mu       *sync.Mutex
	client   *Client
}

func (c *Configuration) ForPath(partialPath string) *MITMPathConfiguration {
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
func (c *Configuration) Execute(inner func()) {
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
			requestCallbackURL := cbServer.SetOnRequestCallback(c.t, func(cd callback.Data) *callback.Response {
				newCount := count.Add(1)
				if pathConfig.blockCount > 0 && newCount > uint32(pathConfig.blockCount) {
					return nil // don't block
				}
				// block this request by sending back a fake response
				return &callback.Response{
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
	listener    func(cd callback.Data) *callback.Response

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

func (p *MITMPathConfiguration) Listen(cb func(cd callback.Data) *callback.Response) *MITMPathConfiguration {
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
