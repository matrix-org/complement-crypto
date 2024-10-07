package rpc

import (
	"bufio"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/ct"
)

// LanguageBindings implements api.LanguageBindings and instead issues RPC calls to a remote server.
type LanguageBindings struct {
	binaryPath    string
	clientType    api.ClientTypeLang
	contextPrefix string
}

func NewLanguageBindings(rpcBinaryPath string, clientType api.ClientTypeLang, contextPrefix string) (*LanguageBindings, error) {
	return &LanguageBindings{
		binaryPath:    rpcBinaryPath,
		clientType:    clientType,
		contextPrefix: contextPrefix,
	}, nil
}

func (r *LanguageBindings) PreTestRun(contextID string) {
	// do nothing, as PreTestRun for all tests is meaningless for RPC clients.
	// If we were to call the underlying bindings, we would delete logs prematurely.
	// Instead, we do this call when RPC clients are made.
}
func (r *LanguageBindings) PostTestRun(contextID string) {
	// do nothing, as PostTestRun for all tests is meaningless for RPC clients.
	// If we were to call the underlying bindings, we would delete logs prematurely.
	// Instead, we do this call when RPC clients are closed.
}

// MustCreateClient starts the RPC server and configures it to use the
// correct language. Returns an error if:
//   - the binary cannot be found or run
//   - the server cannot be started
//   - IPC via stdout fails (used to extract the random high numbered port)
//   - the client cannot talk to the rpc server
func (r *LanguageBindings) MustCreateClient(t ct.TestLike, cfg api.ClientCreationOpts) api.Client {
	contextID := fmt.Sprintf("%s%s_%s", r.contextPrefix, strings.Replace(cfg.UserID[1:], ":", "_", -1), cfg.DeviceID)
	// security: check it is a file not a random bash script...
	if _, err := os.Stat(r.binaryPath); err != nil {
		ct.Fatalf(t, "%s: RPC binary at %s does not exist or cannot be executed/read: %s", contextID, r.binaryPath, err)
	}
	rpcCmd := exec.Command(r.binaryPath)
	stdout, err := rpcCmd.StdoutPipe()
	if err != nil {
		ct.Fatalf(t, "%s: cannot pipe stdout of rpc binary: %s", contextID, err)
	}
	rpcCmd.Stderr = rpcCmd.Stdout
	if err := rpcCmd.Start(); err != nil { // this calls NewRPCServer() effectively
		ct.Fatalf(t, "%s: cannot start RPC binary %s: %s", contextID, r.binaryPath, err)
	}
	// wait until we get a high-numbered port
	portCh := make(chan struct {
		port int
		err  error
	})
	go func() {
		rd := bufio.NewReader(stdout)
		defer close(portCh)
		defer func() {
			// log stdout from the RPC server
			go func() {
				for {
					str, err := rd.ReadString('\n')
					if err != nil {
						log.Print("RPC ERROR: " + err.Error())
						break
					}
					log.Printf("  RPC (%s): %s", contextID, str)
				}
			}()
			// we need to .Wait to ensure we clean up resources when the RPC server dies.
			rpcCmd.Wait()
		}()

		var port int
		for {
			str, err := rd.ReadString('\n')
			if port == 0 { // we need a port
				if err != nil {
					portCh <- struct {
						port int
						err  error
					}{port: 0, err: fmt.Errorf("failed to read stdout line: %s", err)}
					return
				}
				port, err = strconv.Atoi(strings.TrimSpace(str))
				if err != nil {
					log.Printf("  RPC (%s) stdout line isn't a port: %s", contextID, str)
					continue
				}
				portCh <- struct {
					port int
					err  error
				}{
					port: port,
					err:  nil,
				}
				break
			}
		}
	}()
	select {
	case p := <-portCh:
		rpcAddr := fmt.Sprintf("127.0.0.1:%d", p.port)
		var void int
		client, err := rpc.DialHTTP("tcp", rpcAddr)
		if err != nil {
			t.Fatalf("RPC MustCreateClient DialHTTP: %s", err)
		}

		err = client.Call("Server.MustCreateClient", ClientCreationOpts{
			ClientCreationOpts: cfg,
			ContextID:          contextID,
			Lang:               r.clientType,
		}, &void)
		if err != nil {
			ct.Fatalf(t, "%s: failed to create RPC client: %s", contextID, err)
		}
		return &RPCClient{
			client: client,
			lang:   r.clientType,
			rpcCmd: rpcCmd,
		}
	case <-time.After(time.Second):
		ct.Fatalf(t, "%s: timed out waiting for port number to be echoed to stdout. Did the RPC binary run, and is it actually the RPC binary? Path: %s", contextID, r.binaryPath)
	}
	panic("unreachable")
}

