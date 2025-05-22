package matrix_sdk

// #include <matrix_sdk.h>
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// This is needed, because as of go 1.24
// type RustBuffer C.RustBuffer cannot have methods,
// RustBuffer is treated as non-local type
type GoRustBuffer struct {
	inner C.RustBuffer
}

type RustBufferI interface {
	AsReader() *bytes.Reader
	Free()
	ToGoBytes() []byte
	Data() unsafe.Pointer
	Len() uint64
	Capacity() uint64
}

func RustBufferFromExternal(b RustBufferI) GoRustBuffer {
	return GoRustBuffer{
		inner: C.RustBuffer{
			capacity: C.uint64_t(b.Capacity()),
			len:      C.uint64_t(b.Len()),
			data:     (*C.uchar)(b.Data()),
		},
	}
}

func (cb GoRustBuffer) Capacity() uint64 {
	return uint64(cb.inner.capacity)
}

func (cb GoRustBuffer) Len() uint64 {
	return uint64(cb.inner.len)
}

func (cb GoRustBuffer) Data() unsafe.Pointer {
	return unsafe.Pointer(cb.inner.data)
}

func (cb GoRustBuffer) AsReader() *bytes.Reader {
	b := unsafe.Slice((*byte)(cb.inner.data), C.uint64_t(cb.inner.len))
	return bytes.NewReader(b)
}

func (cb GoRustBuffer) Free() {
	rustCall(func(status *C.RustCallStatus) bool {
		C.ffi_matrix_sdk_rustbuffer_free(cb.inner, status)
		return false
	})
}

func (cb GoRustBuffer) ToGoBytes() []byte {
	return C.GoBytes(unsafe.Pointer(cb.inner.data), C.int(cb.inner.len))
}

func stringToRustBuffer(str string) C.RustBuffer {
	return bytesToRustBuffer([]byte(str))
}

func bytesToRustBuffer(b []byte) C.RustBuffer {
	if len(b) == 0 {
		return C.RustBuffer{}
	}
	// We can pass the pointer along here, as it is pinned
	// for the duration of this call
	foreign := C.ForeignBytes{
		len:  C.int(len(b)),
		data: (*C.uchar)(unsafe.Pointer(&b[0])),
	}

	return rustCall(func(status *C.RustCallStatus) C.RustBuffer {
		return C.ffi_matrix_sdk_rustbuffer_from_bytes(foreign, status)
	})
}

type BufLifter[GoType any] interface {
	Lift(value RustBufferI) GoType
}

type BufLowerer[GoType any] interface {
	Lower(value GoType) C.RustBuffer
}

type BufReader[GoType any] interface {
	Read(reader io.Reader) GoType
}

type BufWriter[GoType any] interface {
	Write(writer io.Writer, value GoType)
}

func LowerIntoRustBuffer[GoType any](bufWriter BufWriter[GoType], value GoType) C.RustBuffer {
	// This might be not the most efficient way but it does not require knowing allocation size
	// beforehand
	var buffer bytes.Buffer
	bufWriter.Write(&buffer, value)

	bytes, err := io.ReadAll(&buffer)
	if err != nil {
		panic(fmt.Errorf("reading written data: %w", err))
	}
	return bytesToRustBuffer(bytes)
}

func LiftFromRustBuffer[GoType any](bufReader BufReader[GoType], rbuf RustBufferI) GoType {
	defer rbuf.Free()
	reader := rbuf.AsReader()
	item := bufReader.Read(reader)
	if reader.Len() > 0 {
		// TODO: Remove this
		leftover, _ := io.ReadAll(reader)
		panic(fmt.Errorf("Junk remaining in buffer after lifting: %s", string(leftover)))
	}
	return item
}

func rustCallWithError[E any, U any](converter BufReader[*E], callback func(*C.RustCallStatus) U) (U, *E) {
	var status C.RustCallStatus
	returnValue := callback(&status)
	err := checkCallStatus(converter, status)
	return returnValue, err
}

func checkCallStatus[E any](converter BufReader[*E], status C.RustCallStatus) *E {
	switch status.code {
	case 0:
		return nil
	case 1:
		return LiftFromRustBuffer(converter, GoRustBuffer{inner: status.errorBuf})
	case 2:
		// when the rust code sees a panic, it tries to construct a rustBuffer
		// with the message.  but if that code panics, then it just sends back
		// an empty buffer.
		if status.errorBuf.len > 0 {
			panic(fmt.Errorf("%s", FfiConverterStringINSTANCE.Lift(GoRustBuffer{inner: status.errorBuf})))
		} else {
			panic(fmt.Errorf("Rust panicked while handling Rust panic"))
		}
	default:
		panic(fmt.Errorf("unknown status code: %d", status.code))
	}
}

