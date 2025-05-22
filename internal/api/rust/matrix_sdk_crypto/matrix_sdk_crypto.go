package matrix_sdk_crypto

// #include <matrix_sdk_crypto.h>
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
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
		C.ffi_matrix_sdk_crypto_rustbuffer_free(cb.inner, status)
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
		return C.ffi_matrix_sdk_crypto_rustbuffer_from_bytes(foreign, status)
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
		return C.ffi_matrix_sdk_crypto_uniffi_contract_version()
	})
	if bindingsContractVersion != int(scaffoldingContractVersion) {
		// If this happens try cleaning and rebuilding your project
		panic("matrix_sdk_crypto: UniFFI contract version mismatch")
	}
}

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

// Settings for decrypting messages
type DecryptionSettings struct {
	// The trust level in the sender's device that is required to decrypt the
	// event. If the sender's device is not sufficiently trusted,
	// [`MegolmError::SenderIdentityNotTrusted`] will be returned.
	SenderDeviceTrustRequirement TrustRequirement
}

func (r *DecryptionSettings) Destroy() {
	FfiDestroyerTrustRequirement{}.Destroy(r.SenderDeviceTrustRequirement)
}

type FfiConverterDecryptionSettings struct{}

var FfiConverterDecryptionSettingsINSTANCE = FfiConverterDecryptionSettings{}

func (c FfiConverterDecryptionSettings) Lift(rb RustBufferI) DecryptionSettings {
	return LiftFromRustBuffer[DecryptionSettings](c, rb)
}

func (c FfiConverterDecryptionSettings) Read(reader io.Reader) DecryptionSettings {
	return DecryptionSettings{
		FfiConverterTrustRequirementINSTANCE.Read(reader),
	}
}

func (c FfiConverterDecryptionSettings) Lower(value DecryptionSettings) C.RustBuffer {
	return LowerIntoRustBuffer[DecryptionSettings](c, value)
}

func (c FfiConverterDecryptionSettings) Write(writer io.Writer, value DecryptionSettings) {
	FfiConverterTrustRequirementINSTANCE.Write(writer, value.SenderDeviceTrustRequirement)
}

type FfiDestroyerDecryptionSettings struct{}

func (_ FfiDestroyerDecryptionSettings) Destroy(value DecryptionSettings) {
	value.Destroy()
}

// Strategy to collect the devices that should receive room keys for the
// current discussion.
type CollectStrategy uint

const (
	// Share with all (unblacklisted) devices.
	CollectStrategyAllDevices CollectStrategy = 1
	// Share with all devices, except errors for *verified* users cause sharing
	// to fail with an error.
	//
	// In this strategy, if a verified user has an unsigned device,
	// key sharing will fail with a
	// [`SessionRecipientCollectionError::VerifiedUserHasUnsignedDevice`].
	// If a verified user has replaced their identity, key
	// sharing will fail with a
	// [`SessionRecipientCollectionError::VerifiedUserChangedIdentity`].
	//
	// Otherwise, keys are shared with unsigned devices as normal.
	//
	// Once the problematic devices are blacklisted or whitelisted the
	// caller can retry to share a second time.
	CollectStrategyErrorOnVerifiedUserProblem CollectStrategy = 2
	// Share based on identity. Only distribute to devices signed by their
	// owner. If a user has no published identity he will not receive
	// any room keys.
	CollectStrategyIdentityBasedStrategy CollectStrategy = 3
	// Only share keys with devices that we "trust". A device is trusted if any
	// of the following is true:
	// - It was manually marked as trusted.
	// - It was marked as verified via interactive verification.
	// - It is signed by its owner identity, and this identity has been
	// trusted via interactive verification.
	// - It is the current own device of the user.
	CollectStrategyOnlyTrustedDevices CollectStrategy = 4
)

type FfiConverterCollectStrategy struct{}

var FfiConverterCollectStrategyINSTANCE = FfiConverterCollectStrategy{}

