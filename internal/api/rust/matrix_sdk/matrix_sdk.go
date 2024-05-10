package matrix_sdk

// #include <matrix_sdk.h>
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"unsafe"
)

type RustBuffer = C.RustBuffer

type RustBufferI interface {
	AsReader() *bytes.Reader
	Free()
	ToGoBytes() []byte
	Data() unsafe.Pointer
	Len() int
	Capacity() int
}

func RustBufferFromExternal(b RustBufferI) RustBuffer {
	return RustBuffer{
		capacity: C.int(b.Capacity()),
		len:      C.int(b.Len()),
		data:     (*C.uchar)(b.Data()),
	}
}

func (cb RustBuffer) Capacity() int {
	return int(cb.capacity)
}

func (cb RustBuffer) Len() int {
	return int(cb.len)
}

func (cb RustBuffer) Data() unsafe.Pointer {
	return unsafe.Pointer(cb.data)
}

func (cb RustBuffer) AsReader() *bytes.Reader {
	b := unsafe.Slice((*byte)(cb.data), C.int(cb.len))
	return bytes.NewReader(b)
}

func (cb RustBuffer) Free() {
	rustCall(func(status *C.RustCallStatus) bool {
		C.ffi_matrix_sdk_rustbuffer_free(cb, status)
		return false
	})
}

func (cb RustBuffer) ToGoBytes() []byte {
	return C.GoBytes(unsafe.Pointer(cb.data), C.int(cb.len))
}

func stringToRustBuffer(str string) RustBuffer {
	return bytesToRustBuffer([]byte(str))
}

func bytesToRustBuffer(b []byte) RustBuffer {
	if len(b) == 0 {
		return RustBuffer{}
	}
	// We can pass the pointer along here, as it is pinned
	// for the duration of this call
	foreign := C.ForeignBytes{
		len:  C.int(len(b)),
		data: (*C.uchar)(unsafe.Pointer(&b[0])),
	}

	return rustCall(func(status *C.RustCallStatus) RustBuffer {
		return C.ffi_matrix_sdk_rustbuffer_from_bytes(foreign, status)
	})
}

type BufLifter[GoType any] interface {
	Lift(value RustBufferI) GoType
}

type BufLowerer[GoType any] interface {
	Lower(value GoType) RustBuffer
}

type FfiConverter[GoType any, FfiType any] interface {
	Lift(value FfiType) GoType
	Lower(value GoType) FfiType
}

type BufReader[GoType any] interface {
	Read(reader io.Reader) GoType
}

type BufWriter[GoType any] interface {
	Write(writer io.Writer, value GoType)
}

type FfiRustBufConverter[GoType any, FfiType any] interface {
	FfiConverter[GoType, FfiType]
	BufReader[GoType]
}

func LowerIntoRustBuffer[GoType any](bufWriter BufWriter[GoType], value GoType) RustBuffer {
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

func rustCallWithError[U any](converter BufLifter[error], callback func(*C.RustCallStatus) U) (U, error) {
	var status C.RustCallStatus
	returnValue := callback(&status)
	err := checkCallStatus(converter, status)

	return returnValue, err
}

func checkCallStatus(converter BufLifter[error], status C.RustCallStatus) error {
	switch status.code {
	case 0:
		return nil
	case 1:
		return converter.Lift(status.errorBuf)
	case 2:
		// when the rust code sees a panic, it tries to construct a rustbuffer
		// with the message.  but if that code panics, then it just sends back
		// an empty buffer.
		if status.errorBuf.len > 0 {
			panic(fmt.Errorf("%s", FfiConverterStringINSTANCE.Lift(status.errorBuf)))
		} else {
			panic(fmt.Errorf("Rust panicked while handling Rust panic"))
		}
	default:
		return fmt.Errorf("unknown status code: %d", status.code)
	}
}

func checkCallStatusUnknown(status C.RustCallStatus) error {
	switch status.code {
	case 0:
		return nil
	case 1:
		panic(fmt.Errorf("function not returning an error returned an error"))
	case 2:
		// when the rust code sees a panic, it tries to construct a rustbuffer
		// with the message.  but if that code panics, then it just sends back
		// an empty buffer.
		if status.errorBuf.len > 0 {
			panic(fmt.Errorf("%s", FfiConverterStringINSTANCE.Lift(status.errorBuf)))
		} else {
			panic(fmt.Errorf("Rust panicked while handling Rust panic"))
		}
	default:
		return fmt.Errorf("unknown status code: %d", status.code)
	}
}

func rustCall[U any](callback func(*C.RustCallStatus) U) U {
	returnValue, err := rustCallWithError(nil, callback)
	if err != nil {
		panic(err)
	}
	return returnValue
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
	bindingsContractVersion := 24
	// Get the scaffolding contract version by calling the into the dylib
	scaffoldingContractVersion := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.ffi_matrix_sdk_uniffi_contract_version(uniffiStatus)
	})
	if bindingsContractVersion != int(scaffoldingContractVersion) {
		// If this happens try cleaning and rebuilding your project
		panic("matrix_sdk: UniFFI contract version mismatch")
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
	if err != nil {
		panic(err)
	}
	if read_length != int(length) {
		panic(fmt.Errorf("bad read length when reading string, expected %d, read %d", length, read_length))
	}
	return string(buffer)
}

