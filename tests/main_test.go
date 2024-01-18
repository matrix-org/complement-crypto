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
	js.SetupJSLogs("./logs/js_sdk.log")                  // rust sdk logs on its own
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

func ForEachClientType(t *testing.T, subTest func(tt *testing.T, a api.ClientType)) {
	for _, tc := range []api.ClientType{{Lang: api.ClientTypeRust, HS: "hs1"}, {Lang: api.ClientTypeJS, HS: "hs1"}} {
		tc := tc
		t.Run(string(tc.Lang), func(t *testing.T) {
			subTest(t, tc)
		})
	}
}

func MustCreateClient(t *testing.T, clientType api.ClientType, cfg api.ClientCreationOpts, ssURL string, opts ...func(api.Client, api.ClientCreationOpts)) api.Client {
	var c api.Client
	switch clientType.Lang {
	case api.ClientTypeRust:
		client, err := rust.NewRustClient(t, cfg, ssURL)
		must.NotError(t, "NewRustClient: %s", err)
		c = client
	case api.ClientTypeJS:
		client, err := js.NewJSClient(t, cfg)
		must.NotError(t, "NewJSClient: %s", err)
		c = client
	default:
		t.Fatalf("unknown client type %v", clientType)
	}
	for _, o := range opts {
		o(c, cfg)
	}
	return c
}

func WithDoLogin(t *testing.T) func(api.Client, api.ClientCreationOpts) {
	return func(c api.Client, opts api.ClientCreationOpts) {
		must.NotError(t, "failed to login", c.Login(t, opts))
	}
}

type TestContext struct {
	Deployment *deploy.SlidingSyncDeployment
	Alice      *client.CSAPI
	Bob        *client.CSAPI
}

func CreateTestContext(t *testing.T, clientType ...api.ClientType) *TestContext {
	deployment := Deploy(t)
	tc := &TestContext{
		Deployment: deployment,
	}
	// pre-register alice and bob, if told
	if len(clientType) > 0 {
		tc.Alice = deployment.Register(t, clientType[0].HS, helpers.RegistrationOpts{
			LocalpartSuffix: "alice",
			Password:        "complement-crypto-password",
		})
	}
	if len(clientType) > 1 {
		tc.Bob = deployment.Register(t, clientType[1].HS, helpers.RegistrationOpts{
			LocalpartSuffix: "bob",
			Password:        "complement-crypto-password",
		})
	}
	if len(clientType) > 2 {
		t.Fatalf("CreateTestContext: too many clients: got %d", len(clientType))
	}
	return tc
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
	client := MustCreateClient(t, clientType, cfg, dep.SlidingSyncURL(t))
	must.NotError(t, "failed to login client", client.Login(t, cfg))
	return client
}
