package deploy

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/langs"
)

const InactivityThreshold = 30 * time.Second

// RPCServer exposes the api.Client interface over the wire, consumed via net/rpc.
// Args and return params must be encodable with encoding/gob.
// All functions on this struct must meet the form:
//
//	func (t *T) MethodName(argType T1, replyType *T2) error
type RPCServer struct {
	contextID     string // test|user|device
	bindings      api.LanguageBindings
	activeClient  api.Client
	stopSyncing   func()
	waiters       map[int]*RPCServerWaiter
	nextWaiterID  int
	waitersMu     *sync.Mutex
	lastCmdRecv   time.Time
	lastCmdRecvMu *sync.Mutex
}

func NewRPCServer() *RPCServer {
	srv := &RPCServer{
		waiters:       make(map[int]*RPCServerWaiter),
		waitersMu:     &sync.Mutex{},
		lastCmdRecv:   time.Now(),
		lastCmdRecvMu: &sync.Mutex{},
	}
	go srv.checkKeepAlive()
	return srv
}

type RPCClientCreationOpts struct {
	api.ClientCreationOpts
	Lang      api.ClientTypeLang // need to know the type for pulling out the corret bindings
	ContextID string
}

// When the RPC server is run locally, we want to make sure we don't persist as an orphan process
// if the test suite crashes. We do this by checking that we have seen an RPC command within
// InactivityThreshold duration.
func (s *RPCServer) checkKeepAlive() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		s.lastCmdRecvMu.Lock()
		if time.Since(s.lastCmdRecv) > InactivityThreshold {
			fmt.Printf("terminating RPC server due to inactivity (%v)\n", InactivityThreshold)
			os.Exit(0)
		}
		s.lastCmdRecvMu.Unlock()
	}
}

func (s *RPCServer) keepAlive() {
	s.lastCmdRecvMu.Lock()
	defer s.lastCmdRecvMu.Unlock()
	s.lastCmdRecv = time.Now()
}

// MustCreateClient creates a given client and returns it to the caller, else returns an error.
func (s *RPCServer) MustCreateClient(opts RPCClientCreationOpts, void *int) error {
	defer s.keepAlive()
	fmt.Printf("RPCServer: Received MustCreateClient: %+v\n", opts)
	if s.activeClient != nil {
		return fmt.Errorf("RPC: MustCreateClient: already have an activeClient")
	}
	bindings := langs.GetLanguageBindings(opts.Lang)
	if bindings == nil {
		return fmt.Errorf("RPC: MustCreateClient: unknown language bindings %s : did you build the rpc server with the correct -tags?", opts.Lang)
	}
	bindings.PreTestRun(opts.ContextID) // prepare logs
	s.contextID = opts.ContextID
	s.activeClient = bindings.MustCreateClient(&api.MockT{}, opts.ClientCreationOpts)
	s.bindings = bindings
	return nil
}

func (s *RPCServer) Close(testName string, void *int) error {
	defer s.keepAlive()
	s.activeClient.Close(&api.MockT{TestName: testName})
	// write logs
	s.bindings.PostTestRun(s.contextID)
	return nil
}

func (s *RPCServer) DeletePersistentStorage(testName string, void *int) error {
	defer s.keepAlive()
	s.activeClient.DeletePersistentStorage(&api.MockT{TestName: testName})
	return nil
}

func (s *RPCServer) Login(opts api.ClientCreationOpts, void *int) error {
	defer s.keepAlive()
	return s.activeClient.Login(&api.MockT{}, opts)
}

func (s *RPCServer) MustStartSyncing(testName string, void *int) error {
	defer s.keepAlive()
	s.stopSyncing = s.activeClient.MustStartSyncing(&api.MockT{TestName: testName})
	return nil
}