func (FfiConverterString) Lower(value string) RustBuffer {
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

type RoomPowerLevelChanges struct {
	Ban           *int64
	Invite        *int64
	Kick          *int64
	Redact        *int64
	EventsDefault *int64
	StateDefault  *int64
	UsersDefault  *int64
	RoomName      *int64
	RoomAvatar    *int64
	RoomTopic     *int64
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

type FfiConverterTypeRoomPowerLevelChanges struct{}

var FfiConverterTypeRoomPowerLevelChangesINSTANCE = FfiConverterTypeRoomPowerLevelChanges{}

func (c FfiConverterTypeRoomPowerLevelChanges) Lift(rb RustBufferI) RoomPowerLevelChanges {
	return LiftFromRustBuffer[RoomPowerLevelChanges](c, rb)
}

func (c FfiConverterTypeRoomPowerLevelChanges) Read(reader io.Reader) RoomPowerLevelChanges {
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

func (c FfiConverterTypeRoomPowerLevelChanges) Lower(value RoomPowerLevelChanges) RustBuffer {
	return LowerIntoRustBuffer[RoomPowerLevelChanges](c, value)
}

func (c FfiConverterTypeRoomPowerLevelChanges) Write(writer io.Writer, value RoomPowerLevelChanges) {
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

type FfiDestroyerTypeRoomPowerLevelChanges struct{}

func (_ FfiDestroyerTypeRoomPowerLevelChanges) Destroy(value RoomPowerLevelChanges) {
	value.Destroy()
}

type BackupDownloadStrategy uint

const (
	BackupDownloadStrategyOneShot                BackupDownloadStrategy = 1
	BackupDownloadStrategyAfterDecryptionFailure BackupDownloadStrategy = 2
	BackupDownloadStrategyManual                 BackupDownloadStrategy = 3
)

type FfiConverterTypeBackupDownloadStrategy struct{}

var FfiConverterTypeBackupDownloadStrategyINSTANCE = FfiConverterTypeBackupDownloadStrategy{}

func (c FfiConverterTypeBackupDownloadStrategy) Lift(rb RustBufferI) BackupDownloadStrategy {
	return LiftFromRustBuffer[BackupDownloadStrategy](c, rb)
}

func (c FfiConverterTypeBackupDownloadStrategy) Lower(value BackupDownloadStrategy) RustBuffer {
	return LowerIntoRustBuffer[BackupDownloadStrategy](c, value)
}
func (FfiConverterTypeBackupDownloadStrategy) Read(reader io.Reader) BackupDownloadStrategy {
	id := readInt32(reader)
	return BackupDownloadStrategy(id)
}

func (FfiConverterTypeBackupDownloadStrategy) Write(writer io.Writer, value BackupDownloadStrategy) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeBackupDownloadStrategy struct{}

func (_ FfiDestroyerTypeBackupDownloadStrategy) Destroy(value BackupDownloadStrategy) {
}

type RoomMemberRole uint

const (
	RoomMemberRoleAdministrator RoomMemberRole = 1
	RoomMemberRoleModerator     RoomMemberRole = 2
	RoomMemberRoleUser          RoomMemberRole = 3
)

type FfiConverterTypeRoomMemberRole struct{}

var FfiConverterTypeRoomMemberRoleINSTANCE = FfiConverterTypeRoomMemberRole{}

func (c FfiConverterTypeRoomMemberRole) Lift(rb RustBufferI) RoomMemberRole {
	return LiftFromRustBuffer[RoomMemberRole](c, rb)
}

func (c FfiConverterTypeRoomMemberRole) Lower(value RoomMemberRole) RustBuffer {
	return LowerIntoRustBuffer[RoomMemberRole](c, value)
}
func (FfiConverterTypeRoomMemberRole) Read(reader io.Reader) RoomMemberRole {
	id := readInt32(reader)
	return RoomMemberRole(id)
}

func (FfiConverterTypeRoomMemberRole) Write(writer io.Writer, value RoomMemberRole) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomMemberRole struct{}

func (_ FfiDestroyerTypeRoomMemberRole) Destroy(value RoomMemberRole) {
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

func (c FfiConverterOptionalInt64) Lower(value *int64) RustBuffer {
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