func checkCallStatusUnknown(status C.RustCallStatus) error {
	switch status.code {
	case 0:
		return nil
	case 1:
		panic(fmt.Errorf("function not returning an error returned an error"))
	case 2:
		// when the rust code sees a panic, it tries to construct a C.RustBuffer
		// with the message.  but if that code panics, then it just sends back
		// an empty buffer.
		if status.errorBuf.len > 0 {
			panic(fmt.Errorf("%s", FfiConverterStringINSTANCE.Lift(GoRustBuffer{
				inner: status.errorBuf,
			})))
		} else {
			panic(fmt.Errorf("Rust panicked while handling Rust panic"))
		}
	default:
		return fmt.Errorf("unknown status code: %d", status.code)
	}
}

func rustCall[U any](callback func(*C.RustCallStatus) U) U {
	returnValue, err := rustCallWithError[error](nil, callback)
	if err != nil {
		panic(err)
	}
	return returnValue
}

type NativeError interface {
	AsError() error
}

func writeInt8(writer io.Writer, value int8) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint8(writer io.Writer, value uint8) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeInt16(writer io.Writer, value int16) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint16(writer io.Writer, value uint16) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeInt32(writer io.Writer, value int32) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint32(writer io.Writer, value uint32) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeInt64(writer io.Writer, value int64) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint64(writer io.Writer, value uint64) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeFloat32(writer io.Writer, value float32) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeFloat64(writer io.Writer, value float64) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func readInt8(reader io.Reader) int8 {
	var result int8
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint8(reader io.Reader) uint8 {
	var result uint8
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readInt16(reader io.Reader) int16 {
	var result int16
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint16(reader io.Reader) uint16 {
	var result uint16
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readInt32(reader io.Reader) int32 {
	var result int32
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint32(reader io.Reader) uint32 {
	var result uint32
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readInt64(reader io.Reader) int64 {
	var result int64
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint64(reader io.Reader) uint64 {
	var result uint64
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readFloat32(reader io.Reader) float32 {
	var result float32
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readFloat64(reader io.Reader) float64 {
	var result float64
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func init() {

	uniffiCheckChecksums()
}

func uniffiCheckChecksums() {
	// Get the bindings contract version from our ComponentInterface
	bindingsContractVersion := 26
	// Get the scaffolding contract version by calling the into the dylib
	scaffoldingContractVersion := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.ffi_matrix_sdk_uniffi_contract_version()
	})
	if bindingsContractVersion != int(scaffoldingContractVersion) {
		// If this happens try cleaning and rebuilding your project
		panic("matrix_sdk: UniFFI contract version mismatch")
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_checksum_method_oauthauthorizationdata_login_url()
		})
		if checksum != 25566 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk: uniffi_matrix_sdk_checksum_method_oauthauthorizationdata_login_url: UniFFI API checksum mismatch")
		}
	}
}

type FfiConverterInt64 struct{}

var FfiConverterInt64INSTANCE = FfiConverterInt64{}

func (FfiConverterInt64) Lower(value int64) C.int64_t {
	return C.int64_t(value)
}

func (FfiConverterInt64) Write(writer io.Writer, value int64) {
	writeInt64(writer, value)
}

func (FfiConverterInt64) Lift(value C.int64_t) int64 {
	return int64(value)
}

func (FfiConverterInt64) Read(reader io.Reader) int64 {
	return readInt64(reader)
}

type FfiDestroyerInt64 struct{}

func (FfiDestroyerInt64) Destroy(_ int64) {}

type FfiConverterBool struct{}

var FfiConverterBoolINSTANCE = FfiConverterBool{}

func (FfiConverterBool) Lower(value bool) C.int8_t {
	if value {
		return C.int8_t(1)
	}
	return C.int8_t(0)
}

func (FfiConverterBool) Write(writer io.Writer, value bool) {
	if value {
		writeInt8(writer, 1)
	} else {
		writeInt8(writer, 0)
	}
}

func (FfiConverterBool) Lift(value C.int8_t) bool {
	return value != 0
}

func (FfiConverterBool) Read(reader io.Reader) bool {
	return readInt8(reader) != 0
}

type FfiDestroyerBool struct{}

func (FfiDestroyerBool) Destroy(_ bool) {}

type FfiConverterString struct{}

var FfiConverterStringINSTANCE = FfiConverterString{}

func (FfiConverterString) Lift(rb RustBufferI) string {
	defer rb.Free()
	reader := rb.AsReader()
	b, err := io.ReadAll(reader)
	if err != nil {
		panic(fmt.Errorf("reading reader: %w", err))
	}
	return string(b)
}

func (FfiConverterString) Read(reader io.Reader) string {
	length := readInt32(reader)
	buffer := make([]byte, length)
	read_length, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		panic(err)
	}
	if read_length != int(length) {
		panic(fmt.Errorf("bad read length when reading string, expected %d, read %d", length, read_length))
	}
	return string(buffer)
}

func (FfiConverterString) Lower(value string) C.RustBuffer {
	return stringToRustBuffer(value)
}

func (FfiConverterString) Write(writer io.Writer, value string) {
	if len(value) > math.MaxInt32 {
		panic("String is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	write_length, err := io.WriteString(writer, value)
	if err != nil {
		panic(err)
	}
	if write_length != len(value) {
		panic(fmt.Errorf("bad write length when writing string, expected %d, written %d", len(value), write_length))
	}
}

type FfiDestroyerString struct{}

func (FfiDestroyerString) Destroy(_ string) {}

// Below is an implementation of synchronization requirements outlined in the link.
// https://github.com/mozilla/uniffi-rs/blob/0dc031132d9493ca812c3af6e7dd60ad2ea95bf0/uniffi_bindgen/src/bindings/kotlin/templates/ObjectRuntime.kt#L31

type FfiObject struct {
	pointer       unsafe.Pointer
	callCounter   atomic.Int64
	cloneFunction func(unsafe.Pointer, *C.RustCallStatus) unsafe.Pointer
	freeFunction  func(unsafe.Pointer, *C.RustCallStatus)
	destroyed     atomic.Bool
}

func newFfiObject(
	pointer unsafe.Pointer,
	cloneFunction func(unsafe.Pointer, *C.RustCallStatus) unsafe.Pointer,
	freeFunction func(unsafe.Pointer, *C.RustCallStatus),
) FfiObject {
	return FfiObject{
		pointer:       pointer,
		cloneFunction: cloneFunction,
		freeFunction:  freeFunction,
	}
}

func (ffiObject *FfiObject) incrementPointer(debugName string) unsafe.Pointer {
	for {
		counter := ffiObject.callCounter.Load()
		if counter <= -1 {
			panic(fmt.Errorf("%v object has already been destroyed", debugName))
		}
		if counter == math.MaxInt64 {
			panic(fmt.Errorf("%v object call counter would overflow", debugName))
		}
		if ffiObject.callCounter.CompareAndSwap(counter, counter+1) {
			break
		}
	}

	return rustCall(func(status *C.RustCallStatus) unsafe.Pointer {
		return ffiObject.cloneFunction(ffiObject.pointer, status)
	})
}

func (ffiObject *FfiObject) decrementPointer() {
	if ffiObject.callCounter.Add(-1) == -1 {
		ffiObject.freeRustArcPtr()
	}
}

func (ffiObject *FfiObject) destroy() {
	if ffiObject.destroyed.CompareAndSwap(false, true) {
		if ffiObject.callCounter.Add(-1) == -1 {
			ffiObject.freeRustArcPtr()
		}
	}
}

func (ffiObject *FfiObject) freeRustArcPtr() {
	rustCall(func(status *C.RustCallStatus) int32 {
		ffiObject.freeFunction(ffiObject.pointer, status)
		return 0
	})
}

// The data needed to perform authorization using OAuth 2.0.
type OAuthAuthorizationDataInterface interface {
	// The login URL to use for authorization.
	LoginUrl() string
}

// The data needed to perform authorization using OAuth 2.0.
type OAuthAuthorizationData struct {
	ffiObject FfiObject
}

// The login URL to use for authorization.
func (_self *OAuthAuthorizationData) LoginUrl() string {
	_pointer := _self.ffiObject.incrementPointer("*OAuthAuthorizationData")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_matrix_sdk_fn_method_oauthauthorizationdata_login_url(
				_pointer, _uniffiStatus),
		}
	}))
}
func (object *OAuthAuthorizationData) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterOAuthAuthorizationData struct{}

var FfiConverterOAuthAuthorizationDataINSTANCE = FfiConverterOAuthAuthorizationData{}

func (c FfiConverterOAuthAuthorizationData) Lift(pointer unsafe.Pointer) *OAuthAuthorizationData {
	result := &OAuthAuthorizationData{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_matrix_sdk_fn_clone_oauthauthorizationdata(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_fn_free_oauthauthorizationdata(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*OAuthAuthorizationData).Destroy)
	return result
}

func (c FfiConverterOAuthAuthorizationData) Read(reader io.Reader) *OAuthAuthorizationData {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterOAuthAuthorizationData) Lower(value *OAuthAuthorizationData) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*OAuthAuthorizationData")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterOAuthAuthorizationData) Write(writer io.Writer, value *OAuthAuthorizationData) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerOAuthAuthorizationData struct{}

func (_ FfiDestroyerOAuthAuthorizationData) Destroy(value *OAuthAuthorizationData) {
	value.Destroy()
}

// A set of common power levels required for various operations within a room,
// that can be applied as a single operation. When updating these
// settings, any levels that are `None` will remain unchanged.
type RoomPowerLevelChanges struct {
	// The level required to ban a user.
	Ban *int64
	// The level required to invite a user.
	Invite *int64
	// The level required to kick a user.
	Kick *int64
	// The level required to redact an event.
	Redact *int64
	// The default level required to send message events.
	EventsDefault *int64
	// The default level required to send state events.
	StateDefault *int64
	// The default power level for every user in the room.
	UsersDefault *int64
	// The level required to change the room's name.
	RoomName *int64
	// The level required to change the room's avatar.
	RoomAvatar *int64
	// The level required to change the room's topic.
	RoomTopic *int64
}

func (r *RoomPowerLevelChanges) Destroy() {
	FfiDestroyerOptionalInt64{}.Destroy(r.Ban)
	FfiDestroyerOptionalInt64{}.Destroy(r.Invite)
	FfiDestroyerOptionalInt64{}.Destroy(r.Kick)
	FfiDestroyerOptionalInt64{}.Destroy(r.Redact)
	FfiDestroyerOptionalInt64{}.Destroy(r.EventsDefault)
	FfiDestroyerOptionalInt64{}.Destroy(r.StateDefault)
	FfiDestroyerOptionalInt64{}.Destroy(r.UsersDefault)
	FfiDestroyerOptionalInt64{}.Destroy(r.RoomName)
	FfiDestroyerOptionalInt64{}.Destroy(r.RoomAvatar)
	FfiDestroyerOptionalInt64{}.Destroy(r.RoomTopic)
}

type FfiConverterRoomPowerLevelChanges struct{}

var FfiConverterRoomPowerLevelChangesINSTANCE = FfiConverterRoomPowerLevelChanges{}

func (c FfiConverterRoomPowerLevelChanges) Lift(rb RustBufferI) RoomPowerLevelChanges {
	return LiftFromRustBuffer[RoomPowerLevelChanges](c, rb)
}

func (c FfiConverterRoomPowerLevelChanges) Read(reader io.Reader) RoomPowerLevelChanges {
	return RoomPowerLevelChanges{
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
		FfiConverterOptionalInt64INSTANCE.Read(reader),
	}
}

func (c FfiConverterRoomPowerLevelChanges) Lower(value RoomPowerLevelChanges) C.RustBuffer {
	return LowerIntoRustBuffer[RoomPowerLevelChanges](c, value)
}

func (c FfiConverterRoomPowerLevelChanges) Write(writer io.Writer, value RoomPowerLevelChanges) {
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.Ban)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.Invite)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.Kick)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.Redact)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.EventsDefault)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.StateDefault)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.UsersDefault)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.RoomName)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.RoomAvatar)
	FfiConverterOptionalInt64INSTANCE.Write(writer, value.RoomTopic)
}

