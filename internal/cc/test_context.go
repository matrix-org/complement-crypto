package cc

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/langs"
	"github.com/matrix-org/complement-crypto/internal/deploy"
	"github.com/matrix-org/complement-crypto/internal/deploy/rpc"
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

// User represents a single matrix user ID e.g @alice:example.com, along with
// the complement device for this user.
type User struct {
	*client.CSAPI
	// remember the client types that were supplied so we can seamlessly create the right
	// test client when WithAliceSyncing/etc are called.
	ClientType api.ClientType
}

// TestClientCreationRequest is a request to create a new api.Client.
//
// A request is always based on an existing user e.g Alice, Bob, in which case the user ID / password / HS URL
// will be used. Any options specified in this request are then applied on top e.g PersistentStorage.
type ClientCreationRequest struct {
	User *User
	Opts api.ClientCreationOpts
	// If true, spawn this client in another process
	Multiprocess bool
}

// TestContext provides a consistent set of variables which most tests will need access to.
// The variables are suitable for a single test.
type TestContext struct {
	Deployment    *deploy.ComplementCryptoDeployment
	RPCBinaryPath string
	RPCInstance   atomic.Int32

	// Alice is defined if at least 1 clientType is provided to CreateTestContext.
	Alice *User
	// Bob is defined if at least 2 clientTypes are provided to CreateTestContext.
	Bob *User
	// Charlie is defined if at least 3 clientTypes are provided to CreateTestContext.
	Charlie *User
}

// RegisterNewUser registers a new user on the homeserver. The user ID will include the localpartSuffix.
//
// Returns a User with a single device which represents the Complement client for this registration.
// This User can then be passed to other functions to login on new test devices.
func (c *TestContext) RegisterNewUser(t *testing.T, clientType api.ClientType, localpartSuffix string) *User {
	return &User{
		CSAPI: c.Deployment.Register(t, string(clientType.HS), helpers.RegistrationOpts{
			LocalpartSuffix: localpartSuffix,
			Password:        "complement-crypto-password",
		}),
		ClientType: clientType,
	}
}

// WithClientSyncing is a helper function which creates a test client and automatically logs in the user and starts
// a sync loop for them. Additional options can be specified via ClientCreationRequest, including setting the client
// up as a multiprocess client, with persistent storage, etc.
//
// The callback function is invoked after this, and cleanup functions are called on your behalf when the
// callback function ends.
func (c *TestContext) WithClientSyncing(t *testing.T, req *ClientCreationRequest, callback func(cli api.TestClient)) {
	t.Helper()
	c.WithClientsSyncing(t, []*ClientCreationRequest{req}, func(clients []api.TestClient) {
		callback(clients[0])
	})
}

// WithClientsSyncing is a helper function which creates multiple test clients and automatically logs in all of them
// and starts a sync loop for all of them. Additional options can be specified via ClientCreationRequest, including
// setting clients up as a multiprocess client, with persistent storage, etc.
//
// All clients are logged in FIRST before syncing any one of them. As Login() is supposed to block until all keys
// are uploaded, this guarantees that device keys / OTKs / etc exist prior to syncing. This means it is not
// necessary to synchronise device list changes between these clients.
//
// The callback function is invoked after this, and cleanup functions are called on your behalf when the
// callback function ends.
func (c *TestContext) WithClientsSyncing(t *testing.T, reqs []*ClientCreationRequest, callback func(clients []api.TestClient)) {
	t.Helper()
	cryptoClients := make([]api.TestClient, len(reqs))
	// Login all clients BEFORE starting any of their sync loops.
	// We do this because Login will send device list updates and cause clients to upload OTKs/device keys.
	// We want to make sure ALL these keys are on the server before any test client syncs otherwise it
	// can create flakey tests because a client may see a user joined without device keys, meaning any
	// messages that client sends will not encrypt for that device.
	for i, req := range reqs {
		cryptoClients[i] = c.MustLoginClient(t, req)
		defer cryptoClients[i].Close(t)
	}
	for _, cli := range cryptoClients {
		stopSyncing := cli.MustStartSyncing(t)
		defer stopSyncing()
	}
	callback(cryptoClients)
}

