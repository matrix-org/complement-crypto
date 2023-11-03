package tests

import (
	"os"
	"sync"
	"testing"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/must"
)

const (
	TestClientsMixed    = "mixed"
	TestClientsRustOnly = "rust"
	TestClientsJSOnly   = "js"
)

var (
	ssDeployment *deploy.SlidingSyncDeployment
	ssMutex      *sync.Mutex
	testClients  = TestClientsMixed
)

func TestMain(m *testing.M) {
	ccTestClients := os.Getenv("COMPLEMENT_CRYPTO_TEST_CLIENTS")
	switch ccTestClients {
	case TestClientsRustOnly:
		testClients = TestClientsRustOnly
	case TestClientsJSOnly:
		testClients = TestClientsJSOnly
	default:
		testClients = TestClientsMixed
	}
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

func ClientTypeMatrix(t *testing.T, subTest func(tt *testing.T, a, b api.ClientType)) {
	switch testClients {
	case TestClientsJSOnly:
		t.Run("JS|JS", func(t *testing.T) {
			subTest(t, api.ClientTypeJS, api.ClientTypeJS)
		})
	case TestClientsRustOnly:
		t.Run("Rust|Rust", func(t *testing.T) {
			subTest(t, api.ClientTypeRust, api.ClientTypeRust)
		})
	case TestClientsMixed:
		t.Run("Rust|Rust", func(t *testing.T) {
			subTest(t, api.ClientTypeRust, api.ClientTypeRust)
		})
		t.Run("Rust|JS", func(t *testing.T) {
			subTest(t, api.ClientTypeRust, api.ClientTypeJS)
		})
		t.Run("JS|Rust", func(t *testing.T) {
			subTest(t, api.ClientTypeJS, api.ClientTypeRust)
		})
		t.Run("JS|JS", func(t *testing.T) {
			subTest(t, api.ClientTypeJS, api.ClientTypeJS)
		})
	}
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