// RPCClient implements api.Client by making RPC calls to an RPC server, which actually has a concrete api.Client
type RPCClient struct {
	client *rpc.Client
	lang   api.ClientTypeLang
	rpcCmd *exec.Cmd
}

func (c *RPCClient) ForceClose(t ct.TestLike) {
	t.Helper()
	err := c.rpcCmd.Process.Kill()
	if err != nil {
		t.Fatalf("failed to kill process: %s", err)
	}
}

// Close is called to clean up resources.
// Specifically, we need to shut off existing browsers and any FFI bindings.
// If we get callbacks/events after this point, tests may panic if the callbacks
// log messages.
func (c *RPCClient) Close(t ct.TestLike) {
	t.Helper()
	var void int
	fmt.Println("RPCClient.Close")
	err := c.client.Call("Server.Close", t.Name(), &void)
	if err != nil {
		t.Fatalf("RPCClient.Close: %s", err)
	}
	c.client.Close()
}

func (c *RPCClient) GetNotification(t ct.TestLike, roomID, eventID string) (*api.Notification, error) {
	var notification api.Notification
	input := RPCGetNotification{
		RoomID:  roomID,
		EventID: eventID,
	}
	err := c.client.Call("Server.GetNotification", input, &notification)
	return &notification, err
}

func (c *RPCClient) CurrentAccessToken(t ct.TestLike) string {
	var token string
	err := c.client.Call("Server.CurrentAccessToken", t.Name(), &token)
	if err != nil {
		ct.Fatalf(t, "RPCServer.CurrentAccessToken: %s", err)
	}
	return token
}

func (c *RPCClient) RequestOwnUserVerification(t ct.TestLike) chan api.VerificationStage {
	panic("unimplemented")
}

func (c *RPCClient) ListenForVerificationRequests(t ct.TestLike) chan api.VerificationStage {
	panic("unimplemented")
}

func (c *RPCClient) InviteUser(t ct.TestLike, roomID, userID string) error {
	panic("unimplemented")
}