func (c FfiConverterCollectStrategy) Lift(rb RustBufferI) CollectStrategy {
	return LiftFromRustBuffer[CollectStrategy](c, rb)
}

func (c FfiConverterCollectStrategy) Lower(value CollectStrategy) C.RustBuffer {
	return LowerIntoRustBuffer[CollectStrategy](c, value)
}
func (FfiConverterCollectStrategy) Read(reader io.Reader) CollectStrategy {
	id := readInt32(reader)
	return CollectStrategy(id)
}

func (FfiConverterCollectStrategy) Write(writer io.Writer, value CollectStrategy) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerCollectStrategy struct{}

func (_ FfiDestroyerCollectStrategy) Destroy(value CollectStrategy) {
}

// The state of an identity - verified, pinned etc.
type IdentityState uint

const (
	// The user is verified with us
	IdentityStateVerified IdentityState = 1
	// Either this is the first identity we have seen for this user, or the
	// user has acknowledged a change of identity explicitly e.g. by
	// clicking OK on a notification.
	IdentityStatePinned IdentityState = 2
	// The user's identity has changed since it was pinned. The user should be
	// notified about this and given the opportunity to acknowledge the
	// change, which will make the new identity pinned.
	// When the user acknowledges the change, the app should call
	// [`crate::OtherUserIdentity::pin_current_master_key`].
	IdentityStatePinViolation IdentityState = 3
	// The user's identity has changed, and before that it was verified. This
	// is a serious problem. The user can either verify again to make this
	// identity verified, or withdraw verification
	// [`UserIdentity::withdraw_verification`] to make it pinned.
	IdentityStateVerificationViolation IdentityState = 4
)

type FfiConverterIdentityState struct{}

var FfiConverterIdentityStateINSTANCE = FfiConverterIdentityState{}

func (c FfiConverterIdentityState) Lift(rb RustBufferI) IdentityState {
	return LiftFromRustBuffer[IdentityState](c, rb)
}

func (c FfiConverterIdentityState) Lower(value IdentityState) C.RustBuffer {
	return LowerIntoRustBuffer[IdentityState](c, value)
}
func (FfiConverterIdentityState) Read(reader io.Reader) IdentityState {
	id := readInt32(reader)
	return IdentityState(id)
}