func (s *RPCServer) StartSyncing(testName string, void *int) error {
	defer s.keepAlive()
	stopSyncing, err := s.activeClient.StartSyncing(&api.MockT{TestName: testName})
	if err != nil {
		return fmt.Errorf("%s RPCServer.StartSyncing: %v", testName, err)
	}
	s.stopSyncing = stopSyncing
	return nil
}

func (s *RPCServer) StopSyncing(testName string, void *int) error {
	defer s.keepAlive()
	if s.stopSyncing == nil {
		return fmt.Errorf("%s RPCServer.StopSyncing: cannot stop syncing as StartSyncing wasn't called", testName)
	}
	s.stopSyncing()
	s.stopSyncing = nil
	return nil
}

func (s *RPCServer) IsRoomEncrypted(roomID string, isEncrypted *bool) error {
	defer s.keepAlive()
	var err error
	*isEncrypted, err = s.activeClient.IsRoomEncrypted(&api.MockT{}, roomID)
	return err
}

type RPCSendMessage struct {
	TestName string
	RoomID   string
	Text     string
}

func (s *RPCServer) SendMessage(msg RPCSendMessage, eventID *string) error {
	defer s.keepAlive()
	*eventID = s.activeClient.SendMessage(&api.MockT{TestName: msg.TestName}, msg.RoomID, msg.Text)
	return nil
}

func (s *RPCServer) TrySendMessage(msg RPCSendMessage, eventID *string) error {
	defer s.keepAlive()
	var err error
	*eventID, err = s.activeClient.TrySendMessage(&api.MockT{TestName: msg.TestName}, msg.RoomID, msg.Text)
	if err != nil {
		return err
	}
	return nil
}

type RPCWaitUntilEvent struct {
	TestName string
	RoomID   string
}

func (s *RPCServer) WaitUntilEventInRoom(input RPCWaitUntilEvent, waiterID *int) error {
	defer s.keepAlive()
	waiter := s.activeClient.WaitUntilEventInRoom(&api.MockT{TestName: input.TestName}, input.RoomID, func(e api.Event) bool {
		s.waitersMu.Lock()
		defer s.waitersMu.Unlock()
		rpcWaiter := s.waiters[*waiterID]
		if rpcWaiter == nil {
			panic("waiter did not exist when it should have")
		}
		// remember this event so when the rpc client calls PollWait we can deliver them.
		rpcWaiter.eventsToCheck = append(rpcWaiter.eventsToCheck, e)
		s.waiters[*waiterID] = rpcWaiter
		return false
	})
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()
	nextID := s.nextWaiterID + 1
	s.nextWaiterID = nextID
	s.waiters[s.nextWaiterID] = &RPCServerWaiter{
		Waiter: waiter,
	}
	*waiterID = nextID
	return nil
}

type RPCCheck struct {
}

type RPCWait struct {
	TestName string
	WaiterID int
	Msg      string
	Timeout  time.Duration
}

// WaiterStart is the RPC equivalent to Waiter.Waitf. It begins accumulating events for the RPC client to check.
// Clients need to call WaiterPoll to get these new events.
func (s *RPCServer) WaiterStart(input RPCWait, void *int) error {
	defer s.keepAlive()
	s.waitersMu.Lock()
	w := s.waiters[input.WaiterID]
	if w == nil {
		s.waitersMu.Unlock()
		return fmt.Errorf("RPC: Wait: no waiter found with id %d", input.WaiterID)
	}
	if !w.startedAt.IsZero() {
		s.waitersMu.Unlock()
		return nil // already polling
	}
	w.startedAt = time.Now()
	w.timeout = input.Timeout
	s.waitersMu.Unlock()
	// We do NOT call .Waitf here as timing out will be fatal. Instead, we TryWaitf, and only fail the test
	// _once the client has fetched the events_ because checker functions are arbitrary. Effectively, calling
	// this function just starts populating w.eventsToCheck. An error will ALWAYS be returned here because
	// we ALWAYS return false in the checker function to keep fetching more events, hence consciously drop it.
	// We need to do this in a goroutine so the client can start calling WaiterPoll.
	go w.TryWaitf(&api.MockT{TestName: input.TestName}, input.Timeout, input.Msg)
	return nil
}