// Remove any persistent storage, if it was enabled.
func (c *RPCClient) DeletePersistentStorage(t ct.TestLike) {
	var void int
	err := c.client.Call("Server.DeletePersistentStorage", t.Name(), &void)
	if err != nil {
		t.Fatalf("RPCClient.DeletePersistentStorage: %s", err)
	}
}
func (c *RPCClient) Login(t ct.TestLike, opts api.ClientCreationOpts) error {
	var void int
	fmt.Printf("RPCClient Calling login with %+v\n", opts)
	err := c.client.Call("Server.Login", opts, &void)
	fmt.Println("RPCClient login returned => ", err)
	return err
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
// MUST BLOCK until the initial sync is complete.
// Returns an error if there was a problem syncing.
func (c *RPCClient) StartSyncing(t ct.TestLike) (stopSyncing func(), err error) {
	var void int
	err = c.client.Call("Server.StartSyncing", t.Name(), &void)
	if err != nil {
		return
	}
	return func() {
		err := c.client.Call("Server.StopSyncing", t.Name(), &void)
		if err != nil {
			t.Logf("RPCClient.StopSyncing: %s", err)
		}
	}, nil
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *RPCClient) IsRoomEncrypted(t ct.TestLike, roomID string) (bool, error) {
	var isEncrypted bool
	err := c.client.Call("Server.IsRoomEncrypted", roomID, &isEncrypted)
	return isEncrypted, err
}

// SendMessage tries to send the message, but can fail.
func (c *RPCClient) SendMessage(t ct.TestLike, roomID, text string) (eventID string, err error) {
	err = c.client.Call("Server.SendMessage", RPCSendMessage{
		TestName: t.Name(),
		RoomID:   roomID,
		Text:     text,
	}, &eventID)
	return
}

// Wait until an event is seen in the given room. The checker functions can be custom or you can use
// a pre-defined one like api.CheckEventHasMembership, api.CheckEventHasBody, or api.CheckEventHasEventID.
func (c *RPCClient) WaitUntilEventInRoom(t ct.TestLike, roomID string, checker func(e api.Event) bool) api.Waiter {
	var waiterID int
	err := c.client.Call("Server.WaitUntilEventInRoom", RPCWaitUntilEvent{
		TestName: t.Name(),
		RoomID:   roomID,
	}, &waiterID)
	if err != nil {
		t.Fatalf("RPCClient.WaitUntilEventInRoom: %s", err)
	}
	return &RPCWaiter{
		client:   c.client,
		waiterID: waiterID,
		checker:  checker,
	}
}

// Backpaginate in this room by `count` events.
func (c *RPCClient) Backpaginate(t ct.TestLike, roomID string, count int) error {
	var void int
	err := c.client.Call("Server.Backpaginate", RPCBackpaginate{
		TestName: t.Name(),
		RoomID:   roomID,
		Count:    count,
	}, &void)
	return err
}

// GetEvent will return the client's view of this event, or return an error if the event cannot be found.
func (c *RPCClient) GetEvent(t ct.TestLike, roomID, eventID string) (*api.Event, error) {
	var ev api.Event
	err := c.client.Call("Server.GetEvent", RPCGetEvent{
		TestName: t.Name(),
		RoomID:   roomID,
		EventID:  eventID,
	}, &ev)
	return &ev, err
}

// BackupKeys will backup E2EE keys, else return an error.
func (c *RPCClient) BackupKeys(t ct.TestLike) (recoveryKey string, err error) {
	err = c.client.Call("Server.BackupKeys", 0, &recoveryKey)
	return
}

// LoadBackup will recover E2EE keys from the latest backup, else return an error.
func (c *RPCClient) LoadBackup(t ct.TestLike, recoveryKey string) error {
	var void int
	return c.client.Call("Server.LoadBackup", recoveryKey, &void)
}

// Log something to stdout and the underlying client log file
func (c *RPCClient) Logf(t ct.TestLike, format string, args ...interface{}) {
	str := fmt.Sprintf(format, args...)
	str = t.Name() + ": " + str
	var void int
	err := c.client.Call("Server.Logf", str, &void)
	if err != nil {
		t.Fatalf("RPCClient.Logf: %s", err)
	}
}

func (c *RPCClient) UserID() string {
	var userID string
	c.client.Call("Server.UserID", 0, &userID)
	return userID
}
func (c *RPCClient) Type() api.ClientTypeLang {
	var lang api.ClientTypeLang
	c.client.Call("Server.Type", 0, &lang)
	return lang
}
func (c *RPCClient) Opts() api.ClientCreationOpts {
	var opts api.ClientCreationOpts
	c.client.Call("Server.Opts", 0, &opts)
	return opts
}

type RPCWaiter struct {
	waiterID int
	client   *rpc.Client
	checker  func(e api.Event) bool
}

func (w *RPCWaiter) Waitf(t ct.TestLike, s time.Duration, format string, args ...any) {
	t.Helper()
	err := w.TryWaitf(t, s, format, args...)
	if err != nil {
		ct.Fatalf(t, "RPCWaiter.Wait: %v", err)
	}
}

func (w *RPCWaiter) TryWaitf(t ct.TestLike, s time.Duration, format string, args ...any) error {
	t.Helper()
	var void int
	msg := fmt.Sprintf(format, args...)
	t.Logf("RPCWaiter.TryWaitf: calling RPCServer.WaiterStart")
	err := w.client.Call("Server.WaiterStart", RPCWait{
		TestName: t.Name(),
		WaiterID: w.waiterID,
		Msg:      msg,
		Timeout:  s,
	}, &void)
	if err != nil {
		return fmt.Errorf("WaiterStart: %s", err)
	}
	t.Logf("RPCWaiter.TryWaitf: calling RPCServer.WaiterStart OK")
	// now we need to poll for events from the remote waiter
	for {
		var eventsToCheck []api.Event
		t.Logf("RPCWaiter.TryWaitf: calling RPCServer.WaiterPoll")
		err := w.client.Call("Server.WaiterPoll", w.waiterID, &eventsToCheck)
		if err != nil {
			return fmt.Errorf("%s: %s", err, msg)
		}
		t.Logf("RPCWaiter.TryWaitf: calling RPCServer.WaiterPoll OK with %d events", len(eventsToCheck))
		// for each event, check with the checker function if it passes
		for _, ev := range eventsToCheck {
			if w.checker(ev) {
				// if it passes, we waited successfully!
				t.Logf("RPC: checker function passes for event %+v", ev)
				return nil
			}
		}
		// otherwise, keep trying. The RPC server is tracking timeouts for us.
		time.Sleep(100 * time.Millisecond)
	}
}