func (FfiConverterIdentityState) Write(writer io.Writer, value IdentityState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerIdentityState struct{}

func (_ FfiDestroyerIdentityState) Destroy(value IdentityState) {
}

// The local trust state of a device.
type LocalTrust uint

const (
	// The device has been verified and is trusted.
	LocalTrustVerified LocalTrust = 1
	// The device been blacklisted from communicating.
	LocalTrustBlackListed LocalTrust = 2
	// The trust state of the device is being ignored.
	LocalTrustIgnored LocalTrust = 3
	// The trust state is unset.
	LocalTrustUnset LocalTrust = 4
)

type FfiConverterLocalTrust struct{}

var FfiConverterLocalTrustINSTANCE = FfiConverterLocalTrust{}

func (c FfiConverterLocalTrust) Lift(rb RustBufferI) LocalTrust {
	return LiftFromRustBuffer[LocalTrust](c, rb)
}

func (c FfiConverterLocalTrust) Lower(value LocalTrust) C.RustBuffer {
	return LowerIntoRustBuffer[LocalTrust](c, value)
}
func (FfiConverterLocalTrust) Read(reader io.Reader) LocalTrust {
	id := readInt32(reader)
	return LocalTrust(id)
}

func (FfiConverterLocalTrust) Write(writer io.Writer, value LocalTrust) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerLocalTrust struct{}

func (_ FfiDestroyerLocalTrust) Destroy(value LocalTrust) {
}

// Error type for the decoding of the [`QrCodeData`].
type LoginQrCodeDecodeError struct {
	err error
}

// Convience method to turn *LoginQrCodeDecodeError into error
// Avoiding treating nil pointer as non nil error interface
func (err *LoginQrCodeDecodeError) AsError() error {
	if err == nil {
		return nil
	} else {
		return err
	}
}

func (err LoginQrCodeDecodeError) Error() string {
	return fmt.Sprintf("LoginQrCodeDecodeError: %s", err.err.Error())
}

func (err LoginQrCodeDecodeError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrLoginQrCodeDecodeErrorNotEnoughData = fmt.Errorf("LoginQrCodeDecodeErrorNotEnoughData")
var ErrLoginQrCodeDecodeErrorNotUtf8 = fmt.Errorf("LoginQrCodeDecodeErrorNotUtf8")
var ErrLoginQrCodeDecodeErrorUrlParse = fmt.Errorf("LoginQrCodeDecodeErrorUrlParse")
var ErrLoginQrCodeDecodeErrorInvalidMode = fmt.Errorf("LoginQrCodeDecodeErrorInvalidMode")
var ErrLoginQrCodeDecodeErrorInvalidVersion = fmt.Errorf("LoginQrCodeDecodeErrorInvalidVersion")
var ErrLoginQrCodeDecodeErrorBase64 = fmt.Errorf("LoginQrCodeDecodeErrorBase64")
var ErrLoginQrCodeDecodeErrorInvalidPrefix = fmt.Errorf("LoginQrCodeDecodeErrorInvalidPrefix")

// Variant structs
// The QR code data is no long enough, it's missing some fields.
type LoginQrCodeDecodeErrorNotEnoughData struct {
	message string
}

// The QR code data is no long enough, it's missing some fields.
func NewLoginQrCodeDecodeErrorNotEnoughData() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorNotEnoughData{}}
}

func (e LoginQrCodeDecodeErrorNotEnoughData) destroy() {
}

func (err LoginQrCodeDecodeErrorNotEnoughData) Error() string {
	return fmt.Sprintf("NotEnoughData: %s", err.message)
}

func (self LoginQrCodeDecodeErrorNotEnoughData) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorNotEnoughData
}

// One of the URLs in the QR code data is not a valid UTF-8 encoded string.
type LoginQrCodeDecodeErrorNotUtf8 struct {
	message string
}

// One of the URLs in the QR code data is not a valid UTF-8 encoded string.
func NewLoginQrCodeDecodeErrorNotUtf8() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorNotUtf8{}}
}

func (e LoginQrCodeDecodeErrorNotUtf8) destroy() {
}

func (err LoginQrCodeDecodeErrorNotUtf8) Error() string {
	return fmt.Sprintf("NotUtf8: %s", err.message)
}

func (self LoginQrCodeDecodeErrorNotUtf8) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorNotUtf8
}

// One of the URLs in the QR code data could not be parsed.
type LoginQrCodeDecodeErrorUrlParse struct {
	message string
}

// One of the URLs in the QR code data could not be parsed.
func NewLoginQrCodeDecodeErrorUrlParse() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorUrlParse{}}
}

func (e LoginQrCodeDecodeErrorUrlParse) destroy() {
}

func (err LoginQrCodeDecodeErrorUrlParse) Error() string {
	return fmt.Sprintf("UrlParse: %s", err.message)
}

func (self LoginQrCodeDecodeErrorUrlParse) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorUrlParse
}

// The QR code data contains an invalid mode, we expect the login (0x03)
// mode or the reciprocate mode (0x04).
type LoginQrCodeDecodeErrorInvalidMode struct {
	message string
}

// The QR code data contains an invalid mode, we expect the login (0x03)
// mode or the reciprocate mode (0x04).
func NewLoginQrCodeDecodeErrorInvalidMode() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorInvalidMode{}}
}

func (e LoginQrCodeDecodeErrorInvalidMode) destroy() {
}

func (err LoginQrCodeDecodeErrorInvalidMode) Error() string {
	return fmt.Sprintf("InvalidMode: %s", err.message)
}

