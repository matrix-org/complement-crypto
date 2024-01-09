package tests

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

var (
	ssDeployment     *deploy.SlidingSyncDeployment
	ssMutex          *sync.Mutex
	testClientMatrix = [][2]api.ClientType{} // set in TestMain
)

func TestMain(m *testing.M) {
	ccTestClients := os.Getenv("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX")
	if ccTestClients == "" {
		ccTestClients = "jj,jr,rj,rr"
	}
	segs := strings.Split(ccTestClients, ",")
	for _, val := range segs { // e.g val == 'rj'
		if len(val) != 2 {
			panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX bad value: " + val)
		}
		testCase := [2]api.ClientType{}
		for i, ch := range val {
			switch ch {
			case 'r':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeRust,
					HS:   "hs1",
				}
			case 'j':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeJS,
					HS:   "hs1",
				}
			case 'J':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeJS,
					HS:   "hs2",
				}
			// TODO: case 'R': requires 2x sliding syncs / postgres
			default:
				panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX bad value: " + val)
			}
		}
		testClientMatrix = append(testClientMatrix, testCase)
	}
	ssMutex = &sync.Mutex{}
	api.SetupJSLogs("js_sdk.log")                        // rust sdk logs on its own
	complement.TestMainWithCleanup(m, "crypto", func() { // always teardown even if panicking
		ssMutex.Lock()
		if ssDeployment != nil {
			ssDeployment.Teardown(os.Getenv("COMPLEMENT_CRYPTO_WRITE_CONTAINER_LOGS") == "1")
		}
		ssMutex.Unlock()
		api.WriteJSLogs()
	})
}

func Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	ssMutex.Lock()
	defer ssMutex.Unlock()
	if ssDeployment != nil {
		return ssDeployment
	}
	ssDeployment = deploy.RunNewDeployment(t, os.Getenv("COMPLEMENT_CRYPTO_TCPDUMP") == "1")
	return ssDeployment
}

func ClientTypeMatrix(t *testing.T, subTest func(tt *testing.T, a, b api.ClientType)) {
	for _, tc := range testClientMatrix {
		tc := tc
		t.Run(fmt.Sprintf("%s|%s", tc[0], tc[1]), func(t *testing.T) {
			subTest(t, tc[0], tc[1])
		})
	}
}

func MustLoginClient(t *testing.T, clientType api.ClientType, opts api.ClientCreationOpts, ssURL string) api.Client {
	switch clientType.Lang {
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