type FfiDestroyerRoomPowerLevelChanges struct{}

func (_ FfiDestroyerRoomPowerLevelChanges) Destroy(value RoomPowerLevelChanges) {
	value.Destroy()
}

// Settings for end-to-end encryption features.
type BackupDownloadStrategy uint

const (
	// Automatically download all room keys from the backup when the backup
	// recovery key has been received. The backup recovery key can be received
	// in two ways:
	//
	// 1. Received as a `m.secret.send` to-device event, after a successful
	// interactive verification.
	// 2. Imported from secret storage (4S) using the
	// [`SecretStore::import_secrets()`] method.
	//
	// [`SecretStore::import_secrets()`]: crate::encryption::secret_storage::SecretStore::import_secrets
	BackupDownloadStrategyOneShot BackupDownloadStrategy = 1
	// Attempt to download a single room key if an event fails to be decrypted.
	BackupDownloadStrategyAfterDecryptionFailure BackupDownloadStrategy = 2
	// Don't download any room keys automatically. The user can manually
	// download room keys using the [`Backups::download_room_key()`] methods.
	//
	// This is the default option.
	BackupDownloadStrategyManual BackupDownloadStrategy = 3
)

type FfiConverterBackupDownloadStrategy struct{}

var FfiConverterBackupDownloadStrategyINSTANCE = FfiConverterBackupDownloadStrategy{}