func (self LoginQrCodeDecodeErrorInvalidMode) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorInvalidMode
}

// The QR code data contains an unsupported version.
type LoginQrCodeDecodeErrorInvalidVersion struct {
	message string
}

// The QR code data contains an unsupported version.
func NewLoginQrCodeDecodeErrorInvalidVersion() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorInvalidVersion{}}
}

func (e LoginQrCodeDecodeErrorInvalidVersion) destroy() {
}

func (err LoginQrCodeDecodeErrorInvalidVersion) Error() string {
	return fmt.Sprintf("InvalidVersion: %s", err.message)
}

func (self LoginQrCodeDecodeErrorInvalidVersion) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorInvalidVersion
}

// The base64 encoded variant of the QR code data is not a valid base64
// string.
type LoginQrCodeDecodeErrorBase64 struct {
	message string
}

// The base64 encoded variant of the QR code data is not a valid base64
// string.
func NewLoginQrCodeDecodeErrorBase64() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorBase64{}}
}

func (e LoginQrCodeDecodeErrorBase64) destroy() {
}

func (err LoginQrCodeDecodeErrorBase64) Error() string {
	return fmt.Sprintf("Base64: %s", err.message)
}

func (self LoginQrCodeDecodeErrorBase64) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorBase64
}

// The QR code data doesn't contain the expected `MATRIX` prefix.
type LoginQrCodeDecodeErrorInvalidPrefix struct {
	message string
}

// The QR code data doesn't contain the expected `MATRIX` prefix.
func NewLoginQrCodeDecodeErrorInvalidPrefix() *LoginQrCodeDecodeError {
	return &LoginQrCodeDecodeError{err: &LoginQrCodeDecodeErrorInvalidPrefix{}}
}

func (e LoginQrCodeDecodeErrorInvalidPrefix) destroy() {
}

func (err LoginQrCodeDecodeErrorInvalidPrefix) Error() string {
	return fmt.Sprintf("InvalidPrefix: %s", err.message)
}

func (self LoginQrCodeDecodeErrorInvalidPrefix) Is(target error) bool {
	return target == ErrLoginQrCodeDecodeErrorInvalidPrefix
}

type FfiConverterLoginQrCodeDecodeError struct{}

var FfiConverterLoginQrCodeDecodeErrorINSTANCE = FfiConverterLoginQrCodeDecodeError{}

func (c FfiConverterLoginQrCodeDecodeError) Lift(eb RustBufferI) *LoginQrCodeDecodeError {
	return LiftFromRustBuffer[*LoginQrCodeDecodeError](c, eb)
}

func (c FfiConverterLoginQrCodeDecodeError) Lower(value *LoginQrCodeDecodeError) C.RustBuffer {
	return LowerIntoRustBuffer[*LoginQrCodeDecodeError](c, value)
}

func (c FfiConverterLoginQrCodeDecodeError) Read(reader io.Reader) *LoginQrCodeDecodeError {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorNotEnoughData{message}}
	case 2:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorNotUtf8{message}}
	case 3:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorUrlParse{message}}
	case 4:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorInvalidMode{message}}
	case 5:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorInvalidVersion{message}}
	case 6:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorBase64{message}}
	case 7:
		return &LoginQrCodeDecodeError{&LoginQrCodeDecodeErrorInvalidPrefix{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterLoginQrCodeDecodeError.Read()", errorID))
	}

}

func (c FfiConverterLoginQrCodeDecodeError) Write(writer io.Writer, value *LoginQrCodeDecodeError) {
	switch variantValue := value.err.(type) {
	case *LoginQrCodeDecodeErrorNotEnoughData:
		writeInt32(writer, 1)
	case *LoginQrCodeDecodeErrorNotUtf8:
		writeInt32(writer, 2)
	case *LoginQrCodeDecodeErrorUrlParse:
		writeInt32(writer, 3)
	case *LoginQrCodeDecodeErrorInvalidMode:
		writeInt32(writer, 4)
	case *LoginQrCodeDecodeErrorInvalidVersion:
		writeInt32(writer, 5)
	case *LoginQrCodeDecodeErrorBase64:
		writeInt32(writer, 6)
	case *LoginQrCodeDecodeErrorInvalidPrefix:
		writeInt32(writer, 7)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterLoginQrCodeDecodeError.Write", value))
	}
}