// mustCreateMultiprocessClient creates a new RPC process and instructs it to create a client given by the client creation options.
func (c *TestContext) mustCreateMultiprocessClient(t *testing.T, req *ClientCreationRequest) api.TestClient {
	t.Helper()
	if c.RPCBinaryPath == "" {
		t.Skipf("RPC binary path not provided, skipping multiprocess test. To run this test, set COMPLEMENT_CRYPTO_RPC_BINARY")
		return api.NewTestClient(nil)
	}
	ctxPrefix := fmt.Sprintf("%d", c.RPCInstance.Add(1))
	remoteBindings, err := rpc.NewLanguageBindings(c.RPCBinaryPath, req.User.ClientType.Lang, ctxPrefix)
	if err != nil {
		t.Fatalf("Failed to create new RPC language bindings: %s", err)
	}
	return api.NewTestClient(remoteBindings.MustCreateClient(t, req.Opts))
}

// WithAliceSyncing is a helper function which creates a rust/js client and automatically logs in Alice and starts
// a sync loop for her. For more customisation, see WithClientSyncing.
//
// The callback function is invoked after this, and cleanup functions are called on your behalf when the
// callback function ends.
func (c *TestContext) WithAliceSyncing(t *testing.T, callback func(alice api.TestClient)) {
	t.Helper()
	must.NotEqual(t, c.Alice, nil, "No Alice defined. Call CreateTestContext() with at least 1 api.ClientType.")
	c.WithClientSyncing(t, &ClientCreationRequest{
		User: c.Alice,
	}, callback)
}

// WithAliceAndBobSyncing is a helper function which creates rust/js clients and automatically logs in Alice & Bob
// and starts a sync loop for both. For more customisation, see WithClientsSyncing.
//
// The callback function is invoked after this, and cleanup functions are called on your behalf when the
// callback function ends.
func (c *TestContext) WithAliceAndBobSyncing(t *testing.T, callback func(alice, bob api.TestClient)) {
	t.Helper()
	must.NotEqual(t, c.Bob, nil, "No Bob defined. Call CreateTestContext() with at least 2 api.ClientTypes.")
	c.WithClientsSyncing(t, []*ClientCreationRequest{
		{
			User: c.Alice,
		},
		{
			User: c.Bob,
		},
	}, func(clients []api.TestClient) {
		callback(clients[0], clients[1])
	})
}

// WithAliceBobAndCharlieSyncing is a helper function which creates rust/js clients and automatically logs in Alice, Bob
// and Charlie and starts a sync loop for all. For more customisation, see WithClientsSyncing.
//
// The callback function is invoked after this, and cleanup functions are called on your behalf when the
// callback function ends.
func (c *TestContext) WithAliceBobAndCharlieSyncing(t *testing.T, callback func(alice, bob, charlie api.TestClient)) {
	t.Helper()
	must.NotEqual(t, c.Charlie, nil, "No Charlie defined. Call CreateTestContext() with at least 3 api.ClientTypes.")
	c.WithClientsSyncing(t, []*ClientCreationRequest{
		{
			User: c.Alice,
		},
		{
			User: c.Bob,
		},
		{
			User: c.Charlie,
		},
	}, func(clients []api.TestClient) {
		callback(clients[0], clients[1], clients[2])
	})
}

// An option to customise the behaviour of CreateNewEncryptedRoom
type EncRoomOption = func(reqBody map[string]interface{})

// CreateNewEncryptedRoom calls creator.MustCreateRoom with the correct m.room.encryption state event.
//
// options is a set of EncRoomOption that may be provided using methods on
// EncRoomOptions:
// - Preset*: the preset argument passed to createRoom (default: "private_chat")
// - Invite: a list of usernames to invite to the room (default: empty list)
// - RotationPeriodMsgs: value of the rotation_period_msgs param (default: omitted)
func (c *TestContext) CreateNewEncryptedRoom(
	t *testing.T,
	user *User,
	options ...EncRoomOption,
) (roomID string) {
	t.Helper()

	reqBody := map[string]interface{}{
		"name":   t.Name(),
		"preset": "private_chat",
		"invite": []string{},
		"initial_state": []map[string]interface{}{
			{
				"type":      "m.room.encryption",
				"state_key": "",
				"content": map[string]interface{}{
					"algorithm": "m.megolm.v1.aes-sha2",
				},
			},
		},
	}

	for _, option := range options {
		option(reqBody)
	}

	return user.MustCreateRoom(t, reqBody)
}

type encRoomOptions int