func (c FfiConverterBackupDownloadStrategy) Lift(rb RustBufferI) BackupDownloadStrategy {
	return LiftFromRustBuffer[BackupDownloadStrategy](c, rb)
}

func (c FfiConverterBackupDownloadStrategy) Lower(value BackupDownloadStrategy) C.RustBuffer {
	return LowerIntoRustBuffer[BackupDownloadStrategy](c, value)
}
func (FfiConverterBackupDownloadStrategy) Read(reader io.Reader) BackupDownloadStrategy {
	id := readInt32(reader)
	return BackupDownloadStrategy(id)
}

func (FfiConverterBackupDownloadStrategy) Write(writer io.Writer, value BackupDownloadStrategy) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerBackupDownloadStrategy struct{}

func (_ FfiDestroyerBackupDownloadStrategy) Destroy(value BackupDownloadStrategy) {
}

// Current state of a [`Paginator`].
type PaginatorState uint

const (
	// The initial state of the paginator.
	PaginatorStateInitial PaginatorState = 1
	// The paginator is fetching the target initial event.
	PaginatorStateFetchingTargetEvent PaginatorState = 2
	// The target initial event could be found, zero or more paginations have
	// happened since then, and the paginator is at rest now.
	PaginatorStateIdle PaginatorState = 3
	// The paginator is… paginating one direction or another.
	PaginatorStatePaginating PaginatorState = 4
)

