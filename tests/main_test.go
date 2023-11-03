package tests

import (
	"sync"
	"testing"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/must"
)

var (
	ssDeployment *deploy.SlidingSyncDeployment
	ssMutex      *sync.Mutex
)

func TestMain(m *testing.M) {
	ssMutex = &sync.Mutex{}
	defer func() { // always teardown even if panicking
		ssMutex.Lock()
		if ssDeployment != nil {
			ssDeployment.Teardown()
		}
		ssMutex.Unlock()
	}()
	complement.TestMain(m, "crypto")

}

func Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	ssMutex.Lock()
	defer ssMutex.Unlock()
	if ssDeployment != nil {
		return ssDeployment
	}
	ssDeployment = deploy.RunNewDeployment(t)
	return ssDeployment
}

func MustLoginClient(t *testing.T, clientType api.ClientType, opts api.ClientCreationOpts, ssURL string) api.Client {
	switch clientType {
	case api.ClientTypeRust:
		c, err := api.NewRustClient(t, opts, ssURL)
		must.NotError(t, "NewRustClient: %s", err)
		return c
	case api.ClientTypeJS:
		c, err := api.NewJSClient(t, opts)
		must.NotError(t, "NewJSClient: %s", err)
		return c
	default:
		t.Fatalf("unknown client type %v", clientType)
	}
	panic("unreachable")
}
