package tests

import (
	"fmt"
	"sync"
	"testing"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
	"github.com/matrix-org/complement-crypto/internal/config"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

var (
	ssDeployment           *deploy.SlidingSyncDeployment
	ssMutex                *sync.Mutex
	complementCryptoConfig *config.ComplementCrypto // set in TestMain
)

func TestMain(m *testing.M) {
	complementCryptoConfig = config.NewComplementCryptoConfigFromEnvVars()
	ssMutex = &sync.Mutex{}
	js.SetupJSLogs("js_sdk.log")                         // rust sdk logs on its own
	complement.TestMainWithCleanup(m, "crypto", func() { // always teardown even if panicking
		ssMutex.Lock()
		if ssDeployment != nil {
			ssDeployment.Teardown(complementCryptoConfig.WriteContainerLogs)
		}
		ssMutex.Unlock()
		js.WriteJSLogs()
	})
}

func Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	ssMutex.Lock()
	defer ssMutex.Unlock()
	if ssDeployment != nil {
		return ssDeployment
	}
	ssDeployment = deploy.RunNewDeployment(t, complementCryptoConfig.TCPDump)
	return ssDeployment
}

func ClientTypeMatrix(t *testing.T, subTest func(tt *testing.T, a, b api.ClientType)) {
	for _, tc := range complementCryptoConfig.TestClientMatrix {
		tc := tc
		t.Run(fmt.Sprintf("%s|%s", tc[0], tc[1]), func(t *testing.T) {
			subTest(t, tc[0], tc[1])
		})
	}
}

func MustLoginClient(t *testing.T, clientType api.ClientType, opts api.ClientCreationOpts, ssURL string) api.Client {
	switch clientType.Lang {
	case api.ClientTypeRust:
		c, err := rust.NewRustClient(t, opts, ssURL)
		must.NotError(t, "NewRustClient: %s", err)
		return c
	case api.ClientTypeJS:
		c, err := js.NewJSClient(t, opts)
		must.NotError(t, "NewJSClient: %s", err)
		return c
	default:
		t.Fatalf("unknown client type %v", clientType)
	}
	panic("unreachable")
}

type TestContext struct {
	Deployment *deploy.SlidingSyncDeployment
	Alice      *client.CSAPI
	Bob        *client.CSAPI
}

func CreateTestContext(t *testing.T, clientTypeA, clientTypeB api.ClientType) *TestContext {
	deployment := Deploy(t)
	// pre-register alice and bob
	csapiAlice := deployment.Register(t, clientTypeA.HS, helpers.RegistrationOpts{
		LocalpartSuffix: "alice",
		Password:        "complement-crypto-password",
	})
	csapiBob := deployment.Register(t, clientTypeB.HS, helpers.RegistrationOpts{
		LocalpartSuffix: "bob",
		Password:        "complement-crypto-password",
	})
	return &TestContext{
		Deployment: deployment,
		Alice:      csapiAlice,
		Bob:        csapiBob,
	}
}

func (c *TestContext) CreateNewEncryptedRoom(t *testing.T, creator *client.CSAPI, preset string, invite []string) (roomID string) {
	t.Helper()
	if invite == nil {
		invite = []string{} // else synapse 500s
	}
	return creator.MustCreateRoom(t, map[string]interface{}{
		"name":   t.Name(),
		"preset": preset,
		"invite": invite,
		"initial_state": []map[string]interface{}{
			{
				"type":      "m.room.encryption",
				"state_key": "",
				"content": map[string]interface{}{
					"algorithm": "m.megolm.v1.aes-sha2",
				},
			},
		},
	})
}

func (c *TestContext) MustLoginDevice(t *testing.T, existing *client.CSAPI, clientType api.ClientType, deviceID string) (*client.CSAPI, api.Client) {
	newClient := c.Deployment.Login(t, clientType.HS, existing, helpers.LoginOpts{
		DeviceID: deviceID,
		Password: "complement-crypto-password",
	})
	return newClient, c.MustLoginClient(t, newClient, clientType)
}

func (c *TestContext) MustLoginClient(t *testing.T, cli *client.CSAPI, clientType api.ClientType) api.Client {
	return LoginClientFromComplementClient(t, c.Deployment, cli, clientType)
}

func LoginClientFromComplementClient(t *testing.T, dep *deploy.SlidingSyncDeployment, cli *client.CSAPI, clientType api.ClientType) api.Client {
	t.Helper()
	cfg := api.FromComplementClient(cli, "complement-crypto-password")
	cfg.BaseURL = dep.ReverseProxyURLForHS(clientType.HS)
	return MustLoginClient(t, clientType, cfg, dep.SlidingSyncURL(t))
}