type FfiConverterPaginatorState struct{}

var FfiConverterPaginatorStateINSTANCE = FfiConverterPaginatorState{}

func (c FfiConverterPaginatorState) Lift(rb RustBufferI) PaginatorState {
	return LiftFromRustBuffer[PaginatorState](c, rb)
}

func (c FfiConverterPaginatorState) Lower(value PaginatorState) C.RustBuffer {
	return LowerIntoRustBuffer[PaginatorState](c, value)
}
func (FfiConverterPaginatorState) Read(reader io.Reader) PaginatorState {
	id := readInt32(reader)
	return PaginatorState(id)
}

func (FfiConverterPaginatorState) Write(writer io.Writer, value PaginatorState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerPaginatorState struct{}

func (_ FfiDestroyerPaginatorState) Destroy(value PaginatorState) {
}

// The error type for failures while trying to log in a new device using a QR
// code.
type QrCodeLoginError struct {
	err error
}

// Convience method to turn *QrCodeLoginError into error
// Avoiding treating nil pointer as non nil error interface
func (err *QrCodeLoginError) AsError() error {
	if err == nil {
		return nil
	} else {
		return err
	}
}

func (err QrCodeLoginError) Error() string {
	return fmt.Sprintf("QrCodeLoginError: %s", err.err.Error())
}

func (err QrCodeLoginError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrQrCodeLoginErrorOAuth = fmt.Errorf("QrCodeLoginErrorOAuth")
var ErrQrCodeLoginErrorLoginFailure = fmt.Errorf("QrCodeLoginErrorLoginFailure")
var ErrQrCodeLoginErrorUnexpectedMessage = fmt.Errorf("QrCodeLoginErrorUnexpectedMessage")
var ErrQrCodeLoginErrorSecureChannel = fmt.Errorf("QrCodeLoginErrorSecureChannel")
var ErrQrCodeLoginErrorCrossProcessRefreshLock = fmt.Errorf("QrCodeLoginErrorCrossProcessRefreshLock")
var ErrQrCodeLoginErrorUserIdDiscovery = fmt.Errorf("QrCodeLoginErrorUserIdDiscovery")
var ErrQrCodeLoginErrorSessionTokens = fmt.Errorf("QrCodeLoginErrorSessionTokens")
var ErrQrCodeLoginErrorDeviceKeyUpload = fmt.Errorf("QrCodeLoginErrorDeviceKeyUpload")
var ErrQrCodeLoginErrorSecretImport = fmt.Errorf("QrCodeLoginErrorSecretImport")

// Variant structs
// An error happened while we were communicating with the OAuth 2.0
// authorization server.
type QrCodeLoginErrorOAuth struct {
	message string
}

// An error happened while we were communicating with the OAuth 2.0
// authorization server.
func NewQrCodeLoginErrorOAuth() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorOAuth{}}
}

func (e QrCodeLoginErrorOAuth) destroy() {
}

