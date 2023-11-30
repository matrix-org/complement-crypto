package rp

import (
	"testing"
)

type ReverseProxyController struct {
}

func NewReverseProxyController() *ReverseProxyController {
	return &ReverseProxyController{}
}

// InterceptResponses will interecept responses between the homeserver and client and modify them according to the ResponseTransformer.
// The RequestMatchers are applied IN THE ORDER GIVEN. All request matchers MUST matcher before the response is intercepted.
func (c *ReverseProxyController) InterceptResponses(t *testing.T, rt ResponseTransformer, matchers ...RequestMatcher) (stop func()) {
	return
}