type FfiDestroyerLoginQrCodeDecodeError struct{}

func (_ FfiDestroyerLoginQrCodeDecodeError) Destroy(value *LoginQrCodeDecodeError) {
	switch variantValue := value.err.(type) {
	case LoginQrCodeDecodeErrorNotEnoughData:
		variantValue.destroy()
	case LoginQrCodeDecodeErrorNotUtf8:
		variantValue.destroy()
	case LoginQrCodeDecodeErrorUrlParse:
		variantValue.destroy()
	case LoginQrCodeDecodeErrorInvalidMode:
		variantValue.destroy()
	case LoginQrCodeDecodeErrorInvalidVersion:
		variantValue.destroy()
	case LoginQrCodeDecodeErrorBase64:
		variantValue.destroy()
	case LoginQrCodeDecodeErrorInvalidPrefix:
		variantValue.destroy()
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiDestroyerLoginQrCodeDecodeError.Destroy", value))
	}
}

// The result of a signature check.
type SignatureState uint

const (
	// The signature is missing.
	SignatureStateMissing SignatureState = 1
	// The signature is invalid.
	SignatureStateInvalid SignatureState = 2
	// The signature is valid but the device or user identity that created the
	// signature is not trusted.
	SignatureStateValidButNotTrusted SignatureState = 3
	// The signature is valid and the device or user identity that created the
	// signature is trusted.
	SignatureStateValidAndTrusted SignatureState = 4
)

type FfiConverterSignatureState struct{}

var FfiConverterSignatureStateINSTANCE = FfiConverterSignatureState{}

func (c FfiConverterSignatureState) Lift(rb RustBufferI) SignatureState {
	return LiftFromRustBuffer[SignatureState](c, rb)
}

func (c FfiConverterSignatureState) Lower(value SignatureState) C.RustBuffer {
	return LowerIntoRustBuffer[SignatureState](c, value)
}
func (FfiConverterSignatureState) Read(reader io.Reader) SignatureState {
	id := readInt32(reader)
	return SignatureState(id)
}