func (err QrCodeLoginErrorOAuth) Error() string {
	return fmt.Sprintf("OAuth: %s", err.message)
}

func (self QrCodeLoginErrorOAuth) Is(target error) bool {
	return target == ErrQrCodeLoginErrorOAuth
}

// The other device has signaled to us that the login has failed.
type QrCodeLoginErrorLoginFailure struct {
	message string
}

// The other device has signaled to us that the login has failed.
func NewQrCodeLoginErrorLoginFailure() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorLoginFailure{}}
}

func (e QrCodeLoginErrorLoginFailure) destroy() {
}

func (err QrCodeLoginErrorLoginFailure) Error() string {
	return fmt.Sprintf("LoginFailure: %s", err.message)
}

func (self QrCodeLoginErrorLoginFailure) Is(target error) bool {
	return target == ErrQrCodeLoginErrorLoginFailure
}

// An unexpected message was received from the other device.
type QrCodeLoginErrorUnexpectedMessage struct {
	message string
}

// An unexpected message was received from the other device.
func NewQrCodeLoginErrorUnexpectedMessage() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorUnexpectedMessage{}}
}

func (e QrCodeLoginErrorUnexpectedMessage) destroy() {
}

func (err QrCodeLoginErrorUnexpectedMessage) Error() string {
	return fmt.Sprintf("UnexpectedMessage: %s", err.message)
}

func (self QrCodeLoginErrorUnexpectedMessage) Is(target error) bool {
	return target == ErrQrCodeLoginErrorUnexpectedMessage
}

// An error happened while exchanging messages with the other device.
type QrCodeLoginErrorSecureChannel struct {
	message string
}

// An error happened while exchanging messages with the other device.
func NewQrCodeLoginErrorSecureChannel() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorSecureChannel{}}
}

func (e QrCodeLoginErrorSecureChannel) destroy() {
}

func (err QrCodeLoginErrorSecureChannel) Error() string {
	return fmt.Sprintf("SecureChannel: %s", err.message)
}

func (self QrCodeLoginErrorSecureChannel) Is(target error) bool {
	return target == ErrQrCodeLoginErrorSecureChannel
}

// The cross-process refresh lock failed to be initialized.
type QrCodeLoginErrorCrossProcessRefreshLock struct {
	message string
}

// The cross-process refresh lock failed to be initialized.
func NewQrCodeLoginErrorCrossProcessRefreshLock() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorCrossProcessRefreshLock{}}
}

func (e QrCodeLoginErrorCrossProcessRefreshLock) destroy() {
}

func (err QrCodeLoginErrorCrossProcessRefreshLock) Error() string {
	return fmt.Sprintf("CrossProcessRefreshLock: %s", err.message)
}

func (self QrCodeLoginErrorCrossProcessRefreshLock) Is(target error) bool {
	return target == ErrQrCodeLoginErrorCrossProcessRefreshLock
}

// An error happened while we were trying to discover our user and device
// ID, after we have acquired an access token from the OAuth 2.0
// authorization server.
type QrCodeLoginErrorUserIdDiscovery struct {
	message string
}

// An error happened while we were trying to discover our user and device
// ID, after we have acquired an access token from the OAuth 2.0
// authorization server.
func NewQrCodeLoginErrorUserIdDiscovery() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorUserIdDiscovery{}}
}

func (e QrCodeLoginErrorUserIdDiscovery) destroy() {
}

func (err QrCodeLoginErrorUserIdDiscovery) Error() string {
	return fmt.Sprintf("UserIdDiscovery: %s", err.message)
}

func (self QrCodeLoginErrorUserIdDiscovery) Is(target error) bool {
	return target == ErrQrCodeLoginErrorUserIdDiscovery
}

// We failed to set the session tokens after we figured out our device and
// user IDs.
type QrCodeLoginErrorSessionTokens struct {
	message string
}

// We failed to set the session tokens after we figured out our device and
// user IDs.
func NewQrCodeLoginErrorSessionTokens() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorSessionTokens{}}
}

func (e QrCodeLoginErrorSessionTokens) destroy() {
}

func (err QrCodeLoginErrorSessionTokens) Error() string {
	return fmt.Sprintf("SessionTokens: %s", err.message)
}

func (self QrCodeLoginErrorSessionTokens) Is(target error) bool {
	return target == ErrQrCodeLoginErrorSessionTokens
}

// The device keys failed to be uploaded after we successfully logged in.
type QrCodeLoginErrorDeviceKeyUpload struct {
	message string
}

// The device keys failed to be uploaded after we successfully logged in.
func NewQrCodeLoginErrorDeviceKeyUpload() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorDeviceKeyUpload{}}
}

