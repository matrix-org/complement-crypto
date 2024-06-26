package api

import (
	"fmt"
	"sync"
)

// Verification Stage Machine (all stages can transition to Cancelled (m.key.verification.cancel))
//        +-----------+
//  ----> | Requested |  <-- m.key.verification.request has been sent/received
//        +-----------+
//             |
//        +----v------+
//        |   Ready   |  <--- m.key.verification.ready has been sent/received
//        +-----------+
//             |
//        +----v------+
//        |   Start   |  <---  m.key.verification.start has been sent/received
//        +-----------+
//             |
//        +----v---------+
//    .---| Transitioned | <-- Verification specific, SAS/QR code logic in this stage. Can fire multiple times.
//     `->+--------------+
//             |
//        +----v----+
//        |   Done  |  <--- m.key.verification.done has been sent/received
//        +---------+
//

// For SAS Verification, clients may not expose all of the following stages.
// The Transitioned stages are outlined in
// https://spec.matrix.org/v1.11/client-server-api/#short-authentication-string-sas-verification :
//        ┌───────┐
//        │Created│
//        └───┬───┘
//            │
//        ┌───⌄───┐
//        │Started│
//        └───┬───┘
//            │
//       ┌────⌄───┐
//       │Accepted│
//       └────┬───┘
//            │
//    ┌───────⌄──────┐
//    │Keys Exchanged│ <-- this stage will always be exposed as the emoji are ready
//    └───────┬──────┘
//            │
//    ________⌄________
//   ╱                 ╲       ┌─────────┐
//  ╱   Does the short  ╲______│Cancelled│
//  ╲ auth string match ╱ no   └─────────┘
//   ╲_________________╱
//            │yes
//            │
//       ┌────⌄────┐
//       │Confirmed│
//       └────┬────┘
//            │
//        ┌───⌄───┐
//        │  Done │
//        └───────┘

type VerificationData struct {
	Emojis   []string
	Decimals []uint16
}

type VerificationRequest struct {
	TxnID            string
	SenderUserID     string
	SenderDeviceID   string
	ReceiverUserID   string
	ReceiverDeviceID string
}

type VerificationStage interface{}
type VerificationStageEnum int

const (
	VerificationStageEnumRequested VerificationStageEnum = iota
	VerificationStageEnumRequestedRequetee
	VerificationStageEnumReady
	VerificationStageEnumStart
	VerificationStageEnumTransitioned
	VerificationStageEnumDone
	VerificationStageEnumCancelled
)

type VerificationStageRequested interface {
	Request() VerificationRequest
	Cancel()
}
type verificationStageRequested struct {
	c *VerificationContainer
}

func (v *verificationStageRequested) Request() VerificationRequest {
	return v.c.VReq
}
func (v *verificationStageRequested) Cancel() {
	v.c.SendCancel()
}
func NewVerificationStageRequested(c *VerificationContainer) VerificationStageRequested {
	return &verificationStageRequested{c}
}

type VerificationStageRequestedRequetee interface {
	Request() VerificationRequest
	Cancel()
	Ready()
}
type verificationStageRequestedRequetee struct {
	c *VerificationContainer
}

func (v *verificationStageRequestedRequetee) Request() VerificationRequest {
	return v.c.VReq
}
func (v *verificationStageRequestedRequetee) Cancel() {
	v.c.SendCancel()
}
func (v *verificationStageRequestedRequetee) Ready() {
	v.c.SendReady()
}
func NewVerificationStageRequestedRequetee(c *VerificationContainer) VerificationStageRequestedRequetee {
	return &verificationStageRequestedRequetee{c}
}

type VerificationStageReady interface {
	Start(method string)
	Cancel()
}
type verificationStageReady struct {
	c *VerificationContainer
}

func (v *verificationStageReady) Start(method string) {
	v.c.SendStart(method)
}
func (v *verificationStageReady) Cancel() {
	v.c.SendCancel()
}
func NewVerificationStageReady(c *VerificationContainer) VerificationStageReady {
	return &verificationStageReady{c}
}

type VerificationStageStart interface {
	Transition()
	Cancel()
}
type verificationStageStart struct {
	c *VerificationContainer
}

func (v *verificationStageStart) Transition() {
	v.c.SendTransition()
}
func (v *verificationStageStart) Cancel() {
	v.c.SendCancel()
}
func NewVerificationStageStart(c *VerificationContainer) VerificationStageStart {
	return &verificationStageStart{c}
}

type VerificationStageTransitioned interface {
	Done()
	VerificationData() VerificationData
	Cancel()
	Transition()
}
type verificationStageTransitioned struct {
	c *VerificationContainer
}

func (v *verificationStageTransitioned) Done() {
	v.c.SendDone()
}
func (v *verificationStageTransitioned) VerificationData() VerificationData {
	return v.c.VData
}
func (v *verificationStageTransitioned) Cancel() {
	v.c.SendCancel()
}
func (v *verificationStageTransitioned) Transition() {
	v.c.SendTransition()
}
func NewVerificationStageTransitioned(c *VerificationContainer) VerificationStageTransitioned {
	return &verificationStageTransitioned{c}
}

type VerificationStageDone interface {
	VerificationState() VerificationState
}
type verificationStageDone struct {
	c *VerificationContainer
}

func (v *verificationStageDone) VerificationState() VerificationState {
	return v.c.VState
}
func NewVerificationStageDone(c *VerificationContainer) VerificationStageDone {
	return &verificationStageDone{c}
}

type VerificationStageCancelled interface {
}

func NewVerificationStageCancelled(c *VerificationContainer) VerificationStageCancelled {
	return struct{}{}
}

type VerificationState string

var (
	VerificationStateVerified   VerificationState = "verified"
	VerificationStateUnverified VerificationState = "unverified"
	VerificationStateUnknown    VerificationState = "unknown"
)

// VerificationContainer is a helper struct for client implementations.
// It implements all verification stages, so can be returned at each stage.
// The container is never exposed directly in tests, as interfaces hide
// invalid state transitions.
type VerificationContainer struct {
	VReq           VerificationRequest
	VData          VerificationData
	VState         VerificationState
	SendReady      func()
	SendStart      func(method string)
	SendCancel     func()
	SendDone       func()
	SendTransition func()
	Mutex          *sync.Mutex
}

func (c *VerificationContainer) Modify(fn func(cc *VerificationContainer)) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	fn(c)
}

func (c *VerificationContainer) Stage(stageEnum VerificationStageEnum) VerificationStage {
	switch stageEnum {
	case VerificationStageEnumRequested:
		return NewVerificationStageRequested(c)
	case VerificationStageEnumRequestedRequetee:
		return NewVerificationStageRequestedRequetee(c)
	case VerificationStageEnumReady:
		return NewVerificationStageReady(c)
	case VerificationStageEnumStart:
		return NewVerificationStageStart(c)
	case VerificationStageEnumTransitioned:
		return NewVerificationStageTransitioned(c)
	case VerificationStageEnumDone:
		return NewVerificationStageDone(c)
	case VerificationStageEnumCancelled:
		return struct{}{}
	default:
		panic(fmt.Sprintf("unknown VerificationStageEnum: %v", stageEnum))
	}
}
