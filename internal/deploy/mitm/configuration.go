package mitm

import (
	"strings"
	"testing"

	"github.com/matrix-org/complement-crypto/internal/deploy/callback"
	"github.com/matrix-org/complement/must"
)

// Configuration represent a single mitmproxy configuration, with all options specified.
//
// Tests will typically build up this configuration by calling `Intercept` with the paths
// they are interested in.
type Configuration struct {
	t      *testing.T
	client *Client
}

// Filter represents a mitmproxy filter; see https://docs.mitmproxy.org/stable/concepts-filters/
// for more information.
//
// Filters can either be specified directly as a filter expression string (see FilterExpression),
// or they can be built up in code (see FilterParams). In general, tests should use FilterParams,
// and only use the FilterExpression escape hatch if they cannot express the precise filtering
// rules with FilterParams.
type Filter interface {
	FilterString() string
}

// FilterExpression represents a mitmproxy filter expression
// e.g. "~m PUT ~hq syt_aabbccddeeff"
type FilterExpression string

func (s FilterExpression) FilterString() string {
	return string(s)
}

// FilterParams represents the set of filters which are AND'd together
// to form the final filter.
type FilterParams struct {
	// The URL path must contain this string to match.
	// If unset, no path filtering is applied.
	PathContains string
	// The access token which must appear as a header.
	// If unset, no access token filtering is applied.
	AccessToken string
	// The HTTP method which must be used for this to match.
	// If unset, no HTTP method filtering is applied.
	Method string
}

func (p FilterParams) FilterString() string {
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
	if p.PathContains != "" {
		s.WriteString(" ~u .*" + p.PathContains + ".*")
	}
	if p.Method != "" {
		s.WriteString(" ~m " + strings.ToUpper(p.Method))
	}
	if p.AccessToken != "" {
		s.WriteString(" ~hq " + p.AccessToken)
	}
	return s.String()
}

// InterceptOpts specifies the desired configuration for mitmproxy
type InterceptOpts struct {
	// Which HTTP requests/responses should be caught for this configuration.
	// Any HTTP request/response which does not meet this filter criteria will
	// be passed through without invoking the callback function. If no Filter
	// is provided, all HTTP requests/responses will be caught and the respective
	// callback called for each.
	Filter Filter
	// The function to invoke when an HTTP request has met the filter
	// criteria. This callback is invoked BEFORE the request reaches the server.
	// If this callback function returns a response, the request will never
	// reach the server and the response provided will be returned instead.
	// If this callback function returns nil, or no request callback function
	// is provided, requests will be passed to the server unaltered.
	RequestCallback callback.Fn
	// The function to invoke when an HTTP response has met the filter
	// criteria. This callback is invoked AFTER the request has been processed
	// by the server. If this callback function returns a response, the
	// server response will never reach the client and the response provided
	// will be returned instead. If this callback function returns nil, or no
	// response callback function is provided, responses will be passed to the
	// client unaltered.
	ResponseCallback callback.Fn
}

// WithIntercept provides the intercept options to mitmproxy, and calls the
// `inner` function whilst that configuration has been applied. mitmproxy will
// revert back to its default configuration when `inner` returns.
func (c *Configuration) WithIntercept(opts InterceptOpts, inner func()) {
	// run a callback server
	cbServer, err := callback.NewCallbackServer(c.t, c.client.hostnameRunningComplement)
	must.NotError(c.t, "failed to start callback server", err)
	defer cbServer.Close()

	callbackAddon := map[string]any{}
	if opts.Filter != nil {
		callbackAddon["filter"] = opts.Filter.FilterString()
	}
	if opts.RequestCallback != nil {
		requestCallbackURL := cbServer.SetOnRequestCallback(c.t, opts.RequestCallback)
		callbackAddon["callback_request_url"] = requestCallbackURL
	}
	if opts.ResponseCallback != nil {
		responseCallbackURL := cbServer.SetOnResponseCallback(c.t, opts.ResponseCallback)
		callbackAddon["callback_response_url"] = responseCallbackURL
	}

	// lock the options and call inner(), unlocking afterwards.
	lockID := c.client.LockOptions(c.t, map[string]any{
		"callback": callbackAddon,
	})
	defer c.client.UnlockOptions(c.t, lockID)
	inner()
}