func (e QrCodeLoginErrorDeviceKeyUpload) destroy() {
}

func (err QrCodeLoginErrorDeviceKeyUpload) Error() string {
	return fmt.Sprintf("DeviceKeyUpload: %s", err.message)
}

func (self QrCodeLoginErrorDeviceKeyUpload) Is(target error) bool {
	return target == ErrQrCodeLoginErrorDeviceKeyUpload
}

// The secrets bundle we received from the existing device failed to be
// imported.
type QrCodeLoginErrorSecretImport struct {
	message string
}

// The secrets bundle we received from the existing device failed to be
// imported.
func NewQrCodeLoginErrorSecretImport() *QrCodeLoginError {
	return &QrCodeLoginError{err: &QrCodeLoginErrorSecretImport{}}
}

func (e QrCodeLoginErrorSecretImport) destroy() {
}

func (err QrCodeLoginErrorSecretImport) Error() string {
	return fmt.Sprintf("SecretImport: %s", err.message)
}

func (self QrCodeLoginErrorSecretImport) Is(target error) bool {
	return target == ErrQrCodeLoginErrorSecretImport
}

type FfiConverterQrCodeLoginError struct{}

var FfiConverterQrCodeLoginErrorINSTANCE = FfiConverterQrCodeLoginError{}

func (c FfiConverterQrCodeLoginError) Lift(eb RustBufferI) *QrCodeLoginError {
	return LiftFromRustBuffer[*QrCodeLoginError](c, eb)
}

func (c FfiConverterQrCodeLoginError) Lower(value *QrCodeLoginError) C.RustBuffer {
	return LowerIntoRustBuffer[*QrCodeLoginError](c, value)
}