func (s *RPCServer) WaiterPoll(waiterID int, eventsToCheck *[]api.Event) error {
	defer s.keepAlive()
	fmt.Println("Acquiring lock")
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()
	fmt.Println("Acquired!")
	w := s.waiters[waiterID]
	if w == nil {
		return fmt.Errorf("unknown waiter id %d", waiterID)
	}
	if time.Since(w.startedAt) > w.timeout {
		return fmt.Errorf("timed out after %v", w.timeout)
	}
	eventsToCheckCopy := make([]api.Event, len(w.eventsToCheck))
	for i := range w.eventsToCheck {
		eventsToCheckCopy[i] = w.eventsToCheck[i]
	}
	*eventsToCheck = eventsToCheckCopy
	w.eventsToCheck = nil // reset the events to check
	return nil
}

// Backpaginate in this room by `count` events.
type RPCBackpaginate struct {
	TestName string
	RoomID   string
	Count    int
}

func (s *RPCServer) MustBackpaginate(input RPCBackpaginate, void *int) error {
	defer s.keepAlive()
	s.activeClient.MustBackpaginate(&api.MockT{TestName: input.TestName}, input.RoomID, input.Count)
	return nil
}

type RPCGetEvent struct {
	TestName string
	RoomID   string
	EventID  string
}

// MustGetEvent will return the client's view of this event, or fail the test if the event cannot be found.
func (s *RPCServer) MustGetEvent(input RPCGetEvent, output *api.Event) error {
	defer s.keepAlive()
	*output = s.activeClient.MustGetEvent(&api.MockT{TestName: input.TestName}, input.RoomID, input.EventID)
	return nil
}

// MustBackupKeys will backup E2EE keys, else fail the test.
func (s *RPCServer) MustBackupKeys(testName string, recoveryKey *string) error {
	defer s.keepAlive()
	*recoveryKey = s.activeClient.MustBackupKeys(&api.MockT{TestName: testName})
	return nil
}

type RPCGetNotification struct {
	RoomID  string
	EventID string
}

func (s *RPCServer) GetNotification(input RPCGetNotification, output *api.Notification) (err error) {
	defer s.keepAlive()
	var n *api.Notification
	n, err = s.activeClient.GetNotification(&api.MockT{}, input.RoomID, input.EventID)
	if err == nil {
		*output = *n
	}
	return err
}

// MustLoadBackup will recover E2EE keys from the latest backup, else fail the test.
func (s *RPCServer) MustLoadBackup(recoveryKey string, void *int) error {
	defer s.keepAlive()
	s.activeClient.MustLoadBackup(&api.MockT{}, recoveryKey)
	return nil
}

func (s *RPCServer) LoadBackup(recoveryKey string, void *int) error {
	defer s.keepAlive()
	return s.activeClient.LoadBackup(&api.MockT{}, recoveryKey)
}

func (s *RPCServer) Logf(input string, void *int) error {
	defer s.keepAlive()
	log.Println(input)
	s.activeClient.Logf(&api.MockT{}, input)
	return nil
}

func (s *RPCServer) UserID(void int, userID *string) error {
	defer s.keepAlive()
	*userID = s.activeClient.UserID()
	return nil
}
func (s *RPCServer) Type(void int, clientType *api.ClientTypeLang) error {
	defer s.keepAlive()
	*clientType = s.activeClient.Type()
	return nil
}
func (s *RPCServer) Opts(void int, opts *api.ClientCreationOpts) error {
	defer s.keepAlive()
	*opts = s.activeClient.Opts()
	return nil
}

type RPCServerWaiter struct {
	api.Waiter
	eventsToCheck []api.Event
	startedAt     time.Time
	timeout       time.Duration
}