func (FfiConverterSignatureState) Write(writer io.Writer, value SignatureState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerSignatureState struct{}

func (_ FfiDestroyerSignatureState) Destroy(value SignatureState) {
}

// The trust level in the sender's device that is required to decrypt an
// event.
type TrustRequirement uint

const (
	// Decrypt events from everyone regardless of trust.
	TrustRequirementUntrusted TrustRequirement = 1
	// Only decrypt events from cross-signed devices or legacy sessions (Megolm
	// sessions created before we started collecting trust information).
	TrustRequirementCrossSignedOrLegacy TrustRequirement = 2
	// Only decrypt events from cross-signed devices.
	TrustRequirementCrossSigned TrustRequirement = 3
)

type FfiConverterTrustRequirement struct{}

var FfiConverterTrustRequirementINSTANCE = FfiConverterTrustRequirement{}

func (c FfiConverterTrustRequirement) Lift(rb RustBufferI) TrustRequirement {
	return LiftFromRustBuffer[TrustRequirement](c, rb)
}

func (c FfiConverterTrustRequirement) Lower(value TrustRequirement) C.RustBuffer {
	return LowerIntoRustBuffer[TrustRequirement](c, value)
}
func (FfiConverterTrustRequirement) Read(reader io.Reader) TrustRequirement {
	id := readInt32(reader)
	return TrustRequirement(id)
}

func (FfiConverterTrustRequirement) Write(writer io.Writer, value TrustRequirement) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTrustRequirement struct{}

func (_ FfiDestroyerTrustRequirement) Destroy(value TrustRequirement) {
}

// Our best guess at the reason why an event can't be decrypted.
type UtdCause uint

const (
	// We don't have an explanation for why this UTD happened - it is probably
	// a bug, or a network split between the two homeservers.
	//
	// For example:
	//
	// - the keys for this event are missing, but a key storage backup exists
	// and is working, so we should be able to find the keys in the backup.
	//
	// - the keys for this event are missing, and a key storage backup exists
	// on the server, but that backup is not working on this client even
	// though this device is verified.
	UtdCauseUnknown UtdCause = 1
	// We are missing the keys for this event, and the event was sent when we
	// were not a member of the room (or invited).
	UtdCauseSentBeforeWeJoined UtdCause = 2
	// The message was sent by a user identity we have not verified, but the
	// user was previously verified.
	UtdCauseVerificationViolation UtdCause = 3
	// The [`crate::TrustRequirement`] requires that the sending device be
	// signed by its owner, and it was not.
	UtdCauseUnsignedDevice UtdCause = 4
	// The [`crate::TrustRequirement`] requires that the sending device be
	// signed by its owner, and we were unable to securely find the device.
	//
	// This could be because the device has since been deleted, because we
	// haven't yet downloaded it from the server, or because the session
	// data was obtained from an insecure source (imported from a file,
	// obtained from a legacy (asymmetric) backup, unsafe key forward, etc.)
	UtdCauseUnknownDevice UtdCause = 5
	// We are missing the keys for this event, but it is a "device-historical"
	// message and there is no key storage backup on the server, presumably
	// because the user has turned it off.
	//
	// Device-historical means that the message was sent before the current
	// device existed (but the current user was probably a member of the room
	// at the time the message was sent). Not to
	// be confused with pre-join or pre-invite messages (see
	// [`UtdCause::SentBeforeWeJoined`] for that).
	//
	// Expected message to user: "History is not available on this device".
	UtdCauseHistoricalMessageAndBackupIsDisabled UtdCause = 6
	// The keys for this event are intentionally withheld.
	//
	// The sender has refused to share the key because our device does not meet
	// the sender's security requirements.
	UtdCauseWithheldForUnverifiedOrInsecureDevice UtdCause = 7
	// The keys for this event are missing, likely because the sender was
	// unable to share them (e.g., failure to establish an Olm 1:1
	// channel). Alternatively, the sender may have deliberately excluded
	// this device by cherry-picking and blocking it, in which case, no action
	// can be taken on our side.
	UtdCauseWithheldBySender UtdCause = 8
	// We are missing the keys for this event, but it is a "device-historical"
	// message, and even though a key storage backup does exist, we can't use
	// it because our device is unverified.
	//
	// Device-historical means that the message was sent before the current
	// device existed (but the current user was probably a member of the room
	// at the time the message was sent). Not to
	// be confused with pre-join or pre-invite messages (see
	// [`UtdCause::SentBeforeWeJoined`] for that).
	//
	// Expected message to user: "You need to verify this device".
	UtdCauseHistoricalMessageAndDeviceIsUnverified UtdCause = 9
)

type FfiConverterUtdCause struct{}

var FfiConverterUtdCauseINSTANCE = FfiConverterUtdCause{}

func (c FfiConverterUtdCause) Lift(rb RustBufferI) UtdCause {
	return LiftFromRustBuffer[UtdCause](c, rb)
}

func (c FfiConverterUtdCause) Lower(value UtdCause) C.RustBuffer {
	return LowerIntoRustBuffer[UtdCause](c, value)
}
func (FfiConverterUtdCause) Read(reader io.Reader) UtdCause {
	id := readInt32(reader)
	return UtdCause(id)
}

func (FfiConverterUtdCause) Write(writer io.Writer, value UtdCause) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerUtdCause struct{}

func (_ FfiDestroyerUtdCause) Destroy(value UtdCause) {
}