// A namespace for the various options that may be passed in to CreateNewEncryptedRoom
const EncRoomOptions encRoomOptions = 0

// An option for CreateNewEncryptedRoom that requests the `preset` field to be
// set to `private_chat`.
func (encRoomOptions) PresetPrivateChat() EncRoomOption {
	return setPreset("private_chat")
}

// An option for CreateNewEncryptedRoom that requests the `preset` field to be
// set to `trusted_private_chat`.
func (encRoomOptions) PresetTrustedPrivateChat() EncRoomOption {
	return setPreset("trusted_private_chat")
}

// An option for CreateNewEncryptedRoom that requests the `preset` field to be
// set to `public_chat`.
func (encRoomOptions) PresetPublicChat() EncRoomOption {
	return setPreset("public_chat")
}

func setPreset(preset string) EncRoomOption {
	return func(reqBody map[string]interface{}) {
		reqBody["preset"] = preset
	}
}

// An option for CreateNewEncryptedRoom that provides a list of Matrix usernames
// to be supplied in the `invite` field.
func (encRoomOptions) Invite(invite []string) EncRoomOption {
	return func(reqBody map[string]interface{}) {
		reqBody["invite"] = invite
	}
}

// An option for CreateNewEncryptedRoom that adds a `rotation_period_msgs` field
// to the `m.room.encryption` event supplied when the room is created.
func (encRoomOptions) RotationPeriodMsgs(numMsgs int) EncRoomOption {
	return func(reqBody map[string]interface{}) {
		var initial_state = reqBody["initial_state"].([]map[string]interface{})
		var event = initial_state[0]
		var content = event["content"].(map[string]interface{})
		content["rotation_period_msgs"] = numMsgs
	}
}

// An option for CreateNewEncryptedRoom that adds a `rotation_period_ms` field
// to the `m.room.encryption` event supplied when the room is created.
func (encRoomOptions) RotationPeriodMs(milliseconds int) EncRoomOption {
	return func(reqBody map[string]interface{}) {
		var initial_state = reqBody["initial_state"].([]map[string]interface{})
		var event = initial_state[0]
		var content = event["content"].(map[string]interface{})
		content["rotation_period_ms"] = milliseconds
	}
}

// MustRegisterNewDevice logs in a new device for this client, else fails the test.
func (c *TestContext) MustRegisterNewDevice(t *testing.T, user *User, newDeviceID string) *User {
	newDevice := c.Deployment.Login(t, string(user.ClientType.HS), user.CSAPI, helpers.LoginOpts{
		DeviceID: newDeviceID,
		Password: user.Password, // TODO: remove? not needed as inherited from client?
	})
	return &User{
		CSAPI:      newDevice,
		ClientType: user.ClientType,
	}
}

// MustLoginClient is the same as MustCreateClient but also logs in the client.
func (c *TestContext) MustLoginClient(t *testing.T, req *ClientCreationRequest) api.TestClient {
	t.Helper()
	client := c.MustCreateClient(t, req)
	must.NotError(t, "failed to login client", client.Login(t, client.Opts()))
	return client
}

// MustCreateClient creates an api.Client from an existing Complement client and the specified client type. Additional options
// can be set to configure the client beyond that of the Complement client e.g to add persistent storage.
func (c *TestContext) MustCreateClient(t *testing.T, req *ClientCreationRequest) api.TestClient {
	t.Helper()
	if req.User == nil {
		ct.Fatalf(t, "MustCreateClient: ClientCreationRequest missing 'user', register one with RegisterNewUser or use an existing one.")
	}
	opts := api.NewClientCreationOpts(req.User.CSAPI)
	// now apply the supplied opts on top
	opts.Combine(&req.Opts)
	if req.Multiprocess {
		req.Opts = opts
		return c.mustCreateMultiprocessClient(t, req)
	}
	client := mustCreateClient(t, req.User.ClientType, opts)
	return client
}

// mustCreateClient creates an api.Client with the specified language/server, else fails the test.
//
// Options can be provided to configure clients, such as enabling persistent storage.
func mustCreateClient(t *testing.T, clientType api.ClientType, cfg api.ClientCreationOpts) api.TestClient {
	bindings := langs.GetLanguageBindings(clientType.Lang)
	if bindings == nil {
		t.Fatalf("unknown language: %s", clientType.Lang)
	}
	c := bindings.MustCreateClient(t, cfg)
	return api.NewTestClient(c)
}