func (c FfiConverterQrCodeLoginError) Read(reader io.Reader) *QrCodeLoginError {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &QrCodeLoginError{&QrCodeLoginErrorOAuth{message}}
	case 2:
		return &QrCodeLoginError{&QrCodeLoginErrorLoginFailure{message}}
	case 3:
		return &QrCodeLoginError{&QrCodeLoginErrorUnexpectedMessage{message}}
	case 4:
		return &QrCodeLoginError{&QrCodeLoginErrorSecureChannel{message}}
	case 5:
		return &QrCodeLoginError{&QrCodeLoginErrorCrossProcessRefreshLock{message}}
	case 6:
		return &QrCodeLoginError{&QrCodeLoginErrorUserIdDiscovery{message}}
	case 7:
		return &QrCodeLoginError{&QrCodeLoginErrorSessionTokens{message}}
	case 8:
		return &QrCodeLoginError{&QrCodeLoginErrorDeviceKeyUpload{message}}
	case 9:
		return &QrCodeLoginError{&QrCodeLoginErrorSecretImport{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterQrCodeLoginError.Read()", errorID))
	}

}

func (c FfiConverterQrCodeLoginError) Write(writer io.Writer, value *QrCodeLoginError) {
	switch variantValue := value.err.(type) {
	case *QrCodeLoginErrorOAuth:
		writeInt32(writer, 1)
	case *QrCodeLoginErrorLoginFailure:
		writeInt32(writer, 2)
	case *QrCodeLoginErrorUnexpectedMessage:
		writeInt32(writer, 3)
	case *QrCodeLoginErrorSecureChannel:
		writeInt32(writer, 4)
	case *QrCodeLoginErrorCrossProcessRefreshLock:
		writeInt32(writer, 5)
	case *QrCodeLoginErrorUserIdDiscovery:
		writeInt32(writer, 6)
	case *QrCodeLoginErrorSessionTokens:
		writeInt32(writer, 7)
	case *QrCodeLoginErrorDeviceKeyUpload:
		writeInt32(writer, 8)
	case *QrCodeLoginErrorSecretImport:
		writeInt32(writer, 9)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterQrCodeLoginError.Write", value))
	}
}

type FfiDestroyerQrCodeLoginError struct{}

func (_ FfiDestroyerQrCodeLoginError) Destroy(value *QrCodeLoginError) {
	switch variantValue := value.err.(type) {
	case QrCodeLoginErrorOAuth:
		variantValue.destroy()
	case QrCodeLoginErrorLoginFailure:
		variantValue.destroy()
	case QrCodeLoginErrorUnexpectedMessage:
		variantValue.destroy()
	case QrCodeLoginErrorSecureChannel:
		variantValue.destroy()
	case QrCodeLoginErrorCrossProcessRefreshLock:
		variantValue.destroy()
	case QrCodeLoginErrorUserIdDiscovery:
		variantValue.destroy()
	case QrCodeLoginErrorSessionTokens:
		variantValue.destroy()
	case QrCodeLoginErrorDeviceKeyUpload:
		variantValue.destroy()
	case QrCodeLoginErrorSecretImport:
		variantValue.destroy()
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiDestroyerQrCodeLoginError.Destroy", value))
	}
}

// The role of a member in a room.
type RoomMemberRole uint

const (
	// The member is an administrator.
	RoomMemberRoleAdministrator RoomMemberRole = 1
	// The member is a moderator.
	RoomMemberRoleModerator RoomMemberRole = 2
	// The member is a regular user.
	RoomMemberRoleUser RoomMemberRole = 3
)

type FfiConverterRoomMemberRole struct{}

var FfiConverterRoomMemberRoleINSTANCE = FfiConverterRoomMemberRole{}

func (c FfiConverterRoomMemberRole) Lift(rb RustBufferI) RoomMemberRole {
	return LiftFromRustBuffer[RoomMemberRole](c, rb)
}

func (c FfiConverterRoomMemberRole) Lower(value RoomMemberRole) C.RustBuffer {
	return LowerIntoRustBuffer[RoomMemberRole](c, value)
}
func (FfiConverterRoomMemberRole) Read(reader io.Reader) RoomMemberRole {
	id := readInt32(reader)
	return RoomMemberRole(id)
}

func (FfiConverterRoomMemberRole) Write(writer io.Writer, value RoomMemberRole) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerRoomMemberRole struct{}

func (_ FfiDestroyerRoomMemberRole) Destroy(value RoomMemberRole) {
}

// Status for the back-pagination on a room event cache.
type RoomPaginationStatus interface {
	Destroy()
}

// No back-pagination is happening right now.
type RoomPaginationStatusIdle struct {
	HitTimelineStart bool
}

func (e RoomPaginationStatusIdle) Destroy() {
	FfiDestroyerBool{}.Destroy(e.HitTimelineStart)
}

// Back-pagination is already running in the background.
type RoomPaginationStatusPaginating struct {
}

func (e RoomPaginationStatusPaginating) Destroy() {
}

type FfiConverterRoomPaginationStatus struct{}

var FfiConverterRoomPaginationStatusINSTANCE = FfiConverterRoomPaginationStatus{}

func (c FfiConverterRoomPaginationStatus) Lift(rb RustBufferI) RoomPaginationStatus {
	return LiftFromRustBuffer[RoomPaginationStatus](c, rb)
}

func (c FfiConverterRoomPaginationStatus) Lower(value RoomPaginationStatus) C.RustBuffer {
	return LowerIntoRustBuffer[RoomPaginationStatus](c, value)
}
func (FfiConverterRoomPaginationStatus) Read(reader io.Reader) RoomPaginationStatus {
	id := readInt32(reader)
	switch id {
	case 1:
		return RoomPaginationStatusIdle{
			FfiConverterBoolINSTANCE.Read(reader),
		}
	case 2:
		return RoomPaginationStatusPaginating{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterRoomPaginationStatus.Read()", id))
	}
}

func (FfiConverterRoomPaginationStatus) Write(writer io.Writer, value RoomPaginationStatus) {
	switch variant_value := value.(type) {
	case RoomPaginationStatusIdle:
		writeInt32(writer, 1)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.HitTimelineStart)
	case RoomPaginationStatusPaginating:
		writeInt32(writer, 2)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterRoomPaginationStatus.Write", value))
	}
}

type FfiDestroyerRoomPaginationStatus struct{}

func (_ FfiDestroyerRoomPaginationStatus) Destroy(value RoomPaginationStatus) {
	value.Destroy()
}

type FfiConverterOptionalInt64 struct{}

var FfiConverterOptionalInt64INSTANCE = FfiConverterOptionalInt64{}

func (c FfiConverterOptionalInt64) Lift(rb RustBufferI) *int64 {
	return LiftFromRustBuffer[*int64](c, rb)
}

func (_ FfiConverterOptionalInt64) Read(reader io.Reader) *int64 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterInt64INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalInt64) Lower(value *int64) C.RustBuffer {
	return LowerIntoRustBuffer[*int64](c, value)
}

func (_ FfiConverterOptionalInt64) Write(writer io.Writer, value *int64) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterInt64INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalInt64 struct{}

func (_ FfiDestroyerOptionalInt64) Destroy(value *int64) {
	if value != nil {
		FfiDestroyerInt64{}.Destroy(*value)
	}
}
