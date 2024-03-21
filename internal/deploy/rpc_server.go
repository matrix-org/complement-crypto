package deploy

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/langs"
)

// RPCServer exposes the api.Client interface over the wire, consumed via net/rpc.
// Args and return params must be encodable with encoding/gob.
// All functions on this struct must meet the form:
//
//	func (t *T) MethodName(argType T1, replyType *T2) error
type RPCServer struct {
	activeClient api.Client
	stopSyncing  func()
	waiters      map[int]api.Waiter
	nextWaiterID int
	waitersMu    *sync.Mutex
}

func NewRPCServer() *RPCServer {
	return &RPCServer{
		waiters:   make(map[int]api.Waiter),
		waitersMu: &sync.Mutex{},
	}
}

type RPCClientCreationOpts struct {
	api.ClientCreationOpts
	Lang api.ClientTypeLang // need to know the type for pulling out the corret bindings
}

func (s *RPCServer) PreTestRun(clientLang api.ClientTypeLang, void *int) error {
	bindings := langs.GetLanguageBindings(clientLang)
	if bindings == nil {
		return fmt.Errorf("RPC: PreTestRun: unknown language bindings %s : did you build the rpc server with the correct -tags?", clientLang)
	}
	bindings.PreTestRun()
	return nil
}

func (s *RPCServer) PostTestRun(clientLang api.ClientTypeLang, void2 *int) error {
	bindings := langs.GetLanguageBindings(clientLang)
	if bindings == nil {
		return fmt.Errorf("RPC: PostTestRun: unknown language bindings %s : did you build the rpc server with the correct -tags?", clientLang)
	}
	bindings.PostTestRun()
	return nil
}

// MustCreateClient creates a given client and returns it to the caller, else returns an error.
func (s *RPCServer) MustCreateClient(opts RPCClientCreationOpts, void *int) error {
	log.Printf("MustCreateClient: %+v\n", opts)
	if s.activeClient != nil {
		return fmt.Errorf("RPC: MustCreateClient: already have an activeClient")
	}
	bindings := langs.GetLanguageBindings(opts.Lang)
	if bindings == nil {
		return fmt.Errorf("RPC: MustCreateClient: unknown language bindings %s : did you build the rpc server with the correct -tags?", opts.Lang)
	}
	s.activeClient = bindings.MustCreateClient(&api.MockT{}, opts.ClientCreationOpts)
	return nil
}

func (s *RPCServer) Close(testName string, void *int) error {
	s.activeClient.Close(&api.MockT{TestName: testName})
	// TODO: shutdown the RPC server?
	return nil
}

func (s *RPCServer) DeletePersistentStorage(testName string, void *int) error {
	s.activeClient.DeletePersistentStorage(&api.MockT{TestName: testName})
	return nil
}

func (s *RPCServer) Login(opts api.ClientCreationOpts, void *int) error {
	return s.activeClient.Login(&api.MockT{}, opts)
}

func (s *RPCServer) MustStartSyncing(testName string, void *int) error {
	s.stopSyncing = s.activeClient.MustStartSyncing(&api.MockT{TestName: testName})
	return nil
}

func (s *RPCServer) StartSyncing(testName string, void *int) error {
	stopSyncing, err := s.activeClient.StartSyncing(&api.MockT{TestName: testName})
	if err != nil {
		return fmt.Errorf("%s RPCServer.StartSyncing: %v", testName, err)
	}
	s.stopSyncing = stopSyncing
	return nil
}

func (s *RPCServer) StopSyncing(testName string, void *int) error {
	if s.stopSyncing == nil {
		return fmt.Errorf("%s RPCServer.StopSyncing: cannot stop syncing as StartSyncing wasn't called", testName)
	}
	s.stopSyncing()
	s.stopSyncing = nil
	return nil
}

func (s *RPCServer) IsRoomEncrypted(roomID string, isEncrypted *bool) error {
	isEnc, err := s.activeClient.IsRoomEncrypted(&api.MockT{}, roomID)
	isEncrypted = &isEnc
	return err
}

type RPCSendMessage struct {
	TestName string
	RoomID   string
	Text     string
}

