package cc

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/config"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/helpers"
)

// Instance represents a test instance (process
type Instance struct {
	ssDeployment           *deploy.SlidingSyncDeployment
	ssMutex                *sync.Mutex
	complementCryptoConfig *config.ComplementCrypto
}

func NewInstance(cfg *config.ComplementCrypto) *Instance {
	return &Instance{
		ssMutex:                &sync.Mutex{},
		complementCryptoConfig: cfg,
	}
}

func (i *Instance) TestMain(m *testing.M) {
	//complementCryptoConfig = config.NewComplementCryptoConfigFromEnvVars()
	for _, binding := range i.complementCryptoConfig.Bindings() {
		binding.PreTestRun("")
	}

	complement.TestMainWithCleanup(m, "crypto", func() { // always teardown even if panicking
		i.ssMutex.Lock()
		if i.ssDeployment != nil {
			i.ssDeployment.Teardown()
		}
		i.ssMutex.Unlock()
		for _, binding := range i.complementCryptoConfig.Bindings() {
			binding.PostTestRun("")
		}
	})
}

func (i *Instance) Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	i.ssMutex.Lock()
	defer i.ssMutex.Unlock()
	if i.ssDeployment != nil {
		return i.ssDeployment
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to find working directory: %s", err)
	}
	mitmProxyAddonsDir := filepath.Join(workingDir, "mitmproxy_addons")
	i.ssDeployment = deploy.RunNewDeployment(t, mitmProxyAddonsDir, i.complementCryptoConfig.MITMDump)
	return i.ssDeployment
}

// ClientTypeMatrix enumerates all provided client permutations given by the test client
// matrix `COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX`. Creates sub-tests for each permutation
// and invokes `subTest`. Sub-tests are run in series.
func (i *Instance) ClientTypeMatrix(t *testing.T, subTest func(t *testing.T, clientTypeA, clientTypeB api.ClientType)) {
	for _, tc := range i.complementCryptoConfig.TestClientMatrix {
		tc := tc
		t.Run(fmt.Sprintf("%s|%s", tc[0], tc[1]), func(t *testing.T) {
			subTest(t, tc[0], tc[1])
		})
	}
}

// ShouldTest returns true if this language should be tested.
func (i *Instance) ShouldTest(lang api.ClientTypeLang) bool {
	return i.complementCryptoConfig.ShouldTest(lang)
}

// ForEachClientType enumerates all known client implementations and creates sub-tests for
// each. Sub-tests are run in series. Always defaults to `hs1`.
func (i *Instance) ForEachClientType(t *testing.T, subTest func(t *testing.T, clientType api.ClientType)) {
	for _, tc := range []api.ClientType{{Lang: api.ClientTypeRust, HS: "hs1"}, {Lang: api.ClientTypeJS, HS: "hs1"}} {
		tc := tc
		if !i.complementCryptoConfig.ShouldTest(tc.Lang) {
			continue
		}
		t.Run(string(tc.Lang), func(t *testing.T) {
			subTest(t, tc)
		})
	}
}

// CreateTestContext creates a new test context suitable for immediate use. The variadic clientTypes
// control how many clients are automatically registered:
//   - 1x clientType = Alice
//   - 2x clientType = Alice, Bob
//   - 3x clientType = Alice, Bob, Charlie
//
// You can then either login individual users using testContext.MustLoginClient or use the helper functions
// testContext.WithAliceAndBobSyncing which will automatically create js/rust clients and start sync loops
// for you, along with handling cleanup.
func (i *Instance) CreateTestContext(t *testing.T, clientType ...api.ClientType) *TestContext {
	deployment := i.Deploy(t)
	tc := &TestContext{
		Deployment:    deployment,
		RPCBinaryPath: i.complementCryptoConfig.RPCBinaryPath,
	}
	// pre-register alice and bob, if told
	if len(clientType) > 0 {
		tc.Alice = deployment.Register(t, clientType[0].HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
		tc.AliceClientType = clientType[0]
	}
	if len(clientType) > 1 {
		tc.Bob = deployment.Register(t, clientType[1].HS, helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
		tc.BobClientType = clientType[1]
	}
	if len(clientType) > 2 {
		tc.Charlie = deployment.Register(t, clientType[2].HS, helpers.RegistrationOpts{
			LocalpartSuffix: "charlie",
			Password:        "complement-crypto-password",
		})
		tc.CharlieClientType = clientType[2]
	}
	if len(clientType) > 3 {
		t.Fatalf("CreateTestContext: too many clients: got %d", len(clientType))
	}
	return tc
}