func (s *RPCServer) SendMessage(msg RPCSendMessage, eventID *string) error {
	id := s.activeClient.SendMessage(&api.MockT{TestName: msg.TestName}, msg.RoomID, msg.Text)
	eventID = &id
	return nil
}

func (s *RPCServer) TrySendMessage(msg RPCSendMessage, eventID *string) error {
	id, err := s.activeClient.TrySendMessage(&api.MockT{TestName: msg.TestName}, msg.RoomID, msg.Text)
	if err != nil {
		return err
	}
	eventID = &id
	return nil
}

type RPCWaitUntilEvent struct {
	TestName           string
	RoomID             string
	WantEvent          api.Event
	FailedToDecryptSet bool
}

func (s *RPCServer) WaitUntilEventInRoom(input RPCWaitUntilEvent, waiterID *int) error {
	fieldMatches := func(got, want any, isSet bool) bool {
		if !isSet {
			return true
		}
		return got == want
	}

	waiter := s.activeClient.WaitUntilEventInRoom(&api.MockT{TestName: input.TestName}, input.RoomID, func(e api.Event) bool {
		// all fields must match (which may mean they are unset, which counts as a match)
		if fieldMatches(e.ID, input.WantEvent.ID, input.WantEvent.ID != "") &&
			fieldMatches(e.Membership, input.WantEvent.Membership, input.WantEvent.Membership != "") &&
			fieldMatches(e.Sender, input.WantEvent.Sender, input.WantEvent.Sender != "") &&
			fieldMatches(e.Target, input.WantEvent.Target, input.WantEvent.Target != "") &&
			fieldMatches(e.Text, input.WantEvent.Text, input.WantEvent.Text != "") &&
			fieldMatches(e.FailedToDecrypt, input.WantEvent.FailedToDecrypt, input.FailedToDecryptSet) {
			return true
		}
		return false
	})
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()
	nextID := s.nextWaiterID + 1
	s.nextWaiterID = nextID
	s.waiters[s.nextWaiterID] = waiter
	waiterID = &nextID
	return nil
}

type RPCWait struct {
	TestName string
	WaiterID int
	Timeout  time.Duration
}

func (s *RPCServer) Wait(input RPCWait, void *int) error {
	s.waitersMu.Lock()
	w := s.waiters[input.WaiterID]
	s.waitersMu.Unlock()
	if w == nil {
		return fmt.Errorf("RPC: Wait: no waiter found with id %d", input.WaiterID)
	}
	w.Wait(&api.MockT{TestName: input.TestName}, input.Timeout)
	return nil
}

// Backpaginate in this room by `count` events.
type RPCBackpaginate struct {
	TestName string
	RoomID   string
	Count    int
}

func (s *RPCServer) MustBackpaginate(input RPCBackpaginate, void *int) error {
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
	ev := s.activeClient.MustGetEvent(&api.MockT{TestName: input.TestName}, input.RoomID, input.EventID)
	output = &ev
	return nil
}

// MustBackupKeys will backup E2EE keys, else fail the test.
func (s *RPCServer) MustBackupKeys(testName string, recoveryKey *string) error {
	key := s.activeClient.MustBackupKeys(&api.MockT{TestName: testName})
	recoveryKey = &key
	return nil
}

// MustLoadBackup will recover E2EE keys from the latest backup, else fail the test.
func (s *RPCServer) MustLoadBackup(recoveryKey string, void *int) error {
	s.activeClient.MustLoadBackup(&api.MockT{}, recoveryKey)
	return nil
}

func (s *RPCServer) LoadBackup(recoveryKey string, void *int) error {
	return s.activeClient.LoadBackup(&api.MockT{}, recoveryKey)
}

func (s *RPCServer) Logf(input string, void *int) error {
	log.Println(input)
	s.activeClient.Logf(&api.MockT{}, input)
	return nil
}

func (s *RPCServer) UserID(void int, userID *string) error {
	u := s.activeClient.UserID()
	userID = &u
	return nil
}
func (s *RPCServer) Type(void int, clientType *api.ClientTypeLang) error {
	t := s.activeClient.Type()
	clientType = &t
	return nil
}
func (s *RPCServer) Opts(void int, opts *api.ClientCreationOpts) error {
	o := s.activeClient.Opts()
	opts = &o
	return nil
}
