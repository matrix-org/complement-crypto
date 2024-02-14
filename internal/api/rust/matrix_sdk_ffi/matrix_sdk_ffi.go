package matrix_sdk_ffi

// #include <matrix_sdk_ffi.h>
// #cgo LDFLAGS: -lmatrix_sdk_ffi
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"runtime"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/matrix-org/complement-crypto/internal/api/rust/matrix_sdk"
	"github.com/matrix-org/complement-crypto/internal/api/rust/matrix_sdk_ui"
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
		C.ffi_matrix_sdk_ffi_rustbuffer_free(cb, status)
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
		return C.ffi_matrix_sdk_ffi_rustbuffer_from_bytes(foreign, status)
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

	(&FfiConverterCallbackInterfaceBackPaginationStatusListener{}).register()
	(&FfiConverterCallbackInterfaceBackupStateListener{}).register()
	(&FfiConverterCallbackInterfaceBackupSteadyStateListener{}).register()
	(&FfiConverterCallbackInterfaceClientDelegate{}).register()
	(&FfiConverterCallbackInterfaceClientSessionDelegate{}).register()
	(&FfiConverterCallbackInterfaceEnableRecoveryProgressListener{}).register()
	(&FfiConverterCallbackInterfaceNotificationSettingsDelegate{}).register()
	(&FfiConverterCallbackInterfaceProgressWatcher{}).register()
	(&FfiConverterCallbackInterfaceRecoveryStateListener{}).register()
	(&FfiConverterCallbackInterfaceRoomInfoListener{}).register()
	(&FfiConverterCallbackInterfaceRoomListEntriesListener{}).register()
	(&FfiConverterCallbackInterfaceRoomListLoadingStateListener{}).register()
	(&FfiConverterCallbackInterfaceRoomListServiceStateListener{}).register()
	(&FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListener{}).register()
	(&FfiConverterCallbackInterfaceSessionVerificationControllerDelegate{}).register()
	(&FfiConverterCallbackInterfaceSyncServiceStateObserver{}).register()
	(&FfiConverterCallbackInterfaceTimelineListener{}).register()
	(&FfiConverterCallbackInterfaceTypingNotificationsListener{}).register()
	(&FfiConverterCallbackInterfaceWidgetCapabilitiesProvider{}).register()
	uniffiInitContinuationCallback()
	uniffiCheckChecksums()
}

func uniffiCheckChecksums() {
	// Get the bindings contract version from our ComponentInterface
	bindingsContractVersion := 24
	// Get the scaffolding contract version by calling the into the dylib
	scaffoldingContractVersion := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.ffi_matrix_sdk_ffi_uniffi_contract_version(uniffiStatus)
	})
	if bindingsContractVersion != int(scaffoldingContractVersion) {
		// If this happens try cleaning and rebuilding your project
		panic("matrix_sdk_ffi: UniFFI contract version mismatch")
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_gen_transaction_id(uniffiStatus)
		})
		if checksum != 65533 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_gen_transaction_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_generate_webview_url(uniffiStatus)
		})
		if checksum != 16581 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_generate_webview_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_get_element_call_required_permissions(uniffiStatus)
		})
		if checksum != 51289 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_get_element_call_required_permissions: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_log_event(uniffiStatus)
		})
		if checksum != 58164 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_log_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_make_widget_driver(uniffiStatus)
		})
		if checksum != 16217 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_make_widget_driver: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_media_source_from_url(uniffiStatus)
		})
		if checksum != 28929 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_media_source_from_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_html(uniffiStatus)
		})
		if checksum != 48173 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_html: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_html_as_emote(uniffiStatus)
		})
		if checksum != 30627 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_html_as_emote: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_markdown(uniffiStatus)
		})
		if checksum != 5412 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_markdown: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_markdown_as_emote(uniffiStatus)
		})
		if checksum != 16575 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_message_event_content_from_markdown_as_emote: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_message_event_content_new(uniffiStatus)
		})
		if checksum != 60536 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_message_event_content_new: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_new_virtual_element_call_widget(uniffiStatus)
		})
		if checksum != 13275 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_new_virtual_element_call_widget: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_sdk_git_sha(uniffiStatus)
		})
		if checksum != 11183 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_sdk_git_sha: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_setup_otlp_tracing(uniffiStatus)
		})
		if checksum != 57774 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_setup_otlp_tracing: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_func_setup_tracing(uniffiStatus)
		})
		if checksum != 48899 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_func_setup_tracing: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_mediasource_to_json(uniffiStatus)
		})
		if checksum != 2998 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_mediasource_to_json: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_mediasource_url(uniffiStatus)
		})
		if checksum != 34026 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_mediasource_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommessageeventcontentwithoutrelation_with_mentions(uniffiStatus)
		})
		if checksum != 48900 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommessageeventcontentwithoutrelation_with_mentions: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_configure_homeserver(uniffiStatus)
		})
		if checksum != 20936 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_configure_homeserver: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_homeserver_details(uniffiStatus)
		})
		if checksum != 30828 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_homeserver_details: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_login(uniffiStatus)
		})
		if checksum != 4340 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_login: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_login_with_oidc_callback(uniffiStatus)
		})
		if checksum != 25443 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_login_with_oidc_callback: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_url_for_oidc_login(uniffiStatus)
		})
		if checksum != 6390 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_authenticationservice_url_for_oidc_login: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_account_data(uniffiStatus)
		})
		if checksum != 37263 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_account_data: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_account_url(uniffiStatus)
		})
		if checksum != 57664 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_account_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_avatar_url(uniffiStatus)
		})
		if checksum != 13474 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_avatar_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_cached_avatar_url(uniffiStatus)
		})
		if checksum != 47976 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_cached_avatar_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_create_room(uniffiStatus)
		})
		if checksum != 9095 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_create_room: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_device_id(uniffiStatus)
		})
		if checksum != 30759 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_device_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_display_name(uniffiStatus)
		})
		if checksum != 57766 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_display_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_encryption(uniffiStatus)
		})
		if checksum != 55944 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_encryption: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_dm_room(uniffiStatus)
		})
		if checksum != 2581 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_dm_room: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_media_content(uniffiStatus)
		})
		if checksum != 28329 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_media_content: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_media_file(uniffiStatus)
		})
		if checksum != 22652 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_media_file: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_media_thumbnail(uniffiStatus)
		})
		if checksum != 8016 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_media_thumbnail: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_notification_settings(uniffiStatus)
		})
		if checksum != 43752 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_notification_settings: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_profile(uniffiStatus)
		})
		if checksum != 11465 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_profile: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_get_session_verification_controller(uniffiStatus)
		})
		if checksum != 25701 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_get_session_verification_controller: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_homeserver(uniffiStatus)
		})
		if checksum != 509 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_homeserver: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_ignore_user(uniffiStatus)
		})
		if checksum != 53606 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_ignore_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_login(uniffiStatus)
		})
		if checksum != 62785 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_login: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_logout(uniffiStatus)
		})
		if checksum != 16841 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_logout: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_notification_client(uniffiStatus)
		})
		if checksum != 16860 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_notification_client: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_remove_avatar(uniffiStatus)
		})
		if checksum != 41701 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_remove_avatar: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_restore_session(uniffiStatus)
		})
		if checksum != 19558 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_restore_session: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_rooms(uniffiStatus)
		})
		if checksum != 61954 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_rooms: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_search_users(uniffiStatus)
		})
		if checksum != 1362 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_search_users: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_session(uniffiStatus)
		})
		if checksum != 56470 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_session: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_set_account_data(uniffiStatus)
		})
		if checksum != 32949 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_set_account_data: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_set_delegate(uniffiStatus)
		})
		if checksum != 29180 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_set_delegate: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_set_display_name(uniffiStatus)
		})
		if checksum != 45786 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_set_display_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_set_pusher(uniffiStatus)
		})
		if checksum != 9540 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_set_pusher: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_sync_service(uniffiStatus)
		})
		if checksum != 55738 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_sync_service: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_unignore_user(uniffiStatus)
		})
		if checksum != 6043 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_unignore_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_upload_avatar(uniffiStatus)
		})
		if checksum != 65133 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_upload_avatar: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_upload_media(uniffiStatus)
		})
		if checksum != 29165 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_upload_media: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_client_user_id(uniffiStatus)
		})
		if checksum != 55803 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_client_user_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_base_path(uniffiStatus)
		})
		if checksum != 13781 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_base_path: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_build(uniffiStatus)
		})
		if checksum != 56797 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_build: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_disable_automatic_token_refresh(uniffiStatus)
		})
		if checksum != 50220 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_disable_automatic_token_refresh: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_disable_ssl_verification(uniffiStatus)
		})
		if checksum != 1510 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_disable_ssl_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_enable_cross_process_refresh_lock(uniffiStatus)
		})
		if checksum != 39606 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_enable_cross_process_refresh_lock: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_homeserver_url(uniffiStatus)
		})
		if checksum != 43790 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_homeserver_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_passphrase(uniffiStatus)
		})
		if checksum != 25291 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_passphrase: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_proxy(uniffiStatus)
		})
		if checksum != 61852 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_proxy: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_server_name(uniffiStatus)
		})
		if checksum != 46252 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_server_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_server_versions(uniffiStatus)
		})
		if checksum != 64538 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_server_versions: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_set_session_delegate(uniffiStatus)
		})
		if checksum != 7269 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_set_session_delegate: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_sliding_sync_proxy(uniffiStatus)
		})
		if checksum != 37450 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_sliding_sync_proxy: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_user_agent(uniffiStatus)
		})
		if checksum != 42913 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_user_agent: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_username(uniffiStatus)
		})
		if checksum != 64379 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientbuilder_username: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_backup_exists_on_server(uniffiStatus)
		})
		if checksum != 17130 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_backup_exists_on_server: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_backup_state(uniffiStatus)
		})
		if checksum != 13611 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_backup_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_backup_state_listener(uniffiStatus)
		})
		if checksum != 29 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_backup_state_listener: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_disable_recovery(uniffiStatus)
		})
		if checksum != 38729 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_disable_recovery: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_enable_backups(uniffiStatus)
		})
		if checksum != 30690 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_enable_backups: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_enable_recovery(uniffiStatus)
		})
		if checksum != 60849 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_enable_recovery: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_is_last_device(uniffiStatus)
		})
		if checksum != 34446 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_is_last_device: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_recover(uniffiStatus)
		})
		if checksum != 34668 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_recover: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_recover_and_reset(uniffiStatus)
		})
		if checksum != 52410 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_recover_and_reset: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_recovery_state(uniffiStatus)
		})
		if checksum != 7187 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_recovery_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_recovery_state_listener(uniffiStatus)
		})
		if checksum != 11439 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_recovery_state_listener: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_reset_recovery_key(uniffiStatus)
		})
		if checksum != 40510 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_reset_recovery_key: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_encryption_wait_for_backup_upload_steady_state(uniffiStatus)
		})
		if checksum != 37083 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_encryption_wait_for_backup_upload_steady_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_can_be_replied_to(uniffiStatus)
		})
		if checksum != 42286 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_can_be_replied_to: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_content(uniffiStatus)
		})
		if checksum != 1802 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_content: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_debug_info(uniffiStatus)
		})
		if checksum != 45087 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_debug_info: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_event_id(uniffiStatus)
		})
		if checksum != 57306 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_event_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_editable(uniffiStatus)
		})
		if checksum != 593 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_editable: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_local(uniffiStatus)
		})
		if checksum != 47845 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_local: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_own(uniffiStatus)
		})
		if checksum != 18359 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_own: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_remote(uniffiStatus)
		})
		if checksum != 17688 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_is_remote: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_local_send_state(uniffiStatus)
		})
		if checksum != 22720 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_local_send_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_origin(uniffiStatus)
		})
		if checksum != 512 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_origin: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_reactions(uniffiStatus)
		})
		if checksum != 64143 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_reactions: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_read_receipts(uniffiStatus)
		})
		if checksum != 40784 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_read_receipts: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_sender(uniffiStatus)
		})
		if checksum != 46892 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_sender: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_sender_profile(uniffiStatus)
		})
		if checksum != 42856 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_sender_profile: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_timestamp(uniffiStatus)
		})
		if checksum != 481 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_timestamp: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_transaction_id(uniffiStatus)
		})
		if checksum != 36352 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_eventtimelineitem_transaction_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_homeserverlogindetails_supports_oidc_login(uniffiStatus)
		})
		if checksum != 51854 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_homeserverlogindetails_supports_oidc_login: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_homeserverlogindetails_supports_password_login(uniffiStatus)
		})
		if checksum != 6028 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_homeserverlogindetails_supports_password_login: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_homeserverlogindetails_url(uniffiStatus)
		})
		if checksum != 40398 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_homeserverlogindetails_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_mediafilehandle_path(uniffiStatus)
		})
		if checksum != 2500 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_mediafilehandle_path: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_mediafilehandle_persist(uniffiStatus)
		})
		if checksum != 4346 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_mediafilehandle_persist: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_message_body(uniffiStatus)
		})
		if checksum != 2560 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_message_body: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_message_in_reply_to(uniffiStatus)
		})
		if checksum != 1793 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_message_in_reply_to: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_message_is_edited(uniffiStatus)
		})
		if checksum != 3402 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_message_is_edited: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_message_is_threaded(uniffiStatus)
		})
		if checksum != 29945 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_message_is_threaded: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_message_msgtype(uniffiStatus)
		})
		if checksum != 35166 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_message_msgtype: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationclient_get_notification(uniffiStatus)
		})
		if checksum != 9907 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationclient_get_notification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationclientbuilder_filter_by_push_rules(uniffiStatus)
		})
		if checksum != 10529 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationclientbuilder_filter_by_push_rules: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationclientbuilder_finish(uniffiStatus)
		})
		if checksum != 12382 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationclientbuilder_finish: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_can_push_encrypted_event_to_device(uniffiStatus)
		})
		if checksum != 5028 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_can_push_encrypted_event_to_device: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_contains_keywords_rules(uniffiStatus)
		})
		if checksum != 42972 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_contains_keywords_rules: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_default_room_notification_mode(uniffiStatus)
		})
		if checksum != 7288 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_default_room_notification_mode: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_room_notification_settings(uniffiStatus)
		})
		if checksum != 654 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_room_notification_settings: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_rooms_with_user_defined_rules(uniffiStatus)
		})
		if checksum != 687 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_rooms_with_user_defined_rules: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_user_defined_room_notification_mode(uniffiStatus)
		})
		if checksum != 40224 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_get_user_defined_room_notification_mode: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_call_enabled(uniffiStatus)
		})
		if checksum != 38110 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_call_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_invite_for_me_enabled(uniffiStatus)
		})
		if checksum != 50408 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_invite_for_me_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_room_mention_enabled(uniffiStatus)
		})
		if checksum != 36336 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_room_mention_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_user_mention_enabled(uniffiStatus)
		})
		if checksum != 9844 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_is_user_mention_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_restore_default_room_notification_mode(uniffiStatus)
		})
		if checksum != 43578 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_restore_default_room_notification_mode: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_call_enabled(uniffiStatus)
		})
		if checksum != 61774 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_call_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_default_room_notification_mode(uniffiStatus)
		})
		if checksum != 64886 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_default_room_notification_mode: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_delegate(uniffiStatus)
		})
		if checksum != 22622 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_delegate: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_invite_for_me_enabled(uniffiStatus)
		})
		if checksum != 7240 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_invite_for_me_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_room_mention_enabled(uniffiStatus)
		})
		if checksum != 50730 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_room_mention_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_room_notification_mode(uniffiStatus)
		})
		if checksum != 21294 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_room_notification_mode: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_user_mention_enabled(uniffiStatus)
		})
		if checksum != 63345 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_set_user_mention_enabled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_unmute_room(uniffiStatus)
		})
		if checksum != 33146 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettings_unmute_room: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_oidcauthenticationdata_login_url(uniffiStatus)
		})
		if checksum != 2455 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_oidcauthenticationdata_login_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_active_members_count(uniffiStatus)
		})
		if checksum != 62367 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_active_members_count: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_active_room_call_participants(uniffiStatus)
		})
		if checksum != 5256 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_active_room_call_participants: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_alternative_aliases(uniffiStatus)
		})
		if checksum != 25219 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_alternative_aliases: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_apply_power_level_changes(uniffiStatus)
		})
		if checksum != 27718 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_apply_power_level_changes: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_avatar_url(uniffiStatus)
		})
		if checksum != 38267 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_avatar_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_ban_user(uniffiStatus)
		})
		if checksum != 15134 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_ban_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_build_power_level_changes_from_current(uniffiStatus)
		})
		if checksum != 43034 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_build_power_level_changes_from_current: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_ban(uniffiStatus)
		})
		if checksum != 47371 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_ban: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_invite(uniffiStatus)
		})
		if checksum != 62419 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_invite: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_kick(uniffiStatus)
		})
		if checksum != 47687 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_kick: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_redact_other(uniffiStatus)
		})
		if checksum != 15585 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_redact_other: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_redact_own(uniffiStatus)
		})
		if checksum != 22471 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_redact_own: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_send_message(uniffiStatus)
		})
		if checksum != 28210 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_send_message: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_send_state(uniffiStatus)
		})
		if checksum != 54763 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_send_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_can_user_trigger_room_notification(uniffiStatus)
		})
		if checksum != 8288 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_can_user_trigger_room_notification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_canonical_alias(uniffiStatus)
		})
		if checksum != 15084 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_canonical_alias: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_display_name(uniffiStatus)
		})
		if checksum != 38216 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_display_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_has_active_room_call(uniffiStatus)
		})
		if checksum != 59850 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_has_active_room_call: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_id(uniffiStatus)
		})
		if checksum != 27132 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_ignore_user(uniffiStatus)
		})
		if checksum != 9941 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_ignore_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_invite_user_by_id(uniffiStatus)
		})
		if checksum != 12569 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_invite_user_by_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_invited_members_count(uniffiStatus)
		})
		if checksum != 31452 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_invited_members_count: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_inviter(uniffiStatus)
		})
		if checksum != 8327 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_inviter: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_is_direct(uniffiStatus)
		})
		if checksum != 46881 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_is_direct: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_is_encrypted(uniffiStatus)
		})
		if checksum != 29418 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_is_encrypted: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_is_public(uniffiStatus)
		})
		if checksum != 22937 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_is_public: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_is_space(uniffiStatus)
		})
		if checksum != 8495 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_is_space: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_is_tombstoned(uniffiStatus)
		})
		if checksum != 55887 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_is_tombstoned: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_join(uniffiStatus)
		})
		if checksum != 4883 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_join: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_joined_members_count(uniffiStatus)
		})
		if checksum != 44345 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_joined_members_count: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_kick_user(uniffiStatus)
		})
		if checksum != 50409 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_kick_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_leave(uniffiStatus)
		})
		if checksum != 11928 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_leave: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_mark_as_read(uniffiStatus)
		})
		if checksum != 43113 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_mark_as_read: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_member(uniffiStatus)
		})
		if checksum != 4975 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_member: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_member_avatar_url(uniffiStatus)
		})
		if checksum != 5937 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_member_avatar_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_member_display_name(uniffiStatus)
		})
		if checksum != 4559 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_member_display_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_members(uniffiStatus)
		})
		if checksum != 6390 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_members: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_members_no_sync(uniffiStatus)
		})
		if checksum != 17434 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_members_no_sync: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_membership(uniffiStatus)
		})
		if checksum != 17678 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_membership: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_name(uniffiStatus)
		})
		if checksum != 58791 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_own_user_id(uniffiStatus)
		})
		if checksum != 26241 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_own_user_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_redact(uniffiStatus)
		})
		if checksum != 12809 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_redact: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_remove_avatar(uniffiStatus)
		})
		if checksum != 24698 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_remove_avatar: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_report_content(uniffiStatus)
		})
		if checksum != 58629 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_report_content: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_room_info(uniffiStatus)
		})
		if checksum != 45186 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_room_info: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_set_is_favourite(uniffiStatus)
		})
		if checksum != 47289 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_set_is_favourite: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_set_is_low_priority(uniffiStatus)
		})
		if checksum != 9060 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_set_is_low_priority: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_set_name(uniffiStatus)
		})
		if checksum != 56429 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_set_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_set_topic(uniffiStatus)
		})
		if checksum != 55348 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_set_topic: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_set_unread_flag(uniffiStatus)
		})
		if checksum != 45660 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_set_unread_flag: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_subscribe_to_room_info_updates(uniffiStatus)
		})
		if checksum != 43609 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_subscribe_to_room_info_updates: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_subscribe_to_typing_notifications(uniffiStatus)
		})
		if checksum != 63693 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_subscribe_to_typing_notifications: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_timeline(uniffiStatus)
		})
		if checksum != 12790 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_timeline: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_topic(uniffiStatus)
		})
		if checksum != 23413 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_topic: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_typing_notice(uniffiStatus)
		})
		if checksum != 46496 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_typing_notice: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_unban_user(uniffiStatus)
		})
		if checksum != 46653 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_unban_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_update_power_level_for_user(uniffiStatus)
		})
		if checksum != 15011 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_update_power_level_for_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_room_upload_avatar(uniffiStatus)
		})
		if checksum != 46437 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_room_upload_avatar: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlist_entries(uniffiStatus)
		})
		if checksum != 27911 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlist_entries: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlist_entries_with_dynamic_adapters(uniffiStatus)
		})
		if checksum != 30316 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlist_entries_with_dynamic_adapters: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlist_loading_state(uniffiStatus)
		})
		if checksum != 54823 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlist_loading_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlist_room(uniffiStatus)
		})
		if checksum != 60000 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlist_room: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistdynamicentriescontroller_add_one_page(uniffiStatus)
		})
		if checksum != 1980 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistdynamicentriescontroller_add_one_page: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistdynamicentriescontroller_reset_to_one_page(uniffiStatus)
		})
		if checksum != 48285 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistdynamicentriescontroller_reset_to_one_page: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistdynamicentriescontroller_set_filter(uniffiStatus)
		})
		if checksum != 20071 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistdynamicentriescontroller_set_filter: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_avatar_url(uniffiStatus)
		})
		if checksum != 23609 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_avatar_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_canonical_alias(uniffiStatus)
		})
		if checksum != 56187 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_canonical_alias: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_full_room(uniffiStatus)
		})
		if checksum != 27231 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_full_room: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_id(uniffiStatus)
		})
		if checksum != 35737 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_init_timeline(uniffiStatus)
		})
		if checksum != 50995 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_init_timeline: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_is_direct(uniffiStatus)
		})
		if checksum != 24829 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_is_direct: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_is_timeline_initialized(uniffiStatus)
		})
		if checksum != 33209 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_is_timeline_initialized: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_latest_event(uniffiStatus)
		})
		if checksum != 44019 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_latest_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_name(uniffiStatus)
		})
		if checksum != 5949 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_room_info(uniffiStatus)
		})
		if checksum != 17731 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_room_info: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_subscribe(uniffiStatus)
		})
		if checksum != 16638 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_subscribe: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_unsubscribe(uniffiStatus)
		})
		if checksum != 14844 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistitem_unsubscribe: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_all_rooms(uniffiStatus)
		})
		if checksum != 37160 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_all_rooms: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_apply_input(uniffiStatus)
		})
		if checksum != 46775 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_apply_input: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_invites(uniffiStatus)
		})
		if checksum != 56087 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_invites: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_room(uniffiStatus)
		})
		if checksum != 48446 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_room: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_state(uniffiStatus)
		})
		if checksum != 7038 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_sync_indicator(uniffiStatus)
		})
		if checksum != 5536 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservice_sync_indicator: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_avatar_url(uniffiStatus)
		})
		if checksum != 9148 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_avatar_url: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_ban(uniffiStatus)
		})
		if checksum != 19267 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_ban: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_invite(uniffiStatus)
		})
		if checksum != 36172 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_invite: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_kick(uniffiStatus)
		})
		if checksum != 31109 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_kick: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_redact_other(uniffiStatus)
		})
		if checksum != 6135 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_redact_other: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_redact_own(uniffiStatus)
		})
		if checksum != 7910 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_redact_own: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_send_message(uniffiStatus)
		})
		if checksum != 14989 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_send_message: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_send_state(uniffiStatus)
		})
		if checksum != 43889 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_send_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_can_trigger_room_notification(uniffiStatus)
		})
		if checksum != 62393 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_can_trigger_room_notification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_display_name(uniffiStatus)
		})
		if checksum != 28367 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_display_name: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_ignore(uniffiStatus)
		})
		if checksum != 32455 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_ignore: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_is_account_user(uniffiStatus)
		})
		if checksum != 37767 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_is_account_user: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_is_ignored(uniffiStatus)
		})
		if checksum != 46154 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_is_ignored: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_is_name_ambiguous(uniffiStatus)
		})
		if checksum != 65246 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_is_name_ambiguous: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_membership(uniffiStatus)
		})
		if checksum != 34335 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_membership: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_normalized_power_level(uniffiStatus)
		})
		if checksum != 49076 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_normalized_power_level: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_power_level(uniffiStatus)
		})
		if checksum != 17042 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_power_level: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_suggested_role_for_power_level(uniffiStatus)
		})
		if checksum != 53355 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_suggested_role_for_power_level: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_unignore(uniffiStatus)
		})
		if checksum != 56817 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_unignore: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommember_user_id(uniffiStatus)
		})
		if checksum != 19498 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommember_user_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommembersiterator_len(uniffiStatus)
		})
		if checksum != 32977 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommembersiterator_len: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roommembersiterator_next_chunk(uniffiStatus)
		})
		if checksum != 35645 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roommembersiterator_next_chunk: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sendattachmentjoinhandle_cancel(uniffiStatus)
		})
		if checksum != 58929 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sendattachmentjoinhandle_cancel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sendattachmentjoinhandle_join(uniffiStatus)
		})
		if checksum != 25237 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sendattachmentjoinhandle_join: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_approve_verification(uniffiStatus)
		})
		if checksum != 468 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_approve_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_cancel_verification(uniffiStatus)
		})
		if checksum != 63679 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_cancel_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_decline_verification(uniffiStatus)
		})
		if checksum != 50627 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_decline_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_is_verified(uniffiStatus)
		})
		if checksum != 3866 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_is_verified: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_request_verification(uniffiStatus)
		})
		if checksum != 51679 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_request_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_set_delegate(uniffiStatus)
		})
		if checksum != 24735 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_set_delegate: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_start_sas_verification(uniffiStatus)
		})
		if checksum != 3726 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontroller_start_sas_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationemoji_description(uniffiStatus)
		})
		if checksum != 55458 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationemoji_description: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationemoji_symbol(uniffiStatus)
		})
		if checksum != 1848 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationemoji_symbol: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_span_enter(uniffiStatus)
		})
		if checksum != 56663 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_span_enter: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_span_exit(uniffiStatus)
		})
		if checksum != 6123 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_span_exit: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_span_is_none(uniffiStatus)
		})
		if checksum != 23839 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_span_is_none: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservice_room_list_service(uniffiStatus)
		})
		if checksum != 18295 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservice_room_list_service: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservice_start(uniffiStatus)
		})
		if checksum != 4435 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservice_start: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservice_state(uniffiStatus)
		})
		if checksum != 15048 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservice_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservice_stop(uniffiStatus)
		})
		if checksum != 39770 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservice_stop: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservicebuilder_finish(uniffiStatus)
		})
		if checksum != 61604 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservicebuilder_finish: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservicebuilder_with_cross_process_lock(uniffiStatus)
		})
		if checksum != 29139 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservicebuilder_with_cross_process_lock: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_taskhandle_cancel(uniffiStatus)
		})
		if checksum != 59047 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_taskhandle_cancel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_taskhandle_is_finished(uniffiStatus)
		})
		if checksum != 3905 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_taskhandle_is_finished: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_add_listener(uniffiStatus)
		})
		if checksum != 48101 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_add_listener: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_cancel_send(uniffiStatus)
		})
		if checksum != 51132 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_cancel_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_create_poll(uniffiStatus)
		})
		if checksum != 38825 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_create_poll: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_edit(uniffiStatus)
		})
		if checksum != 58303 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_edit: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_edit_poll(uniffiStatus)
		})
		if checksum != 54368 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_edit_poll: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_end_poll(uniffiStatus)
		})
		if checksum != 53347 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_end_poll: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_fetch_details_for_event(uniffiStatus)
		})
		if checksum != 20642 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_fetch_details_for_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_fetch_members(uniffiStatus)
		})
		if checksum != 11365 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_fetch_members: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_get_event_timeline_item_by_event_id(uniffiStatus)
		})
		if checksum != 62347 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_get_event_timeline_item_by_event_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_get_timeline_event_content_by_event_id(uniffiStatus)
		})
		if checksum != 56265 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_get_timeline_event_content_by_event_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_latest_event(uniffiStatus)
		})
		if checksum != 4413 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_latest_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_mark_as_read(uniffiStatus)
		})
		if checksum != 1835 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_mark_as_read: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_paginate_backwards(uniffiStatus)
		})
		if checksum != 50423 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_paginate_backwards: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_retry_decryption(uniffiStatus)
		})
		if checksum != 26528 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_retry_decryption: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_retry_send(uniffiStatus)
		})
		if checksum != 51479 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_retry_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send(uniffiStatus)
		})
		if checksum != 36960 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_audio(uniffiStatus)
		})
		if checksum != 25012 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_audio: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_file(uniffiStatus)
		})
		if checksum != 34478 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_file: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_image(uniffiStatus)
		})
		if checksum != 21504 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_image: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_location(uniffiStatus)
		})
		if checksum != 61646 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_location: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_poll_response(uniffiStatus)
		})
		if checksum != 51038 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_poll_response: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_read_receipt(uniffiStatus)
		})
		if checksum != 47087 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_read_receipt: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_reply(uniffiStatus)
		})
		if checksum != 11052 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_reply: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_video(uniffiStatus)
		})
		if checksum != 37642 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_video: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_send_voice_message(uniffiStatus)
		})
		if checksum != 7512 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_send_voice_message: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_subscribe_to_back_pagination_status(uniffiStatus)
		})
		if checksum != 38905 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_subscribe_to_back_pagination_status: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timeline_toggle_reaction(uniffiStatus)
		})
		if checksum != 32033 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timeline_toggle_reaction: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_append(uniffiStatus)
		})
		if checksum != 24298 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_append: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_change(uniffiStatus)
		})
		if checksum != 50296 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_change: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_insert(uniffiStatus)
		})
		if checksum != 10002 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_insert: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_push_back(uniffiStatus)
		})
		if checksum != 35483 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_push_back: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_push_front(uniffiStatus)
		})
		if checksum != 40108 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_push_front: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_remove(uniffiStatus)
		})
		if checksum != 13408 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_remove: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_reset(uniffiStatus)
		})
		if checksum != 34789 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_reset: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinediff_set(uniffiStatus)
		})
		if checksum != 45340 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinediff_set: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineevent_event_id(uniffiStatus)
		})
		if checksum != 20444 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineevent_event_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineevent_event_type(uniffiStatus)
		})
		if checksum != 52643 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineevent_event_type: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineevent_sender_id(uniffiStatus)
		})
		if checksum != 9141 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineevent_sender_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineevent_timestamp(uniffiStatus)
		})
		if checksum != 30335 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineevent_timestamp: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineitem_as_event(uniffiStatus)
		})
		if checksum != 755 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineitem_as_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineitem_as_virtual(uniffiStatus)
		})
		if checksum != 10265 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineitem_as_virtual: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineitem_fmt_debug(uniffiStatus)
		})
		if checksum != 25731 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineitem_fmt_debug: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineitem_unique_id(uniffiStatus)
		})
		if checksum != 27999 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineitem_unique_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineitemcontent_as_message(uniffiStatus)
		})
		if checksum != 58545 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineitemcontent_as_message: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelineitemcontent_kind(uniffiStatus)
		})
		if checksum != 60128 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelineitemcontent_kind: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_unreadnotificationscount_has_notifications(uniffiStatus)
		})
		if checksum != 38874 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_unreadnotificationscount_has_notifications: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_unreadnotificationscount_highlight_count(uniffiStatus)
		})
		if checksum != 30763 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_unreadnotificationscount_highlight_count: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_unreadnotificationscount_notification_count(uniffiStatus)
		})
		if checksum != 10233 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_unreadnotificationscount_notification_count: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_widgetdriver_run(uniffiStatus)
		})
		if checksum != 39250 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_widgetdriver_run: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_widgetdriverhandle_recv(uniffiStatus)
		})
		if checksum != 25974 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_widgetdriverhandle_recv: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_widgetdriverhandle_send(uniffiStatus)
		})
		if checksum != 32739 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_widgetdriverhandle_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_mediasource_from_json(uniffiStatus)
		})
		if checksum != 31512 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_mediasource_from_json: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_authenticationservice_new(uniffiStatus)
		})
		if checksum != 41347 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_authenticationservice_new: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_clientbuilder_new(uniffiStatus)
		})
		if checksum != 53567 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_clientbuilder_new: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_span_current(uniffiStatus)
		})
		if checksum != 47163 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_span_current: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_span_new(uniffiStatus)
		})
		if checksum != 47854 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_span_new: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_timelineeventtypefilter_exclude(uniffiStatus)
		})
		if checksum != 36856 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_timelineeventtypefilter_exclude: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_constructor_timelineeventtypefilter_include(uniffiStatus)
		})
		if checksum != 40215 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_constructor_timelineeventtypefilter_include: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_backpaginationstatuslistener_on_update(uniffiStatus)
		})
		if checksum != 13839 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_backpaginationstatuslistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_backupstatelistener_on_update(uniffiStatus)
		})
		if checksum != 32936 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_backupstatelistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_backupsteadystatelistener_on_update(uniffiStatus)
		})
		if checksum != 21611 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_backupsteadystatelistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientdelegate_did_receive_auth_error(uniffiStatus)
		})
		if checksum != 54393 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientdelegate_did_receive_auth_error: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientdelegate_did_refresh_tokens(uniffiStatus)
		})
		if checksum != 32841 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientdelegate_did_refresh_tokens: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientsessiondelegate_retrieve_session_from_keychain(uniffiStatus)
		})
		if checksum != 8049 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientsessiondelegate_retrieve_session_from_keychain: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_clientsessiondelegate_save_session_in_keychain(uniffiStatus)
		})
		if checksum != 30188 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_clientsessiondelegate_save_session_in_keychain: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_enablerecoveryprogresslistener_on_update(uniffiStatus)
		})
		if checksum != 5434 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_enablerecoveryprogresslistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_notificationsettingsdelegate_settings_did_change(uniffiStatus)
		})
		if checksum != 4921 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_notificationsettingsdelegate_settings_did_change: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_progresswatcher_transmission_progress(uniffiStatus)
		})
		if checksum != 12165 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_progresswatcher_transmission_progress: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_recoverystatelistener_on_update(uniffiStatus)
		})
		if checksum != 3601 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_recoverystatelistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roominfolistener_call(uniffiStatus)
		})
		if checksum != 567 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roominfolistener_call: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistentrieslistener_on_update(uniffiStatus)
		})
		if checksum != 36351 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistentrieslistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistloadingstatelistener_on_update(uniffiStatus)
		})
		if checksum != 53567 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistloadingstatelistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservicestatelistener_on_update(uniffiStatus)
		})
		if checksum != 27905 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservicestatelistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_roomlistservicesyncindicatorlistener_on_update(uniffiStatus)
		})
		if checksum != 63691 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_roomlistservicesyncindicatorlistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_accept_verification_request(uniffiStatus)
		})
		if checksum != 59777 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_accept_verification_request: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_start_sas_verification(uniffiStatus)
		})
		if checksum != 15715 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_start_sas_verification: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_receive_verification_data(uniffiStatus)
		})
		if checksum != 37461 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_receive_verification_data: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_fail(uniffiStatus)
		})
		if checksum != 52235 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_fail: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_cancel(uniffiStatus)
		})
		if checksum != 52154 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_cancel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_finish(uniffiStatus)
		})
		if checksum != 45558 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_sessionverificationcontrollerdelegate_did_finish: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_syncservicestateobserver_on_update(uniffiStatus)
		})
		if checksum != 52830 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_syncservicestateobserver_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_timelinelistener_on_update(uniffiStatus)
		})
		if checksum != 974 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_timelinelistener_on_update: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_typingnotificationslistener_call(uniffiStatus)
		})
		if checksum != 12563 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_typingnotificationslistener_call: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_matrix_sdk_ffi_checksum_method_widgetcapabilitiesprovider_acquire_capabilities(uniffiStatus)
		})
		if checksum != 47314 {
			// If this happens try cleaning and rebuilding your project
			panic("matrix_sdk_ffi: uniffi_matrix_sdk_ffi_checksum_method_widgetcapabilitiesprovider_acquire_capabilities: UniFFI API checksum mismatch")
		}
	}
}

type FfiConverterUint8 struct{}

var FfiConverterUint8INSTANCE = FfiConverterUint8{}

func (FfiConverterUint8) Lower(value uint8) C.uint8_t {
	return C.uint8_t(value)
}

func (FfiConverterUint8) Write(writer io.Writer, value uint8) {
	writeUint8(writer, value)
}

func (FfiConverterUint8) Lift(value C.uint8_t) uint8 {
	return uint8(value)
}

func (FfiConverterUint8) Read(reader io.Reader) uint8 {
	return readUint8(reader)
}

type FfiDestroyerUint8 struct{}

func (FfiDestroyerUint8) Destroy(_ uint8) {}

type FfiConverterUint16 struct{}

var FfiConverterUint16INSTANCE = FfiConverterUint16{}

func (FfiConverterUint16) Lower(value uint16) C.uint16_t {
	return C.uint16_t(value)
}

func (FfiConverterUint16) Write(writer io.Writer, value uint16) {
	writeUint16(writer, value)
}

func (FfiConverterUint16) Lift(value C.uint16_t) uint16 {
	return uint16(value)
}

func (FfiConverterUint16) Read(reader io.Reader) uint16 {
	return readUint16(reader)
}

type FfiDestroyerUint16 struct{}

func (FfiDestroyerUint16) Destroy(_ uint16) {}

type FfiConverterUint32 struct{}

var FfiConverterUint32INSTANCE = FfiConverterUint32{}

func (FfiConverterUint32) Lower(value uint32) C.uint32_t {
	return C.uint32_t(value)
}

func (FfiConverterUint32) Write(writer io.Writer, value uint32) {
	writeUint32(writer, value)
}

func (FfiConverterUint32) Lift(value C.uint32_t) uint32 {
	return uint32(value)
}

func (FfiConverterUint32) Read(reader io.Reader) uint32 {
	return readUint32(reader)
}

type FfiDestroyerUint32 struct{}

func (FfiDestroyerUint32) Destroy(_ uint32) {}

type FfiConverterInt32 struct{}

var FfiConverterInt32INSTANCE = FfiConverterInt32{}

func (FfiConverterInt32) Lower(value int32) C.int32_t {
	return C.int32_t(value)
}

func (FfiConverterInt32) Write(writer io.Writer, value int32) {
	writeInt32(writer, value)
}

func (FfiConverterInt32) Lift(value C.int32_t) int32 {
	return int32(value)
}

func (FfiConverterInt32) Read(reader io.Reader) int32 {
	return readInt32(reader)
}

type FfiDestroyerInt32 struct{}

func (FfiDestroyerInt32) Destroy(_ int32) {}

type FfiConverterUint64 struct{}

var FfiConverterUint64INSTANCE = FfiConverterUint64{}

func (FfiConverterUint64) Lower(value uint64) C.uint64_t {
	return C.uint64_t(value)
}

func (FfiConverterUint64) Write(writer io.Writer, value uint64) {
	writeUint64(writer, value)
}

func (FfiConverterUint64) Lift(value C.uint64_t) uint64 {
	return uint64(value)
}

func (FfiConverterUint64) Read(reader io.Reader) uint64 {
	return readUint64(reader)
}

type FfiDestroyerUint64 struct{}

func (FfiDestroyerUint64) Destroy(_ uint64) {}

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

type FfiConverterFloat64 struct{}

var FfiConverterFloat64INSTANCE = FfiConverterFloat64{}

func (FfiConverterFloat64) Lower(value float64) C.double {
	return C.double(value)
}

func (FfiConverterFloat64) Write(writer io.Writer, value float64) {
	writeFloat64(writer, value)
}

func (FfiConverterFloat64) Lift(value C.double) float64 {
	return float64(value)
}

func (FfiConverterFloat64) Read(reader io.Reader) float64 {
	return readFloat64(reader)
}

type FfiDestroyerFloat64 struct{}

func (FfiDestroyerFloat64) Destroy(_ float64) {}

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

type FfiConverterBytes struct{}

var FfiConverterBytesINSTANCE = FfiConverterBytes{}

func (c FfiConverterBytes) Lower(value []byte) RustBuffer {
	return LowerIntoRustBuffer[[]byte](c, value)
}

func (c FfiConverterBytes) Write(writer io.Writer, value []byte) {
	if len(value) > math.MaxInt32 {
		panic("[]byte is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	write_length, err := writer.Write(value)
	if err != nil {
		panic(err)
	}
	if write_length != len(value) {
		panic(fmt.Errorf("bad write length when writing []byte, expected %d, written %d", len(value), write_length))
	}
}

func (c FfiConverterBytes) Lift(rb RustBufferI) []byte {
	return LiftFromRustBuffer[[]byte](c, rb)
}

func (c FfiConverterBytes) Read(reader io.Reader) []byte {
	length := readInt32(reader)
	buffer := make([]byte, length)
	read_length, err := reader.Read(buffer)
	if err != nil {
		panic(err)
	}
	if read_length != int(length) {
		panic(fmt.Errorf("bad read length when reading []byte, expected %d, read %d", length, read_length))
	}
	return buffer
}

type FfiDestroyerBytes struct{}

func (FfiDestroyerBytes) Destroy(_ []byte) {}

// FfiConverterDuration converts between uniffi duration and Go duration.
type FfiConverterDuration struct{}

var FfiConverterDurationINSTANCE = FfiConverterDuration{}

func (c FfiConverterDuration) Lift(rb RustBufferI) time.Duration {
	return LiftFromRustBuffer[time.Duration](c, rb)
}

func (c FfiConverterDuration) Read(reader io.Reader) time.Duration {
	sec := readUint64(reader)
	nsec := readUint32(reader)
	return time.Duration(sec*1_000_000_000 + uint64(nsec))
}

func (c FfiConverterDuration) Lower(value time.Duration) RustBuffer {
	return LowerIntoRustBuffer[time.Duration](c, value)
}

func (c FfiConverterDuration) Write(writer io.Writer, value time.Duration) {
	if value.Nanoseconds() < 0 {
		// Rust does not support negative durations:
		// https://www.reddit.com/r/rust/comments/ljl55u/why_rusts_duration_not_supporting_negative_values/
		// This panic is very bad, because it depends on user input, and in Go user input related
		// error are supposed to be returned as errors, and not cause panics. However, with the
		// current architecture, its not possible to return an error from here, so panic is used as
		// the only other option to signal an error.
		panic("negative duration is not allowed")
	}

	writeUint64(writer, uint64(value)/1_000_000_000)
	writeUint32(writer, uint32(uint64(value)%1_000_000_000))
}

type FfiDestroyerDuration struct{}

func (FfiDestroyerDuration) Destroy(_ time.Duration) {}

// Below is an implementation of synchronization requirements outlined in the link.
// https://github.com/mozilla/uniffi-rs/blob/0dc031132d9493ca812c3af6e7dd60ad2ea95bf0/uniffi_bindgen/src/bindings/kotlin/templates/ObjectRuntime.kt#L31

type FfiObject struct {
	pointer      unsafe.Pointer
	callCounter  atomic.Int64
	freeFunction func(unsafe.Pointer, *C.RustCallStatus)
	destroyed    atomic.Bool
}

func newFfiObject(pointer unsafe.Pointer, freeFunction func(unsafe.Pointer, *C.RustCallStatus)) FfiObject {
	return FfiObject{
		pointer:      pointer,
		freeFunction: freeFunction,
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

	return ffiObject.pointer
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

type AuthenticationService struct {
	ffiObject FfiObject
}

func NewAuthenticationService(basePath string, passphrase *string, userAgent *string, oidcConfiguration *OidcConfiguration, customSlidingSyncProxy *string, sessionDelegate *ClientSessionDelegate, crossProcessRefreshLockId *string) *AuthenticationService {
	return FfiConverterAuthenticationServiceINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_authenticationservice_new(FfiConverterStringINSTANCE.Lower(basePath), FfiConverterOptionalStringINSTANCE.Lower(passphrase), FfiConverterOptionalStringINSTANCE.Lower(userAgent), FfiConverterOptionalTypeOidcConfigurationINSTANCE.Lower(oidcConfiguration), FfiConverterOptionalStringINSTANCE.Lower(customSlidingSyncProxy), FfiConverterOptionalCallbackInterfaceClientSessionDelegateINSTANCE.Lower(sessionDelegate), FfiConverterOptionalStringINSTANCE.Lower(crossProcessRefreshLockId), _uniffiStatus)
	}))
}

func (_self *AuthenticationService) ConfigureHomeserver(serverNameOrHomeserverUrl string) error {
	_pointer := _self.ffiObject.incrementPointer("*AuthenticationService")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeAuthenticationError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_authenticationservice_configure_homeserver(
			_pointer, FfiConverterStringINSTANCE.Lower(serverNameOrHomeserverUrl), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *AuthenticationService) HomeserverDetails() **HomeserverLoginDetails {
	_pointer := _self.ffiObject.incrementPointer("*AuthenticationService")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalHomeserverLoginDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_authenticationservice_homeserver_details(
			_pointer, _uniffiStatus)
	}))
}

func (_self *AuthenticationService) Login(username string, password string, initialDeviceName *string, deviceId *string) (*Client, error) {
	_pointer := _self.ffiObject.incrementPointer("*AuthenticationService")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeAuthenticationError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_authenticationservice_login(
			_pointer, FfiConverterStringINSTANCE.Lower(username), FfiConverterStringINSTANCE.Lower(password), FfiConverterOptionalStringINSTANCE.Lower(initialDeviceName), FfiConverterOptionalStringINSTANCE.Lower(deviceId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Client
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterClientINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *AuthenticationService) LoginWithOidcCallback(authenticationData *OidcAuthenticationData, callbackUrl string) (*Client, error) {
	_pointer := _self.ffiObject.incrementPointer("*AuthenticationService")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeAuthenticationError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_authenticationservice_login_with_oidc_callback(
			_pointer, FfiConverterOidcAuthenticationDataINSTANCE.Lower(authenticationData), FfiConverterStringINSTANCE.Lower(callbackUrl), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Client
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterClientINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *AuthenticationService) UrlForOidcLogin() (*OidcAuthenticationData, error) {
	_pointer := _self.ffiObject.incrementPointer("*AuthenticationService")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeAuthenticationError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_authenticationservice_url_for_oidc_login(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *OidcAuthenticationData
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOidcAuthenticationDataINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (object *AuthenticationService) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterAuthenticationService struct{}

var FfiConverterAuthenticationServiceINSTANCE = FfiConverterAuthenticationService{}

func (c FfiConverterAuthenticationService) Lift(pointer unsafe.Pointer) *AuthenticationService {
	result := &AuthenticationService{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_authenticationservice(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*AuthenticationService).Destroy)
	return result
}

func (c FfiConverterAuthenticationService) Read(reader io.Reader) *AuthenticationService {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterAuthenticationService) Lower(value *AuthenticationService) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*AuthenticationService")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterAuthenticationService) Write(writer io.Writer, value *AuthenticationService) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerAuthenticationService struct{}

func (_ FfiDestroyerAuthenticationService) Destroy(value *AuthenticationService) {
	value.Destroy()
}

type Client struct {
	ffiObject FfiObject
}

func (_self *Client) AccountData(eventType string) (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_account_data(
			_pointer, FfiConverterStringINSTANCE.Lower(eventType), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) AccountUrl(action *AccountManagementAction) (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_account_url(
			_pointer, FfiConverterOptionalTypeAccountManagementActionINSTANCE.Lower(action), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) AvatarUrl() (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_avatar_url(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) CachedAvatarUrl() (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_cached_avatar_url(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) CreateRoom(request CreateRoomParameters) (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_create_room(
			_pointer, FfiConverterTypeCreateRoomParametersINSTANCE.Lower(request), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) DeviceId() (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_device_id(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) DisplayName() (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_display_name(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) Encryption() *Encryption {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterEncryptionINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_encryption(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Client) GetDmRoom(userId string) (**Room, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_get_dm_room(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue **Room
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalRoomINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) GetMediaContent(mediaSource *MediaSource) ([]byte, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_client_get_media_content(
				_pointer, FfiConverterMediaSourceINSTANCE.Lower(mediaSource),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterBytesINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Client) GetMediaFile(mediaSource *MediaSource, body *string, mimeType string, useCache bool, tempDir *string) (*MediaFileHandle, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_client_get_media_file(
				_pointer, FfiConverterMediaSourceINSTANCE.Lower(mediaSource), FfiConverterOptionalStringINSTANCE.Lower(body), FfiConverterStringINSTANCE.Lower(mimeType), FfiConverterBoolINSTANCE.Lower(useCache), FfiConverterOptionalStringINSTANCE.Lower(tempDir),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterMediaFileHandleINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Client) GetMediaThumbnail(mediaSource *MediaSource, width uint64, height uint64) ([]byte, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_client_get_media_thumbnail(
				_pointer, FfiConverterMediaSourceINSTANCE.Lower(mediaSource), FfiConverterUint64INSTANCE.Lower(width), FfiConverterUint64INSTANCE.Lower(height),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterBytesINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Client) GetNotificationSettings() *NotificationSettings {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterNotificationSettingsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_get_notification_settings(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Client) GetProfile(userId string) (UserProfile, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_get_profile(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue UserProfile
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeUserProfileINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) GetSessionVerificationController() (*SessionVerificationController, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_get_session_verification_controller(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *SessionVerificationController
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterSessionVerificationControllerINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) Homeserver() string {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_homeserver(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Client) IgnoreUser(userId string) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_ignore_user(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) Login(username string, password string, initialDeviceName *string, deviceId *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_login(
			_pointer, FfiConverterStringINSTANCE.Lower(username), FfiConverterStringINSTANCE.Lower(password), FfiConverterOptionalStringINSTANCE.Lower(initialDeviceName), FfiConverterOptionalStringINSTANCE.Lower(deviceId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) Logout() (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_logout(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) NotificationClient(processSetup NotificationProcessSetup) (*NotificationClientBuilder, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_notification_client(
			_pointer, FfiConverterTypeNotificationProcessSetupINSTANCE.Lower(processSetup), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *NotificationClientBuilder
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterNotificationClientBuilderINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) RemoveAvatar() error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_remove_avatar(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) RestoreSession(session Session) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_restore_session(
			_pointer, FfiConverterTypeSessionINSTANCE.Lower(session), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) Rooms() []*Room {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceRoomINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_rooms(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Client) SearchUsers(searchTerm string, limit uint64) (SearchUsersResults, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_search_users(
			_pointer, FfiConverterStringINSTANCE.Lower(searchTerm), FfiConverterUint64INSTANCE.Lower(limit), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue SearchUsersResults
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeSearchUsersResultsINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) Session() (Session, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_session(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Session
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeSessionINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Client) SetAccountData(eventType string, content string) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_set_account_data(
			_pointer, FfiConverterStringINSTANCE.Lower(eventType), FfiConverterStringINSTANCE.Lower(content), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) SetDelegate(delegate *ClientDelegate) **TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_set_delegate(
			_pointer, FfiConverterOptionalCallbackInterfaceClientDelegateINSTANCE.Lower(delegate), _uniffiStatus)
	}))
}

func (_self *Client) SetDisplayName(name string) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_set_display_name(
			_pointer, FfiConverterStringINSTANCE.Lower(name), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) SetPusher(identifiers PusherIdentifiers, kind PusherKind, appDisplayName string, deviceDisplayName string, profileTag *string, lang string) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_set_pusher(
			_pointer, FfiConverterTypePusherIdentifiersINSTANCE.Lower(identifiers), FfiConverterTypePusherKindINSTANCE.Lower(kind), FfiConverterStringINSTANCE.Lower(appDisplayName), FfiConverterStringINSTANCE.Lower(deviceDisplayName), FfiConverterOptionalStringINSTANCE.Lower(profileTag), FfiConverterStringINSTANCE.Lower(lang), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) SyncService() *SyncServiceBuilder {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSyncServiceBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_sync_service(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Client) UnignoreUser(userId string) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_unignore_user(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) UploadAvatar(mimeType string, data []byte) error {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_client_upload_avatar(
			_pointer, FfiConverterStringINSTANCE.Lower(mimeType), FfiConverterBytesINSTANCE.Lower(data), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Client) UploadMedia(mimeType string, data []byte, progressWatcher *ProgressWatcher) (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_client_upload_media(
				_pointer, FfiConverterStringINSTANCE.Lower(mimeType), FfiConverterBytesINSTANCE.Lower(data), FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE.Lower(progressWatcher),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Client) UserId() (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Client")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_client_user_id(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (object *Client) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterClient struct{}

var FfiConverterClientINSTANCE = FfiConverterClient{}

func (c FfiConverterClient) Lift(pointer unsafe.Pointer) *Client {
	result := &Client{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_client(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*Client).Destroy)
	return result
}

func (c FfiConverterClient) Read(reader io.Reader) *Client {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterClient) Lower(value *Client) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Client")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterClient) Write(writer io.Writer, value *Client) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerClient struct{}

func (_ FfiDestroyerClient) Destroy(value *Client) {
	value.Destroy()
}

type ClientBuilder struct {
	ffiObject FfiObject
}

func NewClientBuilder() *ClientBuilder {
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_clientbuilder_new(_uniffiStatus)
	}))
}

func (_self *ClientBuilder) BasePath(path string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_base_path(
			_pointer, FfiConverterStringINSTANCE.Lower(path), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) Build() (*Client, error) {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_build(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Client
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterClientINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *ClientBuilder) DisableAutomaticTokenRefresh() *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_disable_automatic_token_refresh(
			_pointer, _uniffiStatus)
	}))
}

func (_self *ClientBuilder) DisableSslVerification() *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_disable_ssl_verification(
			_pointer, _uniffiStatus)
	}))
}

func (_self *ClientBuilder) EnableCrossProcessRefreshLock(processId string, sessionDelegate ClientSessionDelegate) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_enable_cross_process_refresh_lock(
			_pointer, FfiConverterStringINSTANCE.Lower(processId), FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE.Lower(sessionDelegate), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) HomeserverUrl(url string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_homeserver_url(
			_pointer, FfiConverterStringINSTANCE.Lower(url), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) Passphrase(passphrase *string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_passphrase(
			_pointer, FfiConverterOptionalStringINSTANCE.Lower(passphrase), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) Proxy(url string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_proxy(
			_pointer, FfiConverterStringINSTANCE.Lower(url), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) ServerName(serverName string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_server_name(
			_pointer, FfiConverterStringINSTANCE.Lower(serverName), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) ServerVersions(versions []string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_server_versions(
			_pointer, FfiConverterSequenceStringINSTANCE.Lower(versions), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) SetSessionDelegate(sessionDelegate ClientSessionDelegate) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_set_session_delegate(
			_pointer, FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE.Lower(sessionDelegate), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) SlidingSyncProxy(slidingSyncProxy *string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_sliding_sync_proxy(
			_pointer, FfiConverterOptionalStringINSTANCE.Lower(slidingSyncProxy), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) UserAgent(userAgent string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_user_agent(
			_pointer, FfiConverterStringINSTANCE.Lower(userAgent), _uniffiStatus)
	}))
}

func (_self *ClientBuilder) Username(username string) *ClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*ClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_clientbuilder_username(
			_pointer, FfiConverterStringINSTANCE.Lower(username), _uniffiStatus)
	}))
}

func (object *ClientBuilder) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterClientBuilder struct{}

var FfiConverterClientBuilderINSTANCE = FfiConverterClientBuilder{}

func (c FfiConverterClientBuilder) Lift(pointer unsafe.Pointer) *ClientBuilder {
	result := &ClientBuilder{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_clientbuilder(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*ClientBuilder).Destroy)
	return result
}

func (c FfiConverterClientBuilder) Read(reader io.Reader) *ClientBuilder {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterClientBuilder) Lower(value *ClientBuilder) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*ClientBuilder")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterClientBuilder) Write(writer io.Writer, value *ClientBuilder) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerClientBuilder struct{}

func (_ FfiDestroyerClientBuilder) Destroy(value *ClientBuilder) {
	value.Destroy()
}

type Encryption struct {
	ffiObject FfiObject
}

func (_self *Encryption) BackupExistsOnServer() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_backup_exists_on_server(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) BackupState() BackupState {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeBackupStateINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_encryption_backup_state(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Encryption) BackupStateListener(listener BackupStateListener) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_encryption_backup_state_listener(
			_pointer, FfiConverterCallbackInterfaceBackupStateListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *Encryption) DisableRecovery() error {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_disable_recovery(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) EnableBackups() error {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_enable_backups(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) EnableRecovery(waitForBackupsToUpload bool, progressListener EnableRecoveryProgressListener) (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_enable_recovery(
				_pointer, FfiConverterBoolINSTANCE.Lower(waitForBackupsToUpload), FfiConverterCallbackInterfaceEnableRecoveryProgressListenerINSTANCE.Lower(progressListener),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) IsLastDevice() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_is_last_device(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) Recover(recoveryKey string) error {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_recover(
				_pointer, FfiConverterStringINSTANCE.Lower(recoveryKey),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) RecoverAndReset(oldRecoveryKey string) (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_recover_and_reset(
				_pointer, FfiConverterStringINSTANCE.Lower(oldRecoveryKey),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) RecoveryState() RecoveryState {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeRecoveryStateINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_encryption_recovery_state(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Encryption) RecoveryStateListener(listener RecoveryStateListener) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_encryption_recovery_state_listener(
			_pointer, FfiConverterCallbackInterfaceRecoveryStateListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *Encryption) ResetRecoveryKey() (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRecoveryError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_reset_recovery_key(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Encryption) WaitForBackupUploadSteadyState(progressListener *BackupSteadyStateListener) error {
	_pointer := _self.ffiObject.incrementPointer("*Encryption")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeSteadyStateError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_encryption_wait_for_backup_upload_steady_state(
				_pointer, FfiConverterOptionalCallbackInterfaceBackupSteadyStateListenerINSTANCE.Lower(progressListener),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}

func (object *Encryption) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterEncryption struct{}

var FfiConverterEncryptionINSTANCE = FfiConverterEncryption{}

func (c FfiConverterEncryption) Lift(pointer unsafe.Pointer) *Encryption {
	result := &Encryption{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_encryption(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*Encryption).Destroy)
	return result
}

func (c FfiConverterEncryption) Read(reader io.Reader) *Encryption {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterEncryption) Lower(value *Encryption) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Encryption")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterEncryption) Write(writer io.Writer, value *Encryption) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerEncryption struct{}

func (_ FfiDestroyerEncryption) Destroy(value *Encryption) {
	value.Destroy()
}

type EventTimelineItem struct {
	ffiObject FfiObject
}

func (_self *EventTimelineItem) CanBeRepliedTo() bool {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_can_be_replied_to(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) Content() *TimelineItemContent {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTimelineItemContentINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_content(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) DebugInfo() EventTimelineItemDebugInfo {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeEventTimelineItemDebugInfoINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_debug_info(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) EventId() *string {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_event_id(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) IsEditable() bool {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_is_editable(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) IsLocal() bool {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_is_local(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) IsOwn() bool {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_is_own(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) IsRemote() bool {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_is_remote(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) LocalSendState() *EventSendState {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeEventSendStateINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_local_send_state(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) Origin() *matrix_sdk_ui.EventItemOrigin {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeEventItemOriginINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_origin(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) Reactions() []Reaction {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceTypeReactionINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_reactions(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) ReadReceipts() map[string]Receipt {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterMapStringTypeReceiptINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_read_receipts(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) Sender() string {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_sender(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) SenderProfile() ProfileDetails {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeProfileDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_sender_profile(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) Timestamp() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_timestamp(
			_pointer, _uniffiStatus)
	}))
}

func (_self *EventTimelineItem) TransactionId() *string {
	_pointer := _self.ffiObject.incrementPointer("*EventTimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_eventtimelineitem_transaction_id(
			_pointer, _uniffiStatus)
	}))
}

func (object *EventTimelineItem) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterEventTimelineItem struct{}

var FfiConverterEventTimelineItemINSTANCE = FfiConverterEventTimelineItem{}

func (c FfiConverterEventTimelineItem) Lift(pointer unsafe.Pointer) *EventTimelineItem {
	result := &EventTimelineItem{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_eventtimelineitem(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*EventTimelineItem).Destroy)
	return result
}

func (c FfiConverterEventTimelineItem) Read(reader io.Reader) *EventTimelineItem {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterEventTimelineItem) Lower(value *EventTimelineItem) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*EventTimelineItem")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterEventTimelineItem) Write(writer io.Writer, value *EventTimelineItem) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerEventTimelineItem struct{}

func (_ FfiDestroyerEventTimelineItem) Destroy(value *EventTimelineItem) {
	value.Destroy()
}

type HomeserverLoginDetails struct {
	ffiObject FfiObject
}

func (_self *HomeserverLoginDetails) SupportsOidcLogin() bool {
	_pointer := _self.ffiObject.incrementPointer("*HomeserverLoginDetails")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_homeserverlogindetails_supports_oidc_login(
			_pointer, _uniffiStatus)
	}))
}

func (_self *HomeserverLoginDetails) SupportsPasswordLogin() bool {
	_pointer := _self.ffiObject.incrementPointer("*HomeserverLoginDetails")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_homeserverlogindetails_supports_password_login(
			_pointer, _uniffiStatus)
	}))
}

func (_self *HomeserverLoginDetails) Url() string {
	_pointer := _self.ffiObject.incrementPointer("*HomeserverLoginDetails")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_homeserverlogindetails_url(
			_pointer, _uniffiStatus)
	}))
}

func (object *HomeserverLoginDetails) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterHomeserverLoginDetails struct{}

var FfiConverterHomeserverLoginDetailsINSTANCE = FfiConverterHomeserverLoginDetails{}

func (c FfiConverterHomeserverLoginDetails) Lift(pointer unsafe.Pointer) *HomeserverLoginDetails {
	result := &HomeserverLoginDetails{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_homeserverlogindetails(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*HomeserverLoginDetails).Destroy)
	return result
}

func (c FfiConverterHomeserverLoginDetails) Read(reader io.Reader) *HomeserverLoginDetails {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterHomeserverLoginDetails) Lower(value *HomeserverLoginDetails) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*HomeserverLoginDetails")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterHomeserverLoginDetails) Write(writer io.Writer, value *HomeserverLoginDetails) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerHomeserverLoginDetails struct{}

func (_ FfiDestroyerHomeserverLoginDetails) Destroy(value *HomeserverLoginDetails) {
	value.Destroy()
}

type MediaFileHandle struct {
	ffiObject FfiObject
}

func (_self *MediaFileHandle) Path() (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*MediaFileHandle")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_mediafilehandle_path(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *MediaFileHandle) Persist(path string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*MediaFileHandle")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_mediafilehandle_persist(
			_pointer, FfiConverterStringINSTANCE.Lower(path), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue bool
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBoolINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (object *MediaFileHandle) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterMediaFileHandle struct{}

var FfiConverterMediaFileHandleINSTANCE = FfiConverterMediaFileHandle{}

func (c FfiConverterMediaFileHandle) Lift(pointer unsafe.Pointer) *MediaFileHandle {
	result := &MediaFileHandle{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_mediafilehandle(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*MediaFileHandle).Destroy)
	return result
}

func (c FfiConverterMediaFileHandle) Read(reader io.Reader) *MediaFileHandle {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterMediaFileHandle) Lower(value *MediaFileHandle) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*MediaFileHandle")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterMediaFileHandle) Write(writer io.Writer, value *MediaFileHandle) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerMediaFileHandle struct{}

func (_ FfiDestroyerMediaFileHandle) Destroy(value *MediaFileHandle) {
	value.Destroy()
}

type MediaSource struct {
	ffiObject FfiObject
}

func MediaSourceFromJson(json string) (*MediaSource, error) {
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_mediasource_from_json(FfiConverterStringINSTANCE.Lower(json), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *MediaSource
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterMediaSourceINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *MediaSource) ToJson() string {
	_pointer := _self.ffiObject.incrementPointer("*MediaSource")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_mediasource_to_json(
			_pointer, _uniffiStatus)
	}))
}

func (_self *MediaSource) Url() string {
	_pointer := _self.ffiObject.incrementPointer("*MediaSource")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_mediasource_url(
			_pointer, _uniffiStatus)
	}))
}

func (object *MediaSource) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterMediaSource struct{}

var FfiConverterMediaSourceINSTANCE = FfiConverterMediaSource{}

func (c FfiConverterMediaSource) Lift(pointer unsafe.Pointer) *MediaSource {
	result := &MediaSource{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_mediasource(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*MediaSource).Destroy)
	return result
}

func (c FfiConverterMediaSource) Read(reader io.Reader) *MediaSource {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterMediaSource) Lower(value *MediaSource) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*MediaSource")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterMediaSource) Write(writer io.Writer, value *MediaSource) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerMediaSource struct{}

func (_ FfiDestroyerMediaSource) Destroy(value *MediaSource) {
	value.Destroy()
}

type Message struct {
	ffiObject FfiObject
}

func (_self *Message) Body() string {
	_pointer := _self.ffiObject.incrementPointer("*Message")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_message_body(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Message) InReplyTo() *InReplyToDetails {
	_pointer := _self.ffiObject.incrementPointer("*Message")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeInReplyToDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_message_in_reply_to(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Message) IsEdited() bool {
	_pointer := _self.ffiObject.incrementPointer("*Message")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_message_is_edited(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Message) IsThreaded() bool {
	_pointer := _self.ffiObject.incrementPointer("*Message")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_message_is_threaded(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Message) Msgtype() MessageType {
	_pointer := _self.ffiObject.incrementPointer("*Message")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeMessageTypeINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_message_msgtype(
			_pointer, _uniffiStatus)
	}))
}

func (object *Message) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterMessage struct{}

var FfiConverterMessageINSTANCE = FfiConverterMessage{}

func (c FfiConverterMessage) Lift(pointer unsafe.Pointer) *Message {
	result := &Message{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_message(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*Message).Destroy)
	return result
}

func (c FfiConverterMessage) Read(reader io.Reader) *Message {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterMessage) Lower(value *Message) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Message")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterMessage) Write(writer io.Writer, value *Message) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerMessage struct{}

func (_ FfiDestroyerMessage) Destroy(value *Message) {
	value.Destroy()
}

type NotificationClient struct {
	ffiObject FfiObject
}

func (_self *NotificationClient) GetNotification(roomId string, eventId string) (*NotificationItem, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationClient")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_notificationclient_get_notification(
			_pointer, FfiConverterStringINSTANCE.Lower(roomId), FfiConverterStringINSTANCE.Lower(eventId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *NotificationItem
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalTypeNotificationItemINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (object *NotificationClient) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterNotificationClient struct{}

var FfiConverterNotificationClientINSTANCE = FfiConverterNotificationClient{}

func (c FfiConverterNotificationClient) Lift(pointer unsafe.Pointer) *NotificationClient {
	result := &NotificationClient{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_notificationclient(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*NotificationClient).Destroy)
	return result
}

func (c FfiConverterNotificationClient) Read(reader io.Reader) *NotificationClient {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterNotificationClient) Lower(value *NotificationClient) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*NotificationClient")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterNotificationClient) Write(writer io.Writer, value *NotificationClient) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerNotificationClient struct{}

func (_ FfiDestroyerNotificationClient) Destroy(value *NotificationClient) {
	value.Destroy()
}

type NotificationClientBuilder struct {
	ffiObject FfiObject
}

func (_self *NotificationClientBuilder) FilterByPushRules() *NotificationClientBuilder {
	_pointer := _self.ffiObject.incrementPointer("*NotificationClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterNotificationClientBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_notificationclientbuilder_filter_by_push_rules(
			_pointer, _uniffiStatus)
	}))
}

func (_self *NotificationClientBuilder) Finish() *NotificationClient {
	_pointer := _self.ffiObject.incrementPointer("*NotificationClientBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterNotificationClientINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_notificationclientbuilder_finish(
			_pointer, _uniffiStatus)
	}))
}

func (object *NotificationClientBuilder) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterNotificationClientBuilder struct{}

var FfiConverterNotificationClientBuilderINSTANCE = FfiConverterNotificationClientBuilder{}

func (c FfiConverterNotificationClientBuilder) Lift(pointer unsafe.Pointer) *NotificationClientBuilder {
	result := &NotificationClientBuilder{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_notificationclientbuilder(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*NotificationClientBuilder).Destroy)
	return result
}

func (c FfiConverterNotificationClientBuilder) Read(reader io.Reader) *NotificationClientBuilder {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterNotificationClientBuilder) Lower(value *NotificationClientBuilder) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*NotificationClientBuilder")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterNotificationClientBuilder) Write(writer io.Writer, value *NotificationClientBuilder) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerNotificationClientBuilder struct{}

func (_ FfiDestroyerNotificationClientBuilder) Destroy(value *NotificationClientBuilder) {
	value.Destroy()
}

type NotificationSettings struct {
	ffiObject FfiObject
}

func (_self *NotificationSettings) CanPushEncryptedEventToDevice() bool {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_can_push_encrypted_event_to_device(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) ContainsKeywordsRules() bool {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_contains_keywords_rules(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) GetDefaultRoomNotificationMode(isEncrypted bool, isOneToOne bool) RoomNotificationMode {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_get_default_room_notification_mode(
			_pointer, FfiConverterBoolINSTANCE.Lower(isEncrypted), FfiConverterBoolINSTANCE.Lower(isOneToOne),
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterTypeRoomNotificationModeINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) GetRoomNotificationSettings(roomId string, isEncrypted bool, isOneToOne bool) (RoomNotificationSettings, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_get_room_notification_settings(
				_pointer, FfiConverterStringINSTANCE.Lower(roomId), FfiConverterBoolINSTANCE.Lower(isEncrypted), FfiConverterBoolINSTANCE.Lower(isOneToOne),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterTypeRoomNotificationSettingsINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) GetRoomsWithUserDefinedRules(enabled *bool) []string {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_get_rooms_with_user_defined_rules(
			_pointer, FfiConverterOptionalBoolINSTANCE.Lower(enabled),
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterSequenceStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) GetUserDefinedRoomNotificationMode(roomId string) (*RoomNotificationMode, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_get_user_defined_room_notification_mode(
				_pointer, FfiConverterStringINSTANCE.Lower(roomId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterOptionalTypeRoomNotificationModeINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) IsCallEnabled() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_is_call_enabled(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) IsInviteForMeEnabled() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_is_invite_for_me_enabled(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) IsRoomMentionEnabled() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_is_room_mention_enabled(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) IsUserMentionEnabled() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_is_user_mention_enabled(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) RestoreDefaultRoomNotificationMode(roomId string) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_restore_default_room_notification_mode(
				_pointer, FfiConverterStringINSTANCE.Lower(roomId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) SetCallEnabled(enabled bool) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_call_enabled(
				_pointer, FfiConverterBoolINSTANCE.Lower(enabled),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) SetDefaultRoomNotificationMode(isEncrypted bool, isOneToOne bool, mode RoomNotificationMode) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_default_room_notification_mode(
				_pointer, FfiConverterBoolINSTANCE.Lower(isEncrypted), FfiConverterBoolINSTANCE.Lower(isOneToOne), FfiConverterTypeRoomNotificationModeINSTANCE.Lower(mode),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) SetDelegate(delegate *NotificationSettingsDelegate) {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_delegate(
			_pointer, FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegateINSTANCE.Lower(delegate), _uniffiStatus)
		return false
	})
}

func (_self *NotificationSettings) SetInviteForMeEnabled(enabled bool) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_invite_for_me_enabled(
				_pointer, FfiConverterBoolINSTANCE.Lower(enabled),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) SetRoomMentionEnabled(enabled bool) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_room_mention_enabled(
				_pointer, FfiConverterBoolINSTANCE.Lower(enabled),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) SetRoomNotificationMode(roomId string, mode RoomNotificationMode) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_room_notification_mode(
				_pointer, FfiConverterStringINSTANCE.Lower(roomId), FfiConverterTypeRoomNotificationModeINSTANCE.Lower(mode),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) SetUserMentionEnabled(enabled bool) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_set_user_mention_enabled(
				_pointer, FfiConverterBoolINSTANCE.Lower(enabled),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *NotificationSettings) UnmuteRoom(roomId string, isEncrypted bool, isOneToOne bool) error {
	_pointer := _self.ffiObject.incrementPointer("*NotificationSettings")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeNotificationSettingsError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_notificationsettings_unmute_room(
				_pointer, FfiConverterStringINSTANCE.Lower(roomId), FfiConverterBoolINSTANCE.Lower(isEncrypted), FfiConverterBoolINSTANCE.Lower(isOneToOne),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}

func (object *NotificationSettings) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterNotificationSettings struct{}

var FfiConverterNotificationSettingsINSTANCE = FfiConverterNotificationSettings{}

func (c FfiConverterNotificationSettings) Lift(pointer unsafe.Pointer) *NotificationSettings {
	result := &NotificationSettings{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_notificationsettings(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*NotificationSettings).Destroy)
	return result
}

func (c FfiConverterNotificationSettings) Read(reader io.Reader) *NotificationSettings {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterNotificationSettings) Lower(value *NotificationSettings) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*NotificationSettings")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterNotificationSettings) Write(writer io.Writer, value *NotificationSettings) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerNotificationSettings struct{}

func (_ FfiDestroyerNotificationSettings) Destroy(value *NotificationSettings) {
	value.Destroy()
}

type OidcAuthenticationData struct {
	ffiObject FfiObject
}

func (_self *OidcAuthenticationData) LoginUrl() string {
	_pointer := _self.ffiObject.incrementPointer("*OidcAuthenticationData")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_oidcauthenticationdata_login_url(
			_pointer, _uniffiStatus)
	}))
}

func (object *OidcAuthenticationData) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterOidcAuthenticationData struct{}

var FfiConverterOidcAuthenticationDataINSTANCE = FfiConverterOidcAuthenticationData{}

func (c FfiConverterOidcAuthenticationData) Lift(pointer unsafe.Pointer) *OidcAuthenticationData {
	result := &OidcAuthenticationData{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_oidcauthenticationdata(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*OidcAuthenticationData).Destroy)
	return result
}

func (c FfiConverterOidcAuthenticationData) Read(reader io.Reader) *OidcAuthenticationData {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterOidcAuthenticationData) Lower(value *OidcAuthenticationData) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*OidcAuthenticationData")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterOidcAuthenticationData) Write(writer io.Writer, value *OidcAuthenticationData) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerOidcAuthenticationData struct{}

func (_ FfiDestroyerOidcAuthenticationData) Destroy(value *OidcAuthenticationData) {
	value.Destroy()
}

type Room struct {
	ffiObject FfiObject
}

func (_self *Room) ActiveMembersCount() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_active_members_count(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) ActiveRoomCallParticipants() []string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_active_room_call_participants(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) AlternativeAliases() []string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_alternative_aliases(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) ApplyPowerLevelChanges(changes matrix_sdk.RoomPowerLevelChanges) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_apply_power_level_changes(
				_pointer, RustBufferFromExternal(matrix_sdk.FfiConverterTypeRoomPowerLevelChangesINSTANCE.Lower(changes)),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) AvatarUrl() *string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_avatar_url(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) BanUser(userId string, reason *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_ban_user(
				_pointer, FfiConverterStringINSTANCE.Lower(userId), FfiConverterOptionalStringINSTANCE.Lower(reason),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) BuildPowerLevelChangesFromCurrent() (matrix_sdk.RoomPowerLevelChanges, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult[matrix_sdk.RustBufferI, matrix_sdk.RoomPowerLevelChanges](
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_build_power_level_changes_from_current(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) matrix_sdk.RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		matrix_sdk.FfiConverterTypeRoomPowerLevelChangesINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserBan(userId string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_ban(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserInvite(userId string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_invite(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserKick(userId string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_kick(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserRedactOther(userId string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_redact_other(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserRedactOwn(userId string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_redact_own(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserSendMessage(userId string, message MessageLikeEventType) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_send_message(
				_pointer, FfiConverterStringINSTANCE.Lower(userId), FfiConverterTypeMessageLikeEventTypeINSTANCE.Lower(message),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserSendState(userId string, stateEvent StateEventType) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_send_state(
				_pointer, FfiConverterStringINSTANCE.Lower(userId), FfiConverterTypeStateEventTypeINSTANCE.Lower(stateEvent),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanUserTriggerRoomNotification(userId string) (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_can_user_trigger_room_notification(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) CanonicalAlias() *string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_canonical_alias(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) DisplayName() (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_display_name(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Room) HasActiveRoomCall() bool {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_has_active_room_call(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) Id() string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_id(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) IgnoreUser(userId string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_ignore_user(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) InviteUserById(userId string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_invite_user_by_id(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) InvitedMembersCount() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_invited_members_count(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) Inviter() **RoomMember {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalRoomMemberINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_inviter(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) IsDirect() bool {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_is_direct(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) IsEncrypted() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_is_encrypted(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue bool
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBoolINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Room) IsPublic() bool {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_is_public(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) IsSpace() bool {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_is_space(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) IsTombstoned() bool {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_is_tombstoned(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) Join() error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_join(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) JoinedMembersCount() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_joined_members_count(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) KickUser(userId string, reason *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_kick_user(
				_pointer, FfiConverterStringINSTANCE.Lower(userId), FfiConverterOptionalStringINSTANCE.Lower(reason),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) Leave() error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_leave(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) MarkAsRead(receiptType ReceiptType) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_mark_as_read(
				_pointer, FfiConverterTypeReceiptTypeINSTANCE.Lower(receiptType),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) Member(userId string) (*RoomMember, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_member(
				_pointer, FfiConverterStringINSTANCE.Lower(userId),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterRoomMemberINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) MemberAvatarUrl(userId string) (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_member_avatar_url(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Room) MemberDisplayName(userId string) (*string, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_member_display_name(
			_pointer, FfiConverterStringINSTANCE.Lower(userId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOptionalStringINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Room) Members() (*RoomMembersIterator, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_members(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterRoomMembersIteratorINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) MembersNoSync() (*RoomMembersIterator, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_members_no_sync(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterRoomMembersIteratorINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) Membership() Membership {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeMembershipINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_membership(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) Name() *string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_name(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) OwnUserId() string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_own_user_id(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) Redact(eventId string, reason *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_redact(
			_pointer, FfiConverterStringINSTANCE.Lower(eventId), FfiConverterOptionalStringINSTANCE.Lower(reason), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) RemoveAvatar() error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_remove_avatar(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) ReportContent(eventId string, score *int32, reason *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_report_content(
			_pointer, FfiConverterStringINSTANCE.Lower(eventId), FfiConverterOptionalInt32INSTANCE.Lower(score), FfiConverterOptionalStringINSTANCE.Lower(reason), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) RoomInfo() (RoomInfo, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_room_info(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterTypeRoomInfoINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) SetIsFavourite(isFavourite bool, tagOrder *float64) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_set_is_favourite(
				_pointer, FfiConverterBoolINSTANCE.Lower(isFavourite), FfiConverterOptionalFloat64INSTANCE.Lower(tagOrder),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) SetIsLowPriority(isLowPriority bool, tagOrder *float64) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_set_is_low_priority(
				_pointer, FfiConverterBoolINSTANCE.Lower(isLowPriority), FfiConverterOptionalFloat64INSTANCE.Lower(tagOrder),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) SetName(name string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_set_name(
			_pointer, FfiConverterStringINSTANCE.Lower(name), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) SetTopic(topic string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_set_topic(
			_pointer, FfiConverterStringINSTANCE.Lower(topic), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Room) SetUnreadFlag(newValue bool) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_set_unread_flag(
				_pointer, FfiConverterBoolINSTANCE.Lower(newValue),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) SubscribeToRoomInfoUpdates(listener RoomInfoListener) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_subscribe_to_room_info_updates(
			_pointer, FfiConverterCallbackInterfaceRoomInfoListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *Room) SubscribeToTypingNotifications(listener TypingNotificationsListener) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_subscribe_to_typing_notifications(
			_pointer, FfiConverterCallbackInterfaceTypingNotificationsListenerINSTANCE.Lower(listener),
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterTaskHandleINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) Timeline() (*Timeline, error) {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_timeline(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterTimelineINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) Topic() *string {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_room_topic(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Room) TypingNotice(isTyping bool) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_typing_notice(
				_pointer, FfiConverterBoolINSTANCE.Lower(isTyping),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) UnbanUser(userId string, reason *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_unban_user(
				_pointer, FfiConverterStringINSTANCE.Lower(userId), FfiConverterOptionalStringINSTANCE.Lower(reason),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) UpdatePowerLevelForUser(userId string, powerLevel int64) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_room_update_power_level_for_user(
				_pointer, FfiConverterStringINSTANCE.Lower(userId), FfiConverterInt64INSTANCE.Lower(powerLevel),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Room) UploadAvatar(mimeType string, data []byte, mediaInfo *ImageInfo) error {
	_pointer := _self.ffiObject.incrementPointer("*Room")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_room_upload_avatar(
			_pointer, FfiConverterStringINSTANCE.Lower(mimeType), FfiConverterBytesINSTANCE.Lower(data), FfiConverterOptionalTypeImageInfoINSTANCE.Lower(mediaInfo), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (object *Room) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoom struct{}

var FfiConverterRoomINSTANCE = FfiConverterRoom{}

func (c FfiConverterRoom) Lift(pointer unsafe.Pointer) *Room {
	result := &Room{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_room(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*Room).Destroy)
	return result
}

func (c FfiConverterRoom) Read(reader io.Reader) *Room {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoom) Lower(value *Room) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Room")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoom) Write(writer io.Writer, value *Room) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoom struct{}

func (_ FfiDestroyerRoom) Destroy(value *Room) {
	value.Destroy()
}

type RoomList struct {
	ffiObject FfiObject
}

func (_self *RoomList) Entries(listener RoomListEntriesListener) RoomListEntriesResult {
	_pointer := _self.ffiObject.incrementPointer("*RoomList")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeRoomListEntriesResultINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlist_entries(
			_pointer, FfiConverterCallbackInterfaceRoomListEntriesListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *RoomList) EntriesWithDynamicAdapters(pageSize uint32, listener RoomListEntriesListener) RoomListEntriesWithDynamicAdaptersResult {
	_pointer := _self.ffiObject.incrementPointer("*RoomList")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeRoomListEntriesWithDynamicAdaptersResultINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlist_entries_with_dynamic_adapters(
			_pointer, FfiConverterUint32INSTANCE.Lower(pageSize), FfiConverterCallbackInterfaceRoomListEntriesListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *RoomList) LoadingState(listener RoomListLoadingStateListener) (RoomListLoadingStateResult, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomList")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeRoomListError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlist_loading_state(
			_pointer, FfiConverterCallbackInterfaceRoomListLoadingStateListenerINSTANCE.Lower(listener), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue RoomListLoadingStateResult
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeRoomListLoadingStateResultINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *RoomList) Room(roomId string) (*RoomListItem, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomList")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeRoomListError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlist_room(
			_pointer, FfiConverterStringINSTANCE.Lower(roomId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *RoomListItem
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterRoomListItemINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (object *RoomList) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomList struct{}

var FfiConverterRoomListINSTANCE = FfiConverterRoomList{}

func (c FfiConverterRoomList) Lift(pointer unsafe.Pointer) *RoomList {
	result := &RoomList{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roomlist(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomList).Destroy)
	return result
}

func (c FfiConverterRoomList) Read(reader io.Reader) *RoomList {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomList) Lower(value *RoomList) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomList")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomList) Write(writer io.Writer, value *RoomList) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomList struct{}

func (_ FfiDestroyerRoomList) Destroy(value *RoomList) {
	value.Destroy()
}

type RoomListDynamicEntriesController struct {
	ffiObject FfiObject
}

func (_self *RoomListDynamicEntriesController) AddOnePage() {
	_pointer := _self.ffiObject.incrementPointer("*RoomListDynamicEntriesController")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_roomlistdynamicentriescontroller_add_one_page(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *RoomListDynamicEntriesController) ResetToOnePage() {
	_pointer := _self.ffiObject.incrementPointer("*RoomListDynamicEntriesController")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_roomlistdynamicentriescontroller_reset_to_one_page(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *RoomListDynamicEntriesController) SetFilter(kind RoomListEntriesDynamicFilterKind) bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomListDynamicEntriesController")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistdynamicentriescontroller_set_filter(
			_pointer, FfiConverterTypeRoomListEntriesDynamicFilterKindINSTANCE.Lower(kind), _uniffiStatus)
	}))
}

func (object *RoomListDynamicEntriesController) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomListDynamicEntriesController struct{}

var FfiConverterRoomListDynamicEntriesControllerINSTANCE = FfiConverterRoomListDynamicEntriesController{}

func (c FfiConverterRoomListDynamicEntriesController) Lift(pointer unsafe.Pointer) *RoomListDynamicEntriesController {
	result := &RoomListDynamicEntriesController{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roomlistdynamicentriescontroller(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomListDynamicEntriesController).Destroy)
	return result
}

func (c FfiConverterRoomListDynamicEntriesController) Read(reader io.Reader) *RoomListDynamicEntriesController {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomListDynamicEntriesController) Lower(value *RoomListDynamicEntriesController) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomListDynamicEntriesController")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomListDynamicEntriesController) Write(writer io.Writer, value *RoomListDynamicEntriesController) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomListDynamicEntriesController struct{}

func (_ FfiDestroyerRoomListDynamicEntriesController) Destroy(value *RoomListDynamicEntriesController) {
	value.Destroy()
}

type RoomListItem struct {
	ffiObject FfiObject
}

func (_self *RoomListItem) AvatarUrl() *string {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_avatar_url(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomListItem) CanonicalAlias() *string {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_canonical_alias(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomListItem) FullRoom() (*Room, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRoomListError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_full_room(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterRoomINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListItem) Id() string {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_id(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomListItem) InitTimeline(eventTypeFilter **TimelineEventTypeFilter) error {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeRoomListError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_init_timeline(
				_pointer, FfiConverterOptionalTimelineEventTypeFilterINSTANCE.Lower(eventTypeFilter),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListItem) IsDirect() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_is_direct(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomListItem) IsTimelineInitialized() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_is_timeline_initialized(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomListItem) LatestEvent() **EventTimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_latest_event(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterOptionalEventTimelineItemINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListItem) Name() *string {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_name(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomListItem) RoomInfo() (RoomInfo, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_room_info(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterTypeRoomInfoINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListItem) Subscribe(settings *RoomSubscription) {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_subscribe(
			_pointer, FfiConverterOptionalTypeRoomSubscriptionINSTANCE.Lower(settings), _uniffiStatus)
		return false
	})
}

func (_self *RoomListItem) Unsubscribe() {
	_pointer := _self.ffiObject.incrementPointer("*RoomListItem")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_roomlistitem_unsubscribe(
			_pointer, _uniffiStatus)
		return false
	})
}

func (object *RoomListItem) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomListItem struct{}

var FfiConverterRoomListItemINSTANCE = FfiConverterRoomListItem{}

func (c FfiConverterRoomListItem) Lift(pointer unsafe.Pointer) *RoomListItem {
	result := &RoomListItem{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roomlistitem(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomListItem).Destroy)
	return result
}

func (c FfiConverterRoomListItem) Read(reader io.Reader) *RoomListItem {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomListItem) Lower(value *RoomListItem) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomListItem")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomListItem) Write(writer io.Writer, value *RoomListItem) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomListItem struct{}

func (_ FfiDestroyerRoomListItem) Destroy(value *RoomListItem) {
	value.Destroy()
}

type RoomListService struct {
	ffiObject FfiObject
}

func (_self *RoomListService) AllRooms() (*RoomList, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomListService")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRoomListError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistservice_all_rooms(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterRoomListINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListService) ApplyInput(input RoomListInput) error {
	_pointer := _self.ffiObject.incrementPointer("*RoomListService")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeRoomListError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistservice_apply_input(
				_pointer, FfiConverterTypeRoomListInputINSTANCE.Lower(input),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListService) Invites() (*RoomList, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomListService")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeRoomListError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_roomlistservice_invites(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterRoomListINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *RoomListService) Room(roomId string) (*RoomListItem, error) {
	_pointer := _self.ffiObject.incrementPointer("*RoomListService")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeRoomListError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistservice_room(
			_pointer, FfiConverterStringINSTANCE.Lower(roomId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *RoomListItem
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterRoomListItemINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *RoomListService) State(listener RoomListServiceStateListener) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*RoomListService")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistservice_state(
			_pointer, FfiConverterCallbackInterfaceRoomListServiceStateListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *RoomListService) SyncIndicator(delayBeforeShowingInMs uint32, delayBeforeHidingInMs uint32, listener RoomListServiceSyncIndicatorListener) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*RoomListService")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_roomlistservice_sync_indicator(
			_pointer, FfiConverterUint32INSTANCE.Lower(delayBeforeShowingInMs), FfiConverterUint32INSTANCE.Lower(delayBeforeHidingInMs), FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListenerINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (object *RoomListService) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomListService struct{}

var FfiConverterRoomListServiceINSTANCE = FfiConverterRoomListService{}

func (c FfiConverterRoomListService) Lift(pointer unsafe.Pointer) *RoomListService {
	result := &RoomListService{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roomlistservice(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomListService).Destroy)
	return result
}

func (c FfiConverterRoomListService) Read(reader io.Reader) *RoomListService {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomListService) Lower(value *RoomListService) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomListService")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomListService) Write(writer io.Writer, value *RoomListService) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomListService struct{}

func (_ FfiDestroyerRoomListService) Destroy(value *RoomListService) {
	value.Destroy()
}

type RoomMember struct {
	ffiObject FfiObject
}

func (_self *RoomMember) AvatarUrl() *string {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_avatar_url(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) CanBan() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_ban(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) CanInvite() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_invite(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) CanKick() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_kick(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) CanRedactOther() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_redact_other(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) CanRedactOwn() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_redact_own(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) CanSendMessage(event MessageLikeEventType) bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_send_message(
			_pointer, FfiConverterTypeMessageLikeEventTypeINSTANCE.Lower(event), _uniffiStatus)
	}))
}

func (_self *RoomMember) CanSendState(stateEvent StateEventType) bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_send_state(
			_pointer, FfiConverterTypeStateEventTypeINSTANCE.Lower(stateEvent), _uniffiStatus)
	}))
}

func (_self *RoomMember) CanTriggerRoomNotification() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_can_trigger_room_notification(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) DisplayName() *string {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_display_name(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) Ignore() error {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_roommember_ignore(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *RoomMember) IsAccountUser() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_is_account_user(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) IsIgnored() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_is_ignored(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) IsNameAmbiguous() bool {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_is_name_ambiguous(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) Membership() MembershipState {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeMembershipStateINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_membership(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) NormalizedPowerLevel() int64 {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterInt64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_normalized_power_level(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) PowerLevel() int64 {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterInt64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_power_level(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) SuggestedRoleForPowerLevel() matrix_sdk.RoomMemberRole {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return matrix_sdk.FfiConverterTypeRoomMemberRoleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_suggested_role_for_power_level(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMember) Unignore() error {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_roommember_unignore(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *RoomMember) UserId() string {
	_pointer := _self.ffiObject.incrementPointer("*RoomMember")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommember_user_id(
			_pointer, _uniffiStatus)
	}))
}

func (object *RoomMember) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomMember struct{}

var FfiConverterRoomMemberINSTANCE = FfiConverterRoomMember{}

func (c FfiConverterRoomMember) Lift(pointer unsafe.Pointer) *RoomMember {
	result := &RoomMember{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roommember(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomMember).Destroy)
	return result
}

func (c FfiConverterRoomMember) Read(reader io.Reader) *RoomMember {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomMember) Lower(value *RoomMember) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomMember")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomMember) Write(writer io.Writer, value *RoomMember) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomMember struct{}

func (_ FfiDestroyerRoomMember) Destroy(value *RoomMember) {
	value.Destroy()
}

type RoomMembersIterator struct {
	ffiObject FfiObject
}

func (_self *RoomMembersIterator) Len() uint32 {
	_pointer := _self.ffiObject.incrementPointer("*RoomMembersIterator")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint32INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommembersiterator_len(
			_pointer, _uniffiStatus)
	}))
}

func (_self *RoomMembersIterator) NextChunk(chunkSize uint32) *[]*RoomMember {
	_pointer := _self.ffiObject.incrementPointer("*RoomMembersIterator")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceRoomMemberINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommembersiterator_next_chunk(
			_pointer, FfiConverterUint32INSTANCE.Lower(chunkSize), _uniffiStatus)
	}))
}

func (object *RoomMembersIterator) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomMembersIterator struct{}

var FfiConverterRoomMembersIteratorINSTANCE = FfiConverterRoomMembersIterator{}

func (c FfiConverterRoomMembersIterator) Lift(pointer unsafe.Pointer) *RoomMembersIterator {
	result := &RoomMembersIterator{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roommembersiterator(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomMembersIterator).Destroy)
	return result
}

func (c FfiConverterRoomMembersIterator) Read(reader io.Reader) *RoomMembersIterator {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomMembersIterator) Lower(value *RoomMembersIterator) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomMembersIterator")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomMembersIterator) Write(writer io.Writer, value *RoomMembersIterator) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomMembersIterator struct{}

func (_ FfiDestroyerRoomMembersIterator) Destroy(value *RoomMembersIterator) {
	value.Destroy()
}

type RoomMessageEventContentWithoutRelation struct {
	ffiObject FfiObject
}

func (_self *RoomMessageEventContentWithoutRelation) WithMentions(mentions Mentions) *RoomMessageEventContentWithoutRelation {
	_pointer := _self.ffiObject.incrementPointer("*RoomMessageEventContentWithoutRelation")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_roommessageeventcontentwithoutrelation_with_mentions(
			_pointer, FfiConverterTypeMentionsINSTANCE.Lower(mentions), _uniffiStatus)
	}))
}

func (object *RoomMessageEventContentWithoutRelation) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRoomMessageEventContentWithoutRelation struct{}

var FfiConverterRoomMessageEventContentWithoutRelationINSTANCE = FfiConverterRoomMessageEventContentWithoutRelation{}

func (c FfiConverterRoomMessageEventContentWithoutRelation) Lift(pointer unsafe.Pointer) *RoomMessageEventContentWithoutRelation {
	result := &RoomMessageEventContentWithoutRelation{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_roommessageeventcontentwithoutrelation(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*RoomMessageEventContentWithoutRelation).Destroy)
	return result
}

func (c FfiConverterRoomMessageEventContentWithoutRelation) Read(reader io.Reader) *RoomMessageEventContentWithoutRelation {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRoomMessageEventContentWithoutRelation) Lower(value *RoomMessageEventContentWithoutRelation) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*RoomMessageEventContentWithoutRelation")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterRoomMessageEventContentWithoutRelation) Write(writer io.Writer, value *RoomMessageEventContentWithoutRelation) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRoomMessageEventContentWithoutRelation struct{}

func (_ FfiDestroyerRoomMessageEventContentWithoutRelation) Destroy(value *RoomMessageEventContentWithoutRelation) {
	value.Destroy()
}

type SendAttachmentJoinHandle struct {
	ffiObject FfiObject
}

func (_self *SendAttachmentJoinHandle) Cancel() {
	_pointer := _self.ffiObject.incrementPointer("*SendAttachmentJoinHandle")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_sendattachmentjoinhandle_cancel(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *SendAttachmentJoinHandle) Join() error {
	_pointer := _self.ffiObject.incrementPointer("*SendAttachmentJoinHandle")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeRoomError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sendattachmentjoinhandle_join(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}

func (object *SendAttachmentJoinHandle) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSendAttachmentJoinHandle struct{}

var FfiConverterSendAttachmentJoinHandleINSTANCE = FfiConverterSendAttachmentJoinHandle{}

func (c FfiConverterSendAttachmentJoinHandle) Lift(pointer unsafe.Pointer) *SendAttachmentJoinHandle {
	result := &SendAttachmentJoinHandle{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_sendattachmentjoinhandle(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*SendAttachmentJoinHandle).Destroy)
	return result
}

func (c FfiConverterSendAttachmentJoinHandle) Read(reader io.Reader) *SendAttachmentJoinHandle {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSendAttachmentJoinHandle) Lower(value *SendAttachmentJoinHandle) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*SendAttachmentJoinHandle")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterSendAttachmentJoinHandle) Write(writer io.Writer, value *SendAttachmentJoinHandle) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSendAttachmentJoinHandle struct{}

func (_ FfiDestroyerSendAttachmentJoinHandle) Destroy(value *SendAttachmentJoinHandle) {
	value.Destroy()
}

type SessionVerificationController struct {
	ffiObject FfiObject
}

func (_self *SessionVerificationController) ApproveVerification() error {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_approve_verification(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SessionVerificationController) CancelVerification() error {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_cancel_verification(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SessionVerificationController) DeclineVerification() error {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_decline_verification(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SessionVerificationController) IsVerified() (bool, error) {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_is_verified(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SessionVerificationController) RequestVerification() error {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_request_verification(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SessionVerificationController) SetDelegate(delegate *SessionVerificationControllerDelegate) {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_set_delegate(
			_pointer, FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegateINSTANCE.Lower(delegate), _uniffiStatus)
		return false
	})
}

func (_self *SessionVerificationController) StartSasVerification() error {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationController")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationcontroller_start_sas_verification(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}

func (object *SessionVerificationController) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSessionVerificationController struct{}

var FfiConverterSessionVerificationControllerINSTANCE = FfiConverterSessionVerificationController{}

func (c FfiConverterSessionVerificationController) Lift(pointer unsafe.Pointer) *SessionVerificationController {
	result := &SessionVerificationController{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_sessionverificationcontroller(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*SessionVerificationController).Destroy)
	return result
}

func (c FfiConverterSessionVerificationController) Read(reader io.Reader) *SessionVerificationController {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSessionVerificationController) Lower(value *SessionVerificationController) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*SessionVerificationController")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterSessionVerificationController) Write(writer io.Writer, value *SessionVerificationController) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSessionVerificationController struct{}

func (_ FfiDestroyerSessionVerificationController) Destroy(value *SessionVerificationController) {
	value.Destroy()
}

type SessionVerificationEmoji struct {
	ffiObject FfiObject
}

func (_self *SessionVerificationEmoji) Description() string {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationEmoji")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationemoji_description(
			_pointer, _uniffiStatus)
	}))
}

func (_self *SessionVerificationEmoji) Symbol() string {
	_pointer := _self.ffiObject.incrementPointer("*SessionVerificationEmoji")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_sessionverificationemoji_symbol(
			_pointer, _uniffiStatus)
	}))
}

func (object *SessionVerificationEmoji) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSessionVerificationEmoji struct{}

var FfiConverterSessionVerificationEmojiINSTANCE = FfiConverterSessionVerificationEmoji{}

func (c FfiConverterSessionVerificationEmoji) Lift(pointer unsafe.Pointer) *SessionVerificationEmoji {
	result := &SessionVerificationEmoji{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_sessionverificationemoji(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*SessionVerificationEmoji).Destroy)
	return result
}

func (c FfiConverterSessionVerificationEmoji) Read(reader io.Reader) *SessionVerificationEmoji {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSessionVerificationEmoji) Lower(value *SessionVerificationEmoji) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*SessionVerificationEmoji")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterSessionVerificationEmoji) Write(writer io.Writer, value *SessionVerificationEmoji) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSessionVerificationEmoji struct{}

func (_ FfiDestroyerSessionVerificationEmoji) Destroy(value *SessionVerificationEmoji) {
	value.Destroy()
}

type Span struct {
	ffiObject FfiObject
}

func NewSpan(file string, line *uint32, level LogLevel, target string, name string) *Span {
	return FfiConverterSpanINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_span_new(FfiConverterStringINSTANCE.Lower(file), FfiConverterOptionalUint32INSTANCE.Lower(line), FfiConverterTypeLogLevelINSTANCE.Lower(level), FfiConverterStringINSTANCE.Lower(target), FfiConverterStringINSTANCE.Lower(name), _uniffiStatus)
	}))
}

func SpanCurrent() *Span {
	return FfiConverterSpanINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_span_current(_uniffiStatus)
	}))
}

func (_self *Span) Enter() {
	_pointer := _self.ffiObject.incrementPointer("*Span")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_span_enter(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *Span) Exit() {
	_pointer := _self.ffiObject.incrementPointer("*Span")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_span_exit(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *Span) IsNone() bool {
	_pointer := _self.ffiObject.incrementPointer("*Span")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_span_is_none(
			_pointer, _uniffiStatus)
	}))
}

func (object *Span) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSpan struct{}

var FfiConverterSpanINSTANCE = FfiConverterSpan{}

func (c FfiConverterSpan) Lift(pointer unsafe.Pointer) *Span {
	result := &Span{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_span(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*Span).Destroy)
	return result
}

func (c FfiConverterSpan) Read(reader io.Reader) *Span {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSpan) Lower(value *Span) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Span")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterSpan) Write(writer io.Writer, value *Span) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSpan struct{}

func (_ FfiDestroyerSpan) Destroy(value *Span) {
	value.Destroy()
}

type SyncService struct {
	ffiObject FfiObject
}

func (_self *SyncService) RoomListService() *RoomListService {
	_pointer := _self.ffiObject.incrementPointer("*SyncService")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterRoomListServiceINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_syncservice_room_list_service(
			_pointer, _uniffiStatus)
	}))
}

func (_self *SyncService) Start() {
	_pointer := _self.ffiObject.incrementPointer("*SyncService")
	defer _self.ffiObject.decrementPointer()
	uniffiRustCallAsync(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_syncservice_start(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SyncService) State(listener SyncServiceStateObserver) *TaskHandle {
	_pointer := _self.ffiObject.incrementPointer("*SyncService")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTaskHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_syncservice_state(
			_pointer, FfiConverterCallbackInterfaceSyncServiceStateObserverINSTANCE.Lower(listener), _uniffiStatus)
	}))
}

func (_self *SyncService) Stop() error {
	_pointer := _self.ffiObject.incrementPointer("*SyncService")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_syncservice_stop(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}

func (object *SyncService) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSyncService struct{}

var FfiConverterSyncServiceINSTANCE = FfiConverterSyncService{}

func (c FfiConverterSyncService) Lift(pointer unsafe.Pointer) *SyncService {
	result := &SyncService{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_syncservice(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*SyncService).Destroy)
	return result
}

func (c FfiConverterSyncService) Read(reader io.Reader) *SyncService {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSyncService) Lower(value *SyncService) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*SyncService")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterSyncService) Write(writer io.Writer, value *SyncService) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSyncService struct{}

func (_ FfiDestroyerSyncService) Destroy(value *SyncService) {
	value.Destroy()
}

type SyncServiceBuilder struct {
	ffiObject FfiObject
}

func (_self *SyncServiceBuilder) Finish() (*SyncService, error) {
	_pointer := _self.ffiObject.incrementPointer("*SyncServiceBuilder")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_syncservicebuilder_finish(
				_pointer,
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_pointer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) unsafe.Pointer {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_pointer(unsafe.Pointer(handle), status)
		},
		FfiConverterSyncServiceINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_pointer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *SyncServiceBuilder) WithCrossProcessLock(appIdentifier *string) *SyncServiceBuilder {
	_pointer := _self.ffiObject.incrementPointer("*SyncServiceBuilder")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSyncServiceBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_syncservicebuilder_with_cross_process_lock(
			_pointer, FfiConverterOptionalStringINSTANCE.Lower(appIdentifier), _uniffiStatus)
	}))
}

func (object *SyncServiceBuilder) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSyncServiceBuilder struct{}

var FfiConverterSyncServiceBuilderINSTANCE = FfiConverterSyncServiceBuilder{}

func (c FfiConverterSyncServiceBuilder) Lift(pointer unsafe.Pointer) *SyncServiceBuilder {
	result := &SyncServiceBuilder{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_syncservicebuilder(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*SyncServiceBuilder).Destroy)
	return result
}

func (c FfiConverterSyncServiceBuilder) Read(reader io.Reader) *SyncServiceBuilder {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSyncServiceBuilder) Lower(value *SyncServiceBuilder) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*SyncServiceBuilder")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterSyncServiceBuilder) Write(writer io.Writer, value *SyncServiceBuilder) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSyncServiceBuilder struct{}

func (_ FfiDestroyerSyncServiceBuilder) Destroy(value *SyncServiceBuilder) {
	value.Destroy()
}

type TaskHandle struct {
	ffiObject FfiObject
}

func (_self *TaskHandle) Cancel() {
	_pointer := _self.ffiObject.incrementPointer("*TaskHandle")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_taskhandle_cancel(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *TaskHandle) IsFinished() bool {
	_pointer := _self.ffiObject.incrementPointer("*TaskHandle")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_taskhandle_is_finished(
			_pointer, _uniffiStatus)
	}))
}

func (object *TaskHandle) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTaskHandle struct{}

var FfiConverterTaskHandleINSTANCE = FfiConverterTaskHandle{}

func (c FfiConverterTaskHandle) Lift(pointer unsafe.Pointer) *TaskHandle {
	result := &TaskHandle{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_taskhandle(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*TaskHandle).Destroy)
	return result
}

func (c FfiConverterTaskHandle) Read(reader io.Reader) *TaskHandle {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTaskHandle) Lower(value *TaskHandle) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*TaskHandle")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTaskHandle) Write(writer io.Writer, value *TaskHandle) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTaskHandle struct{}

func (_ FfiDestroyerTaskHandle) Destroy(value *TaskHandle) {
	value.Destroy()
}

type Timeline struct {
	ffiObject FfiObject
}

func (_self *Timeline) AddListener(listener TimelineListener) RoomTimelineListenerResult {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_timeline_add_listener(
			_pointer, FfiConverterCallbackInterfaceTimelineListenerINSTANCE.Lower(listener),
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterTypeRoomTimelineListenerResultINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Timeline) CancelSend(txnId string) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_cancel_send(
			_pointer, FfiConverterStringINSTANCE.Lower(txnId), _uniffiStatus)
		return false
	})
}

func (_self *Timeline) CreatePoll(question string, answers []string, maxSelections uint8, pollKind PollKind) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_create_poll(
			_pointer, FfiConverterStringINSTANCE.Lower(question), FfiConverterSequenceStringINSTANCE.Lower(answers), FfiConverterUint8INSTANCE.Lower(maxSelections), FfiConverterTypePollKindINSTANCE.Lower(pollKind), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) Edit(newContent *RoomMessageEventContentWithoutRelation, editItem *EventTimelineItem) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_edit(
			_pointer, FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lower(newContent), FfiConverterEventTimelineItemINSTANCE.Lower(editItem), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) EditPoll(question string, answers []string, maxSelections uint8, pollKind PollKind, editItem *EventTimelineItem) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_timeline_edit_poll(
				_pointer, FfiConverterStringINSTANCE.Lower(question), FfiConverterSequenceStringINSTANCE.Lower(answers), FfiConverterUint8INSTANCE.Lower(maxSelections), FfiConverterTypePollKindINSTANCE.Lower(pollKind), FfiConverterEventTimelineItemINSTANCE.Lower(editItem),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Timeline) EndPoll(pollStartId string, text string) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_end_poll(
			_pointer, FfiConverterStringINSTANCE.Lower(pollStartId), FfiConverterStringINSTANCE.Lower(text), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) FetchDetailsForEvent(eventId string) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_fetch_details_for_event(
			_pointer, FfiConverterStringINSTANCE.Lower(eventId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) FetchMembers() {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	uniffiRustCallAsync(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_timeline_fetch_members(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Timeline) GetEventTimelineItemByEventId(eventId string) (*EventTimelineItem, error) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_get_event_timeline_item_by_event_id(
			_pointer, FfiConverterStringINSTANCE.Lower(eventId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *EventTimelineItem
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterEventTimelineItemINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Timeline) GetTimelineEventContentByEventId(eventId string) (*RoomMessageEventContentWithoutRelation, error) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_get_timeline_event_content_by_event_id(
			_pointer, FfiConverterStringINSTANCE.Lower(eventId), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *RoomMessageEventContentWithoutRelation
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Timeline) LatestEvent() **EventTimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_timeline_latest_event(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterOptionalEventTimelineItemINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Timeline) MarkAsRead(receiptType ReceiptType) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithError(
		FfiConverterTypeClientError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_timeline_mark_as_read(
				_pointer, FfiConverterTypeReceiptTypeINSTANCE.Lower(receiptType),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *Timeline) PaginateBackwards(opts PaginationOptions) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_paginate_backwards(
			_pointer, FfiConverterTypePaginationOptionsINSTANCE.Lower(opts), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) RetryDecryption(sessionIds []string) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_retry_decryption(
			_pointer, FfiConverterSequenceStringINSTANCE.Lower(sessionIds), _uniffiStatus)
		return false
	})
}

func (_self *Timeline) RetrySend(txnId string) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_retry_send(
			_pointer, FfiConverterStringINSTANCE.Lower(txnId), _uniffiStatus)
		return false
	})
}

func (_self *Timeline) Send(msg *RoomMessageEventContentWithoutRelation) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_send(
			_pointer, FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lower(msg), _uniffiStatus)
		return false
	})
}

func (_self *Timeline) SendAudio(url string, audioInfo AudioInfo, progressWatcher *ProgressWatcher) *SendAttachmentJoinHandle {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSendAttachmentJoinHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_audio(
			_pointer, FfiConverterStringINSTANCE.Lower(url), FfiConverterTypeAudioInfoINSTANCE.Lower(audioInfo), FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE.Lower(progressWatcher), _uniffiStatus)
	}))
}

func (_self *Timeline) SendFile(url string, fileInfo FileInfo, progressWatcher *ProgressWatcher) *SendAttachmentJoinHandle {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSendAttachmentJoinHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_file(
			_pointer, FfiConverterStringINSTANCE.Lower(url), FfiConverterTypeFileInfoINSTANCE.Lower(fileInfo), FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE.Lower(progressWatcher), _uniffiStatus)
	}))
}

func (_self *Timeline) SendImage(url string, thumbnailUrl *string, imageInfo ImageInfo, progressWatcher *ProgressWatcher) *SendAttachmentJoinHandle {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSendAttachmentJoinHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_image(
			_pointer, FfiConverterStringINSTANCE.Lower(url), FfiConverterOptionalStringINSTANCE.Lower(thumbnailUrl), FfiConverterTypeImageInfoINSTANCE.Lower(imageInfo), FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE.Lower(progressWatcher), _uniffiStatus)
	}))
}

func (_self *Timeline) SendLocation(body string, geoUri string, description *string, zoomLevel *uint8, assetType *AssetType) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_location(
			_pointer, FfiConverterStringINSTANCE.Lower(body), FfiConverterStringINSTANCE.Lower(geoUri), FfiConverterOptionalStringINSTANCE.Lower(description), FfiConverterOptionalUint8INSTANCE.Lower(zoomLevel), FfiConverterOptionalTypeAssetTypeINSTANCE.Lower(assetType), _uniffiStatus)
		return false
	})
}

func (_self *Timeline) SendPollResponse(pollStartId string, answers []string) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_poll_response(
			_pointer, FfiConverterStringINSTANCE.Lower(pollStartId), FfiConverterSequenceStringINSTANCE.Lower(answers), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) SendReadReceipt(receiptType ReceiptType, eventId string) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_read_receipt(
			_pointer, FfiConverterTypeReceiptTypeINSTANCE.Lower(receiptType), FfiConverterStringINSTANCE.Lower(eventId), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) SendReply(msg *RoomMessageEventContentWithoutRelation, replyItem *EventTimelineItem) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_reply(
			_pointer, FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lower(msg), FfiConverterEventTimelineItemINSTANCE.Lower(replyItem), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (_self *Timeline) SendVideo(url string, thumbnailUrl *string, videoInfo VideoInfo, progressWatcher *ProgressWatcher) *SendAttachmentJoinHandle {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSendAttachmentJoinHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_video(
			_pointer, FfiConverterStringINSTANCE.Lower(url), FfiConverterOptionalStringINSTANCE.Lower(thumbnailUrl), FfiConverterTypeVideoInfoINSTANCE.Lower(videoInfo), FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE.Lower(progressWatcher), _uniffiStatus)
	}))
}

func (_self *Timeline) SendVoiceMessage(url string, audioInfo AudioInfo, waveform []uint16, progressWatcher *ProgressWatcher) *SendAttachmentJoinHandle {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSendAttachmentJoinHandleINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_send_voice_message(
			_pointer, FfiConverterStringINSTANCE.Lower(url), FfiConverterTypeAudioInfoINSTANCE.Lower(audioInfo), FfiConverterSequenceUint16INSTANCE.Lower(waveform), FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE.Lower(progressWatcher), _uniffiStatus)
	}))
}

func (_self *Timeline) SubscribeToBackPaginationStatus(listener BackPaginationStatusListener) (*TaskHandle, error) {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_method_timeline_subscribe_to_back_pagination_status(
			_pointer, FfiConverterCallbackInterfaceBackPaginationStatusListenerINSTANCE.Lower(listener), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *TaskHandle
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTaskHandleINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *Timeline) ToggleReaction(eventId string, key string) error {
	_pointer := _self.ffiObject.incrementPointer("*Timeline")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_method_timeline_toggle_reaction(
			_pointer, FfiConverterStringINSTANCE.Lower(eventId), FfiConverterStringINSTANCE.Lower(key), _uniffiStatus)
		return false
	})
	return _uniffiErr
}

func (object *Timeline) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTimeline struct{}

var FfiConverterTimelineINSTANCE = FfiConverterTimeline{}

func (c FfiConverterTimeline) Lift(pointer unsafe.Pointer) *Timeline {
	result := &Timeline{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_timeline(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*Timeline).Destroy)
	return result
}

func (c FfiConverterTimeline) Read(reader io.Reader) *Timeline {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTimeline) Lower(value *Timeline) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Timeline")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTimeline) Write(writer io.Writer, value *Timeline) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTimeline struct{}

func (_ FfiDestroyerTimeline) Destroy(value *Timeline) {
	value.Destroy()
}

type TimelineDiff struct {
	ffiObject FfiObject
}

func (_self *TimelineDiff) Append() *[]*TimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceTimelineItemINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_append(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) Change() TimelineChange {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeTimelineChangeINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_change(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) Insert() *InsertData {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeInsertDataINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_insert(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) PushBack() **TimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTimelineItemINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_push_back(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) PushFront() **TimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTimelineItemINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_push_front(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) Remove() *uint32 {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint32INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_remove(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) Reset() *[]*TimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceTimelineItemINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_reset(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineDiff) Set() *SetData {
	_pointer := _self.ffiObject.incrementPointer("*TimelineDiff")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeSetDataINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelinediff_set(
			_pointer, _uniffiStatus)
	}))
}

func (object *TimelineDiff) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTimelineDiff struct{}

var FfiConverterTimelineDiffINSTANCE = FfiConverterTimelineDiff{}

func (c FfiConverterTimelineDiff) Lift(pointer unsafe.Pointer) *TimelineDiff {
	result := &TimelineDiff{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_timelinediff(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*TimelineDiff).Destroy)
	return result
}

func (c FfiConverterTimelineDiff) Read(reader io.Reader) *TimelineDiff {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTimelineDiff) Lower(value *TimelineDiff) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*TimelineDiff")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTimelineDiff) Write(writer io.Writer, value *TimelineDiff) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTimelineDiff struct{}

func (_ FfiDestroyerTimelineDiff) Destroy(value *TimelineDiff) {
	value.Destroy()
}

type TimelineEvent struct {
	ffiObject FfiObject
}

func (_self *TimelineEvent) EventId() string {
	_pointer := _self.ffiObject.incrementPointer("*TimelineEvent")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineevent_event_id(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineEvent) EventType() (TimelineEventType, error) {
	_pointer := _self.ffiObject.incrementPointer("*TimelineEvent")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineevent_event_type(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue TimelineEventType
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeTimelineEventTypeINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func (_self *TimelineEvent) SenderId() string {
	_pointer := _self.ffiObject.incrementPointer("*TimelineEvent")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineevent_sender_id(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineEvent) Timestamp() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*TimelineEvent")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineevent_timestamp(
			_pointer, _uniffiStatus)
	}))
}

func (object *TimelineEvent) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTimelineEvent struct{}

var FfiConverterTimelineEventINSTANCE = FfiConverterTimelineEvent{}

func (c FfiConverterTimelineEvent) Lift(pointer unsafe.Pointer) *TimelineEvent {
	result := &TimelineEvent{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_timelineevent(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*TimelineEvent).Destroy)
	return result
}

func (c FfiConverterTimelineEvent) Read(reader io.Reader) *TimelineEvent {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTimelineEvent) Lower(value *TimelineEvent) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*TimelineEvent")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTimelineEvent) Write(writer io.Writer, value *TimelineEvent) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTimelineEvent struct{}

func (_ FfiDestroyerTimelineEvent) Destroy(value *TimelineEvent) {
	value.Destroy()
}

type TimelineEventTypeFilter struct {
	ffiObject FfiObject
}

func TimelineEventTypeFilterExclude(eventTypes []FilterTimelineEventType) *TimelineEventTypeFilter {
	return FfiConverterTimelineEventTypeFilterINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_timelineeventtypefilter_exclude(FfiConverterSequenceTypeFilterTimelineEventTypeINSTANCE.Lower(eventTypes), _uniffiStatus)
	}))
}
func TimelineEventTypeFilterInclude(eventTypes []FilterTimelineEventType) *TimelineEventTypeFilter {
	return FfiConverterTimelineEventTypeFilterINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_constructor_timelineeventtypefilter_include(FfiConverterSequenceTypeFilterTimelineEventTypeINSTANCE.Lower(eventTypes), _uniffiStatus)
	}))
}

func (object *TimelineEventTypeFilter) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTimelineEventTypeFilter struct{}

var FfiConverterTimelineEventTypeFilterINSTANCE = FfiConverterTimelineEventTypeFilter{}

func (c FfiConverterTimelineEventTypeFilter) Lift(pointer unsafe.Pointer) *TimelineEventTypeFilter {
	result := &TimelineEventTypeFilter{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_timelineeventtypefilter(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*TimelineEventTypeFilter).Destroy)
	return result
}

func (c FfiConverterTimelineEventTypeFilter) Read(reader io.Reader) *TimelineEventTypeFilter {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTimelineEventTypeFilter) Lower(value *TimelineEventTypeFilter) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*TimelineEventTypeFilter")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTimelineEventTypeFilter) Write(writer io.Writer, value *TimelineEventTypeFilter) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTimelineEventTypeFilter struct{}

func (_ FfiDestroyerTimelineEventTypeFilter) Destroy(value *TimelineEventTypeFilter) {
	value.Destroy()
}

type TimelineItem struct {
	ffiObject FfiObject
}

func (_self *TimelineItem) AsEvent() **EventTimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*TimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalEventTimelineItemINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineitem_as_event(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineItem) AsVirtual() *VirtualTimelineItem {
	_pointer := _self.ffiObject.incrementPointer("*TimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeVirtualTimelineItemINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineitem_as_virtual(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineItem) FmtDebug() string {
	_pointer := _self.ffiObject.incrementPointer("*TimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineitem_fmt_debug(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineItem) UniqueId() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*TimelineItem")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineitem_unique_id(
			_pointer, _uniffiStatus)
	}))
}

func (object *TimelineItem) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTimelineItem struct{}

var FfiConverterTimelineItemINSTANCE = FfiConverterTimelineItem{}

func (c FfiConverterTimelineItem) Lift(pointer unsafe.Pointer) *TimelineItem {
	result := &TimelineItem{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_timelineitem(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*TimelineItem).Destroy)
	return result
}

func (c FfiConverterTimelineItem) Read(reader io.Reader) *TimelineItem {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTimelineItem) Lower(value *TimelineItem) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*TimelineItem")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTimelineItem) Write(writer io.Writer, value *TimelineItem) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTimelineItem struct{}

func (_ FfiDestroyerTimelineItem) Destroy(value *TimelineItem) {
	value.Destroy()
}

type TimelineItemContent struct {
	ffiObject FfiObject
}

func (_self *TimelineItemContent) AsMessage() **Message {
	_pointer := _self.ffiObject.incrementPointer("*TimelineItemContent")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalMessageINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineitemcontent_as_message(
			_pointer, _uniffiStatus)
	}))
}

func (_self *TimelineItemContent) Kind() TimelineItemContentKind {
	_pointer := _self.ffiObject.incrementPointer("*TimelineItemContent")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeTimelineItemContentKindINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_method_timelineitemcontent_kind(
			_pointer, _uniffiStatus)
	}))
}

func (object *TimelineItemContent) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterTimelineItemContent struct{}

var FfiConverterTimelineItemContentINSTANCE = FfiConverterTimelineItemContent{}

func (c FfiConverterTimelineItemContent) Lift(pointer unsafe.Pointer) *TimelineItemContent {
	result := &TimelineItemContent{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_timelineitemcontent(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*TimelineItemContent).Destroy)
	return result
}

func (c FfiConverterTimelineItemContent) Read(reader io.Reader) *TimelineItemContent {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterTimelineItemContent) Lower(value *TimelineItemContent) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*TimelineItemContent")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterTimelineItemContent) Write(writer io.Writer, value *TimelineItemContent) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerTimelineItemContent struct{}

func (_ FfiDestroyerTimelineItemContent) Destroy(value *TimelineItemContent) {
	value.Destroy()
}

type UnreadNotificationsCount struct {
	ffiObject FfiObject
}

func (_self *UnreadNotificationsCount) HasNotifications() bool {
	_pointer := _self.ffiObject.incrementPointer("*UnreadNotificationsCount")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_unreadnotificationscount_has_notifications(
			_pointer, _uniffiStatus)
	}))
}

func (_self *UnreadNotificationsCount) HighlightCount() uint32 {
	_pointer := _self.ffiObject.incrementPointer("*UnreadNotificationsCount")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint32INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_unreadnotificationscount_highlight_count(
			_pointer, _uniffiStatus)
	}))
}

func (_self *UnreadNotificationsCount) NotificationCount() uint32 {
	_pointer := _self.ffiObject.incrementPointer("*UnreadNotificationsCount")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint32INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.uniffi_matrix_sdk_ffi_fn_method_unreadnotificationscount_notification_count(
			_pointer, _uniffiStatus)
	}))
}

func (object *UnreadNotificationsCount) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterUnreadNotificationsCount struct{}

var FfiConverterUnreadNotificationsCountINSTANCE = FfiConverterUnreadNotificationsCount{}

func (c FfiConverterUnreadNotificationsCount) Lift(pointer unsafe.Pointer) *UnreadNotificationsCount {
	result := &UnreadNotificationsCount{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_unreadnotificationscount(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*UnreadNotificationsCount).Destroy)
	return result
}

func (c FfiConverterUnreadNotificationsCount) Read(reader io.Reader) *UnreadNotificationsCount {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterUnreadNotificationsCount) Lower(value *UnreadNotificationsCount) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*UnreadNotificationsCount")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterUnreadNotificationsCount) Write(writer io.Writer, value *UnreadNotificationsCount) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerUnreadNotificationsCount struct{}

func (_ FfiDestroyerUnreadNotificationsCount) Destroy(value *UnreadNotificationsCount) {
	value.Destroy()
}

type WidgetDriver struct {
	ffiObject FfiObject
}

func (_self *WidgetDriver) Run(room *Room, capabilitiesProvider WidgetCapabilitiesProvider) {
	_pointer := _self.ffiObject.incrementPointer("*WidgetDriver")
	defer _self.ffiObject.decrementPointer()
	uniffiRustCallAsync(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_widgetdriver_run(
			_pointer, FfiConverterRoomINSTANCE.Lower(room), FfiConverterCallbackInterfaceWidgetCapabilitiesProviderINSTANCE.Lower(capabilitiesProvider),
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_void(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) {
			// completeFunc
			C.ffi_matrix_sdk_ffi_rust_future_complete_void(unsafe.Pointer(handle), status)
		},
		func(bool) {}, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_void(unsafe.Pointer(rustFuture), status)
		})
}

func (object *WidgetDriver) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterWidgetDriver struct{}

var FfiConverterWidgetDriverINSTANCE = FfiConverterWidgetDriver{}

func (c FfiConverterWidgetDriver) Lift(pointer unsafe.Pointer) *WidgetDriver {
	result := &WidgetDriver{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_widgetdriver(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*WidgetDriver).Destroy)
	return result
}

func (c FfiConverterWidgetDriver) Read(reader io.Reader) *WidgetDriver {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterWidgetDriver) Lower(value *WidgetDriver) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*WidgetDriver")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterWidgetDriver) Write(writer io.Writer, value *WidgetDriver) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerWidgetDriver struct{}

func (_ FfiDestroyerWidgetDriver) Destroy(value *WidgetDriver) {
	value.Destroy()
}

type WidgetDriverHandle struct {
	ffiObject FfiObject
}

func (_self *WidgetDriverHandle) Recv() *string {
	_pointer := _self.ffiObject.incrementPointer("*WidgetDriverHandle")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_widgetdriverhandle_recv(
			_pointer,
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterOptionalStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}
func (_self *WidgetDriverHandle) Send(msg string) bool {
	_pointer := _self.ffiObject.incrementPointer("*WidgetDriverHandle")
	defer _self.ffiObject.decrementPointer()
	return uniffiRustCallAsyncWithResult(func(status *C.RustCallStatus) *C.void {
		// rustFutureFunc
		return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_method_widgetdriverhandle_send(
			_pointer, FfiConverterStringINSTANCE.Lower(msg),
			status,
		))
	},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_i8(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) C.int8_t {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_i8(unsafe.Pointer(handle), status)
		},
		FfiConverterBoolINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_i8(unsafe.Pointer(rustFuture), status)
		})
}

func (object *WidgetDriverHandle) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterWidgetDriverHandle struct{}

var FfiConverterWidgetDriverHandleINSTANCE = FfiConverterWidgetDriverHandle{}

func (c FfiConverterWidgetDriverHandle) Lift(pointer unsafe.Pointer) *WidgetDriverHandle {
	result := &WidgetDriverHandle{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_matrix_sdk_ffi_fn_free_widgetdriverhandle(pointer, status)
			}),
	}
	runtime.SetFinalizer(result, (*WidgetDriverHandle).Destroy)
	return result
}

func (c FfiConverterWidgetDriverHandle) Read(reader io.Reader) *WidgetDriverHandle {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterWidgetDriverHandle) Lower(value *WidgetDriverHandle) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*WidgetDriverHandle")
	defer value.ffiObject.decrementPointer()
	return pointer
}

func (c FfiConverterWidgetDriverHandle) Write(writer io.Writer, value *WidgetDriverHandle) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerWidgetDriverHandle struct{}

func (_ FfiDestroyerWidgetDriverHandle) Destroy(value *WidgetDriverHandle) {
	value.Destroy()
}

type AudioInfo struct {
	Duration *time.Duration
	Size     *uint64
	Mimetype *string
}

func (r *AudioInfo) Destroy() {
	FfiDestroyerOptionalDuration{}.Destroy(r.Duration)
	FfiDestroyerOptionalUint64{}.Destroy(r.Size)
	FfiDestroyerOptionalString{}.Destroy(r.Mimetype)
}

type FfiConverterTypeAudioInfo struct{}

var FfiConverterTypeAudioInfoINSTANCE = FfiConverterTypeAudioInfo{}

func (c FfiConverterTypeAudioInfo) Lift(rb RustBufferI) AudioInfo {
	return LiftFromRustBuffer[AudioInfo](c, rb)
}

func (c FfiConverterTypeAudioInfo) Read(reader io.Reader) AudioInfo {
	return AudioInfo{
		FfiConverterOptionalDurationINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeAudioInfo) Lower(value AudioInfo) RustBuffer {
	return LowerIntoRustBuffer[AudioInfo](c, value)
}

func (c FfiConverterTypeAudioInfo) Write(writer io.Writer, value AudioInfo) {
	FfiConverterOptionalDurationINSTANCE.Write(writer, value.Duration)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Size)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Mimetype)
}

type FfiDestroyerTypeAudioInfo struct{}

func (_ FfiDestroyerTypeAudioInfo) Destroy(value AudioInfo) {
	value.Destroy()
}

type AudioMessageContent struct {
	Body   string
	Source *MediaSource
	Info   *AudioInfo
	Audio  *UnstableAudioDetailsContent
	Voice  *UnstableVoiceContent
}

func (r *AudioMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerMediaSource{}.Destroy(r.Source)
	FfiDestroyerOptionalTypeAudioInfo{}.Destroy(r.Info)
	FfiDestroyerOptionalTypeUnstableAudioDetailsContent{}.Destroy(r.Audio)
	FfiDestroyerOptionalTypeUnstableVoiceContent{}.Destroy(r.Voice)
}

type FfiConverterTypeAudioMessageContent struct{}

var FfiConverterTypeAudioMessageContentINSTANCE = FfiConverterTypeAudioMessageContent{}

func (c FfiConverterTypeAudioMessageContent) Lift(rb RustBufferI) AudioMessageContent {
	return LiftFromRustBuffer[AudioMessageContent](c, rb)
}

func (c FfiConverterTypeAudioMessageContent) Read(reader io.Reader) AudioMessageContent {
	return AudioMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterMediaSourceINSTANCE.Read(reader),
		FfiConverterOptionalTypeAudioInfoINSTANCE.Read(reader),
		FfiConverterOptionalTypeUnstableAudioDetailsContentINSTANCE.Read(reader),
		FfiConverterOptionalTypeUnstableVoiceContentINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeAudioMessageContent) Lower(value AudioMessageContent) RustBuffer {
	return LowerIntoRustBuffer[AudioMessageContent](c, value)
}

func (c FfiConverterTypeAudioMessageContent) Write(writer io.Writer, value AudioMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterMediaSourceINSTANCE.Write(writer, value.Source)
	FfiConverterOptionalTypeAudioInfoINSTANCE.Write(writer, value.Info)
	FfiConverterOptionalTypeUnstableAudioDetailsContentINSTANCE.Write(writer, value.Audio)
	FfiConverterOptionalTypeUnstableVoiceContentINSTANCE.Write(writer, value.Voice)
}

type FfiDestroyerTypeAudioMessageContent struct{}

func (_ FfiDestroyerTypeAudioMessageContent) Destroy(value AudioMessageContent) {
	value.Destroy()
}

type ClientProperties struct {
	ClientId    string
	LanguageTag *string
	Theme       *string
}

func (r *ClientProperties) Destroy() {
	FfiDestroyerString{}.Destroy(r.ClientId)
	FfiDestroyerOptionalString{}.Destroy(r.LanguageTag)
	FfiDestroyerOptionalString{}.Destroy(r.Theme)
}

type FfiConverterTypeClientProperties struct{}

var FfiConverterTypeClientPropertiesINSTANCE = FfiConverterTypeClientProperties{}

func (c FfiConverterTypeClientProperties) Lift(rb RustBufferI) ClientProperties {
	return LiftFromRustBuffer[ClientProperties](c, rb)
}

func (c FfiConverterTypeClientProperties) Read(reader io.Reader) ClientProperties {
	return ClientProperties{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeClientProperties) Lower(value ClientProperties) RustBuffer {
	return LowerIntoRustBuffer[ClientProperties](c, value)
}

func (c FfiConverterTypeClientProperties) Write(writer io.Writer, value ClientProperties) {
	FfiConverterStringINSTANCE.Write(writer, value.ClientId)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.LanguageTag)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Theme)
}

type FfiDestroyerTypeClientProperties struct{}

func (_ FfiDestroyerTypeClientProperties) Destroy(value ClientProperties) {
	value.Destroy()
}

type CreateRoomParameters struct {
	Name                      *string
	Topic                     *string
	IsEncrypted               bool
	IsDirect                  bool
	Visibility                RoomVisibility
	Preset                    RoomPreset
	Invite                    *[]string
	Avatar                    *string
	PowerLevelContentOverride *PowerLevels
}

func (r *CreateRoomParameters) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(r.Name)
	FfiDestroyerOptionalString{}.Destroy(r.Topic)
	FfiDestroyerBool{}.Destroy(r.IsEncrypted)
	FfiDestroyerBool{}.Destroy(r.IsDirect)
	FfiDestroyerTypeRoomVisibility{}.Destroy(r.Visibility)
	FfiDestroyerTypeRoomPreset{}.Destroy(r.Preset)
	FfiDestroyerOptionalSequenceString{}.Destroy(r.Invite)
	FfiDestroyerOptionalString{}.Destroy(r.Avatar)
	FfiDestroyerOptionalTypePowerLevels{}.Destroy(r.PowerLevelContentOverride)
}

type FfiConverterTypeCreateRoomParameters struct{}

var FfiConverterTypeCreateRoomParametersINSTANCE = FfiConverterTypeCreateRoomParameters{}

func (c FfiConverterTypeCreateRoomParameters) Lift(rb RustBufferI) CreateRoomParameters {
	return LiftFromRustBuffer[CreateRoomParameters](c, rb)
}

func (c FfiConverterTypeCreateRoomParameters) Read(reader io.Reader) CreateRoomParameters {
	return CreateRoomParameters{
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterTypeRoomVisibilityINSTANCE.Read(reader),
		FfiConverterTypeRoomPresetINSTANCE.Read(reader),
		FfiConverterOptionalSequenceStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalTypePowerLevelsINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeCreateRoomParameters) Lower(value CreateRoomParameters) RustBuffer {
	return LowerIntoRustBuffer[CreateRoomParameters](c, value)
}

func (c FfiConverterTypeCreateRoomParameters) Write(writer io.Writer, value CreateRoomParameters) {
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Name)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Topic)
	FfiConverterBoolINSTANCE.Write(writer, value.IsEncrypted)
	FfiConverterBoolINSTANCE.Write(writer, value.IsDirect)
	FfiConverterTypeRoomVisibilityINSTANCE.Write(writer, value.Visibility)
	FfiConverterTypeRoomPresetINSTANCE.Write(writer, value.Preset)
	FfiConverterOptionalSequenceStringINSTANCE.Write(writer, value.Invite)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Avatar)
	FfiConverterOptionalTypePowerLevelsINSTANCE.Write(writer, value.PowerLevelContentOverride)
}

type FfiDestroyerTypeCreateRoomParameters struct{}

func (_ FfiDestroyerTypeCreateRoomParameters) Destroy(value CreateRoomParameters) {
	value.Destroy()
}

type EmoteMessageContent struct {
	Body      string
	Formatted *FormattedBody
}

func (r *EmoteMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerOptionalTypeFormattedBody{}.Destroy(r.Formatted)
}

type FfiConverterTypeEmoteMessageContent struct{}

var FfiConverterTypeEmoteMessageContentINSTANCE = FfiConverterTypeEmoteMessageContent{}

func (c FfiConverterTypeEmoteMessageContent) Lift(rb RustBufferI) EmoteMessageContent {
	return LiftFromRustBuffer[EmoteMessageContent](c, rb)
}

func (c FfiConverterTypeEmoteMessageContent) Read(reader io.Reader) EmoteMessageContent {
	return EmoteMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalTypeFormattedBodyINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeEmoteMessageContent) Lower(value EmoteMessageContent) RustBuffer {
	return LowerIntoRustBuffer[EmoteMessageContent](c, value)
}

func (c FfiConverterTypeEmoteMessageContent) Write(writer io.Writer, value EmoteMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterOptionalTypeFormattedBodyINSTANCE.Write(writer, value.Formatted)
}

type FfiDestroyerTypeEmoteMessageContent struct{}

func (_ FfiDestroyerTypeEmoteMessageContent) Destroy(value EmoteMessageContent) {
	value.Destroy()
}

type EventTimelineItemDebugInfo struct {
	Model          string
	OriginalJson   *string
	LatestEditJson *string
}

func (r *EventTimelineItemDebugInfo) Destroy() {
	FfiDestroyerString{}.Destroy(r.Model)
	FfiDestroyerOptionalString{}.Destroy(r.OriginalJson)
	FfiDestroyerOptionalString{}.Destroy(r.LatestEditJson)
}

type FfiConverterTypeEventTimelineItemDebugInfo struct{}

var FfiConverterTypeEventTimelineItemDebugInfoINSTANCE = FfiConverterTypeEventTimelineItemDebugInfo{}

func (c FfiConverterTypeEventTimelineItemDebugInfo) Lift(rb RustBufferI) EventTimelineItemDebugInfo {
	return LiftFromRustBuffer[EventTimelineItemDebugInfo](c, rb)
}

func (c FfiConverterTypeEventTimelineItemDebugInfo) Read(reader io.Reader) EventTimelineItemDebugInfo {
	return EventTimelineItemDebugInfo{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeEventTimelineItemDebugInfo) Lower(value EventTimelineItemDebugInfo) RustBuffer {
	return LowerIntoRustBuffer[EventTimelineItemDebugInfo](c, value)
}

func (c FfiConverterTypeEventTimelineItemDebugInfo) Write(writer io.Writer, value EventTimelineItemDebugInfo) {
	FfiConverterStringINSTANCE.Write(writer, value.Model)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.OriginalJson)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.LatestEditJson)
}

type FfiDestroyerTypeEventTimelineItemDebugInfo struct{}

func (_ FfiDestroyerTypeEventTimelineItemDebugInfo) Destroy(value EventTimelineItemDebugInfo) {
	value.Destroy()
}

type FileInfo struct {
	Mimetype        *string
	Size            *uint64
	ThumbnailInfo   *ThumbnailInfo
	ThumbnailSource **MediaSource
}

func (r *FileInfo) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(r.Mimetype)
	FfiDestroyerOptionalUint64{}.Destroy(r.Size)
	FfiDestroyerOptionalTypeThumbnailInfo{}.Destroy(r.ThumbnailInfo)
	FfiDestroyerOptionalMediaSource{}.Destroy(r.ThumbnailSource)
}

type FfiConverterTypeFileInfo struct{}

var FfiConverterTypeFileInfoINSTANCE = FfiConverterTypeFileInfo{}

func (c FfiConverterTypeFileInfo) Lift(rb RustBufferI) FileInfo {
	return LiftFromRustBuffer[FileInfo](c, rb)
}

func (c FfiConverterTypeFileInfo) Read(reader io.Reader) FileInfo {
	return FileInfo{
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalTypeThumbnailInfoINSTANCE.Read(reader),
		FfiConverterOptionalMediaSourceINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeFileInfo) Lower(value FileInfo) RustBuffer {
	return LowerIntoRustBuffer[FileInfo](c, value)
}

func (c FfiConverterTypeFileInfo) Write(writer io.Writer, value FileInfo) {
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Mimetype)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Size)
	FfiConverterOptionalTypeThumbnailInfoINSTANCE.Write(writer, value.ThumbnailInfo)
	FfiConverterOptionalMediaSourceINSTANCE.Write(writer, value.ThumbnailSource)
}

type FfiDestroyerTypeFileInfo struct{}

func (_ FfiDestroyerTypeFileInfo) Destroy(value FileInfo) {
	value.Destroy()
}

type FileMessageContent struct {
	Body     string
	Filename *string
	Source   *MediaSource
	Info     *FileInfo
}

func (r *FileMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerOptionalString{}.Destroy(r.Filename)
	FfiDestroyerMediaSource{}.Destroy(r.Source)
	FfiDestroyerOptionalTypeFileInfo{}.Destroy(r.Info)
}

type FfiConverterTypeFileMessageContent struct{}

var FfiConverterTypeFileMessageContentINSTANCE = FfiConverterTypeFileMessageContent{}

func (c FfiConverterTypeFileMessageContent) Lift(rb RustBufferI) FileMessageContent {
	return LiftFromRustBuffer[FileMessageContent](c, rb)
}

func (c FfiConverterTypeFileMessageContent) Read(reader io.Reader) FileMessageContent {
	return FileMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterMediaSourceINSTANCE.Read(reader),
		FfiConverterOptionalTypeFileInfoINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeFileMessageContent) Lower(value FileMessageContent) RustBuffer {
	return LowerIntoRustBuffer[FileMessageContent](c, value)
}

func (c FfiConverterTypeFileMessageContent) Write(writer io.Writer, value FileMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Filename)
	FfiConverterMediaSourceINSTANCE.Write(writer, value.Source)
	FfiConverterOptionalTypeFileInfoINSTANCE.Write(writer, value.Info)
}

type FfiDestroyerTypeFileMessageContent struct{}

func (_ FfiDestroyerTypeFileMessageContent) Destroy(value FileMessageContent) {
	value.Destroy()
}

type FormattedBody struct {
	Format MessageFormat
	Body   string
}

func (r *FormattedBody) Destroy() {
	FfiDestroyerTypeMessageFormat{}.Destroy(r.Format)
	FfiDestroyerString{}.Destroy(r.Body)
}

type FfiConverterTypeFormattedBody struct{}

var FfiConverterTypeFormattedBodyINSTANCE = FfiConverterTypeFormattedBody{}

func (c FfiConverterTypeFormattedBody) Lift(rb RustBufferI) FormattedBody {
	return LiftFromRustBuffer[FormattedBody](c, rb)
}

func (c FfiConverterTypeFormattedBody) Read(reader io.Reader) FormattedBody {
	return FormattedBody{
		FfiConverterTypeMessageFormatINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeFormattedBody) Lower(value FormattedBody) RustBuffer {
	return LowerIntoRustBuffer[FormattedBody](c, value)
}

func (c FfiConverterTypeFormattedBody) Write(writer io.Writer, value FormattedBody) {
	FfiConverterTypeMessageFormatINSTANCE.Write(writer, value.Format)
	FfiConverterStringINSTANCE.Write(writer, value.Body)
}

type FfiDestroyerTypeFormattedBody struct{}

func (_ FfiDestroyerTypeFormattedBody) Destroy(value FormattedBody) {
	value.Destroy()
}

type HttpPusherData struct {
	Url            string
	Format         *PushFormat
	DefaultPayload *string
}

func (r *HttpPusherData) Destroy() {
	FfiDestroyerString{}.Destroy(r.Url)
	FfiDestroyerOptionalTypePushFormat{}.Destroy(r.Format)
	FfiDestroyerOptionalString{}.Destroy(r.DefaultPayload)
}

type FfiConverterTypeHttpPusherData struct{}

var FfiConverterTypeHttpPusherDataINSTANCE = FfiConverterTypeHttpPusherData{}

func (c FfiConverterTypeHttpPusherData) Lift(rb RustBufferI) HttpPusherData {
	return LiftFromRustBuffer[HttpPusherData](c, rb)
}

func (c FfiConverterTypeHttpPusherData) Read(reader io.Reader) HttpPusherData {
	return HttpPusherData{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalTypePushFormatINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeHttpPusherData) Lower(value HttpPusherData) RustBuffer {
	return LowerIntoRustBuffer[HttpPusherData](c, value)
}

func (c FfiConverterTypeHttpPusherData) Write(writer io.Writer, value HttpPusherData) {
	FfiConverterStringINSTANCE.Write(writer, value.Url)
	FfiConverterOptionalTypePushFormatINSTANCE.Write(writer, value.Format)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.DefaultPayload)
}

type FfiDestroyerTypeHttpPusherData struct{}

func (_ FfiDestroyerTypeHttpPusherData) Destroy(value HttpPusherData) {
	value.Destroy()
}

type ImageInfo struct {
	Height          *uint64
	Width           *uint64
	Mimetype        *string
	Size            *uint64
	ThumbnailInfo   *ThumbnailInfo
	ThumbnailSource **MediaSource
	Blurhash        *string
}

func (r *ImageInfo) Destroy() {
	FfiDestroyerOptionalUint64{}.Destroy(r.Height)
	FfiDestroyerOptionalUint64{}.Destroy(r.Width)
	FfiDestroyerOptionalString{}.Destroy(r.Mimetype)
	FfiDestroyerOptionalUint64{}.Destroy(r.Size)
	FfiDestroyerOptionalTypeThumbnailInfo{}.Destroy(r.ThumbnailInfo)
	FfiDestroyerOptionalMediaSource{}.Destroy(r.ThumbnailSource)
	FfiDestroyerOptionalString{}.Destroy(r.Blurhash)
}

type FfiConverterTypeImageInfo struct{}

var FfiConverterTypeImageInfoINSTANCE = FfiConverterTypeImageInfo{}

func (c FfiConverterTypeImageInfo) Lift(rb RustBufferI) ImageInfo {
	return LiftFromRustBuffer[ImageInfo](c, rb)
}

func (c FfiConverterTypeImageInfo) Read(reader io.Reader) ImageInfo {
	return ImageInfo{
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalTypeThumbnailInfoINSTANCE.Read(reader),
		FfiConverterOptionalMediaSourceINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeImageInfo) Lower(value ImageInfo) RustBuffer {
	return LowerIntoRustBuffer[ImageInfo](c, value)
}

func (c FfiConverterTypeImageInfo) Write(writer io.Writer, value ImageInfo) {
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Height)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Width)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Mimetype)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Size)
	FfiConverterOptionalTypeThumbnailInfoINSTANCE.Write(writer, value.ThumbnailInfo)
	FfiConverterOptionalMediaSourceINSTANCE.Write(writer, value.ThumbnailSource)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Blurhash)
}

type FfiDestroyerTypeImageInfo struct{}

func (_ FfiDestroyerTypeImageInfo) Destroy(value ImageInfo) {
	value.Destroy()
}

type ImageMessageContent struct {
	Body   string
	Source *MediaSource
	Info   *ImageInfo
}

func (r *ImageMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerMediaSource{}.Destroy(r.Source)
	FfiDestroyerOptionalTypeImageInfo{}.Destroy(r.Info)
}

type FfiConverterTypeImageMessageContent struct{}

var FfiConverterTypeImageMessageContentINSTANCE = FfiConverterTypeImageMessageContent{}

func (c FfiConverterTypeImageMessageContent) Lift(rb RustBufferI) ImageMessageContent {
	return LiftFromRustBuffer[ImageMessageContent](c, rb)
}

func (c FfiConverterTypeImageMessageContent) Read(reader io.Reader) ImageMessageContent {
	return ImageMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterMediaSourceINSTANCE.Read(reader),
		FfiConverterOptionalTypeImageInfoINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeImageMessageContent) Lower(value ImageMessageContent) RustBuffer {
	return LowerIntoRustBuffer[ImageMessageContent](c, value)
}

func (c FfiConverterTypeImageMessageContent) Write(writer io.Writer, value ImageMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterMediaSourceINSTANCE.Write(writer, value.Source)
	FfiConverterOptionalTypeImageInfoINSTANCE.Write(writer, value.Info)
}

type FfiDestroyerTypeImageMessageContent struct{}

func (_ FfiDestroyerTypeImageMessageContent) Destroy(value ImageMessageContent) {
	value.Destroy()
}

type InReplyToDetails struct {
	EventId string
	Event   RepliedToEventDetails
}

func (r *InReplyToDetails) Destroy() {
	FfiDestroyerString{}.Destroy(r.EventId)
	FfiDestroyerTypeRepliedToEventDetails{}.Destroy(r.Event)
}

type FfiConverterTypeInReplyToDetails struct{}

var FfiConverterTypeInReplyToDetailsINSTANCE = FfiConverterTypeInReplyToDetails{}

func (c FfiConverterTypeInReplyToDetails) Lift(rb RustBufferI) InReplyToDetails {
	return LiftFromRustBuffer[InReplyToDetails](c, rb)
}

func (c FfiConverterTypeInReplyToDetails) Read(reader io.Reader) InReplyToDetails {
	return InReplyToDetails{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterTypeRepliedToEventDetailsINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeInReplyToDetails) Lower(value InReplyToDetails) RustBuffer {
	return LowerIntoRustBuffer[InReplyToDetails](c, value)
}

func (c FfiConverterTypeInReplyToDetails) Write(writer io.Writer, value InReplyToDetails) {
	FfiConverterStringINSTANCE.Write(writer, value.EventId)
	FfiConverterTypeRepliedToEventDetailsINSTANCE.Write(writer, value.Event)
}

type FfiDestroyerTypeInReplyToDetails struct{}

func (_ FfiDestroyerTypeInReplyToDetails) Destroy(value InReplyToDetails) {
	value.Destroy()
}

type InsertData struct {
	Index uint32
	Item  *TimelineItem
}

func (r *InsertData) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.Index)
	FfiDestroyerTimelineItem{}.Destroy(r.Item)
}

type FfiConverterTypeInsertData struct{}

var FfiConverterTypeInsertDataINSTANCE = FfiConverterTypeInsertData{}

func (c FfiConverterTypeInsertData) Lift(rb RustBufferI) InsertData {
	return LiftFromRustBuffer[InsertData](c, rb)
}

func (c FfiConverterTypeInsertData) Read(reader io.Reader) InsertData {
	return InsertData{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterTimelineItemINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeInsertData) Lower(value InsertData) RustBuffer {
	return LowerIntoRustBuffer[InsertData](c, value)
}

func (c FfiConverterTypeInsertData) Write(writer io.Writer, value InsertData) {
	FfiConverterUint32INSTANCE.Write(writer, value.Index)
	FfiConverterTimelineItemINSTANCE.Write(writer, value.Item)
}

type FfiDestroyerTypeInsertData struct{}

func (_ FfiDestroyerTypeInsertData) Destroy(value InsertData) {
	value.Destroy()
}

type LocationContent struct {
	Body        string
	GeoUri      string
	Description *string
	ZoomLevel   *uint8
	Asset       *AssetType
}

func (r *LocationContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerString{}.Destroy(r.GeoUri)
	FfiDestroyerOptionalString{}.Destroy(r.Description)
	FfiDestroyerOptionalUint8{}.Destroy(r.ZoomLevel)
	FfiDestroyerOptionalTypeAssetType{}.Destroy(r.Asset)
}

type FfiConverterTypeLocationContent struct{}

var FfiConverterTypeLocationContentINSTANCE = FfiConverterTypeLocationContent{}

func (c FfiConverterTypeLocationContent) Lift(rb RustBufferI) LocationContent {
	return LiftFromRustBuffer[LocationContent](c, rb)
}

func (c FfiConverterTypeLocationContent) Read(reader io.Reader) LocationContent {
	return LocationContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalUint8INSTANCE.Read(reader),
		FfiConverterOptionalTypeAssetTypeINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeLocationContent) Lower(value LocationContent) RustBuffer {
	return LowerIntoRustBuffer[LocationContent](c, value)
}

func (c FfiConverterTypeLocationContent) Write(writer io.Writer, value LocationContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterStringINSTANCE.Write(writer, value.GeoUri)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Description)
	FfiConverterOptionalUint8INSTANCE.Write(writer, value.ZoomLevel)
	FfiConverterOptionalTypeAssetTypeINSTANCE.Write(writer, value.Asset)
}

type FfiDestroyerTypeLocationContent struct{}

func (_ FfiDestroyerTypeLocationContent) Destroy(value LocationContent) {
	value.Destroy()
}

type Mentions struct {
	UserIds []string
	Room    bool
}

func (r *Mentions) Destroy() {
	FfiDestroyerSequenceString{}.Destroy(r.UserIds)
	FfiDestroyerBool{}.Destroy(r.Room)
}

type FfiConverterTypeMentions struct{}

var FfiConverterTypeMentionsINSTANCE = FfiConverterTypeMentions{}

func (c FfiConverterTypeMentions) Lift(rb RustBufferI) Mentions {
	return LiftFromRustBuffer[Mentions](c, rb)
}

func (c FfiConverterTypeMentions) Read(reader io.Reader) Mentions {
	return Mentions{
		FfiConverterSequenceStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeMentions) Lower(value Mentions) RustBuffer {
	return LowerIntoRustBuffer[Mentions](c, value)
}

func (c FfiConverterTypeMentions) Write(writer io.Writer, value Mentions) {
	FfiConverterSequenceStringINSTANCE.Write(writer, value.UserIds)
	FfiConverterBoolINSTANCE.Write(writer, value.Room)
}

type FfiDestroyerTypeMentions struct{}

func (_ FfiDestroyerTypeMentions) Destroy(value Mentions) {
	value.Destroy()
}

type NoticeMessageContent struct {
	Body      string
	Formatted *FormattedBody
}

func (r *NoticeMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerOptionalTypeFormattedBody{}.Destroy(r.Formatted)
}

type FfiConverterTypeNoticeMessageContent struct{}

var FfiConverterTypeNoticeMessageContentINSTANCE = FfiConverterTypeNoticeMessageContent{}

func (c FfiConverterTypeNoticeMessageContent) Lift(rb RustBufferI) NoticeMessageContent {
	return LiftFromRustBuffer[NoticeMessageContent](c, rb)
}

func (c FfiConverterTypeNoticeMessageContent) Read(reader io.Reader) NoticeMessageContent {
	return NoticeMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalTypeFormattedBodyINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeNoticeMessageContent) Lower(value NoticeMessageContent) RustBuffer {
	return LowerIntoRustBuffer[NoticeMessageContent](c, value)
}

func (c FfiConverterTypeNoticeMessageContent) Write(writer io.Writer, value NoticeMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterOptionalTypeFormattedBodyINSTANCE.Write(writer, value.Formatted)
}

type FfiDestroyerTypeNoticeMessageContent struct{}

func (_ FfiDestroyerTypeNoticeMessageContent) Destroy(value NoticeMessageContent) {
	value.Destroy()
}

type NotificationItem struct {
	Event      NotificationEvent
	SenderInfo NotificationSenderInfo
	RoomInfo   NotificationRoomInfo
	IsNoisy    *bool
	HasMention *bool
}

func (r *NotificationItem) Destroy() {
	FfiDestroyerTypeNotificationEvent{}.Destroy(r.Event)
	FfiDestroyerTypeNotificationSenderInfo{}.Destroy(r.SenderInfo)
	FfiDestroyerTypeNotificationRoomInfo{}.Destroy(r.RoomInfo)
	FfiDestroyerOptionalBool{}.Destroy(r.IsNoisy)
	FfiDestroyerOptionalBool{}.Destroy(r.HasMention)
}

type FfiConverterTypeNotificationItem struct{}

var FfiConverterTypeNotificationItemINSTANCE = FfiConverterTypeNotificationItem{}

func (c FfiConverterTypeNotificationItem) Lift(rb RustBufferI) NotificationItem {
	return LiftFromRustBuffer[NotificationItem](c, rb)
}

func (c FfiConverterTypeNotificationItem) Read(reader io.Reader) NotificationItem {
	return NotificationItem{
		FfiConverterTypeNotificationEventINSTANCE.Read(reader),
		FfiConverterTypeNotificationSenderInfoINSTANCE.Read(reader),
		FfiConverterTypeNotificationRoomInfoINSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeNotificationItem) Lower(value NotificationItem) RustBuffer {
	return LowerIntoRustBuffer[NotificationItem](c, value)
}

func (c FfiConverterTypeNotificationItem) Write(writer io.Writer, value NotificationItem) {
	FfiConverterTypeNotificationEventINSTANCE.Write(writer, value.Event)
	FfiConverterTypeNotificationSenderInfoINSTANCE.Write(writer, value.SenderInfo)
	FfiConverterTypeNotificationRoomInfoINSTANCE.Write(writer, value.RoomInfo)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.IsNoisy)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.HasMention)
}

type FfiDestroyerTypeNotificationItem struct{}

func (_ FfiDestroyerTypeNotificationItem) Destroy(value NotificationItem) {
	value.Destroy()
}

type NotificationPowerLevels struct {
	Room int32
}

func (r *NotificationPowerLevels) Destroy() {
	FfiDestroyerInt32{}.Destroy(r.Room)
}

type FfiConverterTypeNotificationPowerLevels struct{}

var FfiConverterTypeNotificationPowerLevelsINSTANCE = FfiConverterTypeNotificationPowerLevels{}

func (c FfiConverterTypeNotificationPowerLevels) Lift(rb RustBufferI) NotificationPowerLevels {
	return LiftFromRustBuffer[NotificationPowerLevels](c, rb)
}

func (c FfiConverterTypeNotificationPowerLevels) Read(reader io.Reader) NotificationPowerLevels {
	return NotificationPowerLevels{
		FfiConverterInt32INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeNotificationPowerLevels) Lower(value NotificationPowerLevels) RustBuffer {
	return LowerIntoRustBuffer[NotificationPowerLevels](c, value)
}

func (c FfiConverterTypeNotificationPowerLevels) Write(writer io.Writer, value NotificationPowerLevels) {
	FfiConverterInt32INSTANCE.Write(writer, value.Room)
}

type FfiDestroyerTypeNotificationPowerLevels struct{}

func (_ FfiDestroyerTypeNotificationPowerLevels) Destroy(value NotificationPowerLevels) {
	value.Destroy()
}

type NotificationRoomInfo struct {
	DisplayName        string
	AvatarUrl          *string
	CanonicalAlias     *string
	JoinedMembersCount uint64
	IsEncrypted        *bool
	IsDirect           bool
}

func (r *NotificationRoomInfo) Destroy() {
	FfiDestroyerString{}.Destroy(r.DisplayName)
	FfiDestroyerOptionalString{}.Destroy(r.AvatarUrl)
	FfiDestroyerOptionalString{}.Destroy(r.CanonicalAlias)
	FfiDestroyerUint64{}.Destroy(r.JoinedMembersCount)
	FfiDestroyerOptionalBool{}.Destroy(r.IsEncrypted)
	FfiDestroyerBool{}.Destroy(r.IsDirect)
}

type FfiConverterTypeNotificationRoomInfo struct{}

var FfiConverterTypeNotificationRoomInfoINSTANCE = FfiConverterTypeNotificationRoomInfo{}

func (c FfiConverterTypeNotificationRoomInfo) Lift(rb RustBufferI) NotificationRoomInfo {
	return LiftFromRustBuffer[NotificationRoomInfo](c, rb)
}

func (c FfiConverterTypeNotificationRoomInfo) Read(reader io.Reader) NotificationRoomInfo {
	return NotificationRoomInfo{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeNotificationRoomInfo) Lower(value NotificationRoomInfo) RustBuffer {
	return LowerIntoRustBuffer[NotificationRoomInfo](c, value)
}

func (c FfiConverterTypeNotificationRoomInfo) Write(writer io.Writer, value NotificationRoomInfo) {
	FfiConverterStringINSTANCE.Write(writer, value.DisplayName)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.AvatarUrl)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.CanonicalAlias)
	FfiConverterUint64INSTANCE.Write(writer, value.JoinedMembersCount)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.IsEncrypted)
	FfiConverterBoolINSTANCE.Write(writer, value.IsDirect)
}

type FfiDestroyerTypeNotificationRoomInfo struct{}

func (_ FfiDestroyerTypeNotificationRoomInfo) Destroy(value NotificationRoomInfo) {
	value.Destroy()
}

type NotificationSenderInfo struct {
	DisplayName     *string
	AvatarUrl       *string
	IsNameAmbiguous bool
}

func (r *NotificationSenderInfo) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(r.DisplayName)
	FfiDestroyerOptionalString{}.Destroy(r.AvatarUrl)
	FfiDestroyerBool{}.Destroy(r.IsNameAmbiguous)
}

type FfiConverterTypeNotificationSenderInfo struct{}

var FfiConverterTypeNotificationSenderInfoINSTANCE = FfiConverterTypeNotificationSenderInfo{}

func (c FfiConverterTypeNotificationSenderInfo) Lift(rb RustBufferI) NotificationSenderInfo {
	return LiftFromRustBuffer[NotificationSenderInfo](c, rb)
}

func (c FfiConverterTypeNotificationSenderInfo) Read(reader io.Reader) NotificationSenderInfo {
	return NotificationSenderInfo{
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeNotificationSenderInfo) Lower(value NotificationSenderInfo) RustBuffer {
	return LowerIntoRustBuffer[NotificationSenderInfo](c, value)
}

func (c FfiConverterTypeNotificationSenderInfo) Write(writer io.Writer, value NotificationSenderInfo) {
	FfiConverterOptionalStringINSTANCE.Write(writer, value.DisplayName)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.AvatarUrl)
	FfiConverterBoolINSTANCE.Write(writer, value.IsNameAmbiguous)
}

type FfiDestroyerTypeNotificationSenderInfo struct{}

func (_ FfiDestroyerTypeNotificationSenderInfo) Destroy(value NotificationSenderInfo) {
	value.Destroy()
}

type OidcConfiguration struct {
	ClientName          *string
	RedirectUri         string
	ClientUri           *string
	LogoUri             *string
	TosUri              *string
	PolicyUri           *string
	Contacts            *[]string
	StaticRegistrations map[string]string
}

func (r *OidcConfiguration) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(r.ClientName)
	FfiDestroyerString{}.Destroy(r.RedirectUri)
	FfiDestroyerOptionalString{}.Destroy(r.ClientUri)
	FfiDestroyerOptionalString{}.Destroy(r.LogoUri)
	FfiDestroyerOptionalString{}.Destroy(r.TosUri)
	FfiDestroyerOptionalString{}.Destroy(r.PolicyUri)
	FfiDestroyerOptionalSequenceString{}.Destroy(r.Contacts)
	FfiDestroyerMapStringString{}.Destroy(r.StaticRegistrations)
}

type FfiConverterTypeOidcConfiguration struct{}

var FfiConverterTypeOidcConfigurationINSTANCE = FfiConverterTypeOidcConfiguration{}

func (c FfiConverterTypeOidcConfiguration) Lift(rb RustBufferI) OidcConfiguration {
	return LiftFromRustBuffer[OidcConfiguration](c, rb)
}

func (c FfiConverterTypeOidcConfiguration) Read(reader io.Reader) OidcConfiguration {
	return OidcConfiguration{
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalSequenceStringINSTANCE.Read(reader),
		FfiConverterMapStringStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeOidcConfiguration) Lower(value OidcConfiguration) RustBuffer {
	return LowerIntoRustBuffer[OidcConfiguration](c, value)
}

func (c FfiConverterTypeOidcConfiguration) Write(writer io.Writer, value OidcConfiguration) {
	FfiConverterOptionalStringINSTANCE.Write(writer, value.ClientName)
	FfiConverterStringINSTANCE.Write(writer, value.RedirectUri)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.ClientUri)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.LogoUri)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.TosUri)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.PolicyUri)
	FfiConverterOptionalSequenceStringINSTANCE.Write(writer, value.Contacts)
	FfiConverterMapStringStringINSTANCE.Write(writer, value.StaticRegistrations)
}

type FfiDestroyerTypeOidcConfiguration struct{}

func (_ FfiDestroyerTypeOidcConfiguration) Destroy(value OidcConfiguration) {
	value.Destroy()
}

type OtlpTracingConfiguration struct {
	ClientName            string
	User                  string
	Password              string
	OtlpEndpoint          string
	Filter                string
	WriteToStdoutOrSystem bool
	WriteToFiles          *TracingFileConfiguration
}

func (r *OtlpTracingConfiguration) Destroy() {
	FfiDestroyerString{}.Destroy(r.ClientName)
	FfiDestroyerString{}.Destroy(r.User)
	FfiDestroyerString{}.Destroy(r.Password)
	FfiDestroyerString{}.Destroy(r.OtlpEndpoint)
	FfiDestroyerString{}.Destroy(r.Filter)
	FfiDestroyerBool{}.Destroy(r.WriteToStdoutOrSystem)
	FfiDestroyerOptionalTypeTracingFileConfiguration{}.Destroy(r.WriteToFiles)
}

type FfiConverterTypeOtlpTracingConfiguration struct{}

var FfiConverterTypeOtlpTracingConfigurationINSTANCE = FfiConverterTypeOtlpTracingConfiguration{}

func (c FfiConverterTypeOtlpTracingConfiguration) Lift(rb RustBufferI) OtlpTracingConfiguration {
	return LiftFromRustBuffer[OtlpTracingConfiguration](c, rb)
}

func (c FfiConverterTypeOtlpTracingConfiguration) Read(reader io.Reader) OtlpTracingConfiguration {
	return OtlpTracingConfiguration{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterOptionalTypeTracingFileConfigurationINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeOtlpTracingConfiguration) Lower(value OtlpTracingConfiguration) RustBuffer {
	return LowerIntoRustBuffer[OtlpTracingConfiguration](c, value)
}

func (c FfiConverterTypeOtlpTracingConfiguration) Write(writer io.Writer, value OtlpTracingConfiguration) {
	FfiConverterStringINSTANCE.Write(writer, value.ClientName)
	FfiConverterStringINSTANCE.Write(writer, value.User)
	FfiConverterStringINSTANCE.Write(writer, value.Password)
	FfiConverterStringINSTANCE.Write(writer, value.OtlpEndpoint)
	FfiConverterStringINSTANCE.Write(writer, value.Filter)
	FfiConverterBoolINSTANCE.Write(writer, value.WriteToStdoutOrSystem)
	FfiConverterOptionalTypeTracingFileConfigurationINSTANCE.Write(writer, value.WriteToFiles)
}

type FfiDestroyerTypeOtlpTracingConfiguration struct{}

func (_ FfiDestroyerTypeOtlpTracingConfiguration) Destroy(value OtlpTracingConfiguration) {
	value.Destroy()
}

type PollAnswer struct {
	Id   string
	Text string
}

func (r *PollAnswer) Destroy() {
	FfiDestroyerString{}.Destroy(r.Id)
	FfiDestroyerString{}.Destroy(r.Text)
}

type FfiConverterTypePollAnswer struct{}

var FfiConverterTypePollAnswerINSTANCE = FfiConverterTypePollAnswer{}

func (c FfiConverterTypePollAnswer) Lift(rb RustBufferI) PollAnswer {
	return LiftFromRustBuffer[PollAnswer](c, rb)
}

func (c FfiConverterTypePollAnswer) Read(reader io.Reader) PollAnswer {
	return PollAnswer{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypePollAnswer) Lower(value PollAnswer) RustBuffer {
	return LowerIntoRustBuffer[PollAnswer](c, value)
}

func (c FfiConverterTypePollAnswer) Write(writer io.Writer, value PollAnswer) {
	FfiConverterStringINSTANCE.Write(writer, value.Id)
	FfiConverterStringINSTANCE.Write(writer, value.Text)
}

type FfiDestroyerTypePollAnswer struct{}

func (_ FfiDestroyerTypePollAnswer) Destroy(value PollAnswer) {
	value.Destroy()
}

type PowerLevels struct {
	UsersDefault  *int32
	EventsDefault *int32
	StateDefault  *int32
	Ban           *int32
	Kick          *int32
	Redact        *int32
	Invite        *int32
	Notifications *NotificationPowerLevels
	Users         map[string]int32
	Events        map[string]int32
}

func (r *PowerLevels) Destroy() {
	FfiDestroyerOptionalInt32{}.Destroy(r.UsersDefault)
	FfiDestroyerOptionalInt32{}.Destroy(r.EventsDefault)
	FfiDestroyerOptionalInt32{}.Destroy(r.StateDefault)
	FfiDestroyerOptionalInt32{}.Destroy(r.Ban)
	FfiDestroyerOptionalInt32{}.Destroy(r.Kick)
	FfiDestroyerOptionalInt32{}.Destroy(r.Redact)
	FfiDestroyerOptionalInt32{}.Destroy(r.Invite)
	FfiDestroyerOptionalTypeNotificationPowerLevels{}.Destroy(r.Notifications)
	FfiDestroyerMapStringInt32{}.Destroy(r.Users)
	FfiDestroyerMapStringInt32{}.Destroy(r.Events)
}

type FfiConverterTypePowerLevels struct{}

var FfiConverterTypePowerLevelsINSTANCE = FfiConverterTypePowerLevels{}

func (c FfiConverterTypePowerLevels) Lift(rb RustBufferI) PowerLevels {
	return LiftFromRustBuffer[PowerLevels](c, rb)
}

func (c FfiConverterTypePowerLevels) Read(reader io.Reader) PowerLevels {
	return PowerLevels{
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalInt32INSTANCE.Read(reader),
		FfiConverterOptionalTypeNotificationPowerLevelsINSTANCE.Read(reader),
		FfiConverterMapStringInt32INSTANCE.Read(reader),
		FfiConverterMapStringInt32INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypePowerLevels) Lower(value PowerLevels) RustBuffer {
	return LowerIntoRustBuffer[PowerLevels](c, value)
}

func (c FfiConverterTypePowerLevels) Write(writer io.Writer, value PowerLevels) {
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.UsersDefault)
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.EventsDefault)
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.StateDefault)
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.Ban)
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.Kick)
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.Redact)
	FfiConverterOptionalInt32INSTANCE.Write(writer, value.Invite)
	FfiConverterOptionalTypeNotificationPowerLevelsINSTANCE.Write(writer, value.Notifications)
	FfiConverterMapStringInt32INSTANCE.Write(writer, value.Users)
	FfiConverterMapStringInt32INSTANCE.Write(writer, value.Events)
}

type FfiDestroyerTypePowerLevels struct{}

func (_ FfiDestroyerTypePowerLevels) Destroy(value PowerLevels) {
	value.Destroy()
}

type PusherIdentifiers struct {
	Pushkey string
	AppId   string
}

func (r *PusherIdentifiers) Destroy() {
	FfiDestroyerString{}.Destroy(r.Pushkey)
	FfiDestroyerString{}.Destroy(r.AppId)
}

type FfiConverterTypePusherIdentifiers struct{}

var FfiConverterTypePusherIdentifiersINSTANCE = FfiConverterTypePusherIdentifiers{}

func (c FfiConverterTypePusherIdentifiers) Lift(rb RustBufferI) PusherIdentifiers {
	return LiftFromRustBuffer[PusherIdentifiers](c, rb)
}

func (c FfiConverterTypePusherIdentifiers) Read(reader io.Reader) PusherIdentifiers {
	return PusherIdentifiers{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypePusherIdentifiers) Lower(value PusherIdentifiers) RustBuffer {
	return LowerIntoRustBuffer[PusherIdentifiers](c, value)
}

func (c FfiConverterTypePusherIdentifiers) Write(writer io.Writer, value PusherIdentifiers) {
	FfiConverterStringINSTANCE.Write(writer, value.Pushkey)
	FfiConverterStringINSTANCE.Write(writer, value.AppId)
}

type FfiDestroyerTypePusherIdentifiers struct{}

func (_ FfiDestroyerTypePusherIdentifiers) Destroy(value PusherIdentifiers) {
	value.Destroy()
}

type Reaction struct {
	Key     string
	Count   uint64
	Senders []ReactionSenderData
}

func (r *Reaction) Destroy() {
	FfiDestroyerString{}.Destroy(r.Key)
	FfiDestroyerUint64{}.Destroy(r.Count)
	FfiDestroyerSequenceTypeReactionSenderData{}.Destroy(r.Senders)
}

type FfiConverterTypeReaction struct{}

var FfiConverterTypeReactionINSTANCE = FfiConverterTypeReaction{}

func (c FfiConverterTypeReaction) Lift(rb RustBufferI) Reaction {
	return LiftFromRustBuffer[Reaction](c, rb)
}

func (c FfiConverterTypeReaction) Read(reader io.Reader) Reaction {
	return Reaction{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterSequenceTypeReactionSenderDataINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeReaction) Lower(value Reaction) RustBuffer {
	return LowerIntoRustBuffer[Reaction](c, value)
}

func (c FfiConverterTypeReaction) Write(writer io.Writer, value Reaction) {
	FfiConverterStringINSTANCE.Write(writer, value.Key)
	FfiConverterUint64INSTANCE.Write(writer, value.Count)
	FfiConverterSequenceTypeReactionSenderDataINSTANCE.Write(writer, value.Senders)
}

type FfiDestroyerTypeReaction struct{}

func (_ FfiDestroyerTypeReaction) Destroy(value Reaction) {
	value.Destroy()
}

type ReactionSenderData struct {
	SenderId  string
	Timestamp uint64
}

func (r *ReactionSenderData) Destroy() {
	FfiDestroyerString{}.Destroy(r.SenderId)
	FfiDestroyerUint64{}.Destroy(r.Timestamp)
}

type FfiConverterTypeReactionSenderData struct{}

var FfiConverterTypeReactionSenderDataINSTANCE = FfiConverterTypeReactionSenderData{}

func (c FfiConverterTypeReactionSenderData) Lift(rb RustBufferI) ReactionSenderData {
	return LiftFromRustBuffer[ReactionSenderData](c, rb)
}

func (c FfiConverterTypeReactionSenderData) Read(reader io.Reader) ReactionSenderData {
	return ReactionSenderData{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeReactionSenderData) Lower(value ReactionSenderData) RustBuffer {
	return LowerIntoRustBuffer[ReactionSenderData](c, value)
}

func (c FfiConverterTypeReactionSenderData) Write(writer io.Writer, value ReactionSenderData) {
	FfiConverterStringINSTANCE.Write(writer, value.SenderId)
	FfiConverterUint64INSTANCE.Write(writer, value.Timestamp)
}

type FfiDestroyerTypeReactionSenderData struct{}

func (_ FfiDestroyerTypeReactionSenderData) Destroy(value ReactionSenderData) {
	value.Destroy()
}

type Receipt struct {
	Timestamp *uint64
}

func (r *Receipt) Destroy() {
	FfiDestroyerOptionalUint64{}.Destroy(r.Timestamp)
}

type FfiConverterTypeReceipt struct{}

var FfiConverterTypeReceiptINSTANCE = FfiConverterTypeReceipt{}

func (c FfiConverterTypeReceipt) Lift(rb RustBufferI) Receipt {
	return LiftFromRustBuffer[Receipt](c, rb)
}

func (c FfiConverterTypeReceipt) Read(reader io.Reader) Receipt {
	return Receipt{
		FfiConverterOptionalUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeReceipt) Lower(value Receipt) RustBuffer {
	return LowerIntoRustBuffer[Receipt](c, value)
}

func (c FfiConverterTypeReceipt) Write(writer io.Writer, value Receipt) {
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Timestamp)
}

type FfiDestroyerTypeReceipt struct{}

func (_ FfiDestroyerTypeReceipt) Destroy(value Receipt) {
	value.Destroy()
}

type RequiredState struct {
	Key   string
	Value string
}

func (r *RequiredState) Destroy() {
	FfiDestroyerString{}.Destroy(r.Key)
	FfiDestroyerString{}.Destroy(r.Value)
}

type FfiConverterTypeRequiredState struct{}

var FfiConverterTypeRequiredStateINSTANCE = FfiConverterTypeRequiredState{}

func (c FfiConverterTypeRequiredState) Lift(rb RustBufferI) RequiredState {
	return LiftFromRustBuffer[RequiredState](c, rb)
}

func (c FfiConverterTypeRequiredState) Read(reader io.Reader) RequiredState {
	return RequiredState{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRequiredState) Lower(value RequiredState) RustBuffer {
	return LowerIntoRustBuffer[RequiredState](c, value)
}

func (c FfiConverterTypeRequiredState) Write(writer io.Writer, value RequiredState) {
	FfiConverterStringINSTANCE.Write(writer, value.Key)
	FfiConverterStringINSTANCE.Write(writer, value.Value)
}

type FfiDestroyerTypeRequiredState struct{}

func (_ FfiDestroyerTypeRequiredState) Destroy(value RequiredState) {
	value.Destroy()
}

type RoomInfo struct {
	Id                          string
	Name                        *string
	Topic                       *string
	AvatarUrl                   *string
	IsDirect                    bool
	IsPublic                    bool
	IsSpace                     bool
	IsTombstoned                bool
	IsFavourite                 bool
	CanonicalAlias              *string
	AlternativeAliases          []string
	Membership                  Membership
	LatestEvent                 **EventTimelineItem
	Inviter                     **RoomMember
	ActiveMembersCount          uint64
	InvitedMembersCount         uint64
	JoinedMembersCount          uint64
	HighlightCount              uint64
	NotificationCount           uint64
	UserDefinedNotificationMode *RoomNotificationMode
	HasRoomCall                 bool
	ActiveRoomCallParticipants  []string
	IsMarkedUnread              bool
	NumUnreadMessages           uint64
	NumUnreadNotifications      uint64
	NumUnreadMentions           uint64
}

func (r *RoomInfo) Destroy() {
	FfiDestroyerString{}.Destroy(r.Id)
	FfiDestroyerOptionalString{}.Destroy(r.Name)
	FfiDestroyerOptionalString{}.Destroy(r.Topic)
	FfiDestroyerOptionalString{}.Destroy(r.AvatarUrl)
	FfiDestroyerBool{}.Destroy(r.IsDirect)
	FfiDestroyerBool{}.Destroy(r.IsPublic)
	FfiDestroyerBool{}.Destroy(r.IsSpace)
	FfiDestroyerBool{}.Destroy(r.IsTombstoned)
	FfiDestroyerBool{}.Destroy(r.IsFavourite)
	FfiDestroyerOptionalString{}.Destroy(r.CanonicalAlias)
	FfiDestroyerSequenceString{}.Destroy(r.AlternativeAliases)
	FfiDestroyerTypeMembership{}.Destroy(r.Membership)
	FfiDestroyerOptionalEventTimelineItem{}.Destroy(r.LatestEvent)
	FfiDestroyerOptionalRoomMember{}.Destroy(r.Inviter)
	FfiDestroyerUint64{}.Destroy(r.ActiveMembersCount)
	FfiDestroyerUint64{}.Destroy(r.InvitedMembersCount)
	FfiDestroyerUint64{}.Destroy(r.JoinedMembersCount)
	FfiDestroyerUint64{}.Destroy(r.HighlightCount)
	FfiDestroyerUint64{}.Destroy(r.NotificationCount)
	FfiDestroyerOptionalTypeRoomNotificationMode{}.Destroy(r.UserDefinedNotificationMode)
	FfiDestroyerBool{}.Destroy(r.HasRoomCall)
	FfiDestroyerSequenceString{}.Destroy(r.ActiveRoomCallParticipants)
	FfiDestroyerBool{}.Destroy(r.IsMarkedUnread)
	FfiDestroyerUint64{}.Destroy(r.NumUnreadMessages)
	FfiDestroyerUint64{}.Destroy(r.NumUnreadNotifications)
	FfiDestroyerUint64{}.Destroy(r.NumUnreadMentions)
}

type FfiConverterTypeRoomInfo struct{}

var FfiConverterTypeRoomInfoINSTANCE = FfiConverterTypeRoomInfo{}

func (c FfiConverterTypeRoomInfo) Lift(rb RustBufferI) RoomInfo {
	return LiftFromRustBuffer[RoomInfo](c, rb)
}

func (c FfiConverterTypeRoomInfo) Read(reader io.Reader) RoomInfo {
	return RoomInfo{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterSequenceStringINSTANCE.Read(reader),
		FfiConverterTypeMembershipINSTANCE.Read(reader),
		FfiConverterOptionalEventTimelineItemINSTANCE.Read(reader),
		FfiConverterOptionalRoomMemberINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalTypeRoomNotificationModeINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterSequenceStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomInfo) Lower(value RoomInfo) RustBuffer {
	return LowerIntoRustBuffer[RoomInfo](c, value)
}

func (c FfiConverterTypeRoomInfo) Write(writer io.Writer, value RoomInfo) {
	FfiConverterStringINSTANCE.Write(writer, value.Id)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Name)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Topic)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.AvatarUrl)
	FfiConverterBoolINSTANCE.Write(writer, value.IsDirect)
	FfiConverterBoolINSTANCE.Write(writer, value.IsPublic)
	FfiConverterBoolINSTANCE.Write(writer, value.IsSpace)
	FfiConverterBoolINSTANCE.Write(writer, value.IsTombstoned)
	FfiConverterBoolINSTANCE.Write(writer, value.IsFavourite)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.CanonicalAlias)
	FfiConverterSequenceStringINSTANCE.Write(writer, value.AlternativeAliases)
	FfiConverterTypeMembershipINSTANCE.Write(writer, value.Membership)
	FfiConverterOptionalEventTimelineItemINSTANCE.Write(writer, value.LatestEvent)
	FfiConverterOptionalRoomMemberINSTANCE.Write(writer, value.Inviter)
	FfiConverterUint64INSTANCE.Write(writer, value.ActiveMembersCount)
	FfiConverterUint64INSTANCE.Write(writer, value.InvitedMembersCount)
	FfiConverterUint64INSTANCE.Write(writer, value.JoinedMembersCount)
	FfiConverterUint64INSTANCE.Write(writer, value.HighlightCount)
	FfiConverterUint64INSTANCE.Write(writer, value.NotificationCount)
	FfiConverterOptionalTypeRoomNotificationModeINSTANCE.Write(writer, value.UserDefinedNotificationMode)
	FfiConverterBoolINSTANCE.Write(writer, value.HasRoomCall)
	FfiConverterSequenceStringINSTANCE.Write(writer, value.ActiveRoomCallParticipants)
	FfiConverterBoolINSTANCE.Write(writer, value.IsMarkedUnread)
	FfiConverterUint64INSTANCE.Write(writer, value.NumUnreadMessages)
	FfiConverterUint64INSTANCE.Write(writer, value.NumUnreadNotifications)
	FfiConverterUint64INSTANCE.Write(writer, value.NumUnreadMentions)
}

type FfiDestroyerTypeRoomInfo struct{}

func (_ FfiDestroyerTypeRoomInfo) Destroy(value RoomInfo) {
	value.Destroy()
}

type RoomListEntriesResult struct {
	Entries       []RoomListEntry
	EntriesStream *TaskHandle
}

func (r *RoomListEntriesResult) Destroy() {
	FfiDestroyerSequenceTypeRoomListEntry{}.Destroy(r.Entries)
	FfiDestroyerTaskHandle{}.Destroy(r.EntriesStream)
}

type FfiConverterTypeRoomListEntriesResult struct{}

var FfiConverterTypeRoomListEntriesResultINSTANCE = FfiConverterTypeRoomListEntriesResult{}

func (c FfiConverterTypeRoomListEntriesResult) Lift(rb RustBufferI) RoomListEntriesResult {
	return LiftFromRustBuffer[RoomListEntriesResult](c, rb)
}

func (c FfiConverterTypeRoomListEntriesResult) Read(reader io.Reader) RoomListEntriesResult {
	return RoomListEntriesResult{
		FfiConverterSequenceTypeRoomListEntryINSTANCE.Read(reader),
		FfiConverterTaskHandleINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomListEntriesResult) Lower(value RoomListEntriesResult) RustBuffer {
	return LowerIntoRustBuffer[RoomListEntriesResult](c, value)
}

func (c FfiConverterTypeRoomListEntriesResult) Write(writer io.Writer, value RoomListEntriesResult) {
	FfiConverterSequenceTypeRoomListEntryINSTANCE.Write(writer, value.Entries)
	FfiConverterTaskHandleINSTANCE.Write(writer, value.EntriesStream)
}

type FfiDestroyerTypeRoomListEntriesResult struct{}

func (_ FfiDestroyerTypeRoomListEntriesResult) Destroy(value RoomListEntriesResult) {
	value.Destroy()
}

type RoomListEntriesWithDynamicAdaptersResult struct {
	Controller    *RoomListDynamicEntriesController
	EntriesStream *TaskHandle
}

func (r *RoomListEntriesWithDynamicAdaptersResult) Destroy() {
	FfiDestroyerRoomListDynamicEntriesController{}.Destroy(r.Controller)
	FfiDestroyerTaskHandle{}.Destroy(r.EntriesStream)
}

type FfiConverterTypeRoomListEntriesWithDynamicAdaptersResult struct{}

var FfiConverterTypeRoomListEntriesWithDynamicAdaptersResultINSTANCE = FfiConverterTypeRoomListEntriesWithDynamicAdaptersResult{}

func (c FfiConverterTypeRoomListEntriesWithDynamicAdaptersResult) Lift(rb RustBufferI) RoomListEntriesWithDynamicAdaptersResult {
	return LiftFromRustBuffer[RoomListEntriesWithDynamicAdaptersResult](c, rb)
}

func (c FfiConverterTypeRoomListEntriesWithDynamicAdaptersResult) Read(reader io.Reader) RoomListEntriesWithDynamicAdaptersResult {
	return RoomListEntriesWithDynamicAdaptersResult{
		FfiConverterRoomListDynamicEntriesControllerINSTANCE.Read(reader),
		FfiConverterTaskHandleINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomListEntriesWithDynamicAdaptersResult) Lower(value RoomListEntriesWithDynamicAdaptersResult) RustBuffer {
	return LowerIntoRustBuffer[RoomListEntriesWithDynamicAdaptersResult](c, value)
}

func (c FfiConverterTypeRoomListEntriesWithDynamicAdaptersResult) Write(writer io.Writer, value RoomListEntriesWithDynamicAdaptersResult) {
	FfiConverterRoomListDynamicEntriesControllerINSTANCE.Write(writer, value.Controller)
	FfiConverterTaskHandleINSTANCE.Write(writer, value.EntriesStream)
}

type FfiDestroyerTypeRoomListEntriesWithDynamicAdaptersResult struct{}

func (_ FfiDestroyerTypeRoomListEntriesWithDynamicAdaptersResult) Destroy(value RoomListEntriesWithDynamicAdaptersResult) {
	value.Destroy()
}

type RoomListLoadingStateResult struct {
	State       RoomListLoadingState
	StateStream *TaskHandle
}

func (r *RoomListLoadingStateResult) Destroy() {
	FfiDestroyerTypeRoomListLoadingState{}.Destroy(r.State)
	FfiDestroyerTaskHandle{}.Destroy(r.StateStream)
}

type FfiConverterTypeRoomListLoadingStateResult struct{}

var FfiConverterTypeRoomListLoadingStateResultINSTANCE = FfiConverterTypeRoomListLoadingStateResult{}

func (c FfiConverterTypeRoomListLoadingStateResult) Lift(rb RustBufferI) RoomListLoadingStateResult {
	return LiftFromRustBuffer[RoomListLoadingStateResult](c, rb)
}

func (c FfiConverterTypeRoomListLoadingStateResult) Read(reader io.Reader) RoomListLoadingStateResult {
	return RoomListLoadingStateResult{
		FfiConverterTypeRoomListLoadingStateINSTANCE.Read(reader),
		FfiConverterTaskHandleINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomListLoadingStateResult) Lower(value RoomListLoadingStateResult) RustBuffer {
	return LowerIntoRustBuffer[RoomListLoadingStateResult](c, value)
}

func (c FfiConverterTypeRoomListLoadingStateResult) Write(writer io.Writer, value RoomListLoadingStateResult) {
	FfiConverterTypeRoomListLoadingStateINSTANCE.Write(writer, value.State)
	FfiConverterTaskHandleINSTANCE.Write(writer, value.StateStream)
}

type FfiDestroyerTypeRoomListLoadingStateResult struct{}

func (_ FfiDestroyerTypeRoomListLoadingStateResult) Destroy(value RoomListLoadingStateResult) {
	value.Destroy()
}

type RoomListRange struct {
	Start        uint32
	EndInclusive uint32
}

func (r *RoomListRange) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.Start)
	FfiDestroyerUint32{}.Destroy(r.EndInclusive)
}

type FfiConverterTypeRoomListRange struct{}

var FfiConverterTypeRoomListRangeINSTANCE = FfiConverterTypeRoomListRange{}

func (c FfiConverterTypeRoomListRange) Lift(rb RustBufferI) RoomListRange {
	return LiftFromRustBuffer[RoomListRange](c, rb)
}

func (c FfiConverterTypeRoomListRange) Read(reader io.Reader) RoomListRange {
	return RoomListRange{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomListRange) Lower(value RoomListRange) RustBuffer {
	return LowerIntoRustBuffer[RoomListRange](c, value)
}

func (c FfiConverterTypeRoomListRange) Write(writer io.Writer, value RoomListRange) {
	FfiConverterUint32INSTANCE.Write(writer, value.Start)
	FfiConverterUint32INSTANCE.Write(writer, value.EndInclusive)
}

type FfiDestroyerTypeRoomListRange struct{}

func (_ FfiDestroyerTypeRoomListRange) Destroy(value RoomListRange) {
	value.Destroy()
}

type RoomNotificationSettings struct {
	Mode      RoomNotificationMode
	IsDefault bool
}

func (r *RoomNotificationSettings) Destroy() {
	FfiDestroyerTypeRoomNotificationMode{}.Destroy(r.Mode)
	FfiDestroyerBool{}.Destroy(r.IsDefault)
}

type FfiConverterTypeRoomNotificationSettings struct{}

var FfiConverterTypeRoomNotificationSettingsINSTANCE = FfiConverterTypeRoomNotificationSettings{}

func (c FfiConverterTypeRoomNotificationSettings) Lift(rb RustBufferI) RoomNotificationSettings {
	return LiftFromRustBuffer[RoomNotificationSettings](c, rb)
}

func (c FfiConverterTypeRoomNotificationSettings) Read(reader io.Reader) RoomNotificationSettings {
	return RoomNotificationSettings{
		FfiConverterTypeRoomNotificationModeINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomNotificationSettings) Lower(value RoomNotificationSettings) RustBuffer {
	return LowerIntoRustBuffer[RoomNotificationSettings](c, value)
}

func (c FfiConverterTypeRoomNotificationSettings) Write(writer io.Writer, value RoomNotificationSettings) {
	FfiConverterTypeRoomNotificationModeINSTANCE.Write(writer, value.Mode)
	FfiConverterBoolINSTANCE.Write(writer, value.IsDefault)
}

type FfiDestroyerTypeRoomNotificationSettings struct{}

func (_ FfiDestroyerTypeRoomNotificationSettings) Destroy(value RoomNotificationSettings) {
	value.Destroy()
}

type RoomSubscription struct {
	RequiredState *[]RequiredState
	TimelineLimit *uint32
}

func (r *RoomSubscription) Destroy() {
	FfiDestroyerOptionalSequenceTypeRequiredState{}.Destroy(r.RequiredState)
	FfiDestroyerOptionalUint32{}.Destroy(r.TimelineLimit)
}

type FfiConverterTypeRoomSubscription struct{}

var FfiConverterTypeRoomSubscriptionINSTANCE = FfiConverterTypeRoomSubscription{}

func (c FfiConverterTypeRoomSubscription) Lift(rb RustBufferI) RoomSubscription {
	return LiftFromRustBuffer[RoomSubscription](c, rb)
}

func (c FfiConverterTypeRoomSubscription) Read(reader io.Reader) RoomSubscription {
	return RoomSubscription{
		FfiConverterOptionalSequenceTypeRequiredStateINSTANCE.Read(reader),
		FfiConverterOptionalUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomSubscription) Lower(value RoomSubscription) RustBuffer {
	return LowerIntoRustBuffer[RoomSubscription](c, value)
}

func (c FfiConverterTypeRoomSubscription) Write(writer io.Writer, value RoomSubscription) {
	FfiConverterOptionalSequenceTypeRequiredStateINSTANCE.Write(writer, value.RequiredState)
	FfiConverterOptionalUint32INSTANCE.Write(writer, value.TimelineLimit)
}

type FfiDestroyerTypeRoomSubscription struct{}

func (_ FfiDestroyerTypeRoomSubscription) Destroy(value RoomSubscription) {
	value.Destroy()
}

type RoomTimelineListenerResult struct {
	Items       []*TimelineItem
	ItemsStream *TaskHandle
}

func (r *RoomTimelineListenerResult) Destroy() {
	FfiDestroyerSequenceTimelineItem{}.Destroy(r.Items)
	FfiDestroyerTaskHandle{}.Destroy(r.ItemsStream)
}

type FfiConverterTypeRoomTimelineListenerResult struct{}

var FfiConverterTypeRoomTimelineListenerResultINSTANCE = FfiConverterTypeRoomTimelineListenerResult{}

func (c FfiConverterTypeRoomTimelineListenerResult) Lift(rb RustBufferI) RoomTimelineListenerResult {
	return LiftFromRustBuffer[RoomTimelineListenerResult](c, rb)
}

func (c FfiConverterTypeRoomTimelineListenerResult) Read(reader io.Reader) RoomTimelineListenerResult {
	return RoomTimelineListenerResult{
		FfiConverterSequenceTimelineItemINSTANCE.Read(reader),
		FfiConverterTaskHandleINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeRoomTimelineListenerResult) Lower(value RoomTimelineListenerResult) RustBuffer {
	return LowerIntoRustBuffer[RoomTimelineListenerResult](c, value)
}

func (c FfiConverterTypeRoomTimelineListenerResult) Write(writer io.Writer, value RoomTimelineListenerResult) {
	FfiConverterSequenceTimelineItemINSTANCE.Write(writer, value.Items)
	FfiConverterTaskHandleINSTANCE.Write(writer, value.ItemsStream)
}

type FfiDestroyerTypeRoomTimelineListenerResult struct{}

func (_ FfiDestroyerTypeRoomTimelineListenerResult) Destroy(value RoomTimelineListenerResult) {
	value.Destroy()
}

type SearchUsersResults struct {
	Results []UserProfile
	Limited bool
}

func (r *SearchUsersResults) Destroy() {
	FfiDestroyerSequenceTypeUserProfile{}.Destroy(r.Results)
	FfiDestroyerBool{}.Destroy(r.Limited)
}

type FfiConverterTypeSearchUsersResults struct{}

var FfiConverterTypeSearchUsersResultsINSTANCE = FfiConverterTypeSearchUsersResults{}

func (c FfiConverterTypeSearchUsersResults) Lift(rb RustBufferI) SearchUsersResults {
	return LiftFromRustBuffer[SearchUsersResults](c, rb)
}

func (c FfiConverterTypeSearchUsersResults) Read(reader io.Reader) SearchUsersResults {
	return SearchUsersResults{
		FfiConverterSequenceTypeUserProfileINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeSearchUsersResults) Lower(value SearchUsersResults) RustBuffer {
	return LowerIntoRustBuffer[SearchUsersResults](c, value)
}

func (c FfiConverterTypeSearchUsersResults) Write(writer io.Writer, value SearchUsersResults) {
	FfiConverterSequenceTypeUserProfileINSTANCE.Write(writer, value.Results)
	FfiConverterBoolINSTANCE.Write(writer, value.Limited)
}

type FfiDestroyerTypeSearchUsersResults struct{}

func (_ FfiDestroyerTypeSearchUsersResults) Destroy(value SearchUsersResults) {
	value.Destroy()
}

type Session struct {
	AccessToken      string
	RefreshToken     *string
	UserId           string
	DeviceId         string
	HomeserverUrl    string
	OidcData         *string
	SlidingSyncProxy *string
}

func (r *Session) Destroy() {
	FfiDestroyerString{}.Destroy(r.AccessToken)
	FfiDestroyerOptionalString{}.Destroy(r.RefreshToken)
	FfiDestroyerString{}.Destroy(r.UserId)
	FfiDestroyerString{}.Destroy(r.DeviceId)
	FfiDestroyerString{}.Destroy(r.HomeserverUrl)
	FfiDestroyerOptionalString{}.Destroy(r.OidcData)
	FfiDestroyerOptionalString{}.Destroy(r.SlidingSyncProxy)
}

type FfiConverterTypeSession struct{}

var FfiConverterTypeSessionINSTANCE = FfiConverterTypeSession{}

func (c FfiConverterTypeSession) Lift(rb RustBufferI) Session {
	return LiftFromRustBuffer[Session](c, rb)
}

func (c FfiConverterTypeSession) Read(reader io.Reader) Session {
	return Session{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeSession) Lower(value Session) RustBuffer {
	return LowerIntoRustBuffer[Session](c, value)
}

func (c FfiConverterTypeSession) Write(writer io.Writer, value Session) {
	FfiConverterStringINSTANCE.Write(writer, value.AccessToken)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.RefreshToken)
	FfiConverterStringINSTANCE.Write(writer, value.UserId)
	FfiConverterStringINSTANCE.Write(writer, value.DeviceId)
	FfiConverterStringINSTANCE.Write(writer, value.HomeserverUrl)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.OidcData)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.SlidingSyncProxy)
}

type FfiDestroyerTypeSession struct{}

func (_ FfiDestroyerTypeSession) Destroy(value Session) {
	value.Destroy()
}

type SetData struct {
	Index uint32
	Item  *TimelineItem
}

func (r *SetData) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.Index)
	FfiDestroyerTimelineItem{}.Destroy(r.Item)
}

type FfiConverterTypeSetData struct{}

var FfiConverterTypeSetDataINSTANCE = FfiConverterTypeSetData{}

func (c FfiConverterTypeSetData) Lift(rb RustBufferI) SetData {
	return LiftFromRustBuffer[SetData](c, rb)
}

func (c FfiConverterTypeSetData) Read(reader io.Reader) SetData {
	return SetData{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterTimelineItemINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeSetData) Lower(value SetData) RustBuffer {
	return LowerIntoRustBuffer[SetData](c, value)
}

func (c FfiConverterTypeSetData) Write(writer io.Writer, value SetData) {
	FfiConverterUint32INSTANCE.Write(writer, value.Index)
	FfiConverterTimelineItemINSTANCE.Write(writer, value.Item)
}

type FfiDestroyerTypeSetData struct{}

func (_ FfiDestroyerTypeSetData) Destroy(value SetData) {
	value.Destroy()
}

type TextMessageContent struct {
	Body      string
	Formatted *FormattedBody
}

func (r *TextMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerOptionalTypeFormattedBody{}.Destroy(r.Formatted)
}

type FfiConverterTypeTextMessageContent struct{}

var FfiConverterTypeTextMessageContentINSTANCE = FfiConverterTypeTextMessageContent{}

func (c FfiConverterTypeTextMessageContent) Lift(rb RustBufferI) TextMessageContent {
	return LiftFromRustBuffer[TextMessageContent](c, rb)
}

func (c FfiConverterTypeTextMessageContent) Read(reader io.Reader) TextMessageContent {
	return TextMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalTypeFormattedBodyINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeTextMessageContent) Lower(value TextMessageContent) RustBuffer {
	return LowerIntoRustBuffer[TextMessageContent](c, value)
}

func (c FfiConverterTypeTextMessageContent) Write(writer io.Writer, value TextMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterOptionalTypeFormattedBodyINSTANCE.Write(writer, value.Formatted)
}

type FfiDestroyerTypeTextMessageContent struct{}

func (_ FfiDestroyerTypeTextMessageContent) Destroy(value TextMessageContent) {
	value.Destroy()
}

type ThumbnailInfo struct {
	Height   *uint64
	Width    *uint64
	Mimetype *string
	Size     *uint64
}

func (r *ThumbnailInfo) Destroy() {
	FfiDestroyerOptionalUint64{}.Destroy(r.Height)
	FfiDestroyerOptionalUint64{}.Destroy(r.Width)
	FfiDestroyerOptionalString{}.Destroy(r.Mimetype)
	FfiDestroyerOptionalUint64{}.Destroy(r.Size)
}

type FfiConverterTypeThumbnailInfo struct{}

var FfiConverterTypeThumbnailInfoINSTANCE = FfiConverterTypeThumbnailInfo{}

func (c FfiConverterTypeThumbnailInfo) Lift(rb RustBufferI) ThumbnailInfo {
	return LiftFromRustBuffer[ThumbnailInfo](c, rb)
}

func (c FfiConverterTypeThumbnailInfo) Read(reader io.Reader) ThumbnailInfo {
	return ThumbnailInfo{
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeThumbnailInfo) Lower(value ThumbnailInfo) RustBuffer {
	return LowerIntoRustBuffer[ThumbnailInfo](c, value)
}

func (c FfiConverterTypeThumbnailInfo) Write(writer io.Writer, value ThumbnailInfo) {
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Height)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Width)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Mimetype)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Size)
}

type FfiDestroyerTypeThumbnailInfo struct{}

func (_ FfiDestroyerTypeThumbnailInfo) Destroy(value ThumbnailInfo) {
	value.Destroy()
}

type TracingConfiguration struct {
	Filter                string
	WriteToStdoutOrSystem bool
	WriteToFiles          *TracingFileConfiguration
}

func (r *TracingConfiguration) Destroy() {
	FfiDestroyerString{}.Destroy(r.Filter)
	FfiDestroyerBool{}.Destroy(r.WriteToStdoutOrSystem)
	FfiDestroyerOptionalTypeTracingFileConfiguration{}.Destroy(r.WriteToFiles)
}

type FfiConverterTypeTracingConfiguration struct{}

var FfiConverterTypeTracingConfigurationINSTANCE = FfiConverterTypeTracingConfiguration{}

func (c FfiConverterTypeTracingConfiguration) Lift(rb RustBufferI) TracingConfiguration {
	return LiftFromRustBuffer[TracingConfiguration](c, rb)
}

func (c FfiConverterTypeTracingConfiguration) Read(reader io.Reader) TracingConfiguration {
	return TracingConfiguration{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterOptionalTypeTracingFileConfigurationINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeTracingConfiguration) Lower(value TracingConfiguration) RustBuffer {
	return LowerIntoRustBuffer[TracingConfiguration](c, value)
}

func (c FfiConverterTypeTracingConfiguration) Write(writer io.Writer, value TracingConfiguration) {
	FfiConverterStringINSTANCE.Write(writer, value.Filter)
	FfiConverterBoolINSTANCE.Write(writer, value.WriteToStdoutOrSystem)
	FfiConverterOptionalTypeTracingFileConfigurationINSTANCE.Write(writer, value.WriteToFiles)
}

type FfiDestroyerTypeTracingConfiguration struct{}

func (_ FfiDestroyerTypeTracingConfiguration) Destroy(value TracingConfiguration) {
	value.Destroy()
}

type TracingFileConfiguration struct {
	Path       string
	FilePrefix string
}

func (r *TracingFileConfiguration) Destroy() {
	FfiDestroyerString{}.Destroy(r.Path)
	FfiDestroyerString{}.Destroy(r.FilePrefix)
}

type FfiConverterTypeTracingFileConfiguration struct{}

var FfiConverterTypeTracingFileConfigurationINSTANCE = FfiConverterTypeTracingFileConfiguration{}

func (c FfiConverterTypeTracingFileConfiguration) Lift(rb RustBufferI) TracingFileConfiguration {
	return LiftFromRustBuffer[TracingFileConfiguration](c, rb)
}

func (c FfiConverterTypeTracingFileConfiguration) Read(reader io.Reader) TracingFileConfiguration {
	return TracingFileConfiguration{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeTracingFileConfiguration) Lower(value TracingFileConfiguration) RustBuffer {
	return LowerIntoRustBuffer[TracingFileConfiguration](c, value)
}

func (c FfiConverterTypeTracingFileConfiguration) Write(writer io.Writer, value TracingFileConfiguration) {
	FfiConverterStringINSTANCE.Write(writer, value.Path)
	FfiConverterStringINSTANCE.Write(writer, value.FilePrefix)
}

type FfiDestroyerTypeTracingFileConfiguration struct{}

func (_ FfiDestroyerTypeTracingFileConfiguration) Destroy(value TracingFileConfiguration) {
	value.Destroy()
}

type TransmissionProgress struct {
	Current uint64
	Total   uint64
}

func (r *TransmissionProgress) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.Current)
	FfiDestroyerUint64{}.Destroy(r.Total)
}

type FfiConverterTypeTransmissionProgress struct{}

var FfiConverterTypeTransmissionProgressINSTANCE = FfiConverterTypeTransmissionProgress{}

func (c FfiConverterTypeTransmissionProgress) Lift(rb RustBufferI) TransmissionProgress {
	return LiftFromRustBuffer[TransmissionProgress](c, rb)
}

func (c FfiConverterTypeTransmissionProgress) Read(reader io.Reader) TransmissionProgress {
	return TransmissionProgress{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeTransmissionProgress) Lower(value TransmissionProgress) RustBuffer {
	return LowerIntoRustBuffer[TransmissionProgress](c, value)
}

func (c FfiConverterTypeTransmissionProgress) Write(writer io.Writer, value TransmissionProgress) {
	FfiConverterUint64INSTANCE.Write(writer, value.Current)
	FfiConverterUint64INSTANCE.Write(writer, value.Total)
}

type FfiDestroyerTypeTransmissionProgress struct{}

func (_ FfiDestroyerTypeTransmissionProgress) Destroy(value TransmissionProgress) {
	value.Destroy()
}

type UnstableAudioDetailsContent struct {
	Duration time.Duration
	Waveform []uint16
}

func (r *UnstableAudioDetailsContent) Destroy() {
	FfiDestroyerDuration{}.Destroy(r.Duration)
	FfiDestroyerSequenceUint16{}.Destroy(r.Waveform)
}

type FfiConverterTypeUnstableAudioDetailsContent struct{}

var FfiConverterTypeUnstableAudioDetailsContentINSTANCE = FfiConverterTypeUnstableAudioDetailsContent{}

func (c FfiConverterTypeUnstableAudioDetailsContent) Lift(rb RustBufferI) UnstableAudioDetailsContent {
	return LiftFromRustBuffer[UnstableAudioDetailsContent](c, rb)
}

func (c FfiConverterTypeUnstableAudioDetailsContent) Read(reader io.Reader) UnstableAudioDetailsContent {
	return UnstableAudioDetailsContent{
		FfiConverterDurationINSTANCE.Read(reader),
		FfiConverterSequenceUint16INSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeUnstableAudioDetailsContent) Lower(value UnstableAudioDetailsContent) RustBuffer {
	return LowerIntoRustBuffer[UnstableAudioDetailsContent](c, value)
}

func (c FfiConverterTypeUnstableAudioDetailsContent) Write(writer io.Writer, value UnstableAudioDetailsContent) {
	FfiConverterDurationINSTANCE.Write(writer, value.Duration)
	FfiConverterSequenceUint16INSTANCE.Write(writer, value.Waveform)
}

type FfiDestroyerTypeUnstableAudioDetailsContent struct{}

func (_ FfiDestroyerTypeUnstableAudioDetailsContent) Destroy(value UnstableAudioDetailsContent) {
	value.Destroy()
}

type UnstableVoiceContent struct {
}

func (r *UnstableVoiceContent) Destroy() {
}

type FfiConverterTypeUnstableVoiceContent struct{}

var FfiConverterTypeUnstableVoiceContentINSTANCE = FfiConverterTypeUnstableVoiceContent{}

func (c FfiConverterTypeUnstableVoiceContent) Lift(rb RustBufferI) UnstableVoiceContent {
	return LiftFromRustBuffer[UnstableVoiceContent](c, rb)
}

func (c FfiConverterTypeUnstableVoiceContent) Read(reader io.Reader) UnstableVoiceContent {
	return UnstableVoiceContent{}
}

func (c FfiConverterTypeUnstableVoiceContent) Lower(value UnstableVoiceContent) RustBuffer {
	return LowerIntoRustBuffer[UnstableVoiceContent](c, value)
}

func (c FfiConverterTypeUnstableVoiceContent) Write(writer io.Writer, value UnstableVoiceContent) {
}

type FfiDestroyerTypeUnstableVoiceContent struct{}

func (_ FfiDestroyerTypeUnstableVoiceContent) Destroy(value UnstableVoiceContent) {
	value.Destroy()
}

type UserProfile struct {
	UserId      string
	DisplayName *string
	AvatarUrl   *string
}

func (r *UserProfile) Destroy() {
	FfiDestroyerString{}.Destroy(r.UserId)
	FfiDestroyerOptionalString{}.Destroy(r.DisplayName)
	FfiDestroyerOptionalString{}.Destroy(r.AvatarUrl)
}

type FfiConverterTypeUserProfile struct{}

var FfiConverterTypeUserProfileINSTANCE = FfiConverterTypeUserProfile{}

func (c FfiConverterTypeUserProfile) Lift(rb RustBufferI) UserProfile {
	return LiftFromRustBuffer[UserProfile](c, rb)
}

func (c FfiConverterTypeUserProfile) Read(reader io.Reader) UserProfile {
	return UserProfile{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeUserProfile) Lower(value UserProfile) RustBuffer {
	return LowerIntoRustBuffer[UserProfile](c, value)
}

func (c FfiConverterTypeUserProfile) Write(writer io.Writer, value UserProfile) {
	FfiConverterStringINSTANCE.Write(writer, value.UserId)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.DisplayName)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.AvatarUrl)
}

type FfiDestroyerTypeUserProfile struct{}

func (_ FfiDestroyerTypeUserProfile) Destroy(value UserProfile) {
	value.Destroy()
}

type VideoInfo struct {
	Duration        *time.Duration
	Height          *uint64
	Width           *uint64
	Mimetype        *string
	Size            *uint64
	ThumbnailInfo   *ThumbnailInfo
	ThumbnailSource **MediaSource
	Blurhash        *string
}

func (r *VideoInfo) Destroy() {
	FfiDestroyerOptionalDuration{}.Destroy(r.Duration)
	FfiDestroyerOptionalUint64{}.Destroy(r.Height)
	FfiDestroyerOptionalUint64{}.Destroy(r.Width)
	FfiDestroyerOptionalString{}.Destroy(r.Mimetype)
	FfiDestroyerOptionalUint64{}.Destroy(r.Size)
	FfiDestroyerOptionalTypeThumbnailInfo{}.Destroy(r.ThumbnailInfo)
	FfiDestroyerOptionalMediaSource{}.Destroy(r.ThumbnailSource)
	FfiDestroyerOptionalString{}.Destroy(r.Blurhash)
}

type FfiConverterTypeVideoInfo struct{}

var FfiConverterTypeVideoInfoINSTANCE = FfiConverterTypeVideoInfo{}

func (c FfiConverterTypeVideoInfo) Lift(rb RustBufferI) VideoInfo {
	return LiftFromRustBuffer[VideoInfo](c, rb)
}

func (c FfiConverterTypeVideoInfo) Read(reader io.Reader) VideoInfo {
	return VideoInfo{
		FfiConverterOptionalDurationINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalTypeThumbnailInfoINSTANCE.Read(reader),
		FfiConverterOptionalMediaSourceINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeVideoInfo) Lower(value VideoInfo) RustBuffer {
	return LowerIntoRustBuffer[VideoInfo](c, value)
}

func (c FfiConverterTypeVideoInfo) Write(writer io.Writer, value VideoInfo) {
	FfiConverterOptionalDurationINSTANCE.Write(writer, value.Duration)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Height)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Width)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Mimetype)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.Size)
	FfiConverterOptionalTypeThumbnailInfoINSTANCE.Write(writer, value.ThumbnailInfo)
	FfiConverterOptionalMediaSourceINSTANCE.Write(writer, value.ThumbnailSource)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Blurhash)
}

type FfiDestroyerTypeVideoInfo struct{}

func (_ FfiDestroyerTypeVideoInfo) Destroy(value VideoInfo) {
	value.Destroy()
}

type VideoMessageContent struct {
	Body   string
	Source *MediaSource
	Info   *VideoInfo
}

func (r *VideoMessageContent) Destroy() {
	FfiDestroyerString{}.Destroy(r.Body)
	FfiDestroyerMediaSource{}.Destroy(r.Source)
	FfiDestroyerOptionalTypeVideoInfo{}.Destroy(r.Info)
}

type FfiConverterTypeVideoMessageContent struct{}

var FfiConverterTypeVideoMessageContentINSTANCE = FfiConverterTypeVideoMessageContent{}

func (c FfiConverterTypeVideoMessageContent) Lift(rb RustBufferI) VideoMessageContent {
	return LiftFromRustBuffer[VideoMessageContent](c, rb)
}

func (c FfiConverterTypeVideoMessageContent) Read(reader io.Reader) VideoMessageContent {
	return VideoMessageContent{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterMediaSourceINSTANCE.Read(reader),
		FfiConverterOptionalTypeVideoInfoINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeVideoMessageContent) Lower(value VideoMessageContent) RustBuffer {
	return LowerIntoRustBuffer[VideoMessageContent](c, value)
}

func (c FfiConverterTypeVideoMessageContent) Write(writer io.Writer, value VideoMessageContent) {
	FfiConverterStringINSTANCE.Write(writer, value.Body)
	FfiConverterMediaSourceINSTANCE.Write(writer, value.Source)
	FfiConverterOptionalTypeVideoInfoINSTANCE.Write(writer, value.Info)
}

type FfiDestroyerTypeVideoMessageContent struct{}

func (_ FfiDestroyerTypeVideoMessageContent) Destroy(value VideoMessageContent) {
	value.Destroy()
}

type VirtualElementCallWidgetOptions struct {
	ElementCallUrl string
	WidgetId       string
	ParentUrl      *string
	HideHeader     *bool
	Preload        *bool
	FontScale      *float64
	AppPrompt      *bool
	SkipLobby      *bool
	ConfineToRoom  *bool
	Font           *string
	AnalyticsId    *string
	Encryption     EncryptionSystem
}

func (r *VirtualElementCallWidgetOptions) Destroy() {
	FfiDestroyerString{}.Destroy(r.ElementCallUrl)
	FfiDestroyerString{}.Destroy(r.WidgetId)
	FfiDestroyerOptionalString{}.Destroy(r.ParentUrl)
	FfiDestroyerOptionalBool{}.Destroy(r.HideHeader)
	FfiDestroyerOptionalBool{}.Destroy(r.Preload)
	FfiDestroyerOptionalFloat64{}.Destroy(r.FontScale)
	FfiDestroyerOptionalBool{}.Destroy(r.AppPrompt)
	FfiDestroyerOptionalBool{}.Destroy(r.SkipLobby)
	FfiDestroyerOptionalBool{}.Destroy(r.ConfineToRoom)
	FfiDestroyerOptionalString{}.Destroy(r.Font)
	FfiDestroyerOptionalString{}.Destroy(r.AnalyticsId)
	FfiDestroyerTypeEncryptionSystem{}.Destroy(r.Encryption)
}

type FfiConverterTypeVirtualElementCallWidgetOptions struct{}

var FfiConverterTypeVirtualElementCallWidgetOptionsINSTANCE = FfiConverterTypeVirtualElementCallWidgetOptions{}

func (c FfiConverterTypeVirtualElementCallWidgetOptions) Lift(rb RustBufferI) VirtualElementCallWidgetOptions {
	return LiftFromRustBuffer[VirtualElementCallWidgetOptions](c, rb)
}

func (c FfiConverterTypeVirtualElementCallWidgetOptions) Read(reader io.Reader) VirtualElementCallWidgetOptions {
	return VirtualElementCallWidgetOptions{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterOptionalFloat64INSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterOptionalBoolINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterTypeEncryptionSystemINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeVirtualElementCallWidgetOptions) Lower(value VirtualElementCallWidgetOptions) RustBuffer {
	return LowerIntoRustBuffer[VirtualElementCallWidgetOptions](c, value)
}

func (c FfiConverterTypeVirtualElementCallWidgetOptions) Write(writer io.Writer, value VirtualElementCallWidgetOptions) {
	FfiConverterStringINSTANCE.Write(writer, value.ElementCallUrl)
	FfiConverterStringINSTANCE.Write(writer, value.WidgetId)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.ParentUrl)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.HideHeader)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.Preload)
	FfiConverterOptionalFloat64INSTANCE.Write(writer, value.FontScale)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.AppPrompt)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.SkipLobby)
	FfiConverterOptionalBoolINSTANCE.Write(writer, value.ConfineToRoom)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Font)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.AnalyticsId)
	FfiConverterTypeEncryptionSystemINSTANCE.Write(writer, value.Encryption)
}

type FfiDestroyerTypeVirtualElementCallWidgetOptions struct{}

func (_ FfiDestroyerTypeVirtualElementCallWidgetOptions) Destroy(value VirtualElementCallWidgetOptions) {
	value.Destroy()
}

type WidgetCapabilities struct {
	Read           []WidgetEventFilter
	Send           []WidgetEventFilter
	RequiresClient bool
}

func (r *WidgetCapabilities) Destroy() {
	FfiDestroyerSequenceTypeWidgetEventFilter{}.Destroy(r.Read)
	FfiDestroyerSequenceTypeWidgetEventFilter{}.Destroy(r.Send)
	FfiDestroyerBool{}.Destroy(r.RequiresClient)
}

type FfiConverterTypeWidgetCapabilities struct{}

var FfiConverterTypeWidgetCapabilitiesINSTANCE = FfiConverterTypeWidgetCapabilities{}

func (c FfiConverterTypeWidgetCapabilities) Lift(rb RustBufferI) WidgetCapabilities {
	return LiftFromRustBuffer[WidgetCapabilities](c, rb)
}

func (c FfiConverterTypeWidgetCapabilities) Read(reader io.Reader) WidgetCapabilities {
	return WidgetCapabilities{
		FfiConverterSequenceTypeWidgetEventFilterINSTANCE.Read(reader),
		FfiConverterSequenceTypeWidgetEventFilterINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeWidgetCapabilities) Lower(value WidgetCapabilities) RustBuffer {
	return LowerIntoRustBuffer[WidgetCapabilities](c, value)
}

func (c FfiConverterTypeWidgetCapabilities) Write(writer io.Writer, value WidgetCapabilities) {
	FfiConverterSequenceTypeWidgetEventFilterINSTANCE.Write(writer, value.Read)
	FfiConverterSequenceTypeWidgetEventFilterINSTANCE.Write(writer, value.Send)
	FfiConverterBoolINSTANCE.Write(writer, value.RequiresClient)
}

type FfiDestroyerTypeWidgetCapabilities struct{}

func (_ FfiDestroyerTypeWidgetCapabilities) Destroy(value WidgetCapabilities) {
	value.Destroy()
}

type WidgetDriverAndHandle struct {
	Driver *WidgetDriver
	Handle *WidgetDriverHandle
}

func (r *WidgetDriverAndHandle) Destroy() {
	FfiDestroyerWidgetDriver{}.Destroy(r.Driver)
	FfiDestroyerWidgetDriverHandle{}.Destroy(r.Handle)
}

type FfiConverterTypeWidgetDriverAndHandle struct{}

var FfiConverterTypeWidgetDriverAndHandleINSTANCE = FfiConverterTypeWidgetDriverAndHandle{}

func (c FfiConverterTypeWidgetDriverAndHandle) Lift(rb RustBufferI) WidgetDriverAndHandle {
	return LiftFromRustBuffer[WidgetDriverAndHandle](c, rb)
}

func (c FfiConverterTypeWidgetDriverAndHandle) Read(reader io.Reader) WidgetDriverAndHandle {
	return WidgetDriverAndHandle{
		FfiConverterWidgetDriverINSTANCE.Read(reader),
		FfiConverterWidgetDriverHandleINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeWidgetDriverAndHandle) Lower(value WidgetDriverAndHandle) RustBuffer {
	return LowerIntoRustBuffer[WidgetDriverAndHandle](c, value)
}

func (c FfiConverterTypeWidgetDriverAndHandle) Write(writer io.Writer, value WidgetDriverAndHandle) {
	FfiConverterWidgetDriverINSTANCE.Write(writer, value.Driver)
	FfiConverterWidgetDriverHandleINSTANCE.Write(writer, value.Handle)
}

type FfiDestroyerTypeWidgetDriverAndHandle struct{}

func (_ FfiDestroyerTypeWidgetDriverAndHandle) Destroy(value WidgetDriverAndHandle) {
	value.Destroy()
}

type WidgetSettings struct {
	WidgetId             string
	InitAfterContentLoad bool
	RawUrl               string
}

func (r *WidgetSettings) Destroy() {
	FfiDestroyerString{}.Destroy(r.WidgetId)
	FfiDestroyerBool{}.Destroy(r.InitAfterContentLoad)
	FfiDestroyerString{}.Destroy(r.RawUrl)
}

type FfiConverterTypeWidgetSettings struct{}

var FfiConverterTypeWidgetSettingsINSTANCE = FfiConverterTypeWidgetSettings{}

func (c FfiConverterTypeWidgetSettings) Lift(rb RustBufferI) WidgetSettings {
	return LiftFromRustBuffer[WidgetSettings](c, rb)
}

func (c FfiConverterTypeWidgetSettings) Read(reader io.Reader) WidgetSettings {
	return WidgetSettings{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterTypeWidgetSettings) Lower(value WidgetSettings) RustBuffer {
	return LowerIntoRustBuffer[WidgetSettings](c, value)
}

func (c FfiConverterTypeWidgetSettings) Write(writer io.Writer, value WidgetSettings) {
	FfiConverterStringINSTANCE.Write(writer, value.WidgetId)
	FfiConverterBoolINSTANCE.Write(writer, value.InitAfterContentLoad)
	FfiConverterStringINSTANCE.Write(writer, value.RawUrl)
}

type FfiDestroyerTypeWidgetSettings struct{}

func (_ FfiDestroyerTypeWidgetSettings) Destroy(value WidgetSettings) {
	value.Destroy()
}

type AccountManagementAction interface {
	Destroy()
}
type AccountManagementActionProfile struct {
}

func (e AccountManagementActionProfile) Destroy() {
}

type AccountManagementActionSessionsList struct {
}

func (e AccountManagementActionSessionsList) Destroy() {
}

type AccountManagementActionSessionView struct {
	DeviceId string
}

func (e AccountManagementActionSessionView) Destroy() {
	FfiDestroyerString{}.Destroy(e.DeviceId)
}

type AccountManagementActionSessionEnd struct {
	DeviceId string
}

func (e AccountManagementActionSessionEnd) Destroy() {
	FfiDestroyerString{}.Destroy(e.DeviceId)
}

type FfiConverterTypeAccountManagementAction struct{}

var FfiConverterTypeAccountManagementActionINSTANCE = FfiConverterTypeAccountManagementAction{}

func (c FfiConverterTypeAccountManagementAction) Lift(rb RustBufferI) AccountManagementAction {
	return LiftFromRustBuffer[AccountManagementAction](c, rb)
}

func (c FfiConverterTypeAccountManagementAction) Lower(value AccountManagementAction) RustBuffer {
	return LowerIntoRustBuffer[AccountManagementAction](c, value)
}
func (FfiConverterTypeAccountManagementAction) Read(reader io.Reader) AccountManagementAction {
	id := readInt32(reader)
	switch id {
	case 1:
		return AccountManagementActionProfile{}
	case 2:
		return AccountManagementActionSessionsList{}
	case 3:
		return AccountManagementActionSessionView{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 4:
		return AccountManagementActionSessionEnd{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeAccountManagementAction.Read()", id))
	}
}

func (FfiConverterTypeAccountManagementAction) Write(writer io.Writer, value AccountManagementAction) {
	switch variant_value := value.(type) {
	case AccountManagementActionProfile:
		writeInt32(writer, 1)
	case AccountManagementActionSessionsList:
		writeInt32(writer, 2)
	case AccountManagementActionSessionView:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variant_value.DeviceId)
	case AccountManagementActionSessionEnd:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variant_value.DeviceId)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeAccountManagementAction.Write", value))
	}
}

type FfiDestroyerTypeAccountManagementAction struct{}

func (_ FfiDestroyerTypeAccountManagementAction) Destroy(value AccountManagementAction) {
	value.Destroy()
}

type AssetType uint

const (
	AssetTypeSender AssetType = 1
	AssetTypePin    AssetType = 2
)

type FfiConverterTypeAssetType struct{}

var FfiConverterTypeAssetTypeINSTANCE = FfiConverterTypeAssetType{}

func (c FfiConverterTypeAssetType) Lift(rb RustBufferI) AssetType {
	return LiftFromRustBuffer[AssetType](c, rb)
}

func (c FfiConverterTypeAssetType) Lower(value AssetType) RustBuffer {
	return LowerIntoRustBuffer[AssetType](c, value)
}
func (FfiConverterTypeAssetType) Read(reader io.Reader) AssetType {
	id := readInt32(reader)
	return AssetType(id)
}

func (FfiConverterTypeAssetType) Write(writer io.Writer, value AssetType) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeAssetType struct{}

func (_ FfiDestroyerTypeAssetType) Destroy(value AssetType) {
}

type AuthenticationError struct {
	err error
}

func (err AuthenticationError) Error() string {
	return fmt.Sprintf("AuthenticationError: %s", err.err.Error())
}

func (err AuthenticationError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrAuthenticationErrorClientMissing = fmt.Errorf("AuthenticationErrorClientMissing")
var ErrAuthenticationErrorInvalidServerName = fmt.Errorf("AuthenticationErrorInvalidServerName")
var ErrAuthenticationErrorSlidingSyncNotAvailable = fmt.Errorf("AuthenticationErrorSlidingSyncNotAvailable")
var ErrAuthenticationErrorSessionMissing = fmt.Errorf("AuthenticationErrorSessionMissing")
var ErrAuthenticationErrorInvalidBasePath = fmt.Errorf("AuthenticationErrorInvalidBasePath")
var ErrAuthenticationErrorOidcNotSupported = fmt.Errorf("AuthenticationErrorOidcNotSupported")
var ErrAuthenticationErrorOidcMetadataMissing = fmt.Errorf("AuthenticationErrorOidcMetadataMissing")
var ErrAuthenticationErrorOidcMetadataInvalid = fmt.Errorf("AuthenticationErrorOidcMetadataInvalid")
var ErrAuthenticationErrorOidcCallbackUrlInvalid = fmt.Errorf("AuthenticationErrorOidcCallbackUrlInvalid")
var ErrAuthenticationErrorOidcCancelled = fmt.Errorf("AuthenticationErrorOidcCancelled")
var ErrAuthenticationErrorOidcError = fmt.Errorf("AuthenticationErrorOidcError")
var ErrAuthenticationErrorGeneric = fmt.Errorf("AuthenticationErrorGeneric")

// Variant structs
type AuthenticationErrorClientMissing struct {
	message string
}

func NewAuthenticationErrorClientMissing() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorClientMissing{},
	}
}

func (err AuthenticationErrorClientMissing) Error() string {
	return fmt.Sprintf("ClientMissing: %s", err.message)
}

func (self AuthenticationErrorClientMissing) Is(target error) bool {
	return target == ErrAuthenticationErrorClientMissing
}

type AuthenticationErrorInvalidServerName struct {
	message string
}

func NewAuthenticationErrorInvalidServerName() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorInvalidServerName{},
	}
}

func (err AuthenticationErrorInvalidServerName) Error() string {
	return fmt.Sprintf("InvalidServerName: %s", err.message)
}

func (self AuthenticationErrorInvalidServerName) Is(target error) bool {
	return target == ErrAuthenticationErrorInvalidServerName
}

type AuthenticationErrorSlidingSyncNotAvailable struct {
	message string
}

func NewAuthenticationErrorSlidingSyncNotAvailable() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorSlidingSyncNotAvailable{},
	}
}

func (err AuthenticationErrorSlidingSyncNotAvailable) Error() string {
	return fmt.Sprintf("SlidingSyncNotAvailable: %s", err.message)
}

func (self AuthenticationErrorSlidingSyncNotAvailable) Is(target error) bool {
	return target == ErrAuthenticationErrorSlidingSyncNotAvailable
}

type AuthenticationErrorSessionMissing struct {
	message string
}

func NewAuthenticationErrorSessionMissing() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorSessionMissing{},
	}
}

func (err AuthenticationErrorSessionMissing) Error() string {
	return fmt.Sprintf("SessionMissing: %s", err.message)
}

func (self AuthenticationErrorSessionMissing) Is(target error) bool {
	return target == ErrAuthenticationErrorSessionMissing
}

type AuthenticationErrorInvalidBasePath struct {
	message string
}

func NewAuthenticationErrorInvalidBasePath() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorInvalidBasePath{},
	}
}

func (err AuthenticationErrorInvalidBasePath) Error() string {
	return fmt.Sprintf("InvalidBasePath: %s", err.message)
}

func (self AuthenticationErrorInvalidBasePath) Is(target error) bool {
	return target == ErrAuthenticationErrorInvalidBasePath
}

type AuthenticationErrorOidcNotSupported struct {
	message string
}

func NewAuthenticationErrorOidcNotSupported() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorOidcNotSupported{},
	}
}

func (err AuthenticationErrorOidcNotSupported) Error() string {
	return fmt.Sprintf("OidcNotSupported: %s", err.message)
}

func (self AuthenticationErrorOidcNotSupported) Is(target error) bool {
	return target == ErrAuthenticationErrorOidcNotSupported
}

type AuthenticationErrorOidcMetadataMissing struct {
	message string
}

func NewAuthenticationErrorOidcMetadataMissing() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorOidcMetadataMissing{},
	}
}

func (err AuthenticationErrorOidcMetadataMissing) Error() string {
	return fmt.Sprintf("OidcMetadataMissing: %s", err.message)
}

func (self AuthenticationErrorOidcMetadataMissing) Is(target error) bool {
	return target == ErrAuthenticationErrorOidcMetadataMissing
}

type AuthenticationErrorOidcMetadataInvalid struct {
	message string
}

func NewAuthenticationErrorOidcMetadataInvalid() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorOidcMetadataInvalid{},
	}
}

func (err AuthenticationErrorOidcMetadataInvalid) Error() string {
	return fmt.Sprintf("OidcMetadataInvalid: %s", err.message)
}

func (self AuthenticationErrorOidcMetadataInvalid) Is(target error) bool {
	return target == ErrAuthenticationErrorOidcMetadataInvalid
}

type AuthenticationErrorOidcCallbackUrlInvalid struct {
	message string
}

func NewAuthenticationErrorOidcCallbackUrlInvalid() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorOidcCallbackUrlInvalid{},
	}
}

func (err AuthenticationErrorOidcCallbackUrlInvalid) Error() string {
	return fmt.Sprintf("OidcCallbackUrlInvalid: %s", err.message)
}

func (self AuthenticationErrorOidcCallbackUrlInvalid) Is(target error) bool {
	return target == ErrAuthenticationErrorOidcCallbackUrlInvalid
}

type AuthenticationErrorOidcCancelled struct {
	message string
}

func NewAuthenticationErrorOidcCancelled() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorOidcCancelled{},
	}
}

func (err AuthenticationErrorOidcCancelled) Error() string {
	return fmt.Sprintf("OidcCancelled: %s", err.message)
}

func (self AuthenticationErrorOidcCancelled) Is(target error) bool {
	return target == ErrAuthenticationErrorOidcCancelled
}

type AuthenticationErrorOidcError struct {
	message string
}

func NewAuthenticationErrorOidcError() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorOidcError{},
	}
}

func (err AuthenticationErrorOidcError) Error() string {
	return fmt.Sprintf("OidcError: %s", err.message)
}

func (self AuthenticationErrorOidcError) Is(target error) bool {
	return target == ErrAuthenticationErrorOidcError
}

type AuthenticationErrorGeneric struct {
	message string
}

func NewAuthenticationErrorGeneric() *AuthenticationError {
	return &AuthenticationError{
		err: &AuthenticationErrorGeneric{},
	}
}

func (err AuthenticationErrorGeneric) Error() string {
	return fmt.Sprintf("Generic: %s", err.message)
}

func (self AuthenticationErrorGeneric) Is(target error) bool {
	return target == ErrAuthenticationErrorGeneric
}

type FfiConverterTypeAuthenticationError struct{}

var FfiConverterTypeAuthenticationErrorINSTANCE = FfiConverterTypeAuthenticationError{}

func (c FfiConverterTypeAuthenticationError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeAuthenticationError) Lower(value *AuthenticationError) RustBuffer {
	return LowerIntoRustBuffer[*AuthenticationError](c, value)
}

func (c FfiConverterTypeAuthenticationError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &AuthenticationError{&AuthenticationErrorClientMissing{message}}
	case 2:
		return &AuthenticationError{&AuthenticationErrorInvalidServerName{message}}
	case 3:
		return &AuthenticationError{&AuthenticationErrorSlidingSyncNotAvailable{message}}
	case 4:
		return &AuthenticationError{&AuthenticationErrorSessionMissing{message}}
	case 5:
		return &AuthenticationError{&AuthenticationErrorInvalidBasePath{message}}
	case 6:
		return &AuthenticationError{&AuthenticationErrorOidcNotSupported{message}}
	case 7:
		return &AuthenticationError{&AuthenticationErrorOidcMetadataMissing{message}}
	case 8:
		return &AuthenticationError{&AuthenticationErrorOidcMetadataInvalid{message}}
	case 9:
		return &AuthenticationError{&AuthenticationErrorOidcCallbackUrlInvalid{message}}
	case 10:
		return &AuthenticationError{&AuthenticationErrorOidcCancelled{message}}
	case 11:
		return &AuthenticationError{&AuthenticationErrorOidcError{message}}
	case 12:
		return &AuthenticationError{&AuthenticationErrorGeneric{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeAuthenticationError.Read()", errorID))
	}

}

func (c FfiConverterTypeAuthenticationError) Write(writer io.Writer, value *AuthenticationError) {
	switch variantValue := value.err.(type) {
	case *AuthenticationErrorClientMissing:
		writeInt32(writer, 1)
	case *AuthenticationErrorInvalidServerName:
		writeInt32(writer, 2)
	case *AuthenticationErrorSlidingSyncNotAvailable:
		writeInt32(writer, 3)
	case *AuthenticationErrorSessionMissing:
		writeInt32(writer, 4)
	case *AuthenticationErrorInvalidBasePath:
		writeInt32(writer, 5)
	case *AuthenticationErrorOidcNotSupported:
		writeInt32(writer, 6)
	case *AuthenticationErrorOidcMetadataMissing:
		writeInt32(writer, 7)
	case *AuthenticationErrorOidcMetadataInvalid:
		writeInt32(writer, 8)
	case *AuthenticationErrorOidcCallbackUrlInvalid:
		writeInt32(writer, 9)
	case *AuthenticationErrorOidcCancelled:
		writeInt32(writer, 10)
	case *AuthenticationErrorOidcError:
		writeInt32(writer, 11)
	case *AuthenticationErrorGeneric:
		writeInt32(writer, 12)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeAuthenticationError.Write", value))
	}
}

type BackupState uint

const (
	BackupStateUnknown     BackupState = 1
	BackupStateCreating    BackupState = 2
	BackupStateEnabling    BackupState = 3
	BackupStateResuming    BackupState = 4
	BackupStateEnabled     BackupState = 5
	BackupStateDownloading BackupState = 6
	BackupStateDisabling   BackupState = 7
)

type FfiConverterTypeBackupState struct{}

var FfiConverterTypeBackupStateINSTANCE = FfiConverterTypeBackupState{}

func (c FfiConverterTypeBackupState) Lift(rb RustBufferI) BackupState {
	return LiftFromRustBuffer[BackupState](c, rb)
}

func (c FfiConverterTypeBackupState) Lower(value BackupState) RustBuffer {
	return LowerIntoRustBuffer[BackupState](c, value)
}
func (FfiConverterTypeBackupState) Read(reader io.Reader) BackupState {
	id := readInt32(reader)
	return BackupState(id)
}

func (FfiConverterTypeBackupState) Write(writer io.Writer, value BackupState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeBackupState struct{}

func (_ FfiDestroyerTypeBackupState) Destroy(value BackupState) {
}

type BackupUploadState interface {
	Destroy()
}
type BackupUploadStateWaiting struct {
}

func (e BackupUploadStateWaiting) Destroy() {
}

type BackupUploadStateUploading struct {
	BackedUpCount uint32
	TotalCount    uint32
}

func (e BackupUploadStateUploading) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.BackedUpCount)
	FfiDestroyerUint32{}.Destroy(e.TotalCount)
}

type BackupUploadStateError struct {
}

func (e BackupUploadStateError) Destroy() {
}

type BackupUploadStateDone struct {
}

func (e BackupUploadStateDone) Destroy() {
}

type FfiConverterTypeBackupUploadState struct{}

var FfiConverterTypeBackupUploadStateINSTANCE = FfiConverterTypeBackupUploadState{}

func (c FfiConverterTypeBackupUploadState) Lift(rb RustBufferI) BackupUploadState {
	return LiftFromRustBuffer[BackupUploadState](c, rb)
}

func (c FfiConverterTypeBackupUploadState) Lower(value BackupUploadState) RustBuffer {
	return LowerIntoRustBuffer[BackupUploadState](c, value)
}
func (FfiConverterTypeBackupUploadState) Read(reader io.Reader) BackupUploadState {
	id := readInt32(reader)
	switch id {
	case 1:
		return BackupUploadStateWaiting{}
	case 2:
		return BackupUploadStateUploading{
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
		}
	case 3:
		return BackupUploadStateError{}
	case 4:
		return BackupUploadStateDone{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeBackupUploadState.Read()", id))
	}
}

func (FfiConverterTypeBackupUploadState) Write(writer io.Writer, value BackupUploadState) {
	switch variant_value := value.(type) {
	case BackupUploadStateWaiting:
		writeInt32(writer, 1)
	case BackupUploadStateUploading:
		writeInt32(writer, 2)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.BackedUpCount)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.TotalCount)
	case BackupUploadStateError:
		writeInt32(writer, 3)
	case BackupUploadStateDone:
		writeInt32(writer, 4)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeBackupUploadState.Write", value))
	}
}

type FfiDestroyerTypeBackupUploadState struct{}

func (_ FfiDestroyerTypeBackupUploadState) Destroy(value BackupUploadState) {
	value.Destroy()
}

type ClientError struct {
	err error
}

func (err ClientError) Error() string {
	return fmt.Sprintf("ClientError: %s", err.err.Error())
}

func (err ClientError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrClientErrorGeneric = fmt.Errorf("ClientErrorGeneric")

// Variant structs
type ClientErrorGeneric struct {
	Msg string
}

func NewClientErrorGeneric(
	msg string,
) *ClientError {
	return &ClientError{
		err: &ClientErrorGeneric{
			Msg: msg,
		},
	}
}

func (err ClientErrorGeneric) Error() string {
	return fmt.Sprint("Generic",
		": ",

		"Msg=",
		err.Msg,
	)
}

func (self ClientErrorGeneric) Is(target error) bool {
	return target == ErrClientErrorGeneric
}

type FfiConverterTypeClientError struct{}

var FfiConverterTypeClientErrorINSTANCE = FfiConverterTypeClientError{}

func (c FfiConverterTypeClientError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeClientError) Lower(value *ClientError) RustBuffer {
	return LowerIntoRustBuffer[*ClientError](c, value)
}

func (c FfiConverterTypeClientError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	switch errorID {
	case 1:
		return &ClientError{&ClientErrorGeneric{
			Msg: FfiConverterStringINSTANCE.Read(reader),
		}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeClientError.Read()", errorID))
	}
}

func (c FfiConverterTypeClientError) Write(writer io.Writer, value *ClientError) {
	switch variantValue := value.err.(type) {
	case *ClientErrorGeneric:
		writeInt32(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, variantValue.Msg)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeClientError.Write", value))
	}
}

type EnableRecoveryProgress interface {
	Destroy()
}
type EnableRecoveryProgressStarting struct {
}

func (e EnableRecoveryProgressStarting) Destroy() {
}

type EnableRecoveryProgressCreatingBackup struct {
}

func (e EnableRecoveryProgressCreatingBackup) Destroy() {
}

type EnableRecoveryProgressCreatingRecoveryKey struct {
}

func (e EnableRecoveryProgressCreatingRecoveryKey) Destroy() {
}

type EnableRecoveryProgressBackingUp struct {
	BackedUpCount uint32
	TotalCount    uint32
}

func (e EnableRecoveryProgressBackingUp) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.BackedUpCount)
	FfiDestroyerUint32{}.Destroy(e.TotalCount)
}

type EnableRecoveryProgressRoomKeyUploadError struct {
}

func (e EnableRecoveryProgressRoomKeyUploadError) Destroy() {
}

type EnableRecoveryProgressDone struct {
	RecoveryKey string
}

func (e EnableRecoveryProgressDone) Destroy() {
	FfiDestroyerString{}.Destroy(e.RecoveryKey)
}

type FfiConverterTypeEnableRecoveryProgress struct{}

var FfiConverterTypeEnableRecoveryProgressINSTANCE = FfiConverterTypeEnableRecoveryProgress{}

func (c FfiConverterTypeEnableRecoveryProgress) Lift(rb RustBufferI) EnableRecoveryProgress {
	return LiftFromRustBuffer[EnableRecoveryProgress](c, rb)
}

func (c FfiConverterTypeEnableRecoveryProgress) Lower(value EnableRecoveryProgress) RustBuffer {
	return LowerIntoRustBuffer[EnableRecoveryProgress](c, value)
}
func (FfiConverterTypeEnableRecoveryProgress) Read(reader io.Reader) EnableRecoveryProgress {
	id := readInt32(reader)
	switch id {
	case 1:
		return EnableRecoveryProgressStarting{}
	case 2:
		return EnableRecoveryProgressCreatingBackup{}
	case 3:
		return EnableRecoveryProgressCreatingRecoveryKey{}
	case 4:
		return EnableRecoveryProgressBackingUp{
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
		}
	case 5:
		return EnableRecoveryProgressRoomKeyUploadError{}
	case 6:
		return EnableRecoveryProgressDone{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeEnableRecoveryProgress.Read()", id))
	}
}

func (FfiConverterTypeEnableRecoveryProgress) Write(writer io.Writer, value EnableRecoveryProgress) {
	switch variant_value := value.(type) {
	case EnableRecoveryProgressStarting:
		writeInt32(writer, 1)
	case EnableRecoveryProgressCreatingBackup:
		writeInt32(writer, 2)
	case EnableRecoveryProgressCreatingRecoveryKey:
		writeInt32(writer, 3)
	case EnableRecoveryProgressBackingUp:
		writeInt32(writer, 4)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.BackedUpCount)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.TotalCount)
	case EnableRecoveryProgressRoomKeyUploadError:
		writeInt32(writer, 5)
	case EnableRecoveryProgressDone:
		writeInt32(writer, 6)
		FfiConverterStringINSTANCE.Write(writer, variant_value.RecoveryKey)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeEnableRecoveryProgress.Write", value))
	}
}

type FfiDestroyerTypeEnableRecoveryProgress struct{}

func (_ FfiDestroyerTypeEnableRecoveryProgress) Destroy(value EnableRecoveryProgress) {
	value.Destroy()
}

type EncryptedMessage interface {
	Destroy()
}
type EncryptedMessageOlmV1Curve25519AesSha2 struct {
	SenderKey string
}

func (e EncryptedMessageOlmV1Curve25519AesSha2) Destroy() {
	FfiDestroyerString{}.Destroy(e.SenderKey)
}

type EncryptedMessageMegolmV1AesSha2 struct {
	SessionId string
}

func (e EncryptedMessageMegolmV1AesSha2) Destroy() {
	FfiDestroyerString{}.Destroy(e.SessionId)
}

type EncryptedMessageUnknown struct {
}

func (e EncryptedMessageUnknown) Destroy() {
}

type FfiConverterTypeEncryptedMessage struct{}

var FfiConverterTypeEncryptedMessageINSTANCE = FfiConverterTypeEncryptedMessage{}

func (c FfiConverterTypeEncryptedMessage) Lift(rb RustBufferI) EncryptedMessage {
	return LiftFromRustBuffer[EncryptedMessage](c, rb)
}

func (c FfiConverterTypeEncryptedMessage) Lower(value EncryptedMessage) RustBuffer {
	return LowerIntoRustBuffer[EncryptedMessage](c, value)
}
func (FfiConverterTypeEncryptedMessage) Read(reader io.Reader) EncryptedMessage {
	id := readInt32(reader)
	switch id {
	case 1:
		return EncryptedMessageOlmV1Curve25519AesSha2{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 2:
		return EncryptedMessageMegolmV1AesSha2{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 3:
		return EncryptedMessageUnknown{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeEncryptedMessage.Read()", id))
	}
}

func (FfiConverterTypeEncryptedMessage) Write(writer io.Writer, value EncryptedMessage) {
	switch variant_value := value.(type) {
	case EncryptedMessageOlmV1Curve25519AesSha2:
		writeInt32(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, variant_value.SenderKey)
	case EncryptedMessageMegolmV1AesSha2:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.SessionId)
	case EncryptedMessageUnknown:
		writeInt32(writer, 3)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeEncryptedMessage.Write", value))
	}
}

type FfiDestroyerTypeEncryptedMessage struct{}

func (_ FfiDestroyerTypeEncryptedMessage) Destroy(value EncryptedMessage) {
	value.Destroy()
}

type EncryptionSystem interface {
	Destroy()
}
type EncryptionSystemUnencrypted struct {
}

func (e EncryptionSystemUnencrypted) Destroy() {
}

type EncryptionSystemPerParticipantKeys struct {
}

func (e EncryptionSystemPerParticipantKeys) Destroy() {
}

type EncryptionSystemSharedSecret struct {
	Secret string
}

func (e EncryptionSystemSharedSecret) Destroy() {
	FfiDestroyerString{}.Destroy(e.Secret)
}

type FfiConverterTypeEncryptionSystem struct{}

var FfiConverterTypeEncryptionSystemINSTANCE = FfiConverterTypeEncryptionSystem{}

func (c FfiConverterTypeEncryptionSystem) Lift(rb RustBufferI) EncryptionSystem {
	return LiftFromRustBuffer[EncryptionSystem](c, rb)
}

func (c FfiConverterTypeEncryptionSystem) Lower(value EncryptionSystem) RustBuffer {
	return LowerIntoRustBuffer[EncryptionSystem](c, value)
}
func (FfiConverterTypeEncryptionSystem) Read(reader io.Reader) EncryptionSystem {
	id := readInt32(reader)
	switch id {
	case 1:
		return EncryptionSystemUnencrypted{}
	case 2:
		return EncryptionSystemPerParticipantKeys{}
	case 3:
		return EncryptionSystemSharedSecret{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeEncryptionSystem.Read()", id))
	}
}

func (FfiConverterTypeEncryptionSystem) Write(writer io.Writer, value EncryptionSystem) {
	switch variant_value := value.(type) {
	case EncryptionSystemUnencrypted:
		writeInt32(writer, 1)
	case EncryptionSystemPerParticipantKeys:
		writeInt32(writer, 2)
	case EncryptionSystemSharedSecret:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Secret)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeEncryptionSystem.Write", value))
	}
}

type FfiDestroyerTypeEncryptionSystem struct{}

func (_ FfiDestroyerTypeEncryptionSystem) Destroy(value EncryptionSystem) {
	value.Destroy()
}

type EventSendState interface {
	Destroy()
}
type EventSendStateNotSentYet struct {
}

func (e EventSendStateNotSentYet) Destroy() {
}

type EventSendStateSendingFailed struct {
	Error string
}

func (e EventSendStateSendingFailed) Destroy() {
	FfiDestroyerString{}.Destroy(e.Error)
}

type EventSendStateCancelled struct {
}

func (e EventSendStateCancelled) Destroy() {
}

type EventSendStateSent struct {
	EventId string
}

func (e EventSendStateSent) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventId)
}

type FfiConverterTypeEventSendState struct{}

var FfiConverterTypeEventSendStateINSTANCE = FfiConverterTypeEventSendState{}

func (c FfiConverterTypeEventSendState) Lift(rb RustBufferI) EventSendState {
	return LiftFromRustBuffer[EventSendState](c, rb)
}

func (c FfiConverterTypeEventSendState) Lower(value EventSendState) RustBuffer {
	return LowerIntoRustBuffer[EventSendState](c, value)
}
func (FfiConverterTypeEventSendState) Read(reader io.Reader) EventSendState {
	id := readInt32(reader)
	switch id {
	case 1:
		return EventSendStateNotSentYet{}
	case 2:
		return EventSendStateSendingFailed{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 3:
		return EventSendStateCancelled{}
	case 4:
		return EventSendStateSent{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeEventSendState.Read()", id))
	}
}

func (FfiConverterTypeEventSendState) Write(writer io.Writer, value EventSendState) {
	switch variant_value := value.(type) {
	case EventSendStateNotSentYet:
		writeInt32(writer, 1)
	case EventSendStateSendingFailed:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Error)
	case EventSendStateCancelled:
		writeInt32(writer, 3)
	case EventSendStateSent:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventId)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeEventSendState.Write", value))
	}
}

type FfiDestroyerTypeEventSendState struct{}

func (_ FfiDestroyerTypeEventSendState) Destroy(value EventSendState) {
	value.Destroy()
}

type FilterTimelineEventType interface {
	Destroy()
}
type FilterTimelineEventTypeMessageLike struct {
	EventType MessageLikeEventType
}

func (e FilterTimelineEventTypeMessageLike) Destroy() {
	FfiDestroyerTypeMessageLikeEventType{}.Destroy(e.EventType)
}

type FilterTimelineEventTypeState struct {
	EventType StateEventType
}

func (e FilterTimelineEventTypeState) Destroy() {
	FfiDestroyerTypeStateEventType{}.Destroy(e.EventType)
}

type FfiConverterTypeFilterTimelineEventType struct{}

var FfiConverterTypeFilterTimelineEventTypeINSTANCE = FfiConverterTypeFilterTimelineEventType{}

func (c FfiConverterTypeFilterTimelineEventType) Lift(rb RustBufferI) FilterTimelineEventType {
	return LiftFromRustBuffer[FilterTimelineEventType](c, rb)
}

func (c FfiConverterTypeFilterTimelineEventType) Lower(value FilterTimelineEventType) RustBuffer {
	return LowerIntoRustBuffer[FilterTimelineEventType](c, value)
}
func (FfiConverterTypeFilterTimelineEventType) Read(reader io.Reader) FilterTimelineEventType {
	id := readInt32(reader)
	switch id {
	case 1:
		return FilterTimelineEventTypeMessageLike{
			FfiConverterTypeMessageLikeEventTypeINSTANCE.Read(reader),
		}
	case 2:
		return FilterTimelineEventTypeState{
			FfiConverterTypeStateEventTypeINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeFilterTimelineEventType.Read()", id))
	}
}

func (FfiConverterTypeFilterTimelineEventType) Write(writer io.Writer, value FilterTimelineEventType) {
	switch variant_value := value.(type) {
	case FilterTimelineEventTypeMessageLike:
		writeInt32(writer, 1)
		FfiConverterTypeMessageLikeEventTypeINSTANCE.Write(writer, variant_value.EventType)
	case FilterTimelineEventTypeState:
		writeInt32(writer, 2)
		FfiConverterTypeStateEventTypeINSTANCE.Write(writer, variant_value.EventType)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeFilterTimelineEventType.Write", value))
	}
}

type FfiDestroyerTypeFilterTimelineEventType struct{}

func (_ FfiDestroyerTypeFilterTimelineEventType) Destroy(value FilterTimelineEventType) {
	value.Destroy()
}

type LogLevel uint

const (
	LogLevelError LogLevel = 1
	LogLevelWarn  LogLevel = 2
	LogLevelInfo  LogLevel = 3
	LogLevelDebug LogLevel = 4
	LogLevelTrace LogLevel = 5
)

type FfiConverterTypeLogLevel struct{}

var FfiConverterTypeLogLevelINSTANCE = FfiConverterTypeLogLevel{}

func (c FfiConverterTypeLogLevel) Lift(rb RustBufferI) LogLevel {
	return LiftFromRustBuffer[LogLevel](c, rb)
}

func (c FfiConverterTypeLogLevel) Lower(value LogLevel) RustBuffer {
	return LowerIntoRustBuffer[LogLevel](c, value)
}
func (FfiConverterTypeLogLevel) Read(reader io.Reader) LogLevel {
	id := readInt32(reader)
	return LogLevel(id)
}

func (FfiConverterTypeLogLevel) Write(writer io.Writer, value LogLevel) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeLogLevel struct{}

func (_ FfiDestroyerTypeLogLevel) Destroy(value LogLevel) {
}

type MediaInfoError struct {
	err error
}

func (err MediaInfoError) Error() string {
	return fmt.Sprintf("MediaInfoError: %s", err.err.Error())
}

func (err MediaInfoError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrMediaInfoErrorMissingField = fmt.Errorf("MediaInfoErrorMissingField")
var ErrMediaInfoErrorInvalidField = fmt.Errorf("MediaInfoErrorInvalidField")

// Variant structs
type MediaInfoErrorMissingField struct {
	message string
}

func NewMediaInfoErrorMissingField() *MediaInfoError {
	return &MediaInfoError{
		err: &MediaInfoErrorMissingField{},
	}
}

func (err MediaInfoErrorMissingField) Error() string {
	return fmt.Sprintf("MissingField: %s", err.message)
}

func (self MediaInfoErrorMissingField) Is(target error) bool {
	return target == ErrMediaInfoErrorMissingField
}

type MediaInfoErrorInvalidField struct {
	message string
}

func NewMediaInfoErrorInvalidField() *MediaInfoError {
	return &MediaInfoError{
		err: &MediaInfoErrorInvalidField{},
	}
}

func (err MediaInfoErrorInvalidField) Error() string {
	return fmt.Sprintf("InvalidField: %s", err.message)
}

func (self MediaInfoErrorInvalidField) Is(target error) bool {
	return target == ErrMediaInfoErrorInvalidField
}

type FfiConverterTypeMediaInfoError struct{}

var FfiConverterTypeMediaInfoErrorINSTANCE = FfiConverterTypeMediaInfoError{}

func (c FfiConverterTypeMediaInfoError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeMediaInfoError) Lower(value *MediaInfoError) RustBuffer {
	return LowerIntoRustBuffer[*MediaInfoError](c, value)
}

func (c FfiConverterTypeMediaInfoError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &MediaInfoError{&MediaInfoErrorMissingField{message}}
	case 2:
		return &MediaInfoError{&MediaInfoErrorInvalidField{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeMediaInfoError.Read()", errorID))
	}

}

func (c FfiConverterTypeMediaInfoError) Write(writer io.Writer, value *MediaInfoError) {
	switch variantValue := value.err.(type) {
	case *MediaInfoErrorMissingField:
		writeInt32(writer, 1)
	case *MediaInfoErrorInvalidField:
		writeInt32(writer, 2)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeMediaInfoError.Write", value))
	}
}

type Membership uint

const (
	MembershipInvited Membership = 1
	MembershipJoined  Membership = 2
	MembershipLeft    Membership = 3
)

type FfiConverterTypeMembership struct{}

var FfiConverterTypeMembershipINSTANCE = FfiConverterTypeMembership{}

func (c FfiConverterTypeMembership) Lift(rb RustBufferI) Membership {
	return LiftFromRustBuffer[Membership](c, rb)
}

func (c FfiConverterTypeMembership) Lower(value Membership) RustBuffer {
	return LowerIntoRustBuffer[Membership](c, value)
}
func (FfiConverterTypeMembership) Read(reader io.Reader) Membership {
	id := readInt32(reader)
	return Membership(id)
}

func (FfiConverterTypeMembership) Write(writer io.Writer, value Membership) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeMembership struct{}

func (_ FfiDestroyerTypeMembership) Destroy(value Membership) {
}

type MembershipChange uint

const (
	MembershipChangeNone               MembershipChange = 1
	MembershipChangeError              MembershipChange = 2
	MembershipChangeJoined             MembershipChange = 3
	MembershipChangeLeft               MembershipChange = 4
	MembershipChangeBanned             MembershipChange = 5
	MembershipChangeUnbanned           MembershipChange = 6
	MembershipChangeKicked             MembershipChange = 7
	MembershipChangeInvited            MembershipChange = 8
	MembershipChangeKickedAndBanned    MembershipChange = 9
	MembershipChangeInvitationAccepted MembershipChange = 10
	MembershipChangeInvitationRejected MembershipChange = 11
	MembershipChangeInvitationRevoked  MembershipChange = 12
	MembershipChangeKnocked            MembershipChange = 13
	MembershipChangeKnockAccepted      MembershipChange = 14
	MembershipChangeKnockRetracted     MembershipChange = 15
	MembershipChangeKnockDenied        MembershipChange = 16
	MembershipChangeNotImplemented     MembershipChange = 17
)

type FfiConverterTypeMembershipChange struct{}

var FfiConverterTypeMembershipChangeINSTANCE = FfiConverterTypeMembershipChange{}

func (c FfiConverterTypeMembershipChange) Lift(rb RustBufferI) MembershipChange {
	return LiftFromRustBuffer[MembershipChange](c, rb)
}

func (c FfiConverterTypeMembershipChange) Lower(value MembershipChange) RustBuffer {
	return LowerIntoRustBuffer[MembershipChange](c, value)
}
func (FfiConverterTypeMembershipChange) Read(reader io.Reader) MembershipChange {
	id := readInt32(reader)
	return MembershipChange(id)
}

func (FfiConverterTypeMembershipChange) Write(writer io.Writer, value MembershipChange) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeMembershipChange struct{}

func (_ FfiDestroyerTypeMembershipChange) Destroy(value MembershipChange) {
}

type MembershipState uint

const (
	MembershipStateBan    MembershipState = 1
	MembershipStateInvite MembershipState = 2
	MembershipStateJoin   MembershipState = 3
	MembershipStateKnock  MembershipState = 4
	MembershipStateLeave  MembershipState = 5
)

type FfiConverterTypeMembershipState struct{}

var FfiConverterTypeMembershipStateINSTANCE = FfiConverterTypeMembershipState{}

func (c FfiConverterTypeMembershipState) Lift(rb RustBufferI) MembershipState {
	return LiftFromRustBuffer[MembershipState](c, rb)
}

func (c FfiConverterTypeMembershipState) Lower(value MembershipState) RustBuffer {
	return LowerIntoRustBuffer[MembershipState](c, value)
}
func (FfiConverterTypeMembershipState) Read(reader io.Reader) MembershipState {
	id := readInt32(reader)
	return MembershipState(id)
}

func (FfiConverterTypeMembershipState) Write(writer io.Writer, value MembershipState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeMembershipState struct{}

func (_ FfiDestroyerTypeMembershipState) Destroy(value MembershipState) {
}

type MessageFormat interface {
	Destroy()
}
type MessageFormatHtml struct {
}

func (e MessageFormatHtml) Destroy() {
}

type MessageFormatUnknown struct {
	Format string
}

func (e MessageFormatUnknown) Destroy() {
	FfiDestroyerString{}.Destroy(e.Format)
}

type FfiConverterTypeMessageFormat struct{}

var FfiConverterTypeMessageFormatINSTANCE = FfiConverterTypeMessageFormat{}

func (c FfiConverterTypeMessageFormat) Lift(rb RustBufferI) MessageFormat {
	return LiftFromRustBuffer[MessageFormat](c, rb)
}

func (c FfiConverterTypeMessageFormat) Lower(value MessageFormat) RustBuffer {
	return LowerIntoRustBuffer[MessageFormat](c, value)
}
func (FfiConverterTypeMessageFormat) Read(reader io.Reader) MessageFormat {
	id := readInt32(reader)
	switch id {
	case 1:
		return MessageFormatHtml{}
	case 2:
		return MessageFormatUnknown{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeMessageFormat.Read()", id))
	}
}

func (FfiConverterTypeMessageFormat) Write(writer io.Writer, value MessageFormat) {
	switch variant_value := value.(type) {
	case MessageFormatHtml:
		writeInt32(writer, 1)
	case MessageFormatUnknown:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Format)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeMessageFormat.Write", value))
	}
}

type FfiDestroyerTypeMessageFormat struct{}

func (_ FfiDestroyerTypeMessageFormat) Destroy(value MessageFormat) {
	value.Destroy()
}

type MessageLikeEventContent interface {
	Destroy()
}
type MessageLikeEventContentCallAnswer struct {
}

func (e MessageLikeEventContentCallAnswer) Destroy() {
}

type MessageLikeEventContentCallInvite struct {
}

func (e MessageLikeEventContentCallInvite) Destroy() {
}

type MessageLikeEventContentCallHangup struct {
}

func (e MessageLikeEventContentCallHangup) Destroy() {
}

type MessageLikeEventContentCallCandidates struct {
}

func (e MessageLikeEventContentCallCandidates) Destroy() {
}

type MessageLikeEventContentKeyVerificationReady struct {
}

func (e MessageLikeEventContentKeyVerificationReady) Destroy() {
}

type MessageLikeEventContentKeyVerificationStart struct {
}

func (e MessageLikeEventContentKeyVerificationStart) Destroy() {
}

type MessageLikeEventContentKeyVerificationCancel struct {
}

func (e MessageLikeEventContentKeyVerificationCancel) Destroy() {
}

type MessageLikeEventContentKeyVerificationAccept struct {
}

func (e MessageLikeEventContentKeyVerificationAccept) Destroy() {
}

type MessageLikeEventContentKeyVerificationKey struct {
}

func (e MessageLikeEventContentKeyVerificationKey) Destroy() {
}

type MessageLikeEventContentKeyVerificationMac struct {
}

func (e MessageLikeEventContentKeyVerificationMac) Destroy() {
}

type MessageLikeEventContentKeyVerificationDone struct {
}

func (e MessageLikeEventContentKeyVerificationDone) Destroy() {
}

type MessageLikeEventContentPoll struct {
	Question string
}

func (e MessageLikeEventContentPoll) Destroy() {
	FfiDestroyerString{}.Destroy(e.Question)
}

type MessageLikeEventContentReactionContent struct {
	RelatedEventId string
}

func (e MessageLikeEventContentReactionContent) Destroy() {
	FfiDestroyerString{}.Destroy(e.RelatedEventId)
}

type MessageLikeEventContentRoomEncrypted struct {
}

func (e MessageLikeEventContentRoomEncrypted) Destroy() {
}

type MessageLikeEventContentRoomMessage struct {
	MessageType      MessageType
	InReplyToEventId *string
}

func (e MessageLikeEventContentRoomMessage) Destroy() {
	FfiDestroyerTypeMessageType{}.Destroy(e.MessageType)
	FfiDestroyerOptionalString{}.Destroy(e.InReplyToEventId)
}

type MessageLikeEventContentRoomRedaction struct {
}

func (e MessageLikeEventContentRoomRedaction) Destroy() {
}

type MessageLikeEventContentSticker struct {
}

func (e MessageLikeEventContentSticker) Destroy() {
}

type FfiConverterTypeMessageLikeEventContent struct{}

var FfiConverterTypeMessageLikeEventContentINSTANCE = FfiConverterTypeMessageLikeEventContent{}

func (c FfiConverterTypeMessageLikeEventContent) Lift(rb RustBufferI) MessageLikeEventContent {
	return LiftFromRustBuffer[MessageLikeEventContent](c, rb)
}

func (c FfiConverterTypeMessageLikeEventContent) Lower(value MessageLikeEventContent) RustBuffer {
	return LowerIntoRustBuffer[MessageLikeEventContent](c, value)
}
func (FfiConverterTypeMessageLikeEventContent) Read(reader io.Reader) MessageLikeEventContent {
	id := readInt32(reader)
	switch id {
	case 1:
		return MessageLikeEventContentCallAnswer{}
	case 2:
		return MessageLikeEventContentCallInvite{}
	case 3:
		return MessageLikeEventContentCallHangup{}
	case 4:
		return MessageLikeEventContentCallCandidates{}
	case 5:
		return MessageLikeEventContentKeyVerificationReady{}
	case 6:
		return MessageLikeEventContentKeyVerificationStart{}
	case 7:
		return MessageLikeEventContentKeyVerificationCancel{}
	case 8:
		return MessageLikeEventContentKeyVerificationAccept{}
	case 9:
		return MessageLikeEventContentKeyVerificationKey{}
	case 10:
		return MessageLikeEventContentKeyVerificationMac{}
	case 11:
		return MessageLikeEventContentKeyVerificationDone{}
	case 12:
		return MessageLikeEventContentPoll{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 13:
		return MessageLikeEventContentReactionContent{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 14:
		return MessageLikeEventContentRoomEncrypted{}
	case 15:
		return MessageLikeEventContentRoomMessage{
			FfiConverterTypeMessageTypeINSTANCE.Read(reader),
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 16:
		return MessageLikeEventContentRoomRedaction{}
	case 17:
		return MessageLikeEventContentSticker{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeMessageLikeEventContent.Read()", id))
	}
}

func (FfiConverterTypeMessageLikeEventContent) Write(writer io.Writer, value MessageLikeEventContent) {
	switch variant_value := value.(type) {
	case MessageLikeEventContentCallAnswer:
		writeInt32(writer, 1)
	case MessageLikeEventContentCallInvite:
		writeInt32(writer, 2)
	case MessageLikeEventContentCallHangup:
		writeInt32(writer, 3)
	case MessageLikeEventContentCallCandidates:
		writeInt32(writer, 4)
	case MessageLikeEventContentKeyVerificationReady:
		writeInt32(writer, 5)
	case MessageLikeEventContentKeyVerificationStart:
		writeInt32(writer, 6)
	case MessageLikeEventContentKeyVerificationCancel:
		writeInt32(writer, 7)
	case MessageLikeEventContentKeyVerificationAccept:
		writeInt32(writer, 8)
	case MessageLikeEventContentKeyVerificationKey:
		writeInt32(writer, 9)
	case MessageLikeEventContentKeyVerificationMac:
		writeInt32(writer, 10)
	case MessageLikeEventContentKeyVerificationDone:
		writeInt32(writer, 11)
	case MessageLikeEventContentPoll:
		writeInt32(writer, 12)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Question)
	case MessageLikeEventContentReactionContent:
		writeInt32(writer, 13)
		FfiConverterStringINSTANCE.Write(writer, variant_value.RelatedEventId)
	case MessageLikeEventContentRoomEncrypted:
		writeInt32(writer, 14)
	case MessageLikeEventContentRoomMessage:
		writeInt32(writer, 15)
		FfiConverterTypeMessageTypeINSTANCE.Write(writer, variant_value.MessageType)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.InReplyToEventId)
	case MessageLikeEventContentRoomRedaction:
		writeInt32(writer, 16)
	case MessageLikeEventContentSticker:
		writeInt32(writer, 17)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeMessageLikeEventContent.Write", value))
	}
}

type FfiDestroyerTypeMessageLikeEventContent struct{}

func (_ FfiDestroyerTypeMessageLikeEventContent) Destroy(value MessageLikeEventContent) {
	value.Destroy()
}

type MessageLikeEventType uint

const (
	MessageLikeEventTypeCallAnswer            MessageLikeEventType = 1
	MessageLikeEventTypeCallCandidates        MessageLikeEventType = 2
	MessageLikeEventTypeCallHangup            MessageLikeEventType = 3
	MessageLikeEventTypeCallInvite            MessageLikeEventType = 4
	MessageLikeEventTypeKeyVerificationAccept MessageLikeEventType = 5
	MessageLikeEventTypeKeyVerificationCancel MessageLikeEventType = 6
	MessageLikeEventTypeKeyVerificationDone   MessageLikeEventType = 7
	MessageLikeEventTypeKeyVerificationKey    MessageLikeEventType = 8
	MessageLikeEventTypeKeyVerificationMac    MessageLikeEventType = 9
	MessageLikeEventTypeKeyVerificationReady  MessageLikeEventType = 10
	MessageLikeEventTypeKeyVerificationStart  MessageLikeEventType = 11
	MessageLikeEventTypePollEnd               MessageLikeEventType = 12
	MessageLikeEventTypePollResponse          MessageLikeEventType = 13
	MessageLikeEventTypePollStart             MessageLikeEventType = 14
	MessageLikeEventTypeReaction              MessageLikeEventType = 15
	MessageLikeEventTypeRoomEncrypted         MessageLikeEventType = 16
	MessageLikeEventTypeRoomMessage           MessageLikeEventType = 17
	MessageLikeEventTypeRoomRedaction         MessageLikeEventType = 18
	MessageLikeEventTypeSticker               MessageLikeEventType = 19
	MessageLikeEventTypeUnstablePollEnd       MessageLikeEventType = 20
	MessageLikeEventTypeUnstablePollResponse  MessageLikeEventType = 21
	MessageLikeEventTypeUnstablePollStart     MessageLikeEventType = 22
)

type FfiConverterTypeMessageLikeEventType struct{}

var FfiConverterTypeMessageLikeEventTypeINSTANCE = FfiConverterTypeMessageLikeEventType{}

func (c FfiConverterTypeMessageLikeEventType) Lift(rb RustBufferI) MessageLikeEventType {
	return LiftFromRustBuffer[MessageLikeEventType](c, rb)
}

func (c FfiConverterTypeMessageLikeEventType) Lower(value MessageLikeEventType) RustBuffer {
	return LowerIntoRustBuffer[MessageLikeEventType](c, value)
}
func (FfiConverterTypeMessageLikeEventType) Read(reader io.Reader) MessageLikeEventType {
	id := readInt32(reader)
	return MessageLikeEventType(id)
}

func (FfiConverterTypeMessageLikeEventType) Write(writer io.Writer, value MessageLikeEventType) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeMessageLikeEventType struct{}

func (_ FfiDestroyerTypeMessageLikeEventType) Destroy(value MessageLikeEventType) {
}

type MessageType interface {
	Destroy()
}
type MessageTypeEmote struct {
	Content EmoteMessageContent
}

func (e MessageTypeEmote) Destroy() {
	FfiDestroyerTypeEmoteMessageContent{}.Destroy(e.Content)
}

type MessageTypeImage struct {
	Content ImageMessageContent
}

func (e MessageTypeImage) Destroy() {
	FfiDestroyerTypeImageMessageContent{}.Destroy(e.Content)
}

type MessageTypeAudio struct {
	Content AudioMessageContent
}

func (e MessageTypeAudio) Destroy() {
	FfiDestroyerTypeAudioMessageContent{}.Destroy(e.Content)
}

type MessageTypeVideo struct {
	Content VideoMessageContent
}

func (e MessageTypeVideo) Destroy() {
	FfiDestroyerTypeVideoMessageContent{}.Destroy(e.Content)
}

type MessageTypeFile struct {
	Content FileMessageContent
}

func (e MessageTypeFile) Destroy() {
	FfiDestroyerTypeFileMessageContent{}.Destroy(e.Content)
}

type MessageTypeNotice struct {
	Content NoticeMessageContent
}

func (e MessageTypeNotice) Destroy() {
	FfiDestroyerTypeNoticeMessageContent{}.Destroy(e.Content)
}

type MessageTypeText struct {
	Content TextMessageContent
}

func (e MessageTypeText) Destroy() {
	FfiDestroyerTypeTextMessageContent{}.Destroy(e.Content)
}

type MessageTypeLocation struct {
	Content LocationContent
}

func (e MessageTypeLocation) Destroy() {
	FfiDestroyerTypeLocationContent{}.Destroy(e.Content)
}

type MessageTypeOther struct {
	Msgtype string
	Body    string
}

func (e MessageTypeOther) Destroy() {
	FfiDestroyerString{}.Destroy(e.Msgtype)
	FfiDestroyerString{}.Destroy(e.Body)
}

type FfiConverterTypeMessageType struct{}

var FfiConverterTypeMessageTypeINSTANCE = FfiConverterTypeMessageType{}

func (c FfiConverterTypeMessageType) Lift(rb RustBufferI) MessageType {
	return LiftFromRustBuffer[MessageType](c, rb)
}

func (c FfiConverterTypeMessageType) Lower(value MessageType) RustBuffer {
	return LowerIntoRustBuffer[MessageType](c, value)
}
func (FfiConverterTypeMessageType) Read(reader io.Reader) MessageType {
	id := readInt32(reader)
	switch id {
	case 1:
		return MessageTypeEmote{
			FfiConverterTypeEmoteMessageContentINSTANCE.Read(reader),
		}
	case 2:
		return MessageTypeImage{
			FfiConverterTypeImageMessageContentINSTANCE.Read(reader),
		}
	case 3:
		return MessageTypeAudio{
			FfiConverterTypeAudioMessageContentINSTANCE.Read(reader),
		}
	case 4:
		return MessageTypeVideo{
			FfiConverterTypeVideoMessageContentINSTANCE.Read(reader),
		}
	case 5:
		return MessageTypeFile{
			FfiConverterTypeFileMessageContentINSTANCE.Read(reader),
		}
	case 6:
		return MessageTypeNotice{
			FfiConverterTypeNoticeMessageContentINSTANCE.Read(reader),
		}
	case 7:
		return MessageTypeText{
			FfiConverterTypeTextMessageContentINSTANCE.Read(reader),
		}
	case 8:
		return MessageTypeLocation{
			FfiConverterTypeLocationContentINSTANCE.Read(reader),
		}
	case 9:
		return MessageTypeOther{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeMessageType.Read()", id))
	}
}

func (FfiConverterTypeMessageType) Write(writer io.Writer, value MessageType) {
	switch variant_value := value.(type) {
	case MessageTypeEmote:
		writeInt32(writer, 1)
		FfiConverterTypeEmoteMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeImage:
		writeInt32(writer, 2)
		FfiConverterTypeImageMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeAudio:
		writeInt32(writer, 3)
		FfiConverterTypeAudioMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeVideo:
		writeInt32(writer, 4)
		FfiConverterTypeVideoMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeFile:
		writeInt32(writer, 5)
		FfiConverterTypeFileMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeNotice:
		writeInt32(writer, 6)
		FfiConverterTypeNoticeMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeText:
		writeInt32(writer, 7)
		FfiConverterTypeTextMessageContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeLocation:
		writeInt32(writer, 8)
		FfiConverterTypeLocationContentINSTANCE.Write(writer, variant_value.Content)
	case MessageTypeOther:
		writeInt32(writer, 9)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Msgtype)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Body)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeMessageType.Write", value))
	}
}

type FfiDestroyerTypeMessageType struct{}

func (_ FfiDestroyerTypeMessageType) Destroy(value MessageType) {
	value.Destroy()
}

type NotificationEvent interface {
	Destroy()
}
type NotificationEventTimeline struct {
	Event *TimelineEvent
}

func (e NotificationEventTimeline) Destroy() {
	FfiDestroyerTimelineEvent{}.Destroy(e.Event)
}

type NotificationEventInvite struct {
	Sender string
}

func (e NotificationEventInvite) Destroy() {
	FfiDestroyerString{}.Destroy(e.Sender)
}

type FfiConverterTypeNotificationEvent struct{}

var FfiConverterTypeNotificationEventINSTANCE = FfiConverterTypeNotificationEvent{}

func (c FfiConverterTypeNotificationEvent) Lift(rb RustBufferI) NotificationEvent {
	return LiftFromRustBuffer[NotificationEvent](c, rb)
}

func (c FfiConverterTypeNotificationEvent) Lower(value NotificationEvent) RustBuffer {
	return LowerIntoRustBuffer[NotificationEvent](c, value)
}
func (FfiConverterTypeNotificationEvent) Read(reader io.Reader) NotificationEvent {
	id := readInt32(reader)
	switch id {
	case 1:
		return NotificationEventTimeline{
			FfiConverterTimelineEventINSTANCE.Read(reader),
		}
	case 2:
		return NotificationEventInvite{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeNotificationEvent.Read()", id))
	}
}

func (FfiConverterTypeNotificationEvent) Write(writer io.Writer, value NotificationEvent) {
	switch variant_value := value.(type) {
	case NotificationEventTimeline:
		writeInt32(writer, 1)
		FfiConverterTimelineEventINSTANCE.Write(writer, variant_value.Event)
	case NotificationEventInvite:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Sender)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeNotificationEvent.Write", value))
	}
}

type FfiDestroyerTypeNotificationEvent struct{}

func (_ FfiDestroyerTypeNotificationEvent) Destroy(value NotificationEvent) {
	value.Destroy()
}

type NotificationProcessSetup interface {
	Destroy()
}
type NotificationProcessSetupMultipleProcesses struct {
}

func (e NotificationProcessSetupMultipleProcesses) Destroy() {
}

type NotificationProcessSetupSingleProcess struct {
	SyncService *SyncService
}

func (e NotificationProcessSetupSingleProcess) Destroy() {
	FfiDestroyerSyncService{}.Destroy(e.SyncService)
}

type FfiConverterTypeNotificationProcessSetup struct{}

var FfiConverterTypeNotificationProcessSetupINSTANCE = FfiConverterTypeNotificationProcessSetup{}

func (c FfiConverterTypeNotificationProcessSetup) Lift(rb RustBufferI) NotificationProcessSetup {
	return LiftFromRustBuffer[NotificationProcessSetup](c, rb)
}

func (c FfiConverterTypeNotificationProcessSetup) Lower(value NotificationProcessSetup) RustBuffer {
	return LowerIntoRustBuffer[NotificationProcessSetup](c, value)
}
func (FfiConverterTypeNotificationProcessSetup) Read(reader io.Reader) NotificationProcessSetup {
	id := readInt32(reader)
	switch id {
	case 1:
		return NotificationProcessSetupMultipleProcesses{}
	case 2:
		return NotificationProcessSetupSingleProcess{
			FfiConverterSyncServiceINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeNotificationProcessSetup.Read()", id))
	}
}

func (FfiConverterTypeNotificationProcessSetup) Write(writer io.Writer, value NotificationProcessSetup) {
	switch variant_value := value.(type) {
	case NotificationProcessSetupMultipleProcesses:
		writeInt32(writer, 1)
	case NotificationProcessSetupSingleProcess:
		writeInt32(writer, 2)
		FfiConverterSyncServiceINSTANCE.Write(writer, variant_value.SyncService)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeNotificationProcessSetup.Write", value))
	}
}

type FfiDestroyerTypeNotificationProcessSetup struct{}

func (_ FfiDestroyerTypeNotificationProcessSetup) Destroy(value NotificationProcessSetup) {
	value.Destroy()
}

type NotificationSettingsError struct {
	err error
}

func (err NotificationSettingsError) Error() string {
	return fmt.Sprintf("NotificationSettingsError: %s", err.err.Error())
}

func (err NotificationSettingsError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrNotificationSettingsErrorGeneric = fmt.Errorf("NotificationSettingsErrorGeneric")
var ErrNotificationSettingsErrorInvalidParameter = fmt.Errorf("NotificationSettingsErrorInvalidParameter")
var ErrNotificationSettingsErrorInvalidRoomId = fmt.Errorf("NotificationSettingsErrorInvalidRoomId")
var ErrNotificationSettingsErrorRuleNotFound = fmt.Errorf("NotificationSettingsErrorRuleNotFound")
var ErrNotificationSettingsErrorUnableToAddPushRule = fmt.Errorf("NotificationSettingsErrorUnableToAddPushRule")
var ErrNotificationSettingsErrorUnableToRemovePushRule = fmt.Errorf("NotificationSettingsErrorUnableToRemovePushRule")
var ErrNotificationSettingsErrorUnableToSavePushRules = fmt.Errorf("NotificationSettingsErrorUnableToSavePushRules")
var ErrNotificationSettingsErrorUnableToUpdatePushRule = fmt.Errorf("NotificationSettingsErrorUnableToUpdatePushRule")

// Variant structs
type NotificationSettingsErrorGeneric struct {
	Msg string
}

func NewNotificationSettingsErrorGeneric(
	msg string,
) *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorGeneric{
			Msg: msg,
		},
	}
}

func (err NotificationSettingsErrorGeneric) Error() string {
	return fmt.Sprint("Generic",
		": ",

		"Msg=",
		err.Msg,
	)
}

func (self NotificationSettingsErrorGeneric) Is(target error) bool {
	return target == ErrNotificationSettingsErrorGeneric
}

type NotificationSettingsErrorInvalidParameter struct {
	Msg string
}

func NewNotificationSettingsErrorInvalidParameter(
	msg string,
) *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorInvalidParameter{
			Msg: msg,
		},
	}
}

func (err NotificationSettingsErrorInvalidParameter) Error() string {
	return fmt.Sprint("InvalidParameter",
		": ",

		"Msg=",
		err.Msg,
	)
}

func (self NotificationSettingsErrorInvalidParameter) Is(target error) bool {
	return target == ErrNotificationSettingsErrorInvalidParameter
}

type NotificationSettingsErrorInvalidRoomId struct {
	RoomId string
}

func NewNotificationSettingsErrorInvalidRoomId(
	roomId string,
) *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorInvalidRoomId{
			RoomId: roomId,
		},
	}
}

func (err NotificationSettingsErrorInvalidRoomId) Error() string {
	return fmt.Sprint("InvalidRoomId",
		": ",

		"RoomId=",
		err.RoomId,
	)
}

func (self NotificationSettingsErrorInvalidRoomId) Is(target error) bool {
	return target == ErrNotificationSettingsErrorInvalidRoomId
}

type NotificationSettingsErrorRuleNotFound struct {
	RuleId string
}

func NewNotificationSettingsErrorRuleNotFound(
	ruleId string,
) *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorRuleNotFound{
			RuleId: ruleId,
		},
	}
}

func (err NotificationSettingsErrorRuleNotFound) Error() string {
	return fmt.Sprint("RuleNotFound",
		": ",

		"RuleId=",
		err.RuleId,
	)
}

func (self NotificationSettingsErrorRuleNotFound) Is(target error) bool {
	return target == ErrNotificationSettingsErrorRuleNotFound
}

type NotificationSettingsErrorUnableToAddPushRule struct {
}

func NewNotificationSettingsErrorUnableToAddPushRule() *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorUnableToAddPushRule{},
	}
}

func (err NotificationSettingsErrorUnableToAddPushRule) Error() string {
	return fmt.Sprint("UnableToAddPushRule")
}

func (self NotificationSettingsErrorUnableToAddPushRule) Is(target error) bool {
	return target == ErrNotificationSettingsErrorUnableToAddPushRule
}

type NotificationSettingsErrorUnableToRemovePushRule struct {
}

func NewNotificationSettingsErrorUnableToRemovePushRule() *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorUnableToRemovePushRule{},
	}
}

func (err NotificationSettingsErrorUnableToRemovePushRule) Error() string {
	return fmt.Sprint("UnableToRemovePushRule")
}

func (self NotificationSettingsErrorUnableToRemovePushRule) Is(target error) bool {
	return target == ErrNotificationSettingsErrorUnableToRemovePushRule
}

type NotificationSettingsErrorUnableToSavePushRules struct {
}

func NewNotificationSettingsErrorUnableToSavePushRules() *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorUnableToSavePushRules{},
	}
}

func (err NotificationSettingsErrorUnableToSavePushRules) Error() string {
	return fmt.Sprint("UnableToSavePushRules")
}

func (self NotificationSettingsErrorUnableToSavePushRules) Is(target error) bool {
	return target == ErrNotificationSettingsErrorUnableToSavePushRules
}

type NotificationSettingsErrorUnableToUpdatePushRule struct {
}

func NewNotificationSettingsErrorUnableToUpdatePushRule() *NotificationSettingsError {
	return &NotificationSettingsError{
		err: &NotificationSettingsErrorUnableToUpdatePushRule{},
	}
}

func (err NotificationSettingsErrorUnableToUpdatePushRule) Error() string {
	return fmt.Sprint("UnableToUpdatePushRule")
}

func (self NotificationSettingsErrorUnableToUpdatePushRule) Is(target error) bool {
	return target == ErrNotificationSettingsErrorUnableToUpdatePushRule
}

type FfiConverterTypeNotificationSettingsError struct{}

var FfiConverterTypeNotificationSettingsErrorINSTANCE = FfiConverterTypeNotificationSettingsError{}

func (c FfiConverterTypeNotificationSettingsError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeNotificationSettingsError) Lower(value *NotificationSettingsError) RustBuffer {
	return LowerIntoRustBuffer[*NotificationSettingsError](c, value)
}

func (c FfiConverterTypeNotificationSettingsError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	switch errorID {
	case 1:
		return &NotificationSettingsError{&NotificationSettingsErrorGeneric{
			Msg: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 2:
		return &NotificationSettingsError{&NotificationSettingsErrorInvalidParameter{
			Msg: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 3:
		return &NotificationSettingsError{&NotificationSettingsErrorInvalidRoomId{
			RoomId: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 4:
		return &NotificationSettingsError{&NotificationSettingsErrorRuleNotFound{
			RuleId: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 5:
		return &NotificationSettingsError{&NotificationSettingsErrorUnableToAddPushRule{}}
	case 6:
		return &NotificationSettingsError{&NotificationSettingsErrorUnableToRemovePushRule{}}
	case 7:
		return &NotificationSettingsError{&NotificationSettingsErrorUnableToSavePushRules{}}
	case 8:
		return &NotificationSettingsError{&NotificationSettingsErrorUnableToUpdatePushRule{}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeNotificationSettingsError.Read()", errorID))
	}
}

func (c FfiConverterTypeNotificationSettingsError) Write(writer io.Writer, value *NotificationSettingsError) {
	switch variantValue := value.err.(type) {
	case *NotificationSettingsErrorGeneric:
		writeInt32(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, variantValue.Msg)
	case *NotificationSettingsErrorInvalidParameter:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variantValue.Msg)
	case *NotificationSettingsErrorInvalidRoomId:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variantValue.RoomId)
	case *NotificationSettingsErrorRuleNotFound:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variantValue.RuleId)
	case *NotificationSettingsErrorUnableToAddPushRule:
		writeInt32(writer, 5)
	case *NotificationSettingsErrorUnableToRemovePushRule:
		writeInt32(writer, 6)
	case *NotificationSettingsErrorUnableToSavePushRules:
		writeInt32(writer, 7)
	case *NotificationSettingsErrorUnableToUpdatePushRule:
		writeInt32(writer, 8)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeNotificationSettingsError.Write", value))
	}
}

type OtherState interface {
	Destroy()
}
type OtherStatePolicyRuleRoom struct {
}

func (e OtherStatePolicyRuleRoom) Destroy() {
}

type OtherStatePolicyRuleServer struct {
}

func (e OtherStatePolicyRuleServer) Destroy() {
}

type OtherStatePolicyRuleUser struct {
}

func (e OtherStatePolicyRuleUser) Destroy() {
}

type OtherStateRoomAliases struct {
}

func (e OtherStateRoomAliases) Destroy() {
}

type OtherStateRoomAvatar struct {
	Url *string
}

func (e OtherStateRoomAvatar) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(e.Url)
}

type OtherStateRoomCanonicalAlias struct {
}

func (e OtherStateRoomCanonicalAlias) Destroy() {
}

type OtherStateRoomCreate struct {
}

func (e OtherStateRoomCreate) Destroy() {
}

type OtherStateRoomEncryption struct {
}

func (e OtherStateRoomEncryption) Destroy() {
}

type OtherStateRoomGuestAccess struct {
}

func (e OtherStateRoomGuestAccess) Destroy() {
}

type OtherStateRoomHistoryVisibility struct {
}

func (e OtherStateRoomHistoryVisibility) Destroy() {
}

type OtherStateRoomJoinRules struct {
}

func (e OtherStateRoomJoinRules) Destroy() {
}

type OtherStateRoomName struct {
	Name *string
}

func (e OtherStateRoomName) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(e.Name)
}

type OtherStateRoomPinnedEvents struct {
}

func (e OtherStateRoomPinnedEvents) Destroy() {
}

type OtherStateRoomPowerLevels struct {
}

func (e OtherStateRoomPowerLevels) Destroy() {
}

type OtherStateRoomServerAcl struct {
}

func (e OtherStateRoomServerAcl) Destroy() {
}

type OtherStateRoomThirdPartyInvite struct {
	DisplayName *string
}

func (e OtherStateRoomThirdPartyInvite) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(e.DisplayName)
}

type OtherStateRoomTombstone struct {
}

func (e OtherStateRoomTombstone) Destroy() {
}

type OtherStateRoomTopic struct {
	Topic *string
}

func (e OtherStateRoomTopic) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(e.Topic)
}

type OtherStateSpaceChild struct {
}

func (e OtherStateSpaceChild) Destroy() {
}

type OtherStateSpaceParent struct {
}

func (e OtherStateSpaceParent) Destroy() {
}

type OtherStateCustom struct {
	EventType string
}

func (e OtherStateCustom) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventType)
}

type FfiConverterTypeOtherState struct{}

var FfiConverterTypeOtherStateINSTANCE = FfiConverterTypeOtherState{}

func (c FfiConverterTypeOtherState) Lift(rb RustBufferI) OtherState {
	return LiftFromRustBuffer[OtherState](c, rb)
}

func (c FfiConverterTypeOtherState) Lower(value OtherState) RustBuffer {
	return LowerIntoRustBuffer[OtherState](c, value)
}
func (FfiConverterTypeOtherState) Read(reader io.Reader) OtherState {
	id := readInt32(reader)
	switch id {
	case 1:
		return OtherStatePolicyRuleRoom{}
	case 2:
		return OtherStatePolicyRuleServer{}
	case 3:
		return OtherStatePolicyRuleUser{}
	case 4:
		return OtherStateRoomAliases{}
	case 5:
		return OtherStateRoomAvatar{
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 6:
		return OtherStateRoomCanonicalAlias{}
	case 7:
		return OtherStateRoomCreate{}
	case 8:
		return OtherStateRoomEncryption{}
	case 9:
		return OtherStateRoomGuestAccess{}
	case 10:
		return OtherStateRoomHistoryVisibility{}
	case 11:
		return OtherStateRoomJoinRules{}
	case 12:
		return OtherStateRoomName{
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 13:
		return OtherStateRoomPinnedEvents{}
	case 14:
		return OtherStateRoomPowerLevels{}
	case 15:
		return OtherStateRoomServerAcl{}
	case 16:
		return OtherStateRoomThirdPartyInvite{
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 17:
		return OtherStateRoomTombstone{}
	case 18:
		return OtherStateRoomTopic{
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 19:
		return OtherStateSpaceChild{}
	case 20:
		return OtherStateSpaceParent{}
	case 21:
		return OtherStateCustom{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeOtherState.Read()", id))
	}
}

func (FfiConverterTypeOtherState) Write(writer io.Writer, value OtherState) {
	switch variant_value := value.(type) {
	case OtherStatePolicyRuleRoom:
		writeInt32(writer, 1)
	case OtherStatePolicyRuleServer:
		writeInt32(writer, 2)
	case OtherStatePolicyRuleUser:
		writeInt32(writer, 3)
	case OtherStateRoomAliases:
		writeInt32(writer, 4)
	case OtherStateRoomAvatar:
		writeInt32(writer, 5)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.Url)
	case OtherStateRoomCanonicalAlias:
		writeInt32(writer, 6)
	case OtherStateRoomCreate:
		writeInt32(writer, 7)
	case OtherStateRoomEncryption:
		writeInt32(writer, 8)
	case OtherStateRoomGuestAccess:
		writeInt32(writer, 9)
	case OtherStateRoomHistoryVisibility:
		writeInt32(writer, 10)
	case OtherStateRoomJoinRules:
		writeInt32(writer, 11)
	case OtherStateRoomName:
		writeInt32(writer, 12)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.Name)
	case OtherStateRoomPinnedEvents:
		writeInt32(writer, 13)
	case OtherStateRoomPowerLevels:
		writeInt32(writer, 14)
	case OtherStateRoomServerAcl:
		writeInt32(writer, 15)
	case OtherStateRoomThirdPartyInvite:
		writeInt32(writer, 16)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.DisplayName)
	case OtherStateRoomTombstone:
		writeInt32(writer, 17)
	case OtherStateRoomTopic:
		writeInt32(writer, 18)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.Topic)
	case OtherStateSpaceChild:
		writeInt32(writer, 19)
	case OtherStateSpaceParent:
		writeInt32(writer, 20)
	case OtherStateCustom:
		writeInt32(writer, 21)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventType)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeOtherState.Write", value))
	}
}

type FfiDestroyerTypeOtherState struct{}

func (_ FfiDestroyerTypeOtherState) Destroy(value OtherState) {
	value.Destroy()
}

type PaginationOptions interface {
	Destroy()
}
type PaginationOptionsSimpleRequest struct {
	EventLimit   uint16
	WaitForToken bool
}

func (e PaginationOptionsSimpleRequest) Destroy() {
	FfiDestroyerUint16{}.Destroy(e.EventLimit)
	FfiDestroyerBool{}.Destroy(e.WaitForToken)
}

type PaginationOptionsUntilNumItems struct {
	EventLimit   uint16
	Items        uint16
	WaitForToken bool
}

func (e PaginationOptionsUntilNumItems) Destroy() {
	FfiDestroyerUint16{}.Destroy(e.EventLimit)
	FfiDestroyerUint16{}.Destroy(e.Items)
	FfiDestroyerBool{}.Destroy(e.WaitForToken)
}

type FfiConverterTypePaginationOptions struct{}

var FfiConverterTypePaginationOptionsINSTANCE = FfiConverterTypePaginationOptions{}

func (c FfiConverterTypePaginationOptions) Lift(rb RustBufferI) PaginationOptions {
	return LiftFromRustBuffer[PaginationOptions](c, rb)
}

func (c FfiConverterTypePaginationOptions) Lower(value PaginationOptions) RustBuffer {
	return LowerIntoRustBuffer[PaginationOptions](c, value)
}
func (FfiConverterTypePaginationOptions) Read(reader io.Reader) PaginationOptions {
	id := readInt32(reader)
	switch id {
	case 1:
		return PaginationOptionsSimpleRequest{
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterBoolINSTANCE.Read(reader),
		}
	case 2:
		return PaginationOptionsUntilNumItems{
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterBoolINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypePaginationOptions.Read()", id))
	}
}

func (FfiConverterTypePaginationOptions) Write(writer io.Writer, value PaginationOptions) {
	switch variant_value := value.(type) {
	case PaginationOptionsSimpleRequest:
		writeInt32(writer, 1)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.EventLimit)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.WaitForToken)
	case PaginationOptionsUntilNumItems:
		writeInt32(writer, 2)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.EventLimit)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.Items)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.WaitForToken)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypePaginationOptions.Write", value))
	}
}

type FfiDestroyerTypePaginationOptions struct{}

func (_ FfiDestroyerTypePaginationOptions) Destroy(value PaginationOptions) {
	value.Destroy()
}

type ParseError struct {
	err error
}

func (err ParseError) Error() string {
	return fmt.Sprintf("ParseError: %s", err.err.Error())
}

func (err ParseError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrParseErrorEmptyHost = fmt.Errorf("ParseErrorEmptyHost")
var ErrParseErrorIdnaError = fmt.Errorf("ParseErrorIdnaError")
var ErrParseErrorInvalidPort = fmt.Errorf("ParseErrorInvalidPort")
var ErrParseErrorInvalidIpv4Address = fmt.Errorf("ParseErrorInvalidIpv4Address")
var ErrParseErrorInvalidIpv6Address = fmt.Errorf("ParseErrorInvalidIpv6Address")
var ErrParseErrorInvalidDomainCharacter = fmt.Errorf("ParseErrorInvalidDomainCharacter")
var ErrParseErrorRelativeUrlWithoutBase = fmt.Errorf("ParseErrorRelativeUrlWithoutBase")
var ErrParseErrorRelativeUrlWithCannotBeABaseBase = fmt.Errorf("ParseErrorRelativeUrlWithCannotBeABaseBase")
var ErrParseErrorSetHostOnCannotBeABaseUrl = fmt.Errorf("ParseErrorSetHostOnCannotBeABaseUrl")
var ErrParseErrorOverflow = fmt.Errorf("ParseErrorOverflow")
var ErrParseErrorOther = fmt.Errorf("ParseErrorOther")

// Variant structs
type ParseErrorEmptyHost struct {
	message string
}

func NewParseErrorEmptyHost() *ParseError {
	return &ParseError{
		err: &ParseErrorEmptyHost{},
	}
}

func (err ParseErrorEmptyHost) Error() string {
	return fmt.Sprintf("EmptyHost: %s", err.message)
}

func (self ParseErrorEmptyHost) Is(target error) bool {
	return target == ErrParseErrorEmptyHost
}

type ParseErrorIdnaError struct {
	message string
}

func NewParseErrorIdnaError() *ParseError {
	return &ParseError{
		err: &ParseErrorIdnaError{},
	}
}

func (err ParseErrorIdnaError) Error() string {
	return fmt.Sprintf("IdnaError: %s", err.message)
}

func (self ParseErrorIdnaError) Is(target error) bool {
	return target == ErrParseErrorIdnaError
}

type ParseErrorInvalidPort struct {
	message string
}

func NewParseErrorInvalidPort() *ParseError {
	return &ParseError{
		err: &ParseErrorInvalidPort{},
	}
}

func (err ParseErrorInvalidPort) Error() string {
	return fmt.Sprintf("InvalidPort: %s", err.message)
}

func (self ParseErrorInvalidPort) Is(target error) bool {
	return target == ErrParseErrorInvalidPort
}

type ParseErrorInvalidIpv4Address struct {
	message string
}

func NewParseErrorInvalidIpv4Address() *ParseError {
	return &ParseError{
		err: &ParseErrorInvalidIpv4Address{},
	}
}

func (err ParseErrorInvalidIpv4Address) Error() string {
	return fmt.Sprintf("InvalidIpv4Address: %s", err.message)
}

func (self ParseErrorInvalidIpv4Address) Is(target error) bool {
	return target == ErrParseErrorInvalidIpv4Address
}

type ParseErrorInvalidIpv6Address struct {
	message string
}

func NewParseErrorInvalidIpv6Address() *ParseError {
	return &ParseError{
		err: &ParseErrorInvalidIpv6Address{},
	}
}

func (err ParseErrorInvalidIpv6Address) Error() string {
	return fmt.Sprintf("InvalidIpv6Address: %s", err.message)
}

func (self ParseErrorInvalidIpv6Address) Is(target error) bool {
	return target == ErrParseErrorInvalidIpv6Address
}

type ParseErrorInvalidDomainCharacter struct {
	message string
}

func NewParseErrorInvalidDomainCharacter() *ParseError {
	return &ParseError{
		err: &ParseErrorInvalidDomainCharacter{},
	}
}

func (err ParseErrorInvalidDomainCharacter) Error() string {
	return fmt.Sprintf("InvalidDomainCharacter: %s", err.message)
}

func (self ParseErrorInvalidDomainCharacter) Is(target error) bool {
	return target == ErrParseErrorInvalidDomainCharacter
}

type ParseErrorRelativeUrlWithoutBase struct {
	message string
}

func NewParseErrorRelativeUrlWithoutBase() *ParseError {
	return &ParseError{
		err: &ParseErrorRelativeUrlWithoutBase{},
	}
}

func (err ParseErrorRelativeUrlWithoutBase) Error() string {
	return fmt.Sprintf("RelativeUrlWithoutBase: %s", err.message)
}

func (self ParseErrorRelativeUrlWithoutBase) Is(target error) bool {
	return target == ErrParseErrorRelativeUrlWithoutBase
}

type ParseErrorRelativeUrlWithCannotBeABaseBase struct {
	message string
}

func NewParseErrorRelativeUrlWithCannotBeABaseBase() *ParseError {
	return &ParseError{
		err: &ParseErrorRelativeUrlWithCannotBeABaseBase{},
	}
}

func (err ParseErrorRelativeUrlWithCannotBeABaseBase) Error() string {
	return fmt.Sprintf("RelativeUrlWithCannotBeABaseBase: %s", err.message)
}

func (self ParseErrorRelativeUrlWithCannotBeABaseBase) Is(target error) bool {
	return target == ErrParseErrorRelativeUrlWithCannotBeABaseBase
}

type ParseErrorSetHostOnCannotBeABaseUrl struct {
	message string
}

func NewParseErrorSetHostOnCannotBeABaseUrl() *ParseError {
	return &ParseError{
		err: &ParseErrorSetHostOnCannotBeABaseUrl{},
	}
}

func (err ParseErrorSetHostOnCannotBeABaseUrl) Error() string {
	return fmt.Sprintf("SetHostOnCannotBeABaseUrl: %s", err.message)
}

func (self ParseErrorSetHostOnCannotBeABaseUrl) Is(target error) bool {
	return target == ErrParseErrorSetHostOnCannotBeABaseUrl
}

type ParseErrorOverflow struct {
	message string
}

func NewParseErrorOverflow() *ParseError {
	return &ParseError{
		err: &ParseErrorOverflow{},
	}
}

func (err ParseErrorOverflow) Error() string {
	return fmt.Sprintf("Overflow: %s", err.message)
}

func (self ParseErrorOverflow) Is(target error) bool {
	return target == ErrParseErrorOverflow
}

type ParseErrorOther struct {
	message string
}

func NewParseErrorOther() *ParseError {
	return &ParseError{
		err: &ParseErrorOther{},
	}
}

func (err ParseErrorOther) Error() string {
	return fmt.Sprintf("Other: %s", err.message)
}

func (self ParseErrorOther) Is(target error) bool {
	return target == ErrParseErrorOther
}

type FfiConverterTypeParseError struct{}

var FfiConverterTypeParseErrorINSTANCE = FfiConverterTypeParseError{}

func (c FfiConverterTypeParseError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeParseError) Lower(value *ParseError) RustBuffer {
	return LowerIntoRustBuffer[*ParseError](c, value)
}

func (c FfiConverterTypeParseError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &ParseError{&ParseErrorEmptyHost{message}}
	case 2:
		return &ParseError{&ParseErrorIdnaError{message}}
	case 3:
		return &ParseError{&ParseErrorInvalidPort{message}}
	case 4:
		return &ParseError{&ParseErrorInvalidIpv4Address{message}}
	case 5:
		return &ParseError{&ParseErrorInvalidIpv6Address{message}}
	case 6:
		return &ParseError{&ParseErrorInvalidDomainCharacter{message}}
	case 7:
		return &ParseError{&ParseErrorRelativeUrlWithoutBase{message}}
	case 8:
		return &ParseError{&ParseErrorRelativeUrlWithCannotBeABaseBase{message}}
	case 9:
		return &ParseError{&ParseErrorSetHostOnCannotBeABaseUrl{message}}
	case 10:
		return &ParseError{&ParseErrorOverflow{message}}
	case 11:
		return &ParseError{&ParseErrorOther{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeParseError.Read()", errorID))
	}

}

func (c FfiConverterTypeParseError) Write(writer io.Writer, value *ParseError) {
	switch variantValue := value.err.(type) {
	case *ParseErrorEmptyHost:
		writeInt32(writer, 1)
	case *ParseErrorIdnaError:
		writeInt32(writer, 2)
	case *ParseErrorInvalidPort:
		writeInt32(writer, 3)
	case *ParseErrorInvalidIpv4Address:
		writeInt32(writer, 4)
	case *ParseErrorInvalidIpv6Address:
		writeInt32(writer, 5)
	case *ParseErrorInvalidDomainCharacter:
		writeInt32(writer, 6)
	case *ParseErrorRelativeUrlWithoutBase:
		writeInt32(writer, 7)
	case *ParseErrorRelativeUrlWithCannotBeABaseBase:
		writeInt32(writer, 8)
	case *ParseErrorSetHostOnCannotBeABaseUrl:
		writeInt32(writer, 9)
	case *ParseErrorOverflow:
		writeInt32(writer, 10)
	case *ParseErrorOther:
		writeInt32(writer, 11)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeParseError.Write", value))
	}
}

type PollKind uint

const (
	PollKindDisclosed   PollKind = 1
	PollKindUndisclosed PollKind = 2
)

type FfiConverterTypePollKind struct{}

var FfiConverterTypePollKindINSTANCE = FfiConverterTypePollKind{}

func (c FfiConverterTypePollKind) Lift(rb RustBufferI) PollKind {
	return LiftFromRustBuffer[PollKind](c, rb)
}

func (c FfiConverterTypePollKind) Lower(value PollKind) RustBuffer {
	return LowerIntoRustBuffer[PollKind](c, value)
}
func (FfiConverterTypePollKind) Read(reader io.Reader) PollKind {
	id := readInt32(reader)
	return PollKind(id)
}

func (FfiConverterTypePollKind) Write(writer io.Writer, value PollKind) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypePollKind struct{}

func (_ FfiDestroyerTypePollKind) Destroy(value PollKind) {
}

type ProfileDetails interface {
	Destroy()
}
type ProfileDetailsUnavailable struct {
}

func (e ProfileDetailsUnavailable) Destroy() {
}

type ProfileDetailsPending struct {
}

func (e ProfileDetailsPending) Destroy() {
}

type ProfileDetailsReady struct {
	DisplayName          *string
	DisplayNameAmbiguous bool
	AvatarUrl            *string
}

func (e ProfileDetailsReady) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(e.DisplayName)
	FfiDestroyerBool{}.Destroy(e.DisplayNameAmbiguous)
	FfiDestroyerOptionalString{}.Destroy(e.AvatarUrl)
}

type ProfileDetailsError struct {
	Message string
}

func (e ProfileDetailsError) Destroy() {
	FfiDestroyerString{}.Destroy(e.Message)
}

type FfiConverterTypeProfileDetails struct{}

var FfiConverterTypeProfileDetailsINSTANCE = FfiConverterTypeProfileDetails{}

func (c FfiConverterTypeProfileDetails) Lift(rb RustBufferI) ProfileDetails {
	return LiftFromRustBuffer[ProfileDetails](c, rb)
}

func (c FfiConverterTypeProfileDetails) Lower(value ProfileDetails) RustBuffer {
	return LowerIntoRustBuffer[ProfileDetails](c, value)
}
func (FfiConverterTypeProfileDetails) Read(reader io.Reader) ProfileDetails {
	id := readInt32(reader)
	switch id {
	case 1:
		return ProfileDetailsUnavailable{}
	case 2:
		return ProfileDetailsPending{}
	case 3:
		return ProfileDetailsReady{
			FfiConverterOptionalStringINSTANCE.Read(reader),
			FfiConverterBoolINSTANCE.Read(reader),
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 4:
		return ProfileDetailsError{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeProfileDetails.Read()", id))
	}
}

func (FfiConverterTypeProfileDetails) Write(writer io.Writer, value ProfileDetails) {
	switch variant_value := value.(type) {
	case ProfileDetailsUnavailable:
		writeInt32(writer, 1)
	case ProfileDetailsPending:
		writeInt32(writer, 2)
	case ProfileDetailsReady:
		writeInt32(writer, 3)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.DisplayName)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.DisplayNameAmbiguous)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.AvatarUrl)
	case ProfileDetailsError:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Message)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeProfileDetails.Write", value))
	}
}

type FfiDestroyerTypeProfileDetails struct{}

func (_ FfiDestroyerTypeProfileDetails) Destroy(value ProfileDetails) {
	value.Destroy()
}

type PushFormat uint

const (
	PushFormatEventIdOnly PushFormat = 1
)

type FfiConverterTypePushFormat struct{}

var FfiConverterTypePushFormatINSTANCE = FfiConverterTypePushFormat{}

func (c FfiConverterTypePushFormat) Lift(rb RustBufferI) PushFormat {
	return LiftFromRustBuffer[PushFormat](c, rb)
}

func (c FfiConverterTypePushFormat) Lower(value PushFormat) RustBuffer {
	return LowerIntoRustBuffer[PushFormat](c, value)
}
func (FfiConverterTypePushFormat) Read(reader io.Reader) PushFormat {
	id := readInt32(reader)
	return PushFormat(id)
}

func (FfiConverterTypePushFormat) Write(writer io.Writer, value PushFormat) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypePushFormat struct{}

func (_ FfiDestroyerTypePushFormat) Destroy(value PushFormat) {
}

type PusherKind interface {
	Destroy()
}
type PusherKindHttp struct {
	Data HttpPusherData
}

func (e PusherKindHttp) Destroy() {
	FfiDestroyerTypeHttpPusherData{}.Destroy(e.Data)
}

type PusherKindEmail struct {
}

func (e PusherKindEmail) Destroy() {
}

type FfiConverterTypePusherKind struct{}

var FfiConverterTypePusherKindINSTANCE = FfiConverterTypePusherKind{}

func (c FfiConverterTypePusherKind) Lift(rb RustBufferI) PusherKind {
	return LiftFromRustBuffer[PusherKind](c, rb)
}

func (c FfiConverterTypePusherKind) Lower(value PusherKind) RustBuffer {
	return LowerIntoRustBuffer[PusherKind](c, value)
}
func (FfiConverterTypePusherKind) Read(reader io.Reader) PusherKind {
	id := readInt32(reader)
	switch id {
	case 1:
		return PusherKindHttp{
			FfiConverterTypeHttpPusherDataINSTANCE.Read(reader),
		}
	case 2:
		return PusherKindEmail{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypePusherKind.Read()", id))
	}
}

func (FfiConverterTypePusherKind) Write(writer io.Writer, value PusherKind) {
	switch variant_value := value.(type) {
	case PusherKindHttp:
		writeInt32(writer, 1)
		FfiConverterTypeHttpPusherDataINSTANCE.Write(writer, variant_value.Data)
	case PusherKindEmail:
		writeInt32(writer, 2)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypePusherKind.Write", value))
	}
}

type FfiDestroyerTypePusherKind struct{}

func (_ FfiDestroyerTypePusherKind) Destroy(value PusherKind) {
	value.Destroy()
}

type ReceiptType uint

const (
	ReceiptTypeRead        ReceiptType = 1
	ReceiptTypeReadPrivate ReceiptType = 2
	ReceiptTypeFullyRead   ReceiptType = 3
)

type FfiConverterTypeReceiptType struct{}

var FfiConverterTypeReceiptTypeINSTANCE = FfiConverterTypeReceiptType{}

func (c FfiConverterTypeReceiptType) Lift(rb RustBufferI) ReceiptType {
	return LiftFromRustBuffer[ReceiptType](c, rb)
}

func (c FfiConverterTypeReceiptType) Lower(value ReceiptType) RustBuffer {
	return LowerIntoRustBuffer[ReceiptType](c, value)
}
func (FfiConverterTypeReceiptType) Read(reader io.Reader) ReceiptType {
	id := readInt32(reader)
	return ReceiptType(id)
}

func (FfiConverterTypeReceiptType) Write(writer io.Writer, value ReceiptType) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeReceiptType struct{}

func (_ FfiDestroyerTypeReceiptType) Destroy(value ReceiptType) {
}

type RecoveryError struct {
	err error
}

func (err RecoveryError) Error() string {
	return fmt.Sprintf("RecoveryError: %s", err.err.Error())
}

func (err RecoveryError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrRecoveryErrorBackupExistsOnServer = fmt.Errorf("RecoveryErrorBackupExistsOnServer")
var ErrRecoveryErrorClient = fmt.Errorf("RecoveryErrorClient")
var ErrRecoveryErrorSecretStorage = fmt.Errorf("RecoveryErrorSecretStorage")

// Variant structs
type RecoveryErrorBackupExistsOnServer struct {
}

func NewRecoveryErrorBackupExistsOnServer() *RecoveryError {
	return &RecoveryError{
		err: &RecoveryErrorBackupExistsOnServer{},
	}
}

func (err RecoveryErrorBackupExistsOnServer) Error() string {
	return fmt.Sprint("BackupExistsOnServer")
}

func (self RecoveryErrorBackupExistsOnServer) Is(target error) bool {
	return target == ErrRecoveryErrorBackupExistsOnServer
}

type RecoveryErrorClient struct {
	Source ClientError
}

func NewRecoveryErrorClient(
	source ClientError,
) *RecoveryError {
	return &RecoveryError{
		err: &RecoveryErrorClient{
			Source: source,
		},
	}
}

func (err RecoveryErrorClient) Error() string {
	return fmt.Sprint("Client",
		": ",

		"Source=",
		err.Source,
	)
}

func (self RecoveryErrorClient) Is(target error) bool {
	return target == ErrRecoveryErrorClient
}

type RecoveryErrorSecretStorage struct {
	ErrorMessage string
}

func NewRecoveryErrorSecretStorage(
	errorMessage string,
) *RecoveryError {
	return &RecoveryError{
		err: &RecoveryErrorSecretStorage{
			ErrorMessage: errorMessage,
		},
	}
}

func (err RecoveryErrorSecretStorage) Error() string {
	return fmt.Sprint("SecretStorage",
		": ",

		"ErrorMessage=",
		err.ErrorMessage,
	)
}

func (self RecoveryErrorSecretStorage) Is(target error) bool {
	return target == ErrRecoveryErrorSecretStorage
}

type FfiConverterTypeRecoveryError struct{}

var FfiConverterTypeRecoveryErrorINSTANCE = FfiConverterTypeRecoveryError{}

func (c FfiConverterTypeRecoveryError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeRecoveryError) Lower(value *RecoveryError) RustBuffer {
	return LowerIntoRustBuffer[*RecoveryError](c, value)
}

func (c FfiConverterTypeRecoveryError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	switch errorID {
	case 1:
		return &RecoveryError{&RecoveryErrorBackupExistsOnServer{}}
	case 2:
		return &RecoveryError{&RecoveryErrorClient{
			Source: FfiConverterTypeClientErrorINSTANCE.Read(reader).(ClientError),
		}}
	case 3:
		return &RecoveryError{&RecoveryErrorSecretStorage{
			ErrorMessage: FfiConverterStringINSTANCE.Read(reader),
		}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeRecoveryError.Read()", errorID))
	}
}

func (c FfiConverterTypeRecoveryError) Write(writer io.Writer, value *RecoveryError) {
	switch variantValue := value.err.(type) {
	case *RecoveryErrorBackupExistsOnServer:
		writeInt32(writer, 1)
	case *RecoveryErrorClient:
		writeInt32(writer, 2)
		FfiConverterTypeClientErrorINSTANCE.Write(writer, &variantValue.Source)
	case *RecoveryErrorSecretStorage:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variantValue.ErrorMessage)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeRecoveryError.Write", value))
	}
}

type RecoveryState uint

const (
	RecoveryStateUnknown    RecoveryState = 1
	RecoveryStateEnabled    RecoveryState = 2
	RecoveryStateDisabled   RecoveryState = 3
	RecoveryStateIncomplete RecoveryState = 4
)

type FfiConverterTypeRecoveryState struct{}

var FfiConverterTypeRecoveryStateINSTANCE = FfiConverterTypeRecoveryState{}

func (c FfiConverterTypeRecoveryState) Lift(rb RustBufferI) RecoveryState {
	return LiftFromRustBuffer[RecoveryState](c, rb)
}

func (c FfiConverterTypeRecoveryState) Lower(value RecoveryState) RustBuffer {
	return LowerIntoRustBuffer[RecoveryState](c, value)
}
func (FfiConverterTypeRecoveryState) Read(reader io.Reader) RecoveryState {
	id := readInt32(reader)
	return RecoveryState(id)
}

func (FfiConverterTypeRecoveryState) Write(writer io.Writer, value RecoveryState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRecoveryState struct{}

func (_ FfiDestroyerTypeRecoveryState) Destroy(value RecoveryState) {
}

type RepliedToEventDetails interface {
	Destroy()
}
type RepliedToEventDetailsUnavailable struct {
}

func (e RepliedToEventDetailsUnavailable) Destroy() {
}

type RepliedToEventDetailsPending struct {
}

func (e RepliedToEventDetailsPending) Destroy() {
}

type RepliedToEventDetailsReady struct {
	Content       *TimelineItemContent
	Sender        string
	SenderProfile ProfileDetails
}

func (e RepliedToEventDetailsReady) Destroy() {
	FfiDestroyerTimelineItemContent{}.Destroy(e.Content)
	FfiDestroyerString{}.Destroy(e.Sender)
	FfiDestroyerTypeProfileDetails{}.Destroy(e.SenderProfile)
}

type RepliedToEventDetailsError struct {
	Message string
}

func (e RepliedToEventDetailsError) Destroy() {
	FfiDestroyerString{}.Destroy(e.Message)
}

type FfiConverterTypeRepliedToEventDetails struct{}

var FfiConverterTypeRepliedToEventDetailsINSTANCE = FfiConverterTypeRepliedToEventDetails{}

func (c FfiConverterTypeRepliedToEventDetails) Lift(rb RustBufferI) RepliedToEventDetails {
	return LiftFromRustBuffer[RepliedToEventDetails](c, rb)
}

func (c FfiConverterTypeRepliedToEventDetails) Lower(value RepliedToEventDetails) RustBuffer {
	return LowerIntoRustBuffer[RepliedToEventDetails](c, value)
}
func (FfiConverterTypeRepliedToEventDetails) Read(reader io.Reader) RepliedToEventDetails {
	id := readInt32(reader)
	switch id {
	case 1:
		return RepliedToEventDetailsUnavailable{}
	case 2:
		return RepliedToEventDetailsPending{}
	case 3:
		return RepliedToEventDetailsReady{
			FfiConverterTimelineItemContentINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterTypeProfileDetailsINSTANCE.Read(reader),
		}
	case 4:
		return RepliedToEventDetailsError{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeRepliedToEventDetails.Read()", id))
	}
}

func (FfiConverterTypeRepliedToEventDetails) Write(writer io.Writer, value RepliedToEventDetails) {
	switch variant_value := value.(type) {
	case RepliedToEventDetailsUnavailable:
		writeInt32(writer, 1)
	case RepliedToEventDetailsPending:
		writeInt32(writer, 2)
	case RepliedToEventDetailsReady:
		writeInt32(writer, 3)
		FfiConverterTimelineItemContentINSTANCE.Write(writer, variant_value.Content)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Sender)
		FfiConverterTypeProfileDetailsINSTANCE.Write(writer, variant_value.SenderProfile)
	case RepliedToEventDetailsError:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Message)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeRepliedToEventDetails.Write", value))
	}
}

type FfiDestroyerTypeRepliedToEventDetails struct{}

func (_ FfiDestroyerTypeRepliedToEventDetails) Destroy(value RepliedToEventDetails) {
	value.Destroy()
}

type RoomError struct {
	err error
}

func (err RoomError) Error() string {
	return fmt.Sprintf("RoomError: %s", err.err.Error())
}

func (err RoomError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrRoomErrorInvalidAttachmentData = fmt.Errorf("RoomErrorInvalidAttachmentData")
var ErrRoomErrorInvalidAttachmentMimeType = fmt.Errorf("RoomErrorInvalidAttachmentMimeType")
var ErrRoomErrorInvalidMediaInfo = fmt.Errorf("RoomErrorInvalidMediaInfo")
var ErrRoomErrorTimelineUnavailable = fmt.Errorf("RoomErrorTimelineUnavailable")
var ErrRoomErrorInvalidThumbnailData = fmt.Errorf("RoomErrorInvalidThumbnailData")
var ErrRoomErrorFailedSendingAttachment = fmt.Errorf("RoomErrorFailedSendingAttachment")

// Variant structs
type RoomErrorInvalidAttachmentData struct {
	message string
}

func NewRoomErrorInvalidAttachmentData() *RoomError {
	return &RoomError{
		err: &RoomErrorInvalidAttachmentData{},
	}
}

func (err RoomErrorInvalidAttachmentData) Error() string {
	return fmt.Sprintf("InvalidAttachmentData: %s", err.message)
}

func (self RoomErrorInvalidAttachmentData) Is(target error) bool {
	return target == ErrRoomErrorInvalidAttachmentData
}

type RoomErrorInvalidAttachmentMimeType struct {
	message string
}

func NewRoomErrorInvalidAttachmentMimeType() *RoomError {
	return &RoomError{
		err: &RoomErrorInvalidAttachmentMimeType{},
	}
}

func (err RoomErrorInvalidAttachmentMimeType) Error() string {
	return fmt.Sprintf("InvalidAttachmentMimeType: %s", err.message)
}

func (self RoomErrorInvalidAttachmentMimeType) Is(target error) bool {
	return target == ErrRoomErrorInvalidAttachmentMimeType
}

type RoomErrorInvalidMediaInfo struct {
	message string
}

func NewRoomErrorInvalidMediaInfo() *RoomError {
	return &RoomError{
		err: &RoomErrorInvalidMediaInfo{},
	}
}

func (err RoomErrorInvalidMediaInfo) Error() string {
	return fmt.Sprintf("InvalidMediaInfo: %s", err.message)
}

func (self RoomErrorInvalidMediaInfo) Is(target error) bool {
	return target == ErrRoomErrorInvalidMediaInfo
}

type RoomErrorTimelineUnavailable struct {
	message string
}

func NewRoomErrorTimelineUnavailable() *RoomError {
	return &RoomError{
		err: &RoomErrorTimelineUnavailable{},
	}
}

func (err RoomErrorTimelineUnavailable) Error() string {
	return fmt.Sprintf("TimelineUnavailable: %s", err.message)
}

func (self RoomErrorTimelineUnavailable) Is(target error) bool {
	return target == ErrRoomErrorTimelineUnavailable
}

type RoomErrorInvalidThumbnailData struct {
	message string
}

func NewRoomErrorInvalidThumbnailData() *RoomError {
	return &RoomError{
		err: &RoomErrorInvalidThumbnailData{},
	}
}

func (err RoomErrorInvalidThumbnailData) Error() string {
	return fmt.Sprintf("InvalidThumbnailData: %s", err.message)
}

func (self RoomErrorInvalidThumbnailData) Is(target error) bool {
	return target == ErrRoomErrorInvalidThumbnailData
}

type RoomErrorFailedSendingAttachment struct {
	message string
}

func NewRoomErrorFailedSendingAttachment() *RoomError {
	return &RoomError{
		err: &RoomErrorFailedSendingAttachment{},
	}
}

func (err RoomErrorFailedSendingAttachment) Error() string {
	return fmt.Sprintf("FailedSendingAttachment: %s", err.message)
}

func (self RoomErrorFailedSendingAttachment) Is(target error) bool {
	return target == ErrRoomErrorFailedSendingAttachment
}

type FfiConverterTypeRoomError struct{}

var FfiConverterTypeRoomErrorINSTANCE = FfiConverterTypeRoomError{}

func (c FfiConverterTypeRoomError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeRoomError) Lower(value *RoomError) RustBuffer {
	return LowerIntoRustBuffer[*RoomError](c, value)
}

func (c FfiConverterTypeRoomError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &RoomError{&RoomErrorInvalidAttachmentData{message}}
	case 2:
		return &RoomError{&RoomErrorInvalidAttachmentMimeType{message}}
	case 3:
		return &RoomError{&RoomErrorInvalidMediaInfo{message}}
	case 4:
		return &RoomError{&RoomErrorTimelineUnavailable{message}}
	case 5:
		return &RoomError{&RoomErrorInvalidThumbnailData{message}}
	case 6:
		return &RoomError{&RoomErrorFailedSendingAttachment{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeRoomError.Read()", errorID))
	}

}

func (c FfiConverterTypeRoomError) Write(writer io.Writer, value *RoomError) {
	switch variantValue := value.err.(type) {
	case *RoomErrorInvalidAttachmentData:
		writeInt32(writer, 1)
	case *RoomErrorInvalidAttachmentMimeType:
		writeInt32(writer, 2)
	case *RoomErrorInvalidMediaInfo:
		writeInt32(writer, 3)
	case *RoomErrorTimelineUnavailable:
		writeInt32(writer, 4)
	case *RoomErrorInvalidThumbnailData:
		writeInt32(writer, 5)
	case *RoomErrorFailedSendingAttachment:
		writeInt32(writer, 6)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeRoomError.Write", value))
	}
}

type RoomListEntriesDynamicFilterKind interface {
	Destroy()
}
type RoomListEntriesDynamicFilterKindAll struct {
	Filters []RoomListEntriesDynamicFilterKind
}

func (e RoomListEntriesDynamicFilterKindAll) Destroy() {
	FfiDestroyerSequenceTypeRoomListEntriesDynamicFilterKind{}.Destroy(e.Filters)
}

type RoomListEntriesDynamicFilterKindAny struct {
	Filters []RoomListEntriesDynamicFilterKind
}

func (e RoomListEntriesDynamicFilterKindAny) Destroy() {
	FfiDestroyerSequenceTypeRoomListEntriesDynamicFilterKind{}.Destroy(e.Filters)
}

type RoomListEntriesDynamicFilterKindNonLeft struct {
}

func (e RoomListEntriesDynamicFilterKindNonLeft) Destroy() {
}

type RoomListEntriesDynamicFilterKindUnread struct {
}

func (e RoomListEntriesDynamicFilterKindUnread) Destroy() {
}

type RoomListEntriesDynamicFilterKindCategory struct {
	Expect RoomListFilterCategory
}

func (e RoomListEntriesDynamicFilterKindCategory) Destroy() {
	FfiDestroyerTypeRoomListFilterCategory{}.Destroy(e.Expect)
}

type RoomListEntriesDynamicFilterKindNone struct {
}

func (e RoomListEntriesDynamicFilterKindNone) Destroy() {
}

type RoomListEntriesDynamicFilterKindNormalizedMatchRoomName struct {
	Pattern string
}

func (e RoomListEntriesDynamicFilterKindNormalizedMatchRoomName) Destroy() {
	FfiDestroyerString{}.Destroy(e.Pattern)
}

type RoomListEntriesDynamicFilterKindFuzzyMatchRoomName struct {
	Pattern string
}

func (e RoomListEntriesDynamicFilterKindFuzzyMatchRoomName) Destroy() {
	FfiDestroyerString{}.Destroy(e.Pattern)
}

type FfiConverterTypeRoomListEntriesDynamicFilterKind struct{}

var FfiConverterTypeRoomListEntriesDynamicFilterKindINSTANCE = FfiConverterTypeRoomListEntriesDynamicFilterKind{}

func (c FfiConverterTypeRoomListEntriesDynamicFilterKind) Lift(rb RustBufferI) RoomListEntriesDynamicFilterKind {
	return LiftFromRustBuffer[RoomListEntriesDynamicFilterKind](c, rb)
}

func (c FfiConverterTypeRoomListEntriesDynamicFilterKind) Lower(value RoomListEntriesDynamicFilterKind) RustBuffer {
	return LowerIntoRustBuffer[RoomListEntriesDynamicFilterKind](c, value)
}
func (FfiConverterTypeRoomListEntriesDynamicFilterKind) Read(reader io.Reader) RoomListEntriesDynamicFilterKind {
	id := readInt32(reader)
	switch id {
	case 1:
		return RoomListEntriesDynamicFilterKindAll{
			FfiConverterSequenceTypeRoomListEntriesDynamicFilterKindINSTANCE.Read(reader),
		}
	case 2:
		return RoomListEntriesDynamicFilterKindAny{
			FfiConverterSequenceTypeRoomListEntriesDynamicFilterKindINSTANCE.Read(reader),
		}
	case 3:
		return RoomListEntriesDynamicFilterKindNonLeft{}
	case 4:
		return RoomListEntriesDynamicFilterKindUnread{}
	case 5:
		return RoomListEntriesDynamicFilterKindCategory{
			FfiConverterTypeRoomListFilterCategoryINSTANCE.Read(reader),
		}
	case 6:
		return RoomListEntriesDynamicFilterKindNone{}
	case 7:
		return RoomListEntriesDynamicFilterKindNormalizedMatchRoomName{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 8:
		return RoomListEntriesDynamicFilterKindFuzzyMatchRoomName{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeRoomListEntriesDynamicFilterKind.Read()", id))
	}
}

func (FfiConverterTypeRoomListEntriesDynamicFilterKind) Write(writer io.Writer, value RoomListEntriesDynamicFilterKind) {
	switch variant_value := value.(type) {
	case RoomListEntriesDynamicFilterKindAll:
		writeInt32(writer, 1)
		FfiConverterSequenceTypeRoomListEntriesDynamicFilterKindINSTANCE.Write(writer, variant_value.Filters)
	case RoomListEntriesDynamicFilterKindAny:
		writeInt32(writer, 2)
		FfiConverterSequenceTypeRoomListEntriesDynamicFilterKindINSTANCE.Write(writer, variant_value.Filters)
	case RoomListEntriesDynamicFilterKindNonLeft:
		writeInt32(writer, 3)
	case RoomListEntriesDynamicFilterKindUnread:
		writeInt32(writer, 4)
	case RoomListEntriesDynamicFilterKindCategory:
		writeInt32(writer, 5)
		FfiConverterTypeRoomListFilterCategoryINSTANCE.Write(writer, variant_value.Expect)
	case RoomListEntriesDynamicFilterKindNone:
		writeInt32(writer, 6)
	case RoomListEntriesDynamicFilterKindNormalizedMatchRoomName:
		writeInt32(writer, 7)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Pattern)
	case RoomListEntriesDynamicFilterKindFuzzyMatchRoomName:
		writeInt32(writer, 8)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Pattern)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeRoomListEntriesDynamicFilterKind.Write", value))
	}
}

type FfiDestroyerTypeRoomListEntriesDynamicFilterKind struct{}

func (_ FfiDestroyerTypeRoomListEntriesDynamicFilterKind) Destroy(value RoomListEntriesDynamicFilterKind) {
	value.Destroy()
}

type RoomListEntriesUpdate interface {
	Destroy()
}
type RoomListEntriesUpdateAppend struct {
	Values []RoomListEntry
}

func (e RoomListEntriesUpdateAppend) Destroy() {
	FfiDestroyerSequenceTypeRoomListEntry{}.Destroy(e.Values)
}

type RoomListEntriesUpdateClear struct {
}

func (e RoomListEntriesUpdateClear) Destroy() {
}

type RoomListEntriesUpdatePushFront struct {
	Value RoomListEntry
}

func (e RoomListEntriesUpdatePushFront) Destroy() {
	FfiDestroyerTypeRoomListEntry{}.Destroy(e.Value)
}

type RoomListEntriesUpdatePushBack struct {
	Value RoomListEntry
}

func (e RoomListEntriesUpdatePushBack) Destroy() {
	FfiDestroyerTypeRoomListEntry{}.Destroy(e.Value)
}

type RoomListEntriesUpdatePopFront struct {
}

func (e RoomListEntriesUpdatePopFront) Destroy() {
}

type RoomListEntriesUpdatePopBack struct {
}

func (e RoomListEntriesUpdatePopBack) Destroy() {
}

type RoomListEntriesUpdateInsert struct {
	Index uint32
	Value RoomListEntry
}

func (e RoomListEntriesUpdateInsert) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.Index)
	FfiDestroyerTypeRoomListEntry{}.Destroy(e.Value)
}

type RoomListEntriesUpdateSet struct {
	Index uint32
	Value RoomListEntry
}

func (e RoomListEntriesUpdateSet) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.Index)
	FfiDestroyerTypeRoomListEntry{}.Destroy(e.Value)
}

type RoomListEntriesUpdateRemove struct {
	Index uint32
}

func (e RoomListEntriesUpdateRemove) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.Index)
}

type RoomListEntriesUpdateTruncate struct {
	Length uint32
}

func (e RoomListEntriesUpdateTruncate) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.Length)
}

type RoomListEntriesUpdateReset struct {
	Values []RoomListEntry
}

func (e RoomListEntriesUpdateReset) Destroy() {
	FfiDestroyerSequenceTypeRoomListEntry{}.Destroy(e.Values)
}

type FfiConverterTypeRoomListEntriesUpdate struct{}

var FfiConverterTypeRoomListEntriesUpdateINSTANCE = FfiConverterTypeRoomListEntriesUpdate{}

func (c FfiConverterTypeRoomListEntriesUpdate) Lift(rb RustBufferI) RoomListEntriesUpdate {
	return LiftFromRustBuffer[RoomListEntriesUpdate](c, rb)
}

func (c FfiConverterTypeRoomListEntriesUpdate) Lower(value RoomListEntriesUpdate) RustBuffer {
	return LowerIntoRustBuffer[RoomListEntriesUpdate](c, value)
}
func (FfiConverterTypeRoomListEntriesUpdate) Read(reader io.Reader) RoomListEntriesUpdate {
	id := readInt32(reader)
	switch id {
	case 1:
		return RoomListEntriesUpdateAppend{
			FfiConverterSequenceTypeRoomListEntryINSTANCE.Read(reader),
		}
	case 2:
		return RoomListEntriesUpdateClear{}
	case 3:
		return RoomListEntriesUpdatePushFront{
			FfiConverterTypeRoomListEntryINSTANCE.Read(reader),
		}
	case 4:
		return RoomListEntriesUpdatePushBack{
			FfiConverterTypeRoomListEntryINSTANCE.Read(reader),
		}
	case 5:
		return RoomListEntriesUpdatePopFront{}
	case 6:
		return RoomListEntriesUpdatePopBack{}
	case 7:
		return RoomListEntriesUpdateInsert{
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterTypeRoomListEntryINSTANCE.Read(reader),
		}
	case 8:
		return RoomListEntriesUpdateSet{
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterTypeRoomListEntryINSTANCE.Read(reader),
		}
	case 9:
		return RoomListEntriesUpdateRemove{
			FfiConverterUint32INSTANCE.Read(reader),
		}
	case 10:
		return RoomListEntriesUpdateTruncate{
			FfiConverterUint32INSTANCE.Read(reader),
		}
	case 11:
		return RoomListEntriesUpdateReset{
			FfiConverterSequenceTypeRoomListEntryINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeRoomListEntriesUpdate.Read()", id))
	}
}

func (FfiConverterTypeRoomListEntriesUpdate) Write(writer io.Writer, value RoomListEntriesUpdate) {
	switch variant_value := value.(type) {
	case RoomListEntriesUpdateAppend:
		writeInt32(writer, 1)
		FfiConverterSequenceTypeRoomListEntryINSTANCE.Write(writer, variant_value.Values)
	case RoomListEntriesUpdateClear:
		writeInt32(writer, 2)
	case RoomListEntriesUpdatePushFront:
		writeInt32(writer, 3)
		FfiConverterTypeRoomListEntryINSTANCE.Write(writer, variant_value.Value)
	case RoomListEntriesUpdatePushBack:
		writeInt32(writer, 4)
		FfiConverterTypeRoomListEntryINSTANCE.Write(writer, variant_value.Value)
	case RoomListEntriesUpdatePopFront:
		writeInt32(writer, 5)
	case RoomListEntriesUpdatePopBack:
		writeInt32(writer, 6)
	case RoomListEntriesUpdateInsert:
		writeInt32(writer, 7)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.Index)
		FfiConverterTypeRoomListEntryINSTANCE.Write(writer, variant_value.Value)
	case RoomListEntriesUpdateSet:
		writeInt32(writer, 8)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.Index)
		FfiConverterTypeRoomListEntryINSTANCE.Write(writer, variant_value.Value)
	case RoomListEntriesUpdateRemove:
		writeInt32(writer, 9)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.Index)
	case RoomListEntriesUpdateTruncate:
		writeInt32(writer, 10)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.Length)
	case RoomListEntriesUpdateReset:
		writeInt32(writer, 11)
		FfiConverterSequenceTypeRoomListEntryINSTANCE.Write(writer, variant_value.Values)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeRoomListEntriesUpdate.Write", value))
	}
}

type FfiDestroyerTypeRoomListEntriesUpdate struct{}

func (_ FfiDestroyerTypeRoomListEntriesUpdate) Destroy(value RoomListEntriesUpdate) {
	value.Destroy()
}

type RoomListEntry interface {
	Destroy()
}
type RoomListEntryEmpty struct {
}

func (e RoomListEntryEmpty) Destroy() {
}

type RoomListEntryInvalidated struct {
	RoomId string
}

func (e RoomListEntryInvalidated) Destroy() {
	FfiDestroyerString{}.Destroy(e.RoomId)
}

type RoomListEntryFilled struct {
	RoomId string
}

func (e RoomListEntryFilled) Destroy() {
	FfiDestroyerString{}.Destroy(e.RoomId)
}

type FfiConverterTypeRoomListEntry struct{}

var FfiConverterTypeRoomListEntryINSTANCE = FfiConverterTypeRoomListEntry{}

func (c FfiConverterTypeRoomListEntry) Lift(rb RustBufferI) RoomListEntry {
	return LiftFromRustBuffer[RoomListEntry](c, rb)
}

func (c FfiConverterTypeRoomListEntry) Lower(value RoomListEntry) RustBuffer {
	return LowerIntoRustBuffer[RoomListEntry](c, value)
}
func (FfiConverterTypeRoomListEntry) Read(reader io.Reader) RoomListEntry {
	id := readInt32(reader)
	switch id {
	case 1:
		return RoomListEntryEmpty{}
	case 2:
		return RoomListEntryInvalidated{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 3:
		return RoomListEntryFilled{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeRoomListEntry.Read()", id))
	}
}

func (FfiConverterTypeRoomListEntry) Write(writer io.Writer, value RoomListEntry) {
	switch variant_value := value.(type) {
	case RoomListEntryEmpty:
		writeInt32(writer, 1)
	case RoomListEntryInvalidated:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.RoomId)
	case RoomListEntryFilled:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variant_value.RoomId)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeRoomListEntry.Write", value))
	}
}

type FfiDestroyerTypeRoomListEntry struct{}

func (_ FfiDestroyerTypeRoomListEntry) Destroy(value RoomListEntry) {
	value.Destroy()
}

type RoomListError struct {
	err error
}

func (err RoomListError) Error() string {
	return fmt.Sprintf("RoomListError: %s", err.err.Error())
}

func (err RoomListError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrRoomListErrorSlidingSync = fmt.Errorf("RoomListErrorSlidingSync")
var ErrRoomListErrorUnknownList = fmt.Errorf("RoomListErrorUnknownList")
var ErrRoomListErrorInputCannotBeApplied = fmt.Errorf("RoomListErrorInputCannotBeApplied")
var ErrRoomListErrorRoomNotFound = fmt.Errorf("RoomListErrorRoomNotFound")
var ErrRoomListErrorInvalidRoomId = fmt.Errorf("RoomListErrorInvalidRoomId")
var ErrRoomListErrorTimelineAlreadyExists = fmt.Errorf("RoomListErrorTimelineAlreadyExists")
var ErrRoomListErrorTimelineNotInitialized = fmt.Errorf("RoomListErrorTimelineNotInitialized")
var ErrRoomListErrorInitializingTimeline = fmt.Errorf("RoomListErrorInitializingTimeline")

// Variant structs
type RoomListErrorSlidingSync struct {
	Error_ string
}

func NewRoomListErrorSlidingSync(
	error string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorSlidingSync{
			Error_: error,
		},
	}
}

func (err RoomListErrorSlidingSync) Error() string {
	return fmt.Sprint("SlidingSync",
		": ",

		"Error_=",
		err.Error_,
	)
}

func (self RoomListErrorSlidingSync) Is(target error) bool {
	return target == ErrRoomListErrorSlidingSync
}

type RoomListErrorUnknownList struct {
	ListName string
}

func NewRoomListErrorUnknownList(
	listName string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorUnknownList{
			ListName: listName,
		},
	}
}

func (err RoomListErrorUnknownList) Error() string {
	return fmt.Sprint("UnknownList",
		": ",

		"ListName=",
		err.ListName,
	)
}

func (self RoomListErrorUnknownList) Is(target error) bool {
	return target == ErrRoomListErrorUnknownList
}

type RoomListErrorInputCannotBeApplied struct {
}

func NewRoomListErrorInputCannotBeApplied() *RoomListError {
	return &RoomListError{
		err: &RoomListErrorInputCannotBeApplied{},
	}
}

func (err RoomListErrorInputCannotBeApplied) Error() string {
	return fmt.Sprint("InputCannotBeApplied")
}

func (self RoomListErrorInputCannotBeApplied) Is(target error) bool {
	return target == ErrRoomListErrorInputCannotBeApplied
}

type RoomListErrorRoomNotFound struct {
	RoomName string
}

func NewRoomListErrorRoomNotFound(
	roomName string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorRoomNotFound{
			RoomName: roomName,
		},
	}
}

func (err RoomListErrorRoomNotFound) Error() string {
	return fmt.Sprint("RoomNotFound",
		": ",

		"RoomName=",
		err.RoomName,
	)
}

func (self RoomListErrorRoomNotFound) Is(target error) bool {
	return target == ErrRoomListErrorRoomNotFound
}

type RoomListErrorInvalidRoomId struct {
	Error_ string
}

func NewRoomListErrorInvalidRoomId(
	error string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorInvalidRoomId{
			Error_: error,
		},
	}
}

func (err RoomListErrorInvalidRoomId) Error() string {
	return fmt.Sprint("InvalidRoomId",
		": ",

		"Error_=",
		err.Error_,
	)
}

func (self RoomListErrorInvalidRoomId) Is(target error) bool {
	return target == ErrRoomListErrorInvalidRoomId
}

type RoomListErrorTimelineAlreadyExists struct {
	RoomName string
}

func NewRoomListErrorTimelineAlreadyExists(
	roomName string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorTimelineAlreadyExists{
			RoomName: roomName,
		},
	}
}

func (err RoomListErrorTimelineAlreadyExists) Error() string {
	return fmt.Sprint("TimelineAlreadyExists",
		": ",

		"RoomName=",
		err.RoomName,
	)
}

func (self RoomListErrorTimelineAlreadyExists) Is(target error) bool {
	return target == ErrRoomListErrorTimelineAlreadyExists
}

type RoomListErrorTimelineNotInitialized struct {
	RoomName string
}

func NewRoomListErrorTimelineNotInitialized(
	roomName string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorTimelineNotInitialized{
			RoomName: roomName,
		},
	}
}

func (err RoomListErrorTimelineNotInitialized) Error() string {
	return fmt.Sprint("TimelineNotInitialized",
		": ",

		"RoomName=",
		err.RoomName,
	)
}

func (self RoomListErrorTimelineNotInitialized) Is(target error) bool {
	return target == ErrRoomListErrorTimelineNotInitialized
}

type RoomListErrorInitializingTimeline struct {
	Error_ string
}

func NewRoomListErrorInitializingTimeline(
	error string,
) *RoomListError {
	return &RoomListError{
		err: &RoomListErrorInitializingTimeline{
			Error_: error,
		},
	}
}

func (err RoomListErrorInitializingTimeline) Error() string {
	return fmt.Sprint("InitializingTimeline",
		": ",

		"Error_=",
		err.Error_,
	)
}

func (self RoomListErrorInitializingTimeline) Is(target error) bool {
	return target == ErrRoomListErrorInitializingTimeline
}

type FfiConverterTypeRoomListError struct{}

var FfiConverterTypeRoomListErrorINSTANCE = FfiConverterTypeRoomListError{}

func (c FfiConverterTypeRoomListError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeRoomListError) Lower(value *RoomListError) RustBuffer {
	return LowerIntoRustBuffer[*RoomListError](c, value)
}

func (c FfiConverterTypeRoomListError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	switch errorID {
	case 1:
		return &RoomListError{&RoomListErrorSlidingSync{
			Error_: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 2:
		return &RoomListError{&RoomListErrorUnknownList{
			ListName: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 3:
		return &RoomListError{&RoomListErrorInputCannotBeApplied{}}
	case 4:
		return &RoomListError{&RoomListErrorRoomNotFound{
			RoomName: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 5:
		return &RoomListError{&RoomListErrorInvalidRoomId{
			Error_: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 6:
		return &RoomListError{&RoomListErrorTimelineAlreadyExists{
			RoomName: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 7:
		return &RoomListError{&RoomListErrorTimelineNotInitialized{
			RoomName: FfiConverterStringINSTANCE.Read(reader),
		}}
	case 8:
		return &RoomListError{&RoomListErrorInitializingTimeline{
			Error_: FfiConverterStringINSTANCE.Read(reader),
		}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeRoomListError.Read()", errorID))
	}
}

func (c FfiConverterTypeRoomListError) Write(writer io.Writer, value *RoomListError) {
	switch variantValue := value.err.(type) {
	case *RoomListErrorSlidingSync:
		writeInt32(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, variantValue.Error_)
	case *RoomListErrorUnknownList:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variantValue.ListName)
	case *RoomListErrorInputCannotBeApplied:
		writeInt32(writer, 3)
	case *RoomListErrorRoomNotFound:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variantValue.RoomName)
	case *RoomListErrorInvalidRoomId:
		writeInt32(writer, 5)
		FfiConverterStringINSTANCE.Write(writer, variantValue.Error_)
	case *RoomListErrorTimelineAlreadyExists:
		writeInt32(writer, 6)
		FfiConverterStringINSTANCE.Write(writer, variantValue.RoomName)
	case *RoomListErrorTimelineNotInitialized:
		writeInt32(writer, 7)
		FfiConverterStringINSTANCE.Write(writer, variantValue.RoomName)
	case *RoomListErrorInitializingTimeline:
		writeInt32(writer, 8)
		FfiConverterStringINSTANCE.Write(writer, variantValue.Error_)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeRoomListError.Write", value))
	}
}

type RoomListFilterCategory uint

const (
	RoomListFilterCategoryGroup  RoomListFilterCategory = 1
	RoomListFilterCategoryPeople RoomListFilterCategory = 2
)

type FfiConverterTypeRoomListFilterCategory struct{}

var FfiConverterTypeRoomListFilterCategoryINSTANCE = FfiConverterTypeRoomListFilterCategory{}

func (c FfiConverterTypeRoomListFilterCategory) Lift(rb RustBufferI) RoomListFilterCategory {
	return LiftFromRustBuffer[RoomListFilterCategory](c, rb)
}

func (c FfiConverterTypeRoomListFilterCategory) Lower(value RoomListFilterCategory) RustBuffer {
	return LowerIntoRustBuffer[RoomListFilterCategory](c, value)
}
func (FfiConverterTypeRoomListFilterCategory) Read(reader io.Reader) RoomListFilterCategory {
	id := readInt32(reader)
	return RoomListFilterCategory(id)
}

func (FfiConverterTypeRoomListFilterCategory) Write(writer io.Writer, value RoomListFilterCategory) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomListFilterCategory struct{}

func (_ FfiDestroyerTypeRoomListFilterCategory) Destroy(value RoomListFilterCategory) {
}

type RoomListInput interface {
	Destroy()
}
type RoomListInputViewport struct {
	Ranges []RoomListRange
}

func (e RoomListInputViewport) Destroy() {
	FfiDestroyerSequenceTypeRoomListRange{}.Destroy(e.Ranges)
}

type FfiConverterTypeRoomListInput struct{}

var FfiConverterTypeRoomListInputINSTANCE = FfiConverterTypeRoomListInput{}

func (c FfiConverterTypeRoomListInput) Lift(rb RustBufferI) RoomListInput {
	return LiftFromRustBuffer[RoomListInput](c, rb)
}

func (c FfiConverterTypeRoomListInput) Lower(value RoomListInput) RustBuffer {
	return LowerIntoRustBuffer[RoomListInput](c, value)
}
func (FfiConverterTypeRoomListInput) Read(reader io.Reader) RoomListInput {
	id := readInt32(reader)
	switch id {
	case 1:
		return RoomListInputViewport{
			FfiConverterSequenceTypeRoomListRangeINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeRoomListInput.Read()", id))
	}
}

func (FfiConverterTypeRoomListInput) Write(writer io.Writer, value RoomListInput) {
	switch variant_value := value.(type) {
	case RoomListInputViewport:
		writeInt32(writer, 1)
		FfiConverterSequenceTypeRoomListRangeINSTANCE.Write(writer, variant_value.Ranges)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeRoomListInput.Write", value))
	}
}

type FfiDestroyerTypeRoomListInput struct{}

func (_ FfiDestroyerTypeRoomListInput) Destroy(value RoomListInput) {
	value.Destroy()
}

type RoomListLoadingState interface {
	Destroy()
}
type RoomListLoadingStateNotLoaded struct {
}

func (e RoomListLoadingStateNotLoaded) Destroy() {
}

type RoomListLoadingStateLoaded struct {
	MaximumNumberOfRooms *uint32
}

func (e RoomListLoadingStateLoaded) Destroy() {
	FfiDestroyerOptionalUint32{}.Destroy(e.MaximumNumberOfRooms)
}

type FfiConverterTypeRoomListLoadingState struct{}

var FfiConverterTypeRoomListLoadingStateINSTANCE = FfiConverterTypeRoomListLoadingState{}

func (c FfiConverterTypeRoomListLoadingState) Lift(rb RustBufferI) RoomListLoadingState {
	return LiftFromRustBuffer[RoomListLoadingState](c, rb)
}

func (c FfiConverterTypeRoomListLoadingState) Lower(value RoomListLoadingState) RustBuffer {
	return LowerIntoRustBuffer[RoomListLoadingState](c, value)
}
func (FfiConverterTypeRoomListLoadingState) Read(reader io.Reader) RoomListLoadingState {
	id := readInt32(reader)
	switch id {
	case 1:
		return RoomListLoadingStateNotLoaded{}
	case 2:
		return RoomListLoadingStateLoaded{
			FfiConverterOptionalUint32INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeRoomListLoadingState.Read()", id))
	}
}

func (FfiConverterTypeRoomListLoadingState) Write(writer io.Writer, value RoomListLoadingState) {
	switch variant_value := value.(type) {
	case RoomListLoadingStateNotLoaded:
		writeInt32(writer, 1)
	case RoomListLoadingStateLoaded:
		writeInt32(writer, 2)
		FfiConverterOptionalUint32INSTANCE.Write(writer, variant_value.MaximumNumberOfRooms)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeRoomListLoadingState.Write", value))
	}
}

type FfiDestroyerTypeRoomListLoadingState struct{}

func (_ FfiDestroyerTypeRoomListLoadingState) Destroy(value RoomListLoadingState) {
	value.Destroy()
}

type RoomListServiceState uint

const (
	RoomListServiceStateInitial    RoomListServiceState = 1
	RoomListServiceStateSettingUp  RoomListServiceState = 2
	RoomListServiceStateRecovering RoomListServiceState = 3
	RoomListServiceStateRunning    RoomListServiceState = 4
	RoomListServiceStateError      RoomListServiceState = 5
	RoomListServiceStateTerminated RoomListServiceState = 6
)

type FfiConverterTypeRoomListServiceState struct{}

var FfiConverterTypeRoomListServiceStateINSTANCE = FfiConverterTypeRoomListServiceState{}

func (c FfiConverterTypeRoomListServiceState) Lift(rb RustBufferI) RoomListServiceState {
	return LiftFromRustBuffer[RoomListServiceState](c, rb)
}

func (c FfiConverterTypeRoomListServiceState) Lower(value RoomListServiceState) RustBuffer {
	return LowerIntoRustBuffer[RoomListServiceState](c, value)
}
func (FfiConverterTypeRoomListServiceState) Read(reader io.Reader) RoomListServiceState {
	id := readInt32(reader)
	return RoomListServiceState(id)
}

func (FfiConverterTypeRoomListServiceState) Write(writer io.Writer, value RoomListServiceState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomListServiceState struct{}

func (_ FfiDestroyerTypeRoomListServiceState) Destroy(value RoomListServiceState) {
}

type RoomListServiceSyncIndicator uint

const (
	RoomListServiceSyncIndicatorShow RoomListServiceSyncIndicator = 1
	RoomListServiceSyncIndicatorHide RoomListServiceSyncIndicator = 2
)

type FfiConverterTypeRoomListServiceSyncIndicator struct{}

var FfiConverterTypeRoomListServiceSyncIndicatorINSTANCE = FfiConverterTypeRoomListServiceSyncIndicator{}

func (c FfiConverterTypeRoomListServiceSyncIndicator) Lift(rb RustBufferI) RoomListServiceSyncIndicator {
	return LiftFromRustBuffer[RoomListServiceSyncIndicator](c, rb)
}

func (c FfiConverterTypeRoomListServiceSyncIndicator) Lower(value RoomListServiceSyncIndicator) RustBuffer {
	return LowerIntoRustBuffer[RoomListServiceSyncIndicator](c, value)
}
func (FfiConverterTypeRoomListServiceSyncIndicator) Read(reader io.Reader) RoomListServiceSyncIndicator {
	id := readInt32(reader)
	return RoomListServiceSyncIndicator(id)
}

func (FfiConverterTypeRoomListServiceSyncIndicator) Write(writer io.Writer, value RoomListServiceSyncIndicator) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomListServiceSyncIndicator struct{}

func (_ FfiDestroyerTypeRoomListServiceSyncIndicator) Destroy(value RoomListServiceSyncIndicator) {
}

type RoomNotificationMode uint

const (
	RoomNotificationModeAllMessages             RoomNotificationMode = 1
	RoomNotificationModeMentionsAndKeywordsOnly RoomNotificationMode = 2
	RoomNotificationModeMute                    RoomNotificationMode = 3
)

type FfiConverterTypeRoomNotificationMode struct{}

var FfiConverterTypeRoomNotificationModeINSTANCE = FfiConverterTypeRoomNotificationMode{}

func (c FfiConverterTypeRoomNotificationMode) Lift(rb RustBufferI) RoomNotificationMode {
	return LiftFromRustBuffer[RoomNotificationMode](c, rb)
}

func (c FfiConverterTypeRoomNotificationMode) Lower(value RoomNotificationMode) RustBuffer {
	return LowerIntoRustBuffer[RoomNotificationMode](c, value)
}
func (FfiConverterTypeRoomNotificationMode) Read(reader io.Reader) RoomNotificationMode {
	id := readInt32(reader)
	return RoomNotificationMode(id)
}

func (FfiConverterTypeRoomNotificationMode) Write(writer io.Writer, value RoomNotificationMode) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomNotificationMode struct{}

func (_ FfiDestroyerTypeRoomNotificationMode) Destroy(value RoomNotificationMode) {
}

type RoomPreset uint

const (
	RoomPresetPrivateChat        RoomPreset = 1
	RoomPresetPublicChat         RoomPreset = 2
	RoomPresetTrustedPrivateChat RoomPreset = 3
)

type FfiConverterTypeRoomPreset struct{}

var FfiConverterTypeRoomPresetINSTANCE = FfiConverterTypeRoomPreset{}

func (c FfiConverterTypeRoomPreset) Lift(rb RustBufferI) RoomPreset {
	return LiftFromRustBuffer[RoomPreset](c, rb)
}

func (c FfiConverterTypeRoomPreset) Lower(value RoomPreset) RustBuffer {
	return LowerIntoRustBuffer[RoomPreset](c, value)
}
func (FfiConverterTypeRoomPreset) Read(reader io.Reader) RoomPreset {
	id := readInt32(reader)
	return RoomPreset(id)
}

func (FfiConverterTypeRoomPreset) Write(writer io.Writer, value RoomPreset) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomPreset struct{}

func (_ FfiDestroyerTypeRoomPreset) Destroy(value RoomPreset) {
}

type RoomVisibility uint

const (
	RoomVisibilityPublic  RoomVisibility = 1
	RoomVisibilityPrivate RoomVisibility = 2
)

type FfiConverterTypeRoomVisibility struct{}

var FfiConverterTypeRoomVisibilityINSTANCE = FfiConverterTypeRoomVisibility{}

func (c FfiConverterTypeRoomVisibility) Lift(rb RustBufferI) RoomVisibility {
	return LiftFromRustBuffer[RoomVisibility](c, rb)
}

func (c FfiConverterTypeRoomVisibility) Lower(value RoomVisibility) RustBuffer {
	return LowerIntoRustBuffer[RoomVisibility](c, value)
}
func (FfiConverterTypeRoomVisibility) Read(reader io.Reader) RoomVisibility {
	id := readInt32(reader)
	return RoomVisibility(id)
}

func (FfiConverterTypeRoomVisibility) Write(writer io.Writer, value RoomVisibility) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeRoomVisibility struct{}

func (_ FfiDestroyerTypeRoomVisibility) Destroy(value RoomVisibility) {
}

type SessionVerificationData interface {
	Destroy()
}
type SessionVerificationDataEmojis struct {
	Emojis  []*SessionVerificationEmoji
	Indices []byte
}

func (e SessionVerificationDataEmojis) Destroy() {
	FfiDestroyerSequenceSessionVerificationEmoji{}.Destroy(e.Emojis)
	FfiDestroyerBytes{}.Destroy(e.Indices)
}

type SessionVerificationDataDecimals struct {
	Values []uint16
}

func (e SessionVerificationDataDecimals) Destroy() {
	FfiDestroyerSequenceUint16{}.Destroy(e.Values)
}

type FfiConverterTypeSessionVerificationData struct{}

var FfiConverterTypeSessionVerificationDataINSTANCE = FfiConverterTypeSessionVerificationData{}

func (c FfiConverterTypeSessionVerificationData) Lift(rb RustBufferI) SessionVerificationData {
	return LiftFromRustBuffer[SessionVerificationData](c, rb)
}

func (c FfiConverterTypeSessionVerificationData) Lower(value SessionVerificationData) RustBuffer {
	return LowerIntoRustBuffer[SessionVerificationData](c, value)
}
func (FfiConverterTypeSessionVerificationData) Read(reader io.Reader) SessionVerificationData {
	id := readInt32(reader)
	switch id {
	case 1:
		return SessionVerificationDataEmojis{
			FfiConverterSequenceSessionVerificationEmojiINSTANCE.Read(reader),
			FfiConverterBytesINSTANCE.Read(reader),
		}
	case 2:
		return SessionVerificationDataDecimals{
			FfiConverterSequenceUint16INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeSessionVerificationData.Read()", id))
	}
}

func (FfiConverterTypeSessionVerificationData) Write(writer io.Writer, value SessionVerificationData) {
	switch variant_value := value.(type) {
	case SessionVerificationDataEmojis:
		writeInt32(writer, 1)
		FfiConverterSequenceSessionVerificationEmojiINSTANCE.Write(writer, variant_value.Emojis)
		FfiConverterBytesINSTANCE.Write(writer, variant_value.Indices)
	case SessionVerificationDataDecimals:
		writeInt32(writer, 2)
		FfiConverterSequenceUint16INSTANCE.Write(writer, variant_value.Values)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeSessionVerificationData.Write", value))
	}
}

type FfiDestroyerTypeSessionVerificationData struct{}

func (_ FfiDestroyerTypeSessionVerificationData) Destroy(value SessionVerificationData) {
	value.Destroy()
}

type StateEventContent interface {
	Destroy()
}
type StateEventContentPolicyRuleRoom struct {
}

func (e StateEventContentPolicyRuleRoom) Destroy() {
}

type StateEventContentPolicyRuleServer struct {
}

func (e StateEventContentPolicyRuleServer) Destroy() {
}

type StateEventContentPolicyRuleUser struct {
}

func (e StateEventContentPolicyRuleUser) Destroy() {
}

type StateEventContentRoomAliases struct {
}

func (e StateEventContentRoomAliases) Destroy() {
}

type StateEventContentRoomAvatar struct {
}

func (e StateEventContentRoomAvatar) Destroy() {
}

type StateEventContentRoomCanonicalAlias struct {
}

func (e StateEventContentRoomCanonicalAlias) Destroy() {
}

type StateEventContentRoomCreate struct {
}

func (e StateEventContentRoomCreate) Destroy() {
}

type StateEventContentRoomEncryption struct {
}

func (e StateEventContentRoomEncryption) Destroy() {
}

type StateEventContentRoomGuestAccess struct {
}

func (e StateEventContentRoomGuestAccess) Destroy() {
}

type StateEventContentRoomHistoryVisibility struct {
}

func (e StateEventContentRoomHistoryVisibility) Destroy() {
}

type StateEventContentRoomJoinRules struct {
}

func (e StateEventContentRoomJoinRules) Destroy() {
}

type StateEventContentRoomMemberContent struct {
	UserId          string
	MembershipState MembershipState
}

func (e StateEventContentRoomMemberContent) Destroy() {
	FfiDestroyerString{}.Destroy(e.UserId)
	FfiDestroyerTypeMembershipState{}.Destroy(e.MembershipState)
}

type StateEventContentRoomName struct {
}

func (e StateEventContentRoomName) Destroy() {
}

type StateEventContentRoomPinnedEvents struct {
}

func (e StateEventContentRoomPinnedEvents) Destroy() {
}

type StateEventContentRoomPowerLevels struct {
}

func (e StateEventContentRoomPowerLevels) Destroy() {
}

type StateEventContentRoomServerAcl struct {
}

func (e StateEventContentRoomServerAcl) Destroy() {
}

type StateEventContentRoomThirdPartyInvite struct {
}

func (e StateEventContentRoomThirdPartyInvite) Destroy() {
}

type StateEventContentRoomTombstone struct {
}

func (e StateEventContentRoomTombstone) Destroy() {
}

type StateEventContentRoomTopic struct {
}

func (e StateEventContentRoomTopic) Destroy() {
}

type StateEventContentSpaceChild struct {
}

func (e StateEventContentSpaceChild) Destroy() {
}

type StateEventContentSpaceParent struct {
}

func (e StateEventContentSpaceParent) Destroy() {
}

type FfiConverterTypeStateEventContent struct{}

var FfiConverterTypeStateEventContentINSTANCE = FfiConverterTypeStateEventContent{}

func (c FfiConverterTypeStateEventContent) Lift(rb RustBufferI) StateEventContent {
	return LiftFromRustBuffer[StateEventContent](c, rb)
}

func (c FfiConverterTypeStateEventContent) Lower(value StateEventContent) RustBuffer {
	return LowerIntoRustBuffer[StateEventContent](c, value)
}
func (FfiConverterTypeStateEventContent) Read(reader io.Reader) StateEventContent {
	id := readInt32(reader)
	switch id {
	case 1:
		return StateEventContentPolicyRuleRoom{}
	case 2:
		return StateEventContentPolicyRuleServer{}
	case 3:
		return StateEventContentPolicyRuleUser{}
	case 4:
		return StateEventContentRoomAliases{}
	case 5:
		return StateEventContentRoomAvatar{}
	case 6:
		return StateEventContentRoomCanonicalAlias{}
	case 7:
		return StateEventContentRoomCreate{}
	case 8:
		return StateEventContentRoomEncryption{}
	case 9:
		return StateEventContentRoomGuestAccess{}
	case 10:
		return StateEventContentRoomHistoryVisibility{}
	case 11:
		return StateEventContentRoomJoinRules{}
	case 12:
		return StateEventContentRoomMemberContent{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterTypeMembershipStateINSTANCE.Read(reader),
		}
	case 13:
		return StateEventContentRoomName{}
	case 14:
		return StateEventContentRoomPinnedEvents{}
	case 15:
		return StateEventContentRoomPowerLevels{}
	case 16:
		return StateEventContentRoomServerAcl{}
	case 17:
		return StateEventContentRoomThirdPartyInvite{}
	case 18:
		return StateEventContentRoomTombstone{}
	case 19:
		return StateEventContentRoomTopic{}
	case 20:
		return StateEventContentSpaceChild{}
	case 21:
		return StateEventContentSpaceParent{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeStateEventContent.Read()", id))
	}
}

func (FfiConverterTypeStateEventContent) Write(writer io.Writer, value StateEventContent) {
	switch variant_value := value.(type) {
	case StateEventContentPolicyRuleRoom:
		writeInt32(writer, 1)
	case StateEventContentPolicyRuleServer:
		writeInt32(writer, 2)
	case StateEventContentPolicyRuleUser:
		writeInt32(writer, 3)
	case StateEventContentRoomAliases:
		writeInt32(writer, 4)
	case StateEventContentRoomAvatar:
		writeInt32(writer, 5)
	case StateEventContentRoomCanonicalAlias:
		writeInt32(writer, 6)
	case StateEventContentRoomCreate:
		writeInt32(writer, 7)
	case StateEventContentRoomEncryption:
		writeInt32(writer, 8)
	case StateEventContentRoomGuestAccess:
		writeInt32(writer, 9)
	case StateEventContentRoomHistoryVisibility:
		writeInt32(writer, 10)
	case StateEventContentRoomJoinRules:
		writeInt32(writer, 11)
	case StateEventContentRoomMemberContent:
		writeInt32(writer, 12)
		FfiConverterStringINSTANCE.Write(writer, variant_value.UserId)
		FfiConverterTypeMembershipStateINSTANCE.Write(writer, variant_value.MembershipState)
	case StateEventContentRoomName:
		writeInt32(writer, 13)
	case StateEventContentRoomPinnedEvents:
		writeInt32(writer, 14)
	case StateEventContentRoomPowerLevels:
		writeInt32(writer, 15)
	case StateEventContentRoomServerAcl:
		writeInt32(writer, 16)
	case StateEventContentRoomThirdPartyInvite:
		writeInt32(writer, 17)
	case StateEventContentRoomTombstone:
		writeInt32(writer, 18)
	case StateEventContentRoomTopic:
		writeInt32(writer, 19)
	case StateEventContentSpaceChild:
		writeInt32(writer, 20)
	case StateEventContentSpaceParent:
		writeInt32(writer, 21)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeStateEventContent.Write", value))
	}
}

type FfiDestroyerTypeStateEventContent struct{}

func (_ FfiDestroyerTypeStateEventContent) Destroy(value StateEventContent) {
	value.Destroy()
}

type StateEventType uint

const (
	StateEventTypeCallMember            StateEventType = 1
	StateEventTypePolicyRuleRoom        StateEventType = 2
	StateEventTypePolicyRuleServer      StateEventType = 3
	StateEventTypePolicyRuleUser        StateEventType = 4
	StateEventTypeRoomAliases           StateEventType = 5
	StateEventTypeRoomAvatar            StateEventType = 6
	StateEventTypeRoomCanonicalAlias    StateEventType = 7
	StateEventTypeRoomCreate            StateEventType = 8
	StateEventTypeRoomEncryption        StateEventType = 9
	StateEventTypeRoomGuestAccess       StateEventType = 10
	StateEventTypeRoomHistoryVisibility StateEventType = 11
	StateEventTypeRoomJoinRules         StateEventType = 12
	StateEventTypeRoomMemberEvent       StateEventType = 13
	StateEventTypeRoomName              StateEventType = 14
	StateEventTypeRoomPinnedEvents      StateEventType = 15
	StateEventTypeRoomPowerLevels       StateEventType = 16
	StateEventTypeRoomServerAcl         StateEventType = 17
	StateEventTypeRoomThirdPartyInvite  StateEventType = 18
	StateEventTypeRoomTombstone         StateEventType = 19
	StateEventTypeRoomTopic             StateEventType = 20
	StateEventTypeSpaceChild            StateEventType = 21
	StateEventTypeSpaceParent           StateEventType = 22
)

type FfiConverterTypeStateEventType struct{}

var FfiConverterTypeStateEventTypeINSTANCE = FfiConverterTypeStateEventType{}

func (c FfiConverterTypeStateEventType) Lift(rb RustBufferI) StateEventType {
	return LiftFromRustBuffer[StateEventType](c, rb)
}

func (c FfiConverterTypeStateEventType) Lower(value StateEventType) RustBuffer {
	return LowerIntoRustBuffer[StateEventType](c, value)
}
func (FfiConverterTypeStateEventType) Read(reader io.Reader) StateEventType {
	id := readInt32(reader)
	return StateEventType(id)
}

func (FfiConverterTypeStateEventType) Write(writer io.Writer, value StateEventType) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeStateEventType struct{}

func (_ FfiDestroyerTypeStateEventType) Destroy(value StateEventType) {
}

type SteadyStateError struct {
	err error
}

func (err SteadyStateError) Error() string {
	return fmt.Sprintf("SteadyStateError: %s", err.err.Error())
}

func (err SteadyStateError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrSteadyStateErrorBackupDisabled = fmt.Errorf("SteadyStateErrorBackupDisabled")
var ErrSteadyStateErrorConnection = fmt.Errorf("SteadyStateErrorConnection")
var ErrSteadyStateErrorLagged = fmt.Errorf("SteadyStateErrorLagged")

// Variant structs
type SteadyStateErrorBackupDisabled struct {
	message string
}

func NewSteadyStateErrorBackupDisabled() *SteadyStateError {
	return &SteadyStateError{
		err: &SteadyStateErrorBackupDisabled{},
	}
}

func (err SteadyStateErrorBackupDisabled) Error() string {
	return fmt.Sprintf("BackupDisabled: %s", err.message)
}

func (self SteadyStateErrorBackupDisabled) Is(target error) bool {
	return target == ErrSteadyStateErrorBackupDisabled
}

type SteadyStateErrorConnection struct {
	message string
}

func NewSteadyStateErrorConnection() *SteadyStateError {
	return &SteadyStateError{
		err: &SteadyStateErrorConnection{},
	}
}

func (err SteadyStateErrorConnection) Error() string {
	return fmt.Sprintf("Connection: %s", err.message)
}

func (self SteadyStateErrorConnection) Is(target error) bool {
	return target == ErrSteadyStateErrorConnection
}

type SteadyStateErrorLagged struct {
	message string
}

func NewSteadyStateErrorLagged() *SteadyStateError {
	return &SteadyStateError{
		err: &SteadyStateErrorLagged{},
	}
}

func (err SteadyStateErrorLagged) Error() string {
	return fmt.Sprintf("Lagged: %s", err.message)
}

func (self SteadyStateErrorLagged) Is(target error) bool {
	return target == ErrSteadyStateErrorLagged
}

type FfiConverterTypeSteadyStateError struct{}

var FfiConverterTypeSteadyStateErrorINSTANCE = FfiConverterTypeSteadyStateError{}

func (c FfiConverterTypeSteadyStateError) Lift(eb RustBufferI) error {
	return LiftFromRustBuffer[error](c, eb)
}

func (c FfiConverterTypeSteadyStateError) Lower(value *SteadyStateError) RustBuffer {
	return LowerIntoRustBuffer[*SteadyStateError](c, value)
}

func (c FfiConverterTypeSteadyStateError) Read(reader io.Reader) error {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &SteadyStateError{&SteadyStateErrorBackupDisabled{message}}
	case 2:
		return &SteadyStateError{&SteadyStateErrorConnection{message}}
	case 3:
		return &SteadyStateError{&SteadyStateErrorLagged{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterTypeSteadyStateError.Read()", errorID))
	}

}

func (c FfiConverterTypeSteadyStateError) Write(writer io.Writer, value *SteadyStateError) {
	switch variantValue := value.err.(type) {
	case *SteadyStateErrorBackupDisabled:
		writeInt32(writer, 1)
	case *SteadyStateErrorConnection:
		writeInt32(writer, 2)
	case *SteadyStateErrorLagged:
		writeInt32(writer, 3)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterTypeSteadyStateError.Write", value))
	}
}

type SyncServiceState uint

const (
	SyncServiceStateIdle       SyncServiceState = 1
	SyncServiceStateRunning    SyncServiceState = 2
	SyncServiceStateTerminated SyncServiceState = 3
	SyncServiceStateError      SyncServiceState = 4
)

type FfiConverterTypeSyncServiceState struct{}

var FfiConverterTypeSyncServiceStateINSTANCE = FfiConverterTypeSyncServiceState{}

func (c FfiConverterTypeSyncServiceState) Lift(rb RustBufferI) SyncServiceState {
	return LiftFromRustBuffer[SyncServiceState](c, rb)
}

func (c FfiConverterTypeSyncServiceState) Lower(value SyncServiceState) RustBuffer {
	return LowerIntoRustBuffer[SyncServiceState](c, value)
}
func (FfiConverterTypeSyncServiceState) Read(reader io.Reader) SyncServiceState {
	id := readInt32(reader)
	return SyncServiceState(id)
}

func (FfiConverterTypeSyncServiceState) Write(writer io.Writer, value SyncServiceState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeSyncServiceState struct{}

func (_ FfiDestroyerTypeSyncServiceState) Destroy(value SyncServiceState) {
}

type TimelineChange uint

const (
	TimelineChangeAppend    TimelineChange = 1
	TimelineChangeClear     TimelineChange = 2
	TimelineChangeInsert    TimelineChange = 3
	TimelineChangeSet       TimelineChange = 4
	TimelineChangeRemove    TimelineChange = 5
	TimelineChangePushBack  TimelineChange = 6
	TimelineChangePushFront TimelineChange = 7
	TimelineChangePopBack   TimelineChange = 8
	TimelineChangePopFront  TimelineChange = 9
	TimelineChangeTruncate  TimelineChange = 10
	TimelineChangeReset     TimelineChange = 11
)

type FfiConverterTypeTimelineChange struct{}

var FfiConverterTypeTimelineChangeINSTANCE = FfiConverterTypeTimelineChange{}

func (c FfiConverterTypeTimelineChange) Lift(rb RustBufferI) TimelineChange {
	return LiftFromRustBuffer[TimelineChange](c, rb)
}

func (c FfiConverterTypeTimelineChange) Lower(value TimelineChange) RustBuffer {
	return LowerIntoRustBuffer[TimelineChange](c, value)
}
func (FfiConverterTypeTimelineChange) Read(reader io.Reader) TimelineChange {
	id := readInt32(reader)
	return TimelineChange(id)
}

func (FfiConverterTypeTimelineChange) Write(writer io.Writer, value TimelineChange) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerTypeTimelineChange struct{}

func (_ FfiDestroyerTypeTimelineChange) Destroy(value TimelineChange) {
}

type TimelineEventType interface {
	Destroy()
}
type TimelineEventTypeMessageLike struct {
	Content MessageLikeEventContent
}

func (e TimelineEventTypeMessageLike) Destroy() {
	FfiDestroyerTypeMessageLikeEventContent{}.Destroy(e.Content)
}

type TimelineEventTypeState struct {
	Content StateEventContent
}

func (e TimelineEventTypeState) Destroy() {
	FfiDestroyerTypeStateEventContent{}.Destroy(e.Content)
}

type FfiConverterTypeTimelineEventType struct{}

var FfiConverterTypeTimelineEventTypeINSTANCE = FfiConverterTypeTimelineEventType{}

func (c FfiConverterTypeTimelineEventType) Lift(rb RustBufferI) TimelineEventType {
	return LiftFromRustBuffer[TimelineEventType](c, rb)
}

func (c FfiConverterTypeTimelineEventType) Lower(value TimelineEventType) RustBuffer {
	return LowerIntoRustBuffer[TimelineEventType](c, value)
}
func (FfiConverterTypeTimelineEventType) Read(reader io.Reader) TimelineEventType {
	id := readInt32(reader)
	switch id {
	case 1:
		return TimelineEventTypeMessageLike{
			FfiConverterTypeMessageLikeEventContentINSTANCE.Read(reader),
		}
	case 2:
		return TimelineEventTypeState{
			FfiConverterTypeStateEventContentINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeTimelineEventType.Read()", id))
	}
}

func (FfiConverterTypeTimelineEventType) Write(writer io.Writer, value TimelineEventType) {
	switch variant_value := value.(type) {
	case TimelineEventTypeMessageLike:
		writeInt32(writer, 1)
		FfiConverterTypeMessageLikeEventContentINSTANCE.Write(writer, variant_value.Content)
	case TimelineEventTypeState:
		writeInt32(writer, 2)
		FfiConverterTypeStateEventContentINSTANCE.Write(writer, variant_value.Content)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeTimelineEventType.Write", value))
	}
}

type FfiDestroyerTypeTimelineEventType struct{}

func (_ FfiDestroyerTypeTimelineEventType) Destroy(value TimelineEventType) {
	value.Destroy()
}

type TimelineItemContentKind interface {
	Destroy()
}
type TimelineItemContentKindMessage struct {
}

func (e TimelineItemContentKindMessage) Destroy() {
}

type TimelineItemContentKindRedactedMessage struct {
}

func (e TimelineItemContentKindRedactedMessage) Destroy() {
}

type TimelineItemContentKindSticker struct {
	Body string
	Info ImageInfo
	Url  string
}

func (e TimelineItemContentKindSticker) Destroy() {
	FfiDestroyerString{}.Destroy(e.Body)
	FfiDestroyerTypeImageInfo{}.Destroy(e.Info)
	FfiDestroyerString{}.Destroy(e.Url)
}

type TimelineItemContentKindPoll struct {
	Question      string
	Kind          PollKind
	MaxSelections uint64
	Answers       []PollAnswer
	Votes         map[string][]string
	EndTime       *uint64
	HasBeenEdited bool
}

func (e TimelineItemContentKindPoll) Destroy() {
	FfiDestroyerString{}.Destroy(e.Question)
	FfiDestroyerTypePollKind{}.Destroy(e.Kind)
	FfiDestroyerUint64{}.Destroy(e.MaxSelections)
	FfiDestroyerSequenceTypePollAnswer{}.Destroy(e.Answers)
	FfiDestroyerMapStringSequenceString{}.Destroy(e.Votes)
	FfiDestroyerOptionalUint64{}.Destroy(e.EndTime)
	FfiDestroyerBool{}.Destroy(e.HasBeenEdited)
}

type TimelineItemContentKindUnableToDecrypt struct {
	Msg EncryptedMessage
}

func (e TimelineItemContentKindUnableToDecrypt) Destroy() {
	FfiDestroyerTypeEncryptedMessage{}.Destroy(e.Msg)
}

type TimelineItemContentKindRoomMembership struct {
	UserId string
	Change *MembershipChange
}

func (e TimelineItemContentKindRoomMembership) Destroy() {
	FfiDestroyerString{}.Destroy(e.UserId)
	FfiDestroyerOptionalTypeMembershipChange{}.Destroy(e.Change)
}

type TimelineItemContentKindProfileChange struct {
	DisplayName     *string
	PrevDisplayName *string
	AvatarUrl       *string
	PrevAvatarUrl   *string
}

func (e TimelineItemContentKindProfileChange) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(e.DisplayName)
	FfiDestroyerOptionalString{}.Destroy(e.PrevDisplayName)
	FfiDestroyerOptionalString{}.Destroy(e.AvatarUrl)
	FfiDestroyerOptionalString{}.Destroy(e.PrevAvatarUrl)
}

type TimelineItemContentKindState struct {
	StateKey string
	Content  OtherState
}

func (e TimelineItemContentKindState) Destroy() {
	FfiDestroyerString{}.Destroy(e.StateKey)
	FfiDestroyerTypeOtherState{}.Destroy(e.Content)
}

type TimelineItemContentKindFailedToParseMessageLike struct {
	EventType string
	Error     string
}

func (e TimelineItemContentKindFailedToParseMessageLike) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventType)
	FfiDestroyerString{}.Destroy(e.Error)
}

type TimelineItemContentKindFailedToParseState struct {
	EventType string
	StateKey  string
	Error     string
}

func (e TimelineItemContentKindFailedToParseState) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventType)
	FfiDestroyerString{}.Destroy(e.StateKey)
	FfiDestroyerString{}.Destroy(e.Error)
}

type FfiConverterTypeTimelineItemContentKind struct{}

var FfiConverterTypeTimelineItemContentKindINSTANCE = FfiConverterTypeTimelineItemContentKind{}

func (c FfiConverterTypeTimelineItemContentKind) Lift(rb RustBufferI) TimelineItemContentKind {
	return LiftFromRustBuffer[TimelineItemContentKind](c, rb)
}

func (c FfiConverterTypeTimelineItemContentKind) Lower(value TimelineItemContentKind) RustBuffer {
	return LowerIntoRustBuffer[TimelineItemContentKind](c, value)
}
func (FfiConverterTypeTimelineItemContentKind) Read(reader io.Reader) TimelineItemContentKind {
	id := readInt32(reader)
	switch id {
	case 1:
		return TimelineItemContentKindMessage{}
	case 2:
		return TimelineItemContentKindRedactedMessage{}
	case 3:
		return TimelineItemContentKindSticker{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterTypeImageInfoINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 4:
		return TimelineItemContentKindPoll{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterTypePollKindINSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterSequenceTypePollAnswerINSTANCE.Read(reader),
			FfiConverterMapStringSequenceStringINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
			FfiConverterBoolINSTANCE.Read(reader),
		}
	case 5:
		return TimelineItemContentKindUnableToDecrypt{
			FfiConverterTypeEncryptedMessageINSTANCE.Read(reader),
		}
	case 6:
		return TimelineItemContentKindRoomMembership{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterOptionalTypeMembershipChangeINSTANCE.Read(reader),
		}
	case 7:
		return TimelineItemContentKindProfileChange{
			FfiConverterOptionalStringINSTANCE.Read(reader),
			FfiConverterOptionalStringINSTANCE.Read(reader),
			FfiConverterOptionalStringINSTANCE.Read(reader),
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 8:
		return TimelineItemContentKindState{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterTypeOtherStateINSTANCE.Read(reader),
		}
	case 9:
		return TimelineItemContentKindFailedToParseMessageLike{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 10:
		return TimelineItemContentKindFailedToParseState{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeTimelineItemContentKind.Read()", id))
	}
}

func (FfiConverterTypeTimelineItemContentKind) Write(writer io.Writer, value TimelineItemContentKind) {
	switch variant_value := value.(type) {
	case TimelineItemContentKindMessage:
		writeInt32(writer, 1)
	case TimelineItemContentKindRedactedMessage:
		writeInt32(writer, 2)
	case TimelineItemContentKindSticker:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Body)
		FfiConverterTypeImageInfoINSTANCE.Write(writer, variant_value.Info)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Url)
	case TimelineItemContentKindPoll:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Question)
		FfiConverterTypePollKindINSTANCE.Write(writer, variant_value.Kind)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.MaxSelections)
		FfiConverterSequenceTypePollAnswerINSTANCE.Write(writer, variant_value.Answers)
		FfiConverterMapStringSequenceStringINSTANCE.Write(writer, variant_value.Votes)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.EndTime)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.HasBeenEdited)
	case TimelineItemContentKindUnableToDecrypt:
		writeInt32(writer, 5)
		FfiConverterTypeEncryptedMessageINSTANCE.Write(writer, variant_value.Msg)
	case TimelineItemContentKindRoomMembership:
		writeInt32(writer, 6)
		FfiConverterStringINSTANCE.Write(writer, variant_value.UserId)
		FfiConverterOptionalTypeMembershipChangeINSTANCE.Write(writer, variant_value.Change)
	case TimelineItemContentKindProfileChange:
		writeInt32(writer, 7)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.DisplayName)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.PrevDisplayName)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.AvatarUrl)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.PrevAvatarUrl)
	case TimelineItemContentKindState:
		writeInt32(writer, 8)
		FfiConverterStringINSTANCE.Write(writer, variant_value.StateKey)
		FfiConverterTypeOtherStateINSTANCE.Write(writer, variant_value.Content)
	case TimelineItemContentKindFailedToParseMessageLike:
		writeInt32(writer, 9)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventType)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Error)
	case TimelineItemContentKindFailedToParseState:
		writeInt32(writer, 10)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventType)
		FfiConverterStringINSTANCE.Write(writer, variant_value.StateKey)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Error)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeTimelineItemContentKind.Write", value))
	}
}

type FfiDestroyerTypeTimelineItemContentKind struct{}

func (_ FfiDestroyerTypeTimelineItemContentKind) Destroy(value TimelineItemContentKind) {
	value.Destroy()
}

type VirtualTimelineItem interface {
	Destroy()
}
type VirtualTimelineItemDayDivider struct {
	Ts uint64
}

func (e VirtualTimelineItemDayDivider) Destroy() {
	FfiDestroyerUint64{}.Destroy(e.Ts)
}

type VirtualTimelineItemReadMarker struct {
}

func (e VirtualTimelineItemReadMarker) Destroy() {
}

type FfiConverterTypeVirtualTimelineItem struct{}

var FfiConverterTypeVirtualTimelineItemINSTANCE = FfiConverterTypeVirtualTimelineItem{}

func (c FfiConverterTypeVirtualTimelineItem) Lift(rb RustBufferI) VirtualTimelineItem {
	return LiftFromRustBuffer[VirtualTimelineItem](c, rb)
}

func (c FfiConverterTypeVirtualTimelineItem) Lower(value VirtualTimelineItem) RustBuffer {
	return LowerIntoRustBuffer[VirtualTimelineItem](c, value)
}
func (FfiConverterTypeVirtualTimelineItem) Read(reader io.Reader) VirtualTimelineItem {
	id := readInt32(reader)
	switch id {
	case 1:
		return VirtualTimelineItemDayDivider{
			FfiConverterUint64INSTANCE.Read(reader),
		}
	case 2:
		return VirtualTimelineItemReadMarker{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeVirtualTimelineItem.Read()", id))
	}
}

func (FfiConverterTypeVirtualTimelineItem) Write(writer io.Writer, value VirtualTimelineItem) {
	switch variant_value := value.(type) {
	case VirtualTimelineItemDayDivider:
		writeInt32(writer, 1)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.Ts)
	case VirtualTimelineItemReadMarker:
		writeInt32(writer, 2)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeVirtualTimelineItem.Write", value))
	}
}

type FfiDestroyerTypeVirtualTimelineItem struct{}

func (_ FfiDestroyerTypeVirtualTimelineItem) Destroy(value VirtualTimelineItem) {
	value.Destroy()
}

type WidgetEventFilter interface {
	Destroy()
}
type WidgetEventFilterMessageLikeWithType struct {
	EventType string
}

func (e WidgetEventFilterMessageLikeWithType) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventType)
}

type WidgetEventFilterRoomMessageWithMsgtype struct {
	Msgtype string
}

func (e WidgetEventFilterRoomMessageWithMsgtype) Destroy() {
	FfiDestroyerString{}.Destroy(e.Msgtype)
}

type WidgetEventFilterStateWithType struct {
	EventType string
}

func (e WidgetEventFilterStateWithType) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventType)
}

type WidgetEventFilterStateWithTypeAndStateKey struct {
	EventType string
	StateKey  string
}

func (e WidgetEventFilterStateWithTypeAndStateKey) Destroy() {
	FfiDestroyerString{}.Destroy(e.EventType)
	FfiDestroyerString{}.Destroy(e.StateKey)
}

type FfiConverterTypeWidgetEventFilter struct{}

var FfiConverterTypeWidgetEventFilterINSTANCE = FfiConverterTypeWidgetEventFilter{}

func (c FfiConverterTypeWidgetEventFilter) Lift(rb RustBufferI) WidgetEventFilter {
	return LiftFromRustBuffer[WidgetEventFilter](c, rb)
}

func (c FfiConverterTypeWidgetEventFilter) Lower(value WidgetEventFilter) RustBuffer {
	return LowerIntoRustBuffer[WidgetEventFilter](c, value)
}
func (FfiConverterTypeWidgetEventFilter) Read(reader io.Reader) WidgetEventFilter {
	id := readInt32(reader)
	switch id {
	case 1:
		return WidgetEventFilterMessageLikeWithType{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 2:
		return WidgetEventFilterRoomMessageWithMsgtype{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 3:
		return WidgetEventFilterStateWithType{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 4:
		return WidgetEventFilterStateWithTypeAndStateKey{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterTypeWidgetEventFilter.Read()", id))
	}
}

func (FfiConverterTypeWidgetEventFilter) Write(writer io.Writer, value WidgetEventFilter) {
	switch variant_value := value.(type) {
	case WidgetEventFilterMessageLikeWithType:
		writeInt32(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventType)
	case WidgetEventFilterRoomMessageWithMsgtype:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Msgtype)
	case WidgetEventFilterStateWithType:
		writeInt32(writer, 3)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventType)
	case WidgetEventFilterStateWithTypeAndStateKey:
		writeInt32(writer, 4)
		FfiConverterStringINSTANCE.Write(writer, variant_value.EventType)
		FfiConverterStringINSTANCE.Write(writer, variant_value.StateKey)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterTypeWidgetEventFilter.Write", value))
	}
}

type FfiDestroyerTypeWidgetEventFilter struct{}

func (_ FfiDestroyerTypeWidgetEventFilter) Destroy(value WidgetEventFilter) {
	value.Destroy()
}

type uniffiCallbackResult C.int32_t

const (
	uniffiIdxCallbackFree               uniffiCallbackResult = 0
	uniffiCallbackResultSuccess         uniffiCallbackResult = 0
	uniffiCallbackResultError           uniffiCallbackResult = 1
	uniffiCallbackUnexpectedResultError uniffiCallbackResult = 2
	uniffiCallbackCancelled             uniffiCallbackResult = 3
)

type concurrentHandleMap[T any] struct {
	leftMap       map[uint64]*T
	rightMap      map[*T]uint64
	currentHandle uint64
	lock          sync.RWMutex
}

func newConcurrentHandleMap[T any]() *concurrentHandleMap[T] {
	return &concurrentHandleMap[T]{
		leftMap:  map[uint64]*T{},
		rightMap: map[*T]uint64{},
	}
}

func (cm *concurrentHandleMap[T]) insert(obj *T) uint64 {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	if existingHandle, ok := cm.rightMap[obj]; ok {
		return existingHandle
	}
	cm.currentHandle = cm.currentHandle + 1
	cm.leftMap[cm.currentHandle] = obj
	cm.rightMap[obj] = cm.currentHandle
	return cm.currentHandle
}

func (cm *concurrentHandleMap[T]) remove(handle uint64) bool {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	if val, ok := cm.leftMap[handle]; ok {
		delete(cm.leftMap, handle)
		delete(cm.rightMap, val)
	}
	return false
}

func (cm *concurrentHandleMap[T]) tryGet(handle uint64) (*T, bool) {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	val, ok := cm.leftMap[handle]
	return val, ok
}

type FfiConverterCallbackInterface[CallbackInterface any] struct {
	handleMap *concurrentHandleMap[CallbackInterface]
}

func (c *FfiConverterCallbackInterface[CallbackInterface]) drop(handle uint64) RustBuffer {
	c.handleMap.remove(handle)
	return RustBuffer{}
}

func (c *FfiConverterCallbackInterface[CallbackInterface]) Lift(handle uint64) CallbackInterface {
	val, ok := c.handleMap.tryGet(handle)
	if !ok {
		panic(fmt.Errorf("no callback in handle map: %d", handle))
	}
	return *val
}

func (c *FfiConverterCallbackInterface[CallbackInterface]) Read(reader io.Reader) CallbackInterface {
	return c.Lift(readUint64(reader))
}

func (c *FfiConverterCallbackInterface[CallbackInterface]) Lower(value CallbackInterface) C.uint64_t {
	return C.uint64_t(c.handleMap.insert(&value))
}

func (c *FfiConverterCallbackInterface[CallbackInterface]) Write(writer io.Writer, value CallbackInterface) {
	writeUint64(writer, uint64(c.Lower(value)))
}

// Declaration and FfiConverters for BackPaginationStatusListener Callback Interface
type BackPaginationStatusListener interface {
	OnUpdate(status matrix_sdk_ui.BackPaginationStatus)
}

// foreignCallbackCallbackInterfaceBackPaginationStatusListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceBackPaginationStatusListener struct{}

//export matrix_sdk_ffi_cgo_BackPaginationStatusListener
func matrix_sdk_ffi_cgo_BackPaginationStatusListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceBackPaginationStatusListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceBackPaginationStatusListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceBackPaginationStatusListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceBackPaginationStatusListener) InvokeOnUpdate(callback BackPaginationStatusListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(matrix_sdk_ui.FfiConverterTypeBackPaginationStatusINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceBackPaginationStatusListener struct {
	FfiConverterCallbackInterface[BackPaginationStatusListener]
}

var FfiConverterCallbackInterfaceBackPaginationStatusListenerINSTANCE = &FfiConverterCallbackInterfaceBackPaginationStatusListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[BackPaginationStatusListener]{
		handleMap: newConcurrentHandleMap[BackPaginationStatusListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceBackPaginationStatusListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_backpaginationstatuslistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_BackPaginationStatusListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceBackPaginationStatusListener struct{}

func (FfiDestroyerCallbackInterfaceBackPaginationStatusListener) Destroy(value BackPaginationStatusListener) {
}

// Declaration and FfiConverters for BackupStateListener Callback Interface
type BackupStateListener interface {
	OnUpdate(status BackupState)
}

// foreignCallbackCallbackInterfaceBackupStateListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceBackupStateListener struct{}

//export matrix_sdk_ffi_cgo_BackupStateListener
func matrix_sdk_ffi_cgo_BackupStateListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceBackupStateListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceBackupStateListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceBackupStateListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceBackupStateListener) InvokeOnUpdate(callback BackupStateListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeBackupStateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceBackupStateListener struct {
	FfiConverterCallbackInterface[BackupStateListener]
}

var FfiConverterCallbackInterfaceBackupStateListenerINSTANCE = &FfiConverterCallbackInterfaceBackupStateListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[BackupStateListener]{
		handleMap: newConcurrentHandleMap[BackupStateListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceBackupStateListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_backupstatelistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_BackupStateListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceBackupStateListener struct{}

func (FfiDestroyerCallbackInterfaceBackupStateListener) Destroy(value BackupStateListener) {
}

// Declaration and FfiConverters for BackupSteadyStateListener Callback Interface
type BackupSteadyStateListener interface {
	OnUpdate(status BackupUploadState)
}

// foreignCallbackCallbackInterfaceBackupSteadyStateListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceBackupSteadyStateListener struct{}

//export matrix_sdk_ffi_cgo_BackupSteadyStateListener
func matrix_sdk_ffi_cgo_BackupSteadyStateListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceBackupSteadyStateListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceBackupSteadyStateListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceBackupSteadyStateListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceBackupSteadyStateListener) InvokeOnUpdate(callback BackupSteadyStateListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeBackupUploadStateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceBackupSteadyStateListener struct {
	FfiConverterCallbackInterface[BackupSteadyStateListener]
}

var FfiConverterCallbackInterfaceBackupSteadyStateListenerINSTANCE = &FfiConverterCallbackInterfaceBackupSteadyStateListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[BackupSteadyStateListener]{
		handleMap: newConcurrentHandleMap[BackupSteadyStateListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceBackupSteadyStateListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_backupsteadystatelistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_BackupSteadyStateListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceBackupSteadyStateListener struct{}

func (FfiDestroyerCallbackInterfaceBackupSteadyStateListener) Destroy(value BackupSteadyStateListener) {
}

// Declaration and FfiConverters for ClientDelegate Callback Interface
type ClientDelegate interface {
	DidReceiveAuthError(isSoftLogout bool)
	DidRefreshTokens()
}

// foreignCallbackCallbackInterfaceClientDelegate cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceClientDelegate struct{}

//export matrix_sdk_ffi_cgo_ClientDelegate
func matrix_sdk_ffi_cgo_ClientDelegate(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceClientDelegateINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceClientDelegateINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceClientDelegate{}.InvokeDidReceiveAuthError(cb, args, outBuf)
		return C.int32_t(result)
	case 2:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceClientDelegate{}.InvokeDidRefreshTokens(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceClientDelegate) InvokeDidReceiveAuthError(callback ClientDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.DidReceiveAuthError(FfiConverterBoolINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceClientDelegate) InvokeDidRefreshTokens(callback ClientDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.DidRefreshTokens()

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceClientDelegate struct {
	FfiConverterCallbackInterface[ClientDelegate]
}

var FfiConverterCallbackInterfaceClientDelegateINSTANCE = &FfiConverterCallbackInterfaceClientDelegate{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[ClientDelegate]{
		handleMap: newConcurrentHandleMap[ClientDelegate](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceClientDelegate) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_clientdelegate(C.ForeignCallback(C.matrix_sdk_ffi_cgo_ClientDelegate), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceClientDelegate struct{}

func (FfiDestroyerCallbackInterfaceClientDelegate) Destroy(value ClientDelegate) {
}

// Declaration and FfiConverters for ClientSessionDelegate Callback Interface
type ClientSessionDelegate interface {
	RetrieveSessionFromKeychain(userId string) (Session, *ClientError)
	SaveSessionInKeychain(session Session)
}

// foreignCallbackCallbackInterfaceClientSessionDelegate cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceClientSessionDelegate struct{}

//export matrix_sdk_ffi_cgo_ClientSessionDelegate
func matrix_sdk_ffi_cgo_ClientSessionDelegate(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceClientSessionDelegate{}.InvokeRetrieveSessionFromKeychain(cb, args, outBuf)
		return C.int32_t(result)
	case 2:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceClientSessionDelegate{}.InvokeSaveSessionInKeychain(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceClientSessionDelegate) InvokeRetrieveSessionFromKeychain(callback ClientSessionDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	result, err := callback.RetrieveSessionFromKeychain(FfiConverterStringINSTANCE.Read(reader))

	if err != nil {
		// The only way to bypass an unexpected error is to bypass pointer to an empty
		// instance of the error
		if err.err == nil {
			return uniffiCallbackUnexpectedResultError
		}
		*outBuf = LowerIntoRustBuffer[*ClientError](FfiConverterTypeClientErrorINSTANCE, err)
		return uniffiCallbackResultError
	}
	*outBuf = LowerIntoRustBuffer[Session](FfiConverterTypeSessionINSTANCE, result)
	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceClientSessionDelegate) InvokeSaveSessionInKeychain(callback ClientSessionDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.SaveSessionInKeychain(FfiConverterTypeSessionINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceClientSessionDelegate struct {
	FfiConverterCallbackInterface[ClientSessionDelegate]
}

var FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE = &FfiConverterCallbackInterfaceClientSessionDelegate{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[ClientSessionDelegate]{
		handleMap: newConcurrentHandleMap[ClientSessionDelegate](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceClientSessionDelegate) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_clientsessiondelegate(C.ForeignCallback(C.matrix_sdk_ffi_cgo_ClientSessionDelegate), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceClientSessionDelegate struct{}

func (FfiDestroyerCallbackInterfaceClientSessionDelegate) Destroy(value ClientSessionDelegate) {
}

// Declaration and FfiConverters for EnableRecoveryProgressListener Callback Interface
type EnableRecoveryProgressListener interface {
	OnUpdate(status EnableRecoveryProgress)
}

// foreignCallbackCallbackInterfaceEnableRecoveryProgressListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceEnableRecoveryProgressListener struct{}

//export matrix_sdk_ffi_cgo_EnableRecoveryProgressListener
func matrix_sdk_ffi_cgo_EnableRecoveryProgressListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceEnableRecoveryProgressListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceEnableRecoveryProgressListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceEnableRecoveryProgressListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceEnableRecoveryProgressListener) InvokeOnUpdate(callback EnableRecoveryProgressListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeEnableRecoveryProgressINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceEnableRecoveryProgressListener struct {
	FfiConverterCallbackInterface[EnableRecoveryProgressListener]
}

var FfiConverterCallbackInterfaceEnableRecoveryProgressListenerINSTANCE = &FfiConverterCallbackInterfaceEnableRecoveryProgressListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[EnableRecoveryProgressListener]{
		handleMap: newConcurrentHandleMap[EnableRecoveryProgressListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceEnableRecoveryProgressListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_enablerecoveryprogresslistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_EnableRecoveryProgressListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceEnableRecoveryProgressListener struct{}

func (FfiDestroyerCallbackInterfaceEnableRecoveryProgressListener) Destroy(value EnableRecoveryProgressListener) {
}

// Declaration and FfiConverters for NotificationSettingsDelegate Callback Interface
type NotificationSettingsDelegate interface {
	SettingsDidChange()
}

// foreignCallbackCallbackInterfaceNotificationSettingsDelegate cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceNotificationSettingsDelegate struct{}

//export matrix_sdk_ffi_cgo_NotificationSettingsDelegate
func matrix_sdk_ffi_cgo_NotificationSettingsDelegate(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceNotificationSettingsDelegateINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceNotificationSettingsDelegateINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceNotificationSettingsDelegate{}.InvokeSettingsDidChange(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceNotificationSettingsDelegate) InvokeSettingsDidChange(callback NotificationSettingsDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.SettingsDidChange()

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceNotificationSettingsDelegate struct {
	FfiConverterCallbackInterface[NotificationSettingsDelegate]
}

var FfiConverterCallbackInterfaceNotificationSettingsDelegateINSTANCE = &FfiConverterCallbackInterfaceNotificationSettingsDelegate{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[NotificationSettingsDelegate]{
		handleMap: newConcurrentHandleMap[NotificationSettingsDelegate](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceNotificationSettingsDelegate) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_notificationsettingsdelegate(C.ForeignCallback(C.matrix_sdk_ffi_cgo_NotificationSettingsDelegate), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceNotificationSettingsDelegate struct{}

func (FfiDestroyerCallbackInterfaceNotificationSettingsDelegate) Destroy(value NotificationSettingsDelegate) {
}

// Declaration and FfiConverters for ProgressWatcher Callback Interface
type ProgressWatcher interface {
	TransmissionProgress(progress TransmissionProgress)
}

// foreignCallbackCallbackInterfaceProgressWatcher cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceProgressWatcher struct{}

//export matrix_sdk_ffi_cgo_ProgressWatcher
func matrix_sdk_ffi_cgo_ProgressWatcher(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceProgressWatcherINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceProgressWatcherINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceProgressWatcher{}.InvokeTransmissionProgress(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceProgressWatcher) InvokeTransmissionProgress(callback ProgressWatcher, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.TransmissionProgress(FfiConverterTypeTransmissionProgressINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceProgressWatcher struct {
	FfiConverterCallbackInterface[ProgressWatcher]
}

var FfiConverterCallbackInterfaceProgressWatcherINSTANCE = &FfiConverterCallbackInterfaceProgressWatcher{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[ProgressWatcher]{
		handleMap: newConcurrentHandleMap[ProgressWatcher](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceProgressWatcher) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_progresswatcher(C.ForeignCallback(C.matrix_sdk_ffi_cgo_ProgressWatcher), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceProgressWatcher struct{}

func (FfiDestroyerCallbackInterfaceProgressWatcher) Destroy(value ProgressWatcher) {
}

// Declaration and FfiConverters for RecoveryStateListener Callback Interface
type RecoveryStateListener interface {
	OnUpdate(status RecoveryState)
}

// foreignCallbackCallbackInterfaceRecoveryStateListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceRecoveryStateListener struct{}

//export matrix_sdk_ffi_cgo_RecoveryStateListener
func matrix_sdk_ffi_cgo_RecoveryStateListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceRecoveryStateListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceRecoveryStateListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceRecoveryStateListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceRecoveryStateListener) InvokeOnUpdate(callback RecoveryStateListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeRecoveryStateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceRecoveryStateListener struct {
	FfiConverterCallbackInterface[RecoveryStateListener]
}

var FfiConverterCallbackInterfaceRecoveryStateListenerINSTANCE = &FfiConverterCallbackInterfaceRecoveryStateListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[RecoveryStateListener]{
		handleMap: newConcurrentHandleMap[RecoveryStateListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceRecoveryStateListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_recoverystatelistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_RecoveryStateListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceRecoveryStateListener struct{}

func (FfiDestroyerCallbackInterfaceRecoveryStateListener) Destroy(value RecoveryStateListener) {
}

// Declaration and FfiConverters for RoomInfoListener Callback Interface
type RoomInfoListener interface {
	Call(roomInfo RoomInfo)
}

// foreignCallbackCallbackInterfaceRoomInfoListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceRoomInfoListener struct{}

//export matrix_sdk_ffi_cgo_RoomInfoListener
func matrix_sdk_ffi_cgo_RoomInfoListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceRoomInfoListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceRoomInfoListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceRoomInfoListener{}.InvokeCall(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceRoomInfoListener) InvokeCall(callback RoomInfoListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.Call(FfiConverterTypeRoomInfoINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceRoomInfoListener struct {
	FfiConverterCallbackInterface[RoomInfoListener]
}

var FfiConverterCallbackInterfaceRoomInfoListenerINSTANCE = &FfiConverterCallbackInterfaceRoomInfoListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[RoomInfoListener]{
		handleMap: newConcurrentHandleMap[RoomInfoListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceRoomInfoListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_roominfolistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_RoomInfoListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceRoomInfoListener struct{}

func (FfiDestroyerCallbackInterfaceRoomInfoListener) Destroy(value RoomInfoListener) {
}

// Declaration and FfiConverters for RoomListEntriesListener Callback Interface
type RoomListEntriesListener interface {
	OnUpdate(roomEntriesUpdate []RoomListEntriesUpdate)
}

// foreignCallbackCallbackInterfaceRoomListEntriesListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceRoomListEntriesListener struct{}

//export matrix_sdk_ffi_cgo_RoomListEntriesListener
func matrix_sdk_ffi_cgo_RoomListEntriesListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceRoomListEntriesListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceRoomListEntriesListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceRoomListEntriesListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceRoomListEntriesListener) InvokeOnUpdate(callback RoomListEntriesListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterSequenceTypeRoomListEntriesUpdateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceRoomListEntriesListener struct {
	FfiConverterCallbackInterface[RoomListEntriesListener]
}

var FfiConverterCallbackInterfaceRoomListEntriesListenerINSTANCE = &FfiConverterCallbackInterfaceRoomListEntriesListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[RoomListEntriesListener]{
		handleMap: newConcurrentHandleMap[RoomListEntriesListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceRoomListEntriesListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_roomlistentrieslistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_RoomListEntriesListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceRoomListEntriesListener struct{}

func (FfiDestroyerCallbackInterfaceRoomListEntriesListener) Destroy(value RoomListEntriesListener) {
}

// Declaration and FfiConverters for RoomListLoadingStateListener Callback Interface
type RoomListLoadingStateListener interface {
	OnUpdate(state RoomListLoadingState)
}

// foreignCallbackCallbackInterfaceRoomListLoadingStateListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceRoomListLoadingStateListener struct{}

//export matrix_sdk_ffi_cgo_RoomListLoadingStateListener
func matrix_sdk_ffi_cgo_RoomListLoadingStateListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceRoomListLoadingStateListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceRoomListLoadingStateListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceRoomListLoadingStateListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceRoomListLoadingStateListener) InvokeOnUpdate(callback RoomListLoadingStateListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeRoomListLoadingStateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceRoomListLoadingStateListener struct {
	FfiConverterCallbackInterface[RoomListLoadingStateListener]
}

var FfiConverterCallbackInterfaceRoomListLoadingStateListenerINSTANCE = &FfiConverterCallbackInterfaceRoomListLoadingStateListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[RoomListLoadingStateListener]{
		handleMap: newConcurrentHandleMap[RoomListLoadingStateListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceRoomListLoadingStateListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_roomlistloadingstatelistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_RoomListLoadingStateListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceRoomListLoadingStateListener struct{}

func (FfiDestroyerCallbackInterfaceRoomListLoadingStateListener) Destroy(value RoomListLoadingStateListener) {
}

// Declaration and FfiConverters for RoomListServiceStateListener Callback Interface
type RoomListServiceStateListener interface {
	OnUpdate(state RoomListServiceState)
}

// foreignCallbackCallbackInterfaceRoomListServiceStateListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceRoomListServiceStateListener struct{}

//export matrix_sdk_ffi_cgo_RoomListServiceStateListener
func matrix_sdk_ffi_cgo_RoomListServiceStateListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceRoomListServiceStateListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceRoomListServiceStateListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceRoomListServiceStateListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceRoomListServiceStateListener) InvokeOnUpdate(callback RoomListServiceStateListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeRoomListServiceStateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceRoomListServiceStateListener struct {
	FfiConverterCallbackInterface[RoomListServiceStateListener]
}

var FfiConverterCallbackInterfaceRoomListServiceStateListenerINSTANCE = &FfiConverterCallbackInterfaceRoomListServiceStateListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[RoomListServiceStateListener]{
		handleMap: newConcurrentHandleMap[RoomListServiceStateListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceRoomListServiceStateListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_roomlistservicestatelistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_RoomListServiceStateListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceRoomListServiceStateListener struct{}

func (FfiDestroyerCallbackInterfaceRoomListServiceStateListener) Destroy(value RoomListServiceStateListener) {
}

// Declaration and FfiConverters for RoomListServiceSyncIndicatorListener Callback Interface
type RoomListServiceSyncIndicatorListener interface {
	OnUpdate(syncIndicator RoomListServiceSyncIndicator)
}

// foreignCallbackCallbackInterfaceRoomListServiceSyncIndicatorListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceRoomListServiceSyncIndicatorListener struct{}

//export matrix_sdk_ffi_cgo_RoomListServiceSyncIndicatorListener
func matrix_sdk_ffi_cgo_RoomListServiceSyncIndicatorListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceRoomListServiceSyncIndicatorListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceRoomListServiceSyncIndicatorListener) InvokeOnUpdate(callback RoomListServiceSyncIndicatorListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeRoomListServiceSyncIndicatorINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListener struct {
	FfiConverterCallbackInterface[RoomListServiceSyncIndicatorListener]
}

var FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListenerINSTANCE = &FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[RoomListServiceSyncIndicatorListener]{
		handleMap: newConcurrentHandleMap[RoomListServiceSyncIndicatorListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceRoomListServiceSyncIndicatorListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_roomlistservicesyncindicatorlistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_RoomListServiceSyncIndicatorListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceRoomListServiceSyncIndicatorListener struct{}

func (FfiDestroyerCallbackInterfaceRoomListServiceSyncIndicatorListener) Destroy(value RoomListServiceSyncIndicatorListener) {
}

// Declaration and FfiConverters for SessionVerificationControllerDelegate Callback Interface
type SessionVerificationControllerDelegate interface {
	DidAcceptVerificationRequest()
	DidStartSasVerification()
	DidReceiveVerificationData(data SessionVerificationData)
	DidFail()
	DidCancel()
	DidFinish()
}

// foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate struct{}

//export matrix_sdk_ffi_cgo_SessionVerificationControllerDelegate
func matrix_sdk_ffi_cgo_SessionVerificationControllerDelegate(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceSessionVerificationControllerDelegateINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceSessionVerificationControllerDelegateINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate{}.InvokeDidAcceptVerificationRequest(cb, args, outBuf)
		return C.int32_t(result)
	case 2:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate{}.InvokeDidStartSasVerification(cb, args, outBuf)
		return C.int32_t(result)
	case 3:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate{}.InvokeDidReceiveVerificationData(cb, args, outBuf)
		return C.int32_t(result)
	case 4:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate{}.InvokeDidFail(cb, args, outBuf)
		return C.int32_t(result)
	case 5:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate{}.InvokeDidCancel(cb, args, outBuf)
		return C.int32_t(result)
	case 6:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate{}.InvokeDidFinish(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate) InvokeDidAcceptVerificationRequest(callback SessionVerificationControllerDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.DidAcceptVerificationRequest()

	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate) InvokeDidStartSasVerification(callback SessionVerificationControllerDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.DidStartSasVerification()

	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate) InvokeDidReceiveVerificationData(callback SessionVerificationControllerDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.DidReceiveVerificationData(FfiConverterTypeSessionVerificationDataINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate) InvokeDidFail(callback SessionVerificationControllerDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.DidFail()

	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate) InvokeDidCancel(callback SessionVerificationControllerDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.DidCancel()

	return uniffiCallbackResultSuccess
}
func (foreignCallbackCallbackInterfaceSessionVerificationControllerDelegate) InvokeDidFinish(callback SessionVerificationControllerDelegate, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	callback.DidFinish()

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceSessionVerificationControllerDelegate struct {
	FfiConverterCallbackInterface[SessionVerificationControllerDelegate]
}

var FfiConverterCallbackInterfaceSessionVerificationControllerDelegateINSTANCE = &FfiConverterCallbackInterfaceSessionVerificationControllerDelegate{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[SessionVerificationControllerDelegate]{
		handleMap: newConcurrentHandleMap[SessionVerificationControllerDelegate](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceSessionVerificationControllerDelegate) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_sessionverificationcontrollerdelegate(C.ForeignCallback(C.matrix_sdk_ffi_cgo_SessionVerificationControllerDelegate), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceSessionVerificationControllerDelegate struct{}

func (FfiDestroyerCallbackInterfaceSessionVerificationControllerDelegate) Destroy(value SessionVerificationControllerDelegate) {
}

// Declaration and FfiConverters for SyncServiceStateObserver Callback Interface
type SyncServiceStateObserver interface {
	OnUpdate(state SyncServiceState)
}

// foreignCallbackCallbackInterfaceSyncServiceStateObserver cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceSyncServiceStateObserver struct{}

//export matrix_sdk_ffi_cgo_SyncServiceStateObserver
func matrix_sdk_ffi_cgo_SyncServiceStateObserver(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceSyncServiceStateObserverINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceSyncServiceStateObserverINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceSyncServiceStateObserver{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceSyncServiceStateObserver) InvokeOnUpdate(callback SyncServiceStateObserver, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterTypeSyncServiceStateINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceSyncServiceStateObserver struct {
	FfiConverterCallbackInterface[SyncServiceStateObserver]
}

var FfiConverterCallbackInterfaceSyncServiceStateObserverINSTANCE = &FfiConverterCallbackInterfaceSyncServiceStateObserver{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[SyncServiceStateObserver]{
		handleMap: newConcurrentHandleMap[SyncServiceStateObserver](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceSyncServiceStateObserver) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_syncservicestateobserver(C.ForeignCallback(C.matrix_sdk_ffi_cgo_SyncServiceStateObserver), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceSyncServiceStateObserver struct{}

func (FfiDestroyerCallbackInterfaceSyncServiceStateObserver) Destroy(value SyncServiceStateObserver) {
}

// Declaration and FfiConverters for TimelineListener Callback Interface
type TimelineListener interface {
	OnUpdate(diff []*TimelineDiff)
}

// foreignCallbackCallbackInterfaceTimelineListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceTimelineListener struct{}

//export matrix_sdk_ffi_cgo_TimelineListener
func matrix_sdk_ffi_cgo_TimelineListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceTimelineListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceTimelineListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceTimelineListener{}.InvokeOnUpdate(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceTimelineListener) InvokeOnUpdate(callback TimelineListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.OnUpdate(FfiConverterSequenceTimelineDiffINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceTimelineListener struct {
	FfiConverterCallbackInterface[TimelineListener]
}

var FfiConverterCallbackInterfaceTimelineListenerINSTANCE = &FfiConverterCallbackInterfaceTimelineListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[TimelineListener]{
		handleMap: newConcurrentHandleMap[TimelineListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceTimelineListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_timelinelistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_TimelineListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceTimelineListener struct{}

func (FfiDestroyerCallbackInterfaceTimelineListener) Destroy(value TimelineListener) {
}

// Declaration and FfiConverters for TypingNotificationsListener Callback Interface
type TypingNotificationsListener interface {
	Call(typingUserIds []string)
}

// foreignCallbackCallbackInterfaceTypingNotificationsListener cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceTypingNotificationsListener struct{}

//export matrix_sdk_ffi_cgo_TypingNotificationsListener
func matrix_sdk_ffi_cgo_TypingNotificationsListener(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceTypingNotificationsListenerINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceTypingNotificationsListenerINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceTypingNotificationsListener{}.InvokeCall(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceTypingNotificationsListener) InvokeCall(callback TypingNotificationsListener, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	callback.Call(FfiConverterSequenceStringINSTANCE.Read(reader))

	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceTypingNotificationsListener struct {
	FfiConverterCallbackInterface[TypingNotificationsListener]
}

var FfiConverterCallbackInterfaceTypingNotificationsListenerINSTANCE = &FfiConverterCallbackInterfaceTypingNotificationsListener{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[TypingNotificationsListener]{
		handleMap: newConcurrentHandleMap[TypingNotificationsListener](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceTypingNotificationsListener) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_typingnotificationslistener(C.ForeignCallback(C.matrix_sdk_ffi_cgo_TypingNotificationsListener), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceTypingNotificationsListener struct{}

func (FfiDestroyerCallbackInterfaceTypingNotificationsListener) Destroy(value TypingNotificationsListener) {
}

// Declaration and FfiConverters for WidgetCapabilitiesProvider Callback Interface
type WidgetCapabilitiesProvider interface {
	AcquireCapabilities(capabilities WidgetCapabilities) WidgetCapabilities
}

// foreignCallbackCallbackInterfaceWidgetCapabilitiesProvider cannot be callable be a compiled function at a same time
type foreignCallbackCallbackInterfaceWidgetCapabilitiesProvider struct{}

//export matrix_sdk_ffi_cgo_WidgetCapabilitiesProvider
func matrix_sdk_ffi_cgo_WidgetCapabilitiesProvider(handle C.uint64_t, method C.int32_t, argsPtr *C.uint8_t, argsLen C.int32_t, outBuf *C.RustBuffer) C.int32_t {
	cb := FfiConverterCallbackInterfaceWidgetCapabilitiesProviderINSTANCE.Lift(uint64(handle))
	switch method {
	case 0:
		// 0 means Rust is done with the callback, and the callback
		// can be dropped by the foreign language.
		*outBuf = FfiConverterCallbackInterfaceWidgetCapabilitiesProviderINSTANCE.drop(uint64(handle))
		// See docs of ForeignCallback in `uniffi/src/ffi/foreigncallbacks.rs`
		return C.int32_t(uniffiIdxCallbackFree)

	case 1:
		var result uniffiCallbackResult
		args := unsafe.Slice((*byte)(argsPtr), argsLen)
		result = foreignCallbackCallbackInterfaceWidgetCapabilitiesProvider{}.InvokeAcquireCapabilities(cb, args, outBuf)
		return C.int32_t(result)

	default:
		// This should never happen, because an out of bounds method index won't
		// ever be used. Once we can catch errors, we should return an InternalException.
		// https://github.com/mozilla/uniffi-rs/issues/351
		return C.int32_t(uniffiCallbackUnexpectedResultError)
	}
}

func (foreignCallbackCallbackInterfaceWidgetCapabilitiesProvider) InvokeAcquireCapabilities(callback WidgetCapabilitiesProvider, args []byte, outBuf *C.RustBuffer) uniffiCallbackResult {
	reader := bytes.NewReader(args)
	result := callback.AcquireCapabilities(FfiConverterTypeWidgetCapabilitiesINSTANCE.Read(reader))

	*outBuf = LowerIntoRustBuffer[WidgetCapabilities](FfiConverterTypeWidgetCapabilitiesINSTANCE, result)
	return uniffiCallbackResultSuccess
}

type FfiConverterCallbackInterfaceWidgetCapabilitiesProvider struct {
	FfiConverterCallbackInterface[WidgetCapabilitiesProvider]
}

var FfiConverterCallbackInterfaceWidgetCapabilitiesProviderINSTANCE = &FfiConverterCallbackInterfaceWidgetCapabilitiesProvider{
	FfiConverterCallbackInterface: FfiConverterCallbackInterface[WidgetCapabilitiesProvider]{
		handleMap: newConcurrentHandleMap[WidgetCapabilitiesProvider](),
	},
}

// This is a static function because only 1 instance is supported for registering
func (c *FfiConverterCallbackInterfaceWidgetCapabilitiesProvider) register() {
	rustCall(func(status *C.RustCallStatus) int32 {
		C.uniffi_matrix_sdk_ffi_fn_init_callback_widgetcapabilitiesprovider(C.ForeignCallback(C.matrix_sdk_ffi_cgo_WidgetCapabilitiesProvider), status)
		return 0
	})
}

type FfiDestroyerCallbackInterfaceWidgetCapabilitiesProvider struct{}

func (FfiDestroyerCallbackInterfaceWidgetCapabilitiesProvider) Destroy(value WidgetCapabilitiesProvider) {
}

type FfiConverterOptionalUint8 struct{}

var FfiConverterOptionalUint8INSTANCE = FfiConverterOptionalUint8{}

func (c FfiConverterOptionalUint8) Lift(rb RustBufferI) *uint8 {
	return LiftFromRustBuffer[*uint8](c, rb)
}

func (_ FfiConverterOptionalUint8) Read(reader io.Reader) *uint8 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterUint8INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalUint8) Lower(value *uint8) RustBuffer {
	return LowerIntoRustBuffer[*uint8](c, value)
}

func (_ FfiConverterOptionalUint8) Write(writer io.Writer, value *uint8) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterUint8INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalUint8 struct{}

func (_ FfiDestroyerOptionalUint8) Destroy(value *uint8) {
	if value != nil {
		FfiDestroyerUint8{}.Destroy(*value)
	}
}

type FfiConverterOptionalUint32 struct{}

var FfiConverterOptionalUint32INSTANCE = FfiConverterOptionalUint32{}

func (c FfiConverterOptionalUint32) Lift(rb RustBufferI) *uint32 {
	return LiftFromRustBuffer[*uint32](c, rb)
}

func (_ FfiConverterOptionalUint32) Read(reader io.Reader) *uint32 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterUint32INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalUint32) Lower(value *uint32) RustBuffer {
	return LowerIntoRustBuffer[*uint32](c, value)
}

func (_ FfiConverterOptionalUint32) Write(writer io.Writer, value *uint32) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterUint32INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalUint32 struct{}

func (_ FfiDestroyerOptionalUint32) Destroy(value *uint32) {
	if value != nil {
		FfiDestroyerUint32{}.Destroy(*value)
	}
}

type FfiConverterOptionalInt32 struct{}

var FfiConverterOptionalInt32INSTANCE = FfiConverterOptionalInt32{}

func (c FfiConverterOptionalInt32) Lift(rb RustBufferI) *int32 {
	return LiftFromRustBuffer[*int32](c, rb)
}

func (_ FfiConverterOptionalInt32) Read(reader io.Reader) *int32 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterInt32INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalInt32) Lower(value *int32) RustBuffer {
	return LowerIntoRustBuffer[*int32](c, value)
}

func (_ FfiConverterOptionalInt32) Write(writer io.Writer, value *int32) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterInt32INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalInt32 struct{}

func (_ FfiDestroyerOptionalInt32) Destroy(value *int32) {
	if value != nil {
		FfiDestroyerInt32{}.Destroy(*value)
	}
}

type FfiConverterOptionalUint64 struct{}

var FfiConverterOptionalUint64INSTANCE = FfiConverterOptionalUint64{}

func (c FfiConverterOptionalUint64) Lift(rb RustBufferI) *uint64 {
	return LiftFromRustBuffer[*uint64](c, rb)
}

func (_ FfiConverterOptionalUint64) Read(reader io.Reader) *uint64 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterUint64INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalUint64) Lower(value *uint64) RustBuffer {
	return LowerIntoRustBuffer[*uint64](c, value)
}

func (_ FfiConverterOptionalUint64) Write(writer io.Writer, value *uint64) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterUint64INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalUint64 struct{}

func (_ FfiDestroyerOptionalUint64) Destroy(value *uint64) {
	if value != nil {
		FfiDestroyerUint64{}.Destroy(*value)
	}
}

type FfiConverterOptionalFloat64 struct{}

var FfiConverterOptionalFloat64INSTANCE = FfiConverterOptionalFloat64{}

func (c FfiConverterOptionalFloat64) Lift(rb RustBufferI) *float64 {
	return LiftFromRustBuffer[*float64](c, rb)
}

func (_ FfiConverterOptionalFloat64) Read(reader io.Reader) *float64 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterFloat64INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalFloat64) Lower(value *float64) RustBuffer {
	return LowerIntoRustBuffer[*float64](c, value)
}

func (_ FfiConverterOptionalFloat64) Write(writer io.Writer, value *float64) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterFloat64INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalFloat64 struct{}

func (_ FfiDestroyerOptionalFloat64) Destroy(value *float64) {
	if value != nil {
		FfiDestroyerFloat64{}.Destroy(*value)
	}
}

type FfiConverterOptionalBool struct{}

var FfiConverterOptionalBoolINSTANCE = FfiConverterOptionalBool{}

func (c FfiConverterOptionalBool) Lift(rb RustBufferI) *bool {
	return LiftFromRustBuffer[*bool](c, rb)
}

func (_ FfiConverterOptionalBool) Read(reader io.Reader) *bool {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterBoolINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalBool) Lower(value *bool) RustBuffer {
	return LowerIntoRustBuffer[*bool](c, value)
}

func (_ FfiConverterOptionalBool) Write(writer io.Writer, value *bool) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterBoolINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalBool struct{}

func (_ FfiDestroyerOptionalBool) Destroy(value *bool) {
	if value != nil {
		FfiDestroyerBool{}.Destroy(*value)
	}
}

type FfiConverterOptionalString struct{}

var FfiConverterOptionalStringINSTANCE = FfiConverterOptionalString{}

func (c FfiConverterOptionalString) Lift(rb RustBufferI) *string {
	return LiftFromRustBuffer[*string](c, rb)
}

func (_ FfiConverterOptionalString) Read(reader io.Reader) *string {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterStringINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalString) Lower(value *string) RustBuffer {
	return LowerIntoRustBuffer[*string](c, value)
}

func (_ FfiConverterOptionalString) Write(writer io.Writer, value *string) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalString struct{}

func (_ FfiDestroyerOptionalString) Destroy(value *string) {
	if value != nil {
		FfiDestroyerString{}.Destroy(*value)
	}
}

type FfiConverterOptionalDuration struct{}

var FfiConverterOptionalDurationINSTANCE = FfiConverterOptionalDuration{}

func (c FfiConverterOptionalDuration) Lift(rb RustBufferI) *time.Duration {
	return LiftFromRustBuffer[*time.Duration](c, rb)
}

func (_ FfiConverterOptionalDuration) Read(reader io.Reader) *time.Duration {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterDurationINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalDuration) Lower(value *time.Duration) RustBuffer {
	return LowerIntoRustBuffer[*time.Duration](c, value)
}

func (_ FfiConverterOptionalDuration) Write(writer io.Writer, value *time.Duration) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterDurationINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalDuration struct{}

func (_ FfiDestroyerOptionalDuration) Destroy(value *time.Duration) {
	if value != nil {
		FfiDestroyerDuration{}.Destroy(*value)
	}
}

type FfiConverterOptionalEventTimelineItem struct{}

var FfiConverterOptionalEventTimelineItemINSTANCE = FfiConverterOptionalEventTimelineItem{}

func (c FfiConverterOptionalEventTimelineItem) Lift(rb RustBufferI) **EventTimelineItem {
	return LiftFromRustBuffer[**EventTimelineItem](c, rb)
}

func (_ FfiConverterOptionalEventTimelineItem) Read(reader io.Reader) **EventTimelineItem {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterEventTimelineItemINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalEventTimelineItem) Lower(value **EventTimelineItem) RustBuffer {
	return LowerIntoRustBuffer[**EventTimelineItem](c, value)
}

func (_ FfiConverterOptionalEventTimelineItem) Write(writer io.Writer, value **EventTimelineItem) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterEventTimelineItemINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalEventTimelineItem struct{}

func (_ FfiDestroyerOptionalEventTimelineItem) Destroy(value **EventTimelineItem) {
	if value != nil {
		FfiDestroyerEventTimelineItem{}.Destroy(*value)
	}
}

type FfiConverterOptionalHomeserverLoginDetails struct{}

var FfiConverterOptionalHomeserverLoginDetailsINSTANCE = FfiConverterOptionalHomeserverLoginDetails{}

func (c FfiConverterOptionalHomeserverLoginDetails) Lift(rb RustBufferI) **HomeserverLoginDetails {
	return LiftFromRustBuffer[**HomeserverLoginDetails](c, rb)
}

func (_ FfiConverterOptionalHomeserverLoginDetails) Read(reader io.Reader) **HomeserverLoginDetails {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterHomeserverLoginDetailsINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalHomeserverLoginDetails) Lower(value **HomeserverLoginDetails) RustBuffer {
	return LowerIntoRustBuffer[**HomeserverLoginDetails](c, value)
}

func (_ FfiConverterOptionalHomeserverLoginDetails) Write(writer io.Writer, value **HomeserverLoginDetails) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterHomeserverLoginDetailsINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalHomeserverLoginDetails struct{}

func (_ FfiDestroyerOptionalHomeserverLoginDetails) Destroy(value **HomeserverLoginDetails) {
	if value != nil {
		FfiDestroyerHomeserverLoginDetails{}.Destroy(*value)
	}
}

type FfiConverterOptionalMediaSource struct{}

var FfiConverterOptionalMediaSourceINSTANCE = FfiConverterOptionalMediaSource{}

func (c FfiConverterOptionalMediaSource) Lift(rb RustBufferI) **MediaSource {
	return LiftFromRustBuffer[**MediaSource](c, rb)
}

func (_ FfiConverterOptionalMediaSource) Read(reader io.Reader) **MediaSource {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterMediaSourceINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalMediaSource) Lower(value **MediaSource) RustBuffer {
	return LowerIntoRustBuffer[**MediaSource](c, value)
}

func (_ FfiConverterOptionalMediaSource) Write(writer io.Writer, value **MediaSource) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterMediaSourceINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalMediaSource struct{}

func (_ FfiDestroyerOptionalMediaSource) Destroy(value **MediaSource) {
	if value != nil {
		FfiDestroyerMediaSource{}.Destroy(*value)
	}
}

type FfiConverterOptionalMessage struct{}

var FfiConverterOptionalMessageINSTANCE = FfiConverterOptionalMessage{}

func (c FfiConverterOptionalMessage) Lift(rb RustBufferI) **Message {
	return LiftFromRustBuffer[**Message](c, rb)
}

func (_ FfiConverterOptionalMessage) Read(reader io.Reader) **Message {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterMessageINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalMessage) Lower(value **Message) RustBuffer {
	return LowerIntoRustBuffer[**Message](c, value)
}

func (_ FfiConverterOptionalMessage) Write(writer io.Writer, value **Message) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterMessageINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalMessage struct{}

func (_ FfiDestroyerOptionalMessage) Destroy(value **Message) {
	if value != nil {
		FfiDestroyerMessage{}.Destroy(*value)
	}
}

type FfiConverterOptionalRoom struct{}

var FfiConverterOptionalRoomINSTANCE = FfiConverterOptionalRoom{}

func (c FfiConverterOptionalRoom) Lift(rb RustBufferI) **Room {
	return LiftFromRustBuffer[**Room](c, rb)
}

func (_ FfiConverterOptionalRoom) Read(reader io.Reader) **Room {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterRoomINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalRoom) Lower(value **Room) RustBuffer {
	return LowerIntoRustBuffer[**Room](c, value)
}

func (_ FfiConverterOptionalRoom) Write(writer io.Writer, value **Room) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterRoomINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalRoom struct{}

func (_ FfiDestroyerOptionalRoom) Destroy(value **Room) {
	if value != nil {
		FfiDestroyerRoom{}.Destroy(*value)
	}
}

type FfiConverterOptionalRoomMember struct{}

var FfiConverterOptionalRoomMemberINSTANCE = FfiConverterOptionalRoomMember{}

func (c FfiConverterOptionalRoomMember) Lift(rb RustBufferI) **RoomMember {
	return LiftFromRustBuffer[**RoomMember](c, rb)
}

func (_ FfiConverterOptionalRoomMember) Read(reader io.Reader) **RoomMember {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterRoomMemberINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalRoomMember) Lower(value **RoomMember) RustBuffer {
	return LowerIntoRustBuffer[**RoomMember](c, value)
}

func (_ FfiConverterOptionalRoomMember) Write(writer io.Writer, value **RoomMember) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterRoomMemberINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalRoomMember struct{}

func (_ FfiDestroyerOptionalRoomMember) Destroy(value **RoomMember) {
	if value != nil {
		FfiDestroyerRoomMember{}.Destroy(*value)
	}
}

type FfiConverterOptionalTaskHandle struct{}

var FfiConverterOptionalTaskHandleINSTANCE = FfiConverterOptionalTaskHandle{}

func (c FfiConverterOptionalTaskHandle) Lift(rb RustBufferI) **TaskHandle {
	return LiftFromRustBuffer[**TaskHandle](c, rb)
}

func (_ FfiConverterOptionalTaskHandle) Read(reader io.Reader) **TaskHandle {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTaskHandleINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTaskHandle) Lower(value **TaskHandle) RustBuffer {
	return LowerIntoRustBuffer[**TaskHandle](c, value)
}

func (_ FfiConverterOptionalTaskHandle) Write(writer io.Writer, value **TaskHandle) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTaskHandleINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTaskHandle struct{}

func (_ FfiDestroyerOptionalTaskHandle) Destroy(value **TaskHandle) {
	if value != nil {
		FfiDestroyerTaskHandle{}.Destroy(*value)
	}
}

type FfiConverterOptionalTimelineEventTypeFilter struct{}

var FfiConverterOptionalTimelineEventTypeFilterINSTANCE = FfiConverterOptionalTimelineEventTypeFilter{}

func (c FfiConverterOptionalTimelineEventTypeFilter) Lift(rb RustBufferI) **TimelineEventTypeFilter {
	return LiftFromRustBuffer[**TimelineEventTypeFilter](c, rb)
}

func (_ FfiConverterOptionalTimelineEventTypeFilter) Read(reader io.Reader) **TimelineEventTypeFilter {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTimelineEventTypeFilterINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTimelineEventTypeFilter) Lower(value **TimelineEventTypeFilter) RustBuffer {
	return LowerIntoRustBuffer[**TimelineEventTypeFilter](c, value)
}

func (_ FfiConverterOptionalTimelineEventTypeFilter) Write(writer io.Writer, value **TimelineEventTypeFilter) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTimelineEventTypeFilterINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTimelineEventTypeFilter struct{}

func (_ FfiDestroyerOptionalTimelineEventTypeFilter) Destroy(value **TimelineEventTypeFilter) {
	if value != nil {
		FfiDestroyerTimelineEventTypeFilter{}.Destroy(*value)
	}
}

type FfiConverterOptionalTimelineItem struct{}

var FfiConverterOptionalTimelineItemINSTANCE = FfiConverterOptionalTimelineItem{}

func (c FfiConverterOptionalTimelineItem) Lift(rb RustBufferI) **TimelineItem {
	return LiftFromRustBuffer[**TimelineItem](c, rb)
}

func (_ FfiConverterOptionalTimelineItem) Read(reader io.Reader) **TimelineItem {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTimelineItemINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTimelineItem) Lower(value **TimelineItem) RustBuffer {
	return LowerIntoRustBuffer[**TimelineItem](c, value)
}

func (_ FfiConverterOptionalTimelineItem) Write(writer io.Writer, value **TimelineItem) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTimelineItemINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTimelineItem struct{}

func (_ FfiDestroyerOptionalTimelineItem) Destroy(value **TimelineItem) {
	if value != nil {
		FfiDestroyerTimelineItem{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeAudioInfo struct{}

var FfiConverterOptionalTypeAudioInfoINSTANCE = FfiConverterOptionalTypeAudioInfo{}

func (c FfiConverterOptionalTypeAudioInfo) Lift(rb RustBufferI) *AudioInfo {
	return LiftFromRustBuffer[*AudioInfo](c, rb)
}

func (_ FfiConverterOptionalTypeAudioInfo) Read(reader io.Reader) *AudioInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeAudioInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeAudioInfo) Lower(value *AudioInfo) RustBuffer {
	return LowerIntoRustBuffer[*AudioInfo](c, value)
}

func (_ FfiConverterOptionalTypeAudioInfo) Write(writer io.Writer, value *AudioInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeAudioInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeAudioInfo struct{}

func (_ FfiDestroyerOptionalTypeAudioInfo) Destroy(value *AudioInfo) {
	if value != nil {
		FfiDestroyerTypeAudioInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeFileInfo struct{}

var FfiConverterOptionalTypeFileInfoINSTANCE = FfiConverterOptionalTypeFileInfo{}

func (c FfiConverterOptionalTypeFileInfo) Lift(rb RustBufferI) *FileInfo {
	return LiftFromRustBuffer[*FileInfo](c, rb)
}

func (_ FfiConverterOptionalTypeFileInfo) Read(reader io.Reader) *FileInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeFileInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeFileInfo) Lower(value *FileInfo) RustBuffer {
	return LowerIntoRustBuffer[*FileInfo](c, value)
}

func (_ FfiConverterOptionalTypeFileInfo) Write(writer io.Writer, value *FileInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeFileInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeFileInfo struct{}

func (_ FfiDestroyerOptionalTypeFileInfo) Destroy(value *FileInfo) {
	if value != nil {
		FfiDestroyerTypeFileInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeFormattedBody struct{}

var FfiConverterOptionalTypeFormattedBodyINSTANCE = FfiConverterOptionalTypeFormattedBody{}

func (c FfiConverterOptionalTypeFormattedBody) Lift(rb RustBufferI) *FormattedBody {
	return LiftFromRustBuffer[*FormattedBody](c, rb)
}

func (_ FfiConverterOptionalTypeFormattedBody) Read(reader io.Reader) *FormattedBody {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeFormattedBodyINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeFormattedBody) Lower(value *FormattedBody) RustBuffer {
	return LowerIntoRustBuffer[*FormattedBody](c, value)
}

func (_ FfiConverterOptionalTypeFormattedBody) Write(writer io.Writer, value *FormattedBody) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeFormattedBodyINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeFormattedBody struct{}

func (_ FfiDestroyerOptionalTypeFormattedBody) Destroy(value *FormattedBody) {
	if value != nil {
		FfiDestroyerTypeFormattedBody{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeImageInfo struct{}

var FfiConverterOptionalTypeImageInfoINSTANCE = FfiConverterOptionalTypeImageInfo{}

func (c FfiConverterOptionalTypeImageInfo) Lift(rb RustBufferI) *ImageInfo {
	return LiftFromRustBuffer[*ImageInfo](c, rb)
}

func (_ FfiConverterOptionalTypeImageInfo) Read(reader io.Reader) *ImageInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeImageInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeImageInfo) Lower(value *ImageInfo) RustBuffer {
	return LowerIntoRustBuffer[*ImageInfo](c, value)
}

func (_ FfiConverterOptionalTypeImageInfo) Write(writer io.Writer, value *ImageInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeImageInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeImageInfo struct{}

func (_ FfiDestroyerOptionalTypeImageInfo) Destroy(value *ImageInfo) {
	if value != nil {
		FfiDestroyerTypeImageInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeInReplyToDetails struct{}

var FfiConverterOptionalTypeInReplyToDetailsINSTANCE = FfiConverterOptionalTypeInReplyToDetails{}

func (c FfiConverterOptionalTypeInReplyToDetails) Lift(rb RustBufferI) *InReplyToDetails {
	return LiftFromRustBuffer[*InReplyToDetails](c, rb)
}

func (_ FfiConverterOptionalTypeInReplyToDetails) Read(reader io.Reader) *InReplyToDetails {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeInReplyToDetailsINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeInReplyToDetails) Lower(value *InReplyToDetails) RustBuffer {
	return LowerIntoRustBuffer[*InReplyToDetails](c, value)
}

func (_ FfiConverterOptionalTypeInReplyToDetails) Write(writer io.Writer, value *InReplyToDetails) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeInReplyToDetailsINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeInReplyToDetails struct{}

func (_ FfiDestroyerOptionalTypeInReplyToDetails) Destroy(value *InReplyToDetails) {
	if value != nil {
		FfiDestroyerTypeInReplyToDetails{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeInsertData struct{}

var FfiConverterOptionalTypeInsertDataINSTANCE = FfiConverterOptionalTypeInsertData{}

func (c FfiConverterOptionalTypeInsertData) Lift(rb RustBufferI) *InsertData {
	return LiftFromRustBuffer[*InsertData](c, rb)
}

func (_ FfiConverterOptionalTypeInsertData) Read(reader io.Reader) *InsertData {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeInsertDataINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeInsertData) Lower(value *InsertData) RustBuffer {
	return LowerIntoRustBuffer[*InsertData](c, value)
}

func (_ FfiConverterOptionalTypeInsertData) Write(writer io.Writer, value *InsertData) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeInsertDataINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeInsertData struct{}

func (_ FfiDestroyerOptionalTypeInsertData) Destroy(value *InsertData) {
	if value != nil {
		FfiDestroyerTypeInsertData{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeNotificationItem struct{}

var FfiConverterOptionalTypeNotificationItemINSTANCE = FfiConverterOptionalTypeNotificationItem{}

func (c FfiConverterOptionalTypeNotificationItem) Lift(rb RustBufferI) *NotificationItem {
	return LiftFromRustBuffer[*NotificationItem](c, rb)
}

func (_ FfiConverterOptionalTypeNotificationItem) Read(reader io.Reader) *NotificationItem {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeNotificationItemINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeNotificationItem) Lower(value *NotificationItem) RustBuffer {
	return LowerIntoRustBuffer[*NotificationItem](c, value)
}

func (_ FfiConverterOptionalTypeNotificationItem) Write(writer io.Writer, value *NotificationItem) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeNotificationItemINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeNotificationItem struct{}

func (_ FfiDestroyerOptionalTypeNotificationItem) Destroy(value *NotificationItem) {
	if value != nil {
		FfiDestroyerTypeNotificationItem{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeNotificationPowerLevels struct{}

var FfiConverterOptionalTypeNotificationPowerLevelsINSTANCE = FfiConverterOptionalTypeNotificationPowerLevels{}

func (c FfiConverterOptionalTypeNotificationPowerLevels) Lift(rb RustBufferI) *NotificationPowerLevels {
	return LiftFromRustBuffer[*NotificationPowerLevels](c, rb)
}

func (_ FfiConverterOptionalTypeNotificationPowerLevels) Read(reader io.Reader) *NotificationPowerLevels {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeNotificationPowerLevelsINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeNotificationPowerLevels) Lower(value *NotificationPowerLevels) RustBuffer {
	return LowerIntoRustBuffer[*NotificationPowerLevels](c, value)
}

func (_ FfiConverterOptionalTypeNotificationPowerLevels) Write(writer io.Writer, value *NotificationPowerLevels) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeNotificationPowerLevelsINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeNotificationPowerLevels struct{}

func (_ FfiDestroyerOptionalTypeNotificationPowerLevels) Destroy(value *NotificationPowerLevels) {
	if value != nil {
		FfiDestroyerTypeNotificationPowerLevels{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeOidcConfiguration struct{}

var FfiConverterOptionalTypeOidcConfigurationINSTANCE = FfiConverterOptionalTypeOidcConfiguration{}

func (c FfiConverterOptionalTypeOidcConfiguration) Lift(rb RustBufferI) *OidcConfiguration {
	return LiftFromRustBuffer[*OidcConfiguration](c, rb)
}

func (_ FfiConverterOptionalTypeOidcConfiguration) Read(reader io.Reader) *OidcConfiguration {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeOidcConfigurationINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeOidcConfiguration) Lower(value *OidcConfiguration) RustBuffer {
	return LowerIntoRustBuffer[*OidcConfiguration](c, value)
}

func (_ FfiConverterOptionalTypeOidcConfiguration) Write(writer io.Writer, value *OidcConfiguration) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeOidcConfigurationINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeOidcConfiguration struct{}

func (_ FfiDestroyerOptionalTypeOidcConfiguration) Destroy(value *OidcConfiguration) {
	if value != nil {
		FfiDestroyerTypeOidcConfiguration{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePowerLevels struct{}

var FfiConverterOptionalTypePowerLevelsINSTANCE = FfiConverterOptionalTypePowerLevels{}

func (c FfiConverterOptionalTypePowerLevels) Lift(rb RustBufferI) *PowerLevels {
	return LiftFromRustBuffer[*PowerLevels](c, rb)
}

func (_ FfiConverterOptionalTypePowerLevels) Read(reader io.Reader) *PowerLevels {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePowerLevelsINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePowerLevels) Lower(value *PowerLevels) RustBuffer {
	return LowerIntoRustBuffer[*PowerLevels](c, value)
}

func (_ FfiConverterOptionalTypePowerLevels) Write(writer io.Writer, value *PowerLevels) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePowerLevelsINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePowerLevels struct{}

func (_ FfiDestroyerOptionalTypePowerLevels) Destroy(value *PowerLevels) {
	if value != nil {
		FfiDestroyerTypePowerLevels{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeRoomSubscription struct{}

var FfiConverterOptionalTypeRoomSubscriptionINSTANCE = FfiConverterOptionalTypeRoomSubscription{}

func (c FfiConverterOptionalTypeRoomSubscription) Lift(rb RustBufferI) *RoomSubscription {
	return LiftFromRustBuffer[*RoomSubscription](c, rb)
}

func (_ FfiConverterOptionalTypeRoomSubscription) Read(reader io.Reader) *RoomSubscription {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeRoomSubscriptionINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeRoomSubscription) Lower(value *RoomSubscription) RustBuffer {
	return LowerIntoRustBuffer[*RoomSubscription](c, value)
}

func (_ FfiConverterOptionalTypeRoomSubscription) Write(writer io.Writer, value *RoomSubscription) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeRoomSubscriptionINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeRoomSubscription struct{}

func (_ FfiDestroyerOptionalTypeRoomSubscription) Destroy(value *RoomSubscription) {
	if value != nil {
		FfiDestroyerTypeRoomSubscription{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeSetData struct{}

var FfiConverterOptionalTypeSetDataINSTANCE = FfiConverterOptionalTypeSetData{}

func (c FfiConverterOptionalTypeSetData) Lift(rb RustBufferI) *SetData {
	return LiftFromRustBuffer[*SetData](c, rb)
}

func (_ FfiConverterOptionalTypeSetData) Read(reader io.Reader) *SetData {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeSetDataINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeSetData) Lower(value *SetData) RustBuffer {
	return LowerIntoRustBuffer[*SetData](c, value)
}

func (_ FfiConverterOptionalTypeSetData) Write(writer io.Writer, value *SetData) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeSetDataINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeSetData struct{}

func (_ FfiDestroyerOptionalTypeSetData) Destroy(value *SetData) {
	if value != nil {
		FfiDestroyerTypeSetData{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeThumbnailInfo struct{}

var FfiConverterOptionalTypeThumbnailInfoINSTANCE = FfiConverterOptionalTypeThumbnailInfo{}

func (c FfiConverterOptionalTypeThumbnailInfo) Lift(rb RustBufferI) *ThumbnailInfo {
	return LiftFromRustBuffer[*ThumbnailInfo](c, rb)
}

func (_ FfiConverterOptionalTypeThumbnailInfo) Read(reader io.Reader) *ThumbnailInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeThumbnailInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeThumbnailInfo) Lower(value *ThumbnailInfo) RustBuffer {
	return LowerIntoRustBuffer[*ThumbnailInfo](c, value)
}

func (_ FfiConverterOptionalTypeThumbnailInfo) Write(writer io.Writer, value *ThumbnailInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeThumbnailInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeThumbnailInfo struct{}

func (_ FfiDestroyerOptionalTypeThumbnailInfo) Destroy(value *ThumbnailInfo) {
	if value != nil {
		FfiDestroyerTypeThumbnailInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeTracingFileConfiguration struct{}

var FfiConverterOptionalTypeTracingFileConfigurationINSTANCE = FfiConverterOptionalTypeTracingFileConfiguration{}

func (c FfiConverterOptionalTypeTracingFileConfiguration) Lift(rb RustBufferI) *TracingFileConfiguration {
	return LiftFromRustBuffer[*TracingFileConfiguration](c, rb)
}

func (_ FfiConverterOptionalTypeTracingFileConfiguration) Read(reader io.Reader) *TracingFileConfiguration {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeTracingFileConfigurationINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeTracingFileConfiguration) Lower(value *TracingFileConfiguration) RustBuffer {
	return LowerIntoRustBuffer[*TracingFileConfiguration](c, value)
}

func (_ FfiConverterOptionalTypeTracingFileConfiguration) Write(writer io.Writer, value *TracingFileConfiguration) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeTracingFileConfigurationINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeTracingFileConfiguration struct{}

func (_ FfiDestroyerOptionalTypeTracingFileConfiguration) Destroy(value *TracingFileConfiguration) {
	if value != nil {
		FfiDestroyerTypeTracingFileConfiguration{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeUnstableAudioDetailsContent struct{}

var FfiConverterOptionalTypeUnstableAudioDetailsContentINSTANCE = FfiConverterOptionalTypeUnstableAudioDetailsContent{}

func (c FfiConverterOptionalTypeUnstableAudioDetailsContent) Lift(rb RustBufferI) *UnstableAudioDetailsContent {
	return LiftFromRustBuffer[*UnstableAudioDetailsContent](c, rb)
}

func (_ FfiConverterOptionalTypeUnstableAudioDetailsContent) Read(reader io.Reader) *UnstableAudioDetailsContent {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeUnstableAudioDetailsContentINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeUnstableAudioDetailsContent) Lower(value *UnstableAudioDetailsContent) RustBuffer {
	return LowerIntoRustBuffer[*UnstableAudioDetailsContent](c, value)
}

func (_ FfiConverterOptionalTypeUnstableAudioDetailsContent) Write(writer io.Writer, value *UnstableAudioDetailsContent) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeUnstableAudioDetailsContentINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeUnstableAudioDetailsContent struct{}

func (_ FfiDestroyerOptionalTypeUnstableAudioDetailsContent) Destroy(value *UnstableAudioDetailsContent) {
	if value != nil {
		FfiDestroyerTypeUnstableAudioDetailsContent{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeUnstableVoiceContent struct{}

var FfiConverterOptionalTypeUnstableVoiceContentINSTANCE = FfiConverterOptionalTypeUnstableVoiceContent{}

func (c FfiConverterOptionalTypeUnstableVoiceContent) Lift(rb RustBufferI) *UnstableVoiceContent {
	return LiftFromRustBuffer[*UnstableVoiceContent](c, rb)
}

func (_ FfiConverterOptionalTypeUnstableVoiceContent) Read(reader io.Reader) *UnstableVoiceContent {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeUnstableVoiceContentINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeUnstableVoiceContent) Lower(value *UnstableVoiceContent) RustBuffer {
	return LowerIntoRustBuffer[*UnstableVoiceContent](c, value)
}

func (_ FfiConverterOptionalTypeUnstableVoiceContent) Write(writer io.Writer, value *UnstableVoiceContent) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeUnstableVoiceContentINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeUnstableVoiceContent struct{}

func (_ FfiDestroyerOptionalTypeUnstableVoiceContent) Destroy(value *UnstableVoiceContent) {
	if value != nil {
		FfiDestroyerTypeUnstableVoiceContent{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeVideoInfo struct{}

var FfiConverterOptionalTypeVideoInfoINSTANCE = FfiConverterOptionalTypeVideoInfo{}

func (c FfiConverterOptionalTypeVideoInfo) Lift(rb RustBufferI) *VideoInfo {
	return LiftFromRustBuffer[*VideoInfo](c, rb)
}

func (_ FfiConverterOptionalTypeVideoInfo) Read(reader io.Reader) *VideoInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeVideoInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeVideoInfo) Lower(value *VideoInfo) RustBuffer {
	return LowerIntoRustBuffer[*VideoInfo](c, value)
}

func (_ FfiConverterOptionalTypeVideoInfo) Write(writer io.Writer, value *VideoInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeVideoInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeVideoInfo struct{}

func (_ FfiDestroyerOptionalTypeVideoInfo) Destroy(value *VideoInfo) {
	if value != nil {
		FfiDestroyerTypeVideoInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeAccountManagementAction struct{}

var FfiConverterOptionalTypeAccountManagementActionINSTANCE = FfiConverterOptionalTypeAccountManagementAction{}

func (c FfiConverterOptionalTypeAccountManagementAction) Lift(rb RustBufferI) *AccountManagementAction {
	return LiftFromRustBuffer[*AccountManagementAction](c, rb)
}

func (_ FfiConverterOptionalTypeAccountManagementAction) Read(reader io.Reader) *AccountManagementAction {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeAccountManagementActionINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeAccountManagementAction) Lower(value *AccountManagementAction) RustBuffer {
	return LowerIntoRustBuffer[*AccountManagementAction](c, value)
}

func (_ FfiConverterOptionalTypeAccountManagementAction) Write(writer io.Writer, value *AccountManagementAction) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeAccountManagementActionINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeAccountManagementAction struct{}

func (_ FfiDestroyerOptionalTypeAccountManagementAction) Destroy(value *AccountManagementAction) {
	if value != nil {
		FfiDestroyerTypeAccountManagementAction{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeAssetType struct{}

var FfiConverterOptionalTypeAssetTypeINSTANCE = FfiConverterOptionalTypeAssetType{}

func (c FfiConverterOptionalTypeAssetType) Lift(rb RustBufferI) *AssetType {
	return LiftFromRustBuffer[*AssetType](c, rb)
}

func (_ FfiConverterOptionalTypeAssetType) Read(reader io.Reader) *AssetType {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeAssetTypeINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeAssetType) Lower(value *AssetType) RustBuffer {
	return LowerIntoRustBuffer[*AssetType](c, value)
}

func (_ FfiConverterOptionalTypeAssetType) Write(writer io.Writer, value *AssetType) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeAssetTypeINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeAssetType struct{}

func (_ FfiDestroyerOptionalTypeAssetType) Destroy(value *AssetType) {
	if value != nil {
		FfiDestroyerTypeAssetType{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeEventSendState struct{}

var FfiConverterOptionalTypeEventSendStateINSTANCE = FfiConverterOptionalTypeEventSendState{}

func (c FfiConverterOptionalTypeEventSendState) Lift(rb RustBufferI) *EventSendState {
	return LiftFromRustBuffer[*EventSendState](c, rb)
}

func (_ FfiConverterOptionalTypeEventSendState) Read(reader io.Reader) *EventSendState {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeEventSendStateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeEventSendState) Lower(value *EventSendState) RustBuffer {
	return LowerIntoRustBuffer[*EventSendState](c, value)
}

func (_ FfiConverterOptionalTypeEventSendState) Write(writer io.Writer, value *EventSendState) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeEventSendStateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeEventSendState struct{}

func (_ FfiDestroyerOptionalTypeEventSendState) Destroy(value *EventSendState) {
	if value != nil {
		FfiDestroyerTypeEventSendState{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeMembershipChange struct{}

var FfiConverterOptionalTypeMembershipChangeINSTANCE = FfiConverterOptionalTypeMembershipChange{}

func (c FfiConverterOptionalTypeMembershipChange) Lift(rb RustBufferI) *MembershipChange {
	return LiftFromRustBuffer[*MembershipChange](c, rb)
}

func (_ FfiConverterOptionalTypeMembershipChange) Read(reader io.Reader) *MembershipChange {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeMembershipChangeINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeMembershipChange) Lower(value *MembershipChange) RustBuffer {
	return LowerIntoRustBuffer[*MembershipChange](c, value)
}

func (_ FfiConverterOptionalTypeMembershipChange) Write(writer io.Writer, value *MembershipChange) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeMembershipChangeINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeMembershipChange struct{}

func (_ FfiDestroyerOptionalTypeMembershipChange) Destroy(value *MembershipChange) {
	if value != nil {
		FfiDestroyerTypeMembershipChange{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePushFormat struct{}

var FfiConverterOptionalTypePushFormatINSTANCE = FfiConverterOptionalTypePushFormat{}

func (c FfiConverterOptionalTypePushFormat) Lift(rb RustBufferI) *PushFormat {
	return LiftFromRustBuffer[*PushFormat](c, rb)
}

func (_ FfiConverterOptionalTypePushFormat) Read(reader io.Reader) *PushFormat {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePushFormatINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePushFormat) Lower(value *PushFormat) RustBuffer {
	return LowerIntoRustBuffer[*PushFormat](c, value)
}

func (_ FfiConverterOptionalTypePushFormat) Write(writer io.Writer, value *PushFormat) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePushFormatINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePushFormat struct{}

func (_ FfiDestroyerOptionalTypePushFormat) Destroy(value *PushFormat) {
	if value != nil {
		FfiDestroyerTypePushFormat{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeRoomNotificationMode struct{}

var FfiConverterOptionalTypeRoomNotificationModeINSTANCE = FfiConverterOptionalTypeRoomNotificationMode{}

func (c FfiConverterOptionalTypeRoomNotificationMode) Lift(rb RustBufferI) *RoomNotificationMode {
	return LiftFromRustBuffer[*RoomNotificationMode](c, rb)
}

func (_ FfiConverterOptionalTypeRoomNotificationMode) Read(reader io.Reader) *RoomNotificationMode {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeRoomNotificationModeINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeRoomNotificationMode) Lower(value *RoomNotificationMode) RustBuffer {
	return LowerIntoRustBuffer[*RoomNotificationMode](c, value)
}

func (_ FfiConverterOptionalTypeRoomNotificationMode) Write(writer io.Writer, value *RoomNotificationMode) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeRoomNotificationModeINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeRoomNotificationMode struct{}

func (_ FfiDestroyerOptionalTypeRoomNotificationMode) Destroy(value *RoomNotificationMode) {
	if value != nil {
		FfiDestroyerTypeRoomNotificationMode{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeVirtualTimelineItem struct{}

var FfiConverterOptionalTypeVirtualTimelineItemINSTANCE = FfiConverterOptionalTypeVirtualTimelineItem{}

func (c FfiConverterOptionalTypeVirtualTimelineItem) Lift(rb RustBufferI) *VirtualTimelineItem {
	return LiftFromRustBuffer[*VirtualTimelineItem](c, rb)
}

func (_ FfiConverterOptionalTypeVirtualTimelineItem) Read(reader io.Reader) *VirtualTimelineItem {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeVirtualTimelineItemINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeVirtualTimelineItem) Lower(value *VirtualTimelineItem) RustBuffer {
	return LowerIntoRustBuffer[*VirtualTimelineItem](c, value)
}

func (_ FfiConverterOptionalTypeVirtualTimelineItem) Write(writer io.Writer, value *VirtualTimelineItem) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeVirtualTimelineItemINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeVirtualTimelineItem struct{}

func (_ FfiDestroyerOptionalTypeVirtualTimelineItem) Destroy(value *VirtualTimelineItem) {
	if value != nil {
		FfiDestroyerTypeVirtualTimelineItem{}.Destroy(*value)
	}
}

type FfiConverterOptionalCallbackInterfaceBackupSteadyStateListener struct{}

var FfiConverterOptionalCallbackInterfaceBackupSteadyStateListenerINSTANCE = FfiConverterOptionalCallbackInterfaceBackupSteadyStateListener{}

func (c FfiConverterOptionalCallbackInterfaceBackupSteadyStateListener) Lift(rb RustBufferI) *BackupSteadyStateListener {
	return LiftFromRustBuffer[*BackupSteadyStateListener](c, rb)
}

func (_ FfiConverterOptionalCallbackInterfaceBackupSteadyStateListener) Read(reader io.Reader) *BackupSteadyStateListener {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterCallbackInterfaceBackupSteadyStateListenerINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalCallbackInterfaceBackupSteadyStateListener) Lower(value *BackupSteadyStateListener) RustBuffer {
	return LowerIntoRustBuffer[*BackupSteadyStateListener](c, value)
}

func (_ FfiConverterOptionalCallbackInterfaceBackupSteadyStateListener) Write(writer io.Writer, value *BackupSteadyStateListener) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterCallbackInterfaceBackupSteadyStateListenerINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalCallbackInterfaceBackupSteadyStateListener struct{}

func (_ FfiDestroyerOptionalCallbackInterfaceBackupSteadyStateListener) Destroy(value *BackupSteadyStateListener) {
	if value != nil {
		FfiDestroyerCallbackInterfaceBackupSteadyStateListener{}.Destroy(*value)
	}
}

type FfiConverterOptionalCallbackInterfaceClientDelegate struct{}

var FfiConverterOptionalCallbackInterfaceClientDelegateINSTANCE = FfiConverterOptionalCallbackInterfaceClientDelegate{}

func (c FfiConverterOptionalCallbackInterfaceClientDelegate) Lift(rb RustBufferI) *ClientDelegate {
	return LiftFromRustBuffer[*ClientDelegate](c, rb)
}

func (_ FfiConverterOptionalCallbackInterfaceClientDelegate) Read(reader io.Reader) *ClientDelegate {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterCallbackInterfaceClientDelegateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalCallbackInterfaceClientDelegate) Lower(value *ClientDelegate) RustBuffer {
	return LowerIntoRustBuffer[*ClientDelegate](c, value)
}

func (_ FfiConverterOptionalCallbackInterfaceClientDelegate) Write(writer io.Writer, value *ClientDelegate) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterCallbackInterfaceClientDelegateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalCallbackInterfaceClientDelegate struct{}

func (_ FfiDestroyerOptionalCallbackInterfaceClientDelegate) Destroy(value *ClientDelegate) {
	if value != nil {
		FfiDestroyerCallbackInterfaceClientDelegate{}.Destroy(*value)
	}
}

type FfiConverterOptionalCallbackInterfaceClientSessionDelegate struct{}

var FfiConverterOptionalCallbackInterfaceClientSessionDelegateINSTANCE = FfiConverterOptionalCallbackInterfaceClientSessionDelegate{}

func (c FfiConverterOptionalCallbackInterfaceClientSessionDelegate) Lift(rb RustBufferI) *ClientSessionDelegate {
	return LiftFromRustBuffer[*ClientSessionDelegate](c, rb)
}

func (_ FfiConverterOptionalCallbackInterfaceClientSessionDelegate) Read(reader io.Reader) *ClientSessionDelegate {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalCallbackInterfaceClientSessionDelegate) Lower(value *ClientSessionDelegate) RustBuffer {
	return LowerIntoRustBuffer[*ClientSessionDelegate](c, value)
}

func (_ FfiConverterOptionalCallbackInterfaceClientSessionDelegate) Write(writer io.Writer, value *ClientSessionDelegate) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterCallbackInterfaceClientSessionDelegateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalCallbackInterfaceClientSessionDelegate struct{}

func (_ FfiDestroyerOptionalCallbackInterfaceClientSessionDelegate) Destroy(value *ClientSessionDelegate) {
	if value != nil {
		FfiDestroyerCallbackInterfaceClientSessionDelegate{}.Destroy(*value)
	}
}

type FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegate struct{}

var FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegateINSTANCE = FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegate{}

func (c FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegate) Lift(rb RustBufferI) *NotificationSettingsDelegate {
	return LiftFromRustBuffer[*NotificationSettingsDelegate](c, rb)
}

func (_ FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegate) Read(reader io.Reader) *NotificationSettingsDelegate {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterCallbackInterfaceNotificationSettingsDelegateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegate) Lower(value *NotificationSettingsDelegate) RustBuffer {
	return LowerIntoRustBuffer[*NotificationSettingsDelegate](c, value)
}

func (_ FfiConverterOptionalCallbackInterfaceNotificationSettingsDelegate) Write(writer io.Writer, value *NotificationSettingsDelegate) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterCallbackInterfaceNotificationSettingsDelegateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalCallbackInterfaceNotificationSettingsDelegate struct{}

func (_ FfiDestroyerOptionalCallbackInterfaceNotificationSettingsDelegate) Destroy(value *NotificationSettingsDelegate) {
	if value != nil {
		FfiDestroyerCallbackInterfaceNotificationSettingsDelegate{}.Destroy(*value)
	}
}

type FfiConverterOptionalCallbackInterfaceProgressWatcher struct{}

var FfiConverterOptionalCallbackInterfaceProgressWatcherINSTANCE = FfiConverterOptionalCallbackInterfaceProgressWatcher{}

func (c FfiConverterOptionalCallbackInterfaceProgressWatcher) Lift(rb RustBufferI) *ProgressWatcher {
	return LiftFromRustBuffer[*ProgressWatcher](c, rb)
}

func (_ FfiConverterOptionalCallbackInterfaceProgressWatcher) Read(reader io.Reader) *ProgressWatcher {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterCallbackInterfaceProgressWatcherINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalCallbackInterfaceProgressWatcher) Lower(value *ProgressWatcher) RustBuffer {
	return LowerIntoRustBuffer[*ProgressWatcher](c, value)
}

func (_ FfiConverterOptionalCallbackInterfaceProgressWatcher) Write(writer io.Writer, value *ProgressWatcher) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterCallbackInterfaceProgressWatcherINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalCallbackInterfaceProgressWatcher struct{}

func (_ FfiDestroyerOptionalCallbackInterfaceProgressWatcher) Destroy(value *ProgressWatcher) {
	if value != nil {
		FfiDestroyerCallbackInterfaceProgressWatcher{}.Destroy(*value)
	}
}

type FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegate struct{}

var FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegateINSTANCE = FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegate{}

func (c FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegate) Lift(rb RustBufferI) *SessionVerificationControllerDelegate {
	return LiftFromRustBuffer[*SessionVerificationControllerDelegate](c, rb)
}

func (_ FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegate) Read(reader io.Reader) *SessionVerificationControllerDelegate {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterCallbackInterfaceSessionVerificationControllerDelegateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegate) Lower(value *SessionVerificationControllerDelegate) RustBuffer {
	return LowerIntoRustBuffer[*SessionVerificationControllerDelegate](c, value)
}

func (_ FfiConverterOptionalCallbackInterfaceSessionVerificationControllerDelegate) Write(writer io.Writer, value *SessionVerificationControllerDelegate) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterCallbackInterfaceSessionVerificationControllerDelegateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalCallbackInterfaceSessionVerificationControllerDelegate struct{}

func (_ FfiDestroyerOptionalCallbackInterfaceSessionVerificationControllerDelegate) Destroy(value *SessionVerificationControllerDelegate) {
	if value != nil {
		FfiDestroyerCallbackInterfaceSessionVerificationControllerDelegate{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceString struct{}

var FfiConverterOptionalSequenceStringINSTANCE = FfiConverterOptionalSequenceString{}

func (c FfiConverterOptionalSequenceString) Lift(rb RustBufferI) *[]string {
	return LiftFromRustBuffer[*[]string](c, rb)
}

func (_ FfiConverterOptionalSequenceString) Read(reader io.Reader) *[]string {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceStringINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceString) Lower(value *[]string) RustBuffer {
	return LowerIntoRustBuffer[*[]string](c, value)
}

func (_ FfiConverterOptionalSequenceString) Write(writer io.Writer, value *[]string) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceStringINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceString struct{}

func (_ FfiDestroyerOptionalSequenceString) Destroy(value *[]string) {
	if value != nil {
		FfiDestroyerSequenceString{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceRoomMember struct{}

var FfiConverterOptionalSequenceRoomMemberINSTANCE = FfiConverterOptionalSequenceRoomMember{}

func (c FfiConverterOptionalSequenceRoomMember) Lift(rb RustBufferI) *[]*RoomMember {
	return LiftFromRustBuffer[*[]*RoomMember](c, rb)
}

func (_ FfiConverterOptionalSequenceRoomMember) Read(reader io.Reader) *[]*RoomMember {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceRoomMemberINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceRoomMember) Lower(value *[]*RoomMember) RustBuffer {
	return LowerIntoRustBuffer[*[]*RoomMember](c, value)
}

func (_ FfiConverterOptionalSequenceRoomMember) Write(writer io.Writer, value *[]*RoomMember) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceRoomMemberINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceRoomMember struct{}

func (_ FfiDestroyerOptionalSequenceRoomMember) Destroy(value *[]*RoomMember) {
	if value != nil {
		FfiDestroyerSequenceRoomMember{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceTimelineItem struct{}

var FfiConverterOptionalSequenceTimelineItemINSTANCE = FfiConverterOptionalSequenceTimelineItem{}

func (c FfiConverterOptionalSequenceTimelineItem) Lift(rb RustBufferI) *[]*TimelineItem {
	return LiftFromRustBuffer[*[]*TimelineItem](c, rb)
}

func (_ FfiConverterOptionalSequenceTimelineItem) Read(reader io.Reader) *[]*TimelineItem {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceTimelineItemINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceTimelineItem) Lower(value *[]*TimelineItem) RustBuffer {
	return LowerIntoRustBuffer[*[]*TimelineItem](c, value)
}

func (_ FfiConverterOptionalSequenceTimelineItem) Write(writer io.Writer, value *[]*TimelineItem) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceTimelineItemINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceTimelineItem struct{}

func (_ FfiDestroyerOptionalSequenceTimelineItem) Destroy(value *[]*TimelineItem) {
	if value != nil {
		FfiDestroyerSequenceTimelineItem{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceTypeRequiredState struct{}

var FfiConverterOptionalSequenceTypeRequiredStateINSTANCE = FfiConverterOptionalSequenceTypeRequiredState{}

func (c FfiConverterOptionalSequenceTypeRequiredState) Lift(rb RustBufferI) *[]RequiredState {
	return LiftFromRustBuffer[*[]RequiredState](c, rb)
}

func (_ FfiConverterOptionalSequenceTypeRequiredState) Read(reader io.Reader) *[]RequiredState {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceTypeRequiredStateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceTypeRequiredState) Lower(value *[]RequiredState) RustBuffer {
	return LowerIntoRustBuffer[*[]RequiredState](c, value)
}

func (_ FfiConverterOptionalSequenceTypeRequiredState) Write(writer io.Writer, value *[]RequiredState) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceTypeRequiredStateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceTypeRequiredState struct{}

func (_ FfiDestroyerOptionalSequenceTypeRequiredState) Destroy(value *[]RequiredState) {
	if value != nil {
		FfiDestroyerSequenceTypeRequiredState{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeEventItemOrigin struct{}

var FfiConverterOptionalTypeEventItemOriginINSTANCE = FfiConverterOptionalTypeEventItemOrigin{}

func (c FfiConverterOptionalTypeEventItemOrigin) Lift(rb RustBufferI) *matrix_sdk_ui.EventItemOrigin {
	return LiftFromRustBuffer[*matrix_sdk_ui.EventItemOrigin](c, rb)
}

func (_ FfiConverterOptionalTypeEventItemOrigin) Read(reader io.Reader) *matrix_sdk_ui.EventItemOrigin {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := matrix_sdk_ui.FfiConverterTypeEventItemOriginINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeEventItemOrigin) Lower(value *matrix_sdk_ui.EventItemOrigin) RustBuffer {
	return LowerIntoRustBuffer[*matrix_sdk_ui.EventItemOrigin](c, value)
}

func (_ FfiConverterOptionalTypeEventItemOrigin) Write(writer io.Writer, value *matrix_sdk_ui.EventItemOrigin) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		matrix_sdk_ui.FfiConverterTypeEventItemOriginINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeEventItemOrigin struct{}

func (_ FfiDestroyerOptionalTypeEventItemOrigin) Destroy(value *matrix_sdk_ui.EventItemOrigin) {
	if value != nil {
		matrix_sdk_ui.FfiDestroyerTypeEventItemOrigin{}.Destroy(*value)
	}
}

type FfiConverterSequenceUint16 struct{}

var FfiConverterSequenceUint16INSTANCE = FfiConverterSequenceUint16{}

func (c FfiConverterSequenceUint16) Lift(rb RustBufferI) []uint16 {
	return LiftFromRustBuffer[[]uint16](c, rb)
}

func (c FfiConverterSequenceUint16) Read(reader io.Reader) []uint16 {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]uint16, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterUint16INSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceUint16) Lower(value []uint16) RustBuffer {
	return LowerIntoRustBuffer[[]uint16](c, value)
}

func (c FfiConverterSequenceUint16) Write(writer io.Writer, value []uint16) {
	if len(value) > math.MaxInt32 {
		panic("[]uint16 is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterUint16INSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceUint16 struct{}

func (FfiDestroyerSequenceUint16) Destroy(sequence []uint16) {
	for _, value := range sequence {
		FfiDestroyerUint16{}.Destroy(value)
	}
}

type FfiConverterSequenceString struct{}

var FfiConverterSequenceStringINSTANCE = FfiConverterSequenceString{}

func (c FfiConverterSequenceString) Lift(rb RustBufferI) []string {
	return LiftFromRustBuffer[[]string](c, rb)
}

func (c FfiConverterSequenceString) Read(reader io.Reader) []string {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]string, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterStringINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceString) Lower(value []string) RustBuffer {
	return LowerIntoRustBuffer[[]string](c, value)
}

func (c FfiConverterSequenceString) Write(writer io.Writer, value []string) {
	if len(value) > math.MaxInt32 {
		panic("[]string is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterStringINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceString struct{}

func (FfiDestroyerSequenceString) Destroy(sequence []string) {
	for _, value := range sequence {
		FfiDestroyerString{}.Destroy(value)
	}
}

type FfiConverterSequenceRoom struct{}

var FfiConverterSequenceRoomINSTANCE = FfiConverterSequenceRoom{}

func (c FfiConverterSequenceRoom) Lift(rb RustBufferI) []*Room {
	return LiftFromRustBuffer[[]*Room](c, rb)
}

func (c FfiConverterSequenceRoom) Read(reader io.Reader) []*Room {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]*Room, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterRoomINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceRoom) Lower(value []*Room) RustBuffer {
	return LowerIntoRustBuffer[[]*Room](c, value)
}

func (c FfiConverterSequenceRoom) Write(writer io.Writer, value []*Room) {
	if len(value) > math.MaxInt32 {
		panic("[]*Room is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterRoomINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceRoom struct{}

func (FfiDestroyerSequenceRoom) Destroy(sequence []*Room) {
	for _, value := range sequence {
		FfiDestroyerRoom{}.Destroy(value)
	}
}

type FfiConverterSequenceRoomMember struct{}

var FfiConverterSequenceRoomMemberINSTANCE = FfiConverterSequenceRoomMember{}

func (c FfiConverterSequenceRoomMember) Lift(rb RustBufferI) []*RoomMember {
	return LiftFromRustBuffer[[]*RoomMember](c, rb)
}

func (c FfiConverterSequenceRoomMember) Read(reader io.Reader) []*RoomMember {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]*RoomMember, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterRoomMemberINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceRoomMember) Lower(value []*RoomMember) RustBuffer {
	return LowerIntoRustBuffer[[]*RoomMember](c, value)
}

func (c FfiConverterSequenceRoomMember) Write(writer io.Writer, value []*RoomMember) {
	if len(value) > math.MaxInt32 {
		panic("[]*RoomMember is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterRoomMemberINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceRoomMember struct{}

func (FfiDestroyerSequenceRoomMember) Destroy(sequence []*RoomMember) {
	for _, value := range sequence {
		FfiDestroyerRoomMember{}.Destroy(value)
	}
}

type FfiConverterSequenceSessionVerificationEmoji struct{}

var FfiConverterSequenceSessionVerificationEmojiINSTANCE = FfiConverterSequenceSessionVerificationEmoji{}

func (c FfiConverterSequenceSessionVerificationEmoji) Lift(rb RustBufferI) []*SessionVerificationEmoji {
	return LiftFromRustBuffer[[]*SessionVerificationEmoji](c, rb)
}

func (c FfiConverterSequenceSessionVerificationEmoji) Read(reader io.Reader) []*SessionVerificationEmoji {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]*SessionVerificationEmoji, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterSessionVerificationEmojiINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceSessionVerificationEmoji) Lower(value []*SessionVerificationEmoji) RustBuffer {
	return LowerIntoRustBuffer[[]*SessionVerificationEmoji](c, value)
}

func (c FfiConverterSequenceSessionVerificationEmoji) Write(writer io.Writer, value []*SessionVerificationEmoji) {
	if len(value) > math.MaxInt32 {
		panic("[]*SessionVerificationEmoji is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterSessionVerificationEmojiINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceSessionVerificationEmoji struct{}

func (FfiDestroyerSequenceSessionVerificationEmoji) Destroy(sequence []*SessionVerificationEmoji) {
	for _, value := range sequence {
		FfiDestroyerSessionVerificationEmoji{}.Destroy(value)
	}
}

type FfiConverterSequenceTimelineDiff struct{}

var FfiConverterSequenceTimelineDiffINSTANCE = FfiConverterSequenceTimelineDiff{}

func (c FfiConverterSequenceTimelineDiff) Lift(rb RustBufferI) []*TimelineDiff {
	return LiftFromRustBuffer[[]*TimelineDiff](c, rb)
}

func (c FfiConverterSequenceTimelineDiff) Read(reader io.Reader) []*TimelineDiff {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]*TimelineDiff, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTimelineDiffINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTimelineDiff) Lower(value []*TimelineDiff) RustBuffer {
	return LowerIntoRustBuffer[[]*TimelineDiff](c, value)
}

func (c FfiConverterSequenceTimelineDiff) Write(writer io.Writer, value []*TimelineDiff) {
	if len(value) > math.MaxInt32 {
		panic("[]*TimelineDiff is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTimelineDiffINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTimelineDiff struct{}

func (FfiDestroyerSequenceTimelineDiff) Destroy(sequence []*TimelineDiff) {
	for _, value := range sequence {
		FfiDestroyerTimelineDiff{}.Destroy(value)
	}
}

type FfiConverterSequenceTimelineItem struct{}

var FfiConverterSequenceTimelineItemINSTANCE = FfiConverterSequenceTimelineItem{}

func (c FfiConverterSequenceTimelineItem) Lift(rb RustBufferI) []*TimelineItem {
	return LiftFromRustBuffer[[]*TimelineItem](c, rb)
}

func (c FfiConverterSequenceTimelineItem) Read(reader io.Reader) []*TimelineItem {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]*TimelineItem, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTimelineItemINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTimelineItem) Lower(value []*TimelineItem) RustBuffer {
	return LowerIntoRustBuffer[[]*TimelineItem](c, value)
}

func (c FfiConverterSequenceTimelineItem) Write(writer io.Writer, value []*TimelineItem) {
	if len(value) > math.MaxInt32 {
		panic("[]*TimelineItem is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTimelineItemINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTimelineItem struct{}

func (FfiDestroyerSequenceTimelineItem) Destroy(sequence []*TimelineItem) {
	for _, value := range sequence {
		FfiDestroyerTimelineItem{}.Destroy(value)
	}
}

type FfiConverterSequenceTypePollAnswer struct{}

var FfiConverterSequenceTypePollAnswerINSTANCE = FfiConverterSequenceTypePollAnswer{}

func (c FfiConverterSequenceTypePollAnswer) Lift(rb RustBufferI) []PollAnswer {
	return LiftFromRustBuffer[[]PollAnswer](c, rb)
}

func (c FfiConverterSequenceTypePollAnswer) Read(reader io.Reader) []PollAnswer {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]PollAnswer, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypePollAnswerINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypePollAnswer) Lower(value []PollAnswer) RustBuffer {
	return LowerIntoRustBuffer[[]PollAnswer](c, value)
}

func (c FfiConverterSequenceTypePollAnswer) Write(writer io.Writer, value []PollAnswer) {
	if len(value) > math.MaxInt32 {
		panic("[]PollAnswer is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypePollAnswerINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypePollAnswer struct{}

func (FfiDestroyerSequenceTypePollAnswer) Destroy(sequence []PollAnswer) {
	for _, value := range sequence {
		FfiDestroyerTypePollAnswer{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeReaction struct{}

var FfiConverterSequenceTypeReactionINSTANCE = FfiConverterSequenceTypeReaction{}

func (c FfiConverterSequenceTypeReaction) Lift(rb RustBufferI) []Reaction {
	return LiftFromRustBuffer[[]Reaction](c, rb)
}

func (c FfiConverterSequenceTypeReaction) Read(reader io.Reader) []Reaction {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]Reaction, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeReactionINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeReaction) Lower(value []Reaction) RustBuffer {
	return LowerIntoRustBuffer[[]Reaction](c, value)
}

func (c FfiConverterSequenceTypeReaction) Write(writer io.Writer, value []Reaction) {
	if len(value) > math.MaxInt32 {
		panic("[]Reaction is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeReactionINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeReaction struct{}

func (FfiDestroyerSequenceTypeReaction) Destroy(sequence []Reaction) {
	for _, value := range sequence {
		FfiDestroyerTypeReaction{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeReactionSenderData struct{}

var FfiConverterSequenceTypeReactionSenderDataINSTANCE = FfiConverterSequenceTypeReactionSenderData{}

func (c FfiConverterSequenceTypeReactionSenderData) Lift(rb RustBufferI) []ReactionSenderData {
	return LiftFromRustBuffer[[]ReactionSenderData](c, rb)
}

func (c FfiConverterSequenceTypeReactionSenderData) Read(reader io.Reader) []ReactionSenderData {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]ReactionSenderData, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeReactionSenderDataINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeReactionSenderData) Lower(value []ReactionSenderData) RustBuffer {
	return LowerIntoRustBuffer[[]ReactionSenderData](c, value)
}

func (c FfiConverterSequenceTypeReactionSenderData) Write(writer io.Writer, value []ReactionSenderData) {
	if len(value) > math.MaxInt32 {
		panic("[]ReactionSenderData is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeReactionSenderDataINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeReactionSenderData struct{}

func (FfiDestroyerSequenceTypeReactionSenderData) Destroy(sequence []ReactionSenderData) {
	for _, value := range sequence {
		FfiDestroyerTypeReactionSenderData{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeRequiredState struct{}

var FfiConverterSequenceTypeRequiredStateINSTANCE = FfiConverterSequenceTypeRequiredState{}

func (c FfiConverterSequenceTypeRequiredState) Lift(rb RustBufferI) []RequiredState {
	return LiftFromRustBuffer[[]RequiredState](c, rb)
}

func (c FfiConverterSequenceTypeRequiredState) Read(reader io.Reader) []RequiredState {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]RequiredState, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeRequiredStateINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeRequiredState) Lower(value []RequiredState) RustBuffer {
	return LowerIntoRustBuffer[[]RequiredState](c, value)
}

func (c FfiConverterSequenceTypeRequiredState) Write(writer io.Writer, value []RequiredState) {
	if len(value) > math.MaxInt32 {
		panic("[]RequiredState is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeRequiredStateINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeRequiredState struct{}

func (FfiDestroyerSequenceTypeRequiredState) Destroy(sequence []RequiredState) {
	for _, value := range sequence {
		FfiDestroyerTypeRequiredState{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeRoomListRange struct{}

var FfiConverterSequenceTypeRoomListRangeINSTANCE = FfiConverterSequenceTypeRoomListRange{}

func (c FfiConverterSequenceTypeRoomListRange) Lift(rb RustBufferI) []RoomListRange {
	return LiftFromRustBuffer[[]RoomListRange](c, rb)
}

func (c FfiConverterSequenceTypeRoomListRange) Read(reader io.Reader) []RoomListRange {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]RoomListRange, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeRoomListRangeINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeRoomListRange) Lower(value []RoomListRange) RustBuffer {
	return LowerIntoRustBuffer[[]RoomListRange](c, value)
}

func (c FfiConverterSequenceTypeRoomListRange) Write(writer io.Writer, value []RoomListRange) {
	if len(value) > math.MaxInt32 {
		panic("[]RoomListRange is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeRoomListRangeINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeRoomListRange struct{}

func (FfiDestroyerSequenceTypeRoomListRange) Destroy(sequence []RoomListRange) {
	for _, value := range sequence {
		FfiDestroyerTypeRoomListRange{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeUserProfile struct{}

var FfiConverterSequenceTypeUserProfileINSTANCE = FfiConverterSequenceTypeUserProfile{}

func (c FfiConverterSequenceTypeUserProfile) Lift(rb RustBufferI) []UserProfile {
	return LiftFromRustBuffer[[]UserProfile](c, rb)
}

func (c FfiConverterSequenceTypeUserProfile) Read(reader io.Reader) []UserProfile {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]UserProfile, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeUserProfileINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeUserProfile) Lower(value []UserProfile) RustBuffer {
	return LowerIntoRustBuffer[[]UserProfile](c, value)
}

func (c FfiConverterSequenceTypeUserProfile) Write(writer io.Writer, value []UserProfile) {
	if len(value) > math.MaxInt32 {
		panic("[]UserProfile is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeUserProfileINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeUserProfile struct{}

func (FfiDestroyerSequenceTypeUserProfile) Destroy(sequence []UserProfile) {
	for _, value := range sequence {
		FfiDestroyerTypeUserProfile{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeFilterTimelineEventType struct{}

var FfiConverterSequenceTypeFilterTimelineEventTypeINSTANCE = FfiConverterSequenceTypeFilterTimelineEventType{}

func (c FfiConverterSequenceTypeFilterTimelineEventType) Lift(rb RustBufferI) []FilterTimelineEventType {
	return LiftFromRustBuffer[[]FilterTimelineEventType](c, rb)
}

func (c FfiConverterSequenceTypeFilterTimelineEventType) Read(reader io.Reader) []FilterTimelineEventType {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]FilterTimelineEventType, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeFilterTimelineEventTypeINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeFilterTimelineEventType) Lower(value []FilterTimelineEventType) RustBuffer {
	return LowerIntoRustBuffer[[]FilterTimelineEventType](c, value)
}

func (c FfiConverterSequenceTypeFilterTimelineEventType) Write(writer io.Writer, value []FilterTimelineEventType) {
	if len(value) > math.MaxInt32 {
		panic("[]FilterTimelineEventType is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeFilterTimelineEventTypeINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeFilterTimelineEventType struct{}

func (FfiDestroyerSequenceTypeFilterTimelineEventType) Destroy(sequence []FilterTimelineEventType) {
	for _, value := range sequence {
		FfiDestroyerTypeFilterTimelineEventType{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeRoomListEntriesDynamicFilterKind struct{}

var FfiConverterSequenceTypeRoomListEntriesDynamicFilterKindINSTANCE = FfiConverterSequenceTypeRoomListEntriesDynamicFilterKind{}

func (c FfiConverterSequenceTypeRoomListEntriesDynamicFilterKind) Lift(rb RustBufferI) []RoomListEntriesDynamicFilterKind {
	return LiftFromRustBuffer[[]RoomListEntriesDynamicFilterKind](c, rb)
}

func (c FfiConverterSequenceTypeRoomListEntriesDynamicFilterKind) Read(reader io.Reader) []RoomListEntriesDynamicFilterKind {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]RoomListEntriesDynamicFilterKind, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeRoomListEntriesDynamicFilterKindINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeRoomListEntriesDynamicFilterKind) Lower(value []RoomListEntriesDynamicFilterKind) RustBuffer {
	return LowerIntoRustBuffer[[]RoomListEntriesDynamicFilterKind](c, value)
}

func (c FfiConverterSequenceTypeRoomListEntriesDynamicFilterKind) Write(writer io.Writer, value []RoomListEntriesDynamicFilterKind) {
	if len(value) > math.MaxInt32 {
		panic("[]RoomListEntriesDynamicFilterKind is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeRoomListEntriesDynamicFilterKindINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeRoomListEntriesDynamicFilterKind struct{}

func (FfiDestroyerSequenceTypeRoomListEntriesDynamicFilterKind) Destroy(sequence []RoomListEntriesDynamicFilterKind) {
	for _, value := range sequence {
		FfiDestroyerTypeRoomListEntriesDynamicFilterKind{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeRoomListEntriesUpdate struct{}

var FfiConverterSequenceTypeRoomListEntriesUpdateINSTANCE = FfiConverterSequenceTypeRoomListEntriesUpdate{}

func (c FfiConverterSequenceTypeRoomListEntriesUpdate) Lift(rb RustBufferI) []RoomListEntriesUpdate {
	return LiftFromRustBuffer[[]RoomListEntriesUpdate](c, rb)
}

func (c FfiConverterSequenceTypeRoomListEntriesUpdate) Read(reader io.Reader) []RoomListEntriesUpdate {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]RoomListEntriesUpdate, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeRoomListEntriesUpdateINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeRoomListEntriesUpdate) Lower(value []RoomListEntriesUpdate) RustBuffer {
	return LowerIntoRustBuffer[[]RoomListEntriesUpdate](c, value)
}

func (c FfiConverterSequenceTypeRoomListEntriesUpdate) Write(writer io.Writer, value []RoomListEntriesUpdate) {
	if len(value) > math.MaxInt32 {
		panic("[]RoomListEntriesUpdate is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeRoomListEntriesUpdateINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeRoomListEntriesUpdate struct{}

func (FfiDestroyerSequenceTypeRoomListEntriesUpdate) Destroy(sequence []RoomListEntriesUpdate) {
	for _, value := range sequence {
		FfiDestroyerTypeRoomListEntriesUpdate{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeRoomListEntry struct{}

var FfiConverterSequenceTypeRoomListEntryINSTANCE = FfiConverterSequenceTypeRoomListEntry{}

func (c FfiConverterSequenceTypeRoomListEntry) Lift(rb RustBufferI) []RoomListEntry {
	return LiftFromRustBuffer[[]RoomListEntry](c, rb)
}

func (c FfiConverterSequenceTypeRoomListEntry) Read(reader io.Reader) []RoomListEntry {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]RoomListEntry, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeRoomListEntryINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeRoomListEntry) Lower(value []RoomListEntry) RustBuffer {
	return LowerIntoRustBuffer[[]RoomListEntry](c, value)
}

func (c FfiConverterSequenceTypeRoomListEntry) Write(writer io.Writer, value []RoomListEntry) {
	if len(value) > math.MaxInt32 {
		panic("[]RoomListEntry is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeRoomListEntryINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeRoomListEntry struct{}

func (FfiDestroyerSequenceTypeRoomListEntry) Destroy(sequence []RoomListEntry) {
	for _, value := range sequence {
		FfiDestroyerTypeRoomListEntry{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeWidgetEventFilter struct{}

var FfiConverterSequenceTypeWidgetEventFilterINSTANCE = FfiConverterSequenceTypeWidgetEventFilter{}

func (c FfiConverterSequenceTypeWidgetEventFilter) Lift(rb RustBufferI) []WidgetEventFilter {
	return LiftFromRustBuffer[[]WidgetEventFilter](c, rb)
}

func (c FfiConverterSequenceTypeWidgetEventFilter) Read(reader io.Reader) []WidgetEventFilter {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]WidgetEventFilter, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeWidgetEventFilterINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeWidgetEventFilter) Lower(value []WidgetEventFilter) RustBuffer {
	return LowerIntoRustBuffer[[]WidgetEventFilter](c, value)
}

func (c FfiConverterSequenceTypeWidgetEventFilter) Write(writer io.Writer, value []WidgetEventFilter) {
	if len(value) > math.MaxInt32 {
		panic("[]WidgetEventFilter is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeWidgetEventFilterINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeWidgetEventFilter struct{}

func (FfiDestroyerSequenceTypeWidgetEventFilter) Destroy(sequence []WidgetEventFilter) {
	for _, value := range sequence {
		FfiDestroyerTypeWidgetEventFilter{}.Destroy(value)
	}
}

type FfiConverterMapStringInt32 struct{}

var FfiConverterMapStringInt32INSTANCE = FfiConverterMapStringInt32{}

func (c FfiConverterMapStringInt32) Lift(rb RustBufferI) map[string]int32 {
	return LiftFromRustBuffer[map[string]int32](c, rb)
}

func (_ FfiConverterMapStringInt32) Read(reader io.Reader) map[string]int32 {
	result := make(map[string]int32)
	length := readInt32(reader)
	for i := int32(0); i < length; i++ {
		key := FfiConverterStringINSTANCE.Read(reader)
		value := FfiConverterInt32INSTANCE.Read(reader)
		result[key] = value
	}
	return result
}

func (c FfiConverterMapStringInt32) Lower(value map[string]int32) RustBuffer {
	return LowerIntoRustBuffer[map[string]int32](c, value)
}

func (_ FfiConverterMapStringInt32) Write(writer io.Writer, mapValue map[string]int32) {
	if len(mapValue) > math.MaxInt32 {
		panic("map[string]int32 is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(mapValue)))
	for key, value := range mapValue {
		FfiConverterStringINSTANCE.Write(writer, key)
		FfiConverterInt32INSTANCE.Write(writer, value)
	}
}

type FfiDestroyerMapStringInt32 struct{}

func (_ FfiDestroyerMapStringInt32) Destroy(mapValue map[string]int32) {
	for key, value := range mapValue {
		FfiDestroyerString{}.Destroy(key)
		FfiDestroyerInt32{}.Destroy(value)
	}
}

type FfiConverterMapStringString struct{}

var FfiConverterMapStringStringINSTANCE = FfiConverterMapStringString{}

func (c FfiConverterMapStringString) Lift(rb RustBufferI) map[string]string {
	return LiftFromRustBuffer[map[string]string](c, rb)
}

func (_ FfiConverterMapStringString) Read(reader io.Reader) map[string]string {
	result := make(map[string]string)
	length := readInt32(reader)
	for i := int32(0); i < length; i++ {
		key := FfiConverterStringINSTANCE.Read(reader)
		value := FfiConverterStringINSTANCE.Read(reader)
		result[key] = value
	}
	return result
}

func (c FfiConverterMapStringString) Lower(value map[string]string) RustBuffer {
	return LowerIntoRustBuffer[map[string]string](c, value)
}

func (_ FfiConverterMapStringString) Write(writer io.Writer, mapValue map[string]string) {
	if len(mapValue) > math.MaxInt32 {
		panic("map[string]string is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(mapValue)))
	for key, value := range mapValue {
		FfiConverterStringINSTANCE.Write(writer, key)
		FfiConverterStringINSTANCE.Write(writer, value)
	}
}

type FfiDestroyerMapStringString struct{}

func (_ FfiDestroyerMapStringString) Destroy(mapValue map[string]string) {
	for key, value := range mapValue {
		FfiDestroyerString{}.Destroy(key)
		FfiDestroyerString{}.Destroy(value)
	}
}

type FfiConverterMapStringTypeReceipt struct{}

var FfiConverterMapStringTypeReceiptINSTANCE = FfiConverterMapStringTypeReceipt{}

func (c FfiConverterMapStringTypeReceipt) Lift(rb RustBufferI) map[string]Receipt {
	return LiftFromRustBuffer[map[string]Receipt](c, rb)
}

func (_ FfiConverterMapStringTypeReceipt) Read(reader io.Reader) map[string]Receipt {
	result := make(map[string]Receipt)
	length := readInt32(reader)
	for i := int32(0); i < length; i++ {
		key := FfiConverterStringINSTANCE.Read(reader)
		value := FfiConverterTypeReceiptINSTANCE.Read(reader)
		result[key] = value
	}
	return result
}

func (c FfiConverterMapStringTypeReceipt) Lower(value map[string]Receipt) RustBuffer {
	return LowerIntoRustBuffer[map[string]Receipt](c, value)
}

func (_ FfiConverterMapStringTypeReceipt) Write(writer io.Writer, mapValue map[string]Receipt) {
	if len(mapValue) > math.MaxInt32 {
		panic("map[string]Receipt is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(mapValue)))
	for key, value := range mapValue {
		FfiConverterStringINSTANCE.Write(writer, key)
		FfiConverterTypeReceiptINSTANCE.Write(writer, value)
	}
}

type FfiDestroyerMapStringTypeReceipt struct{}

func (_ FfiDestroyerMapStringTypeReceipt) Destroy(mapValue map[string]Receipt) {
	for key, value := range mapValue {
		FfiDestroyerString{}.Destroy(key)
		FfiDestroyerTypeReceipt{}.Destroy(value)
	}
}

type FfiConverterMapStringSequenceString struct{}

var FfiConverterMapStringSequenceStringINSTANCE = FfiConverterMapStringSequenceString{}

func (c FfiConverterMapStringSequenceString) Lift(rb RustBufferI) map[string][]string {
	return LiftFromRustBuffer[map[string][]string](c, rb)
}

func (_ FfiConverterMapStringSequenceString) Read(reader io.Reader) map[string][]string {
	result := make(map[string][]string)
	length := readInt32(reader)
	for i := int32(0); i < length; i++ {
		key := FfiConverterStringINSTANCE.Read(reader)
		value := FfiConverterSequenceStringINSTANCE.Read(reader)
		result[key] = value
	}
	return result
}

func (c FfiConverterMapStringSequenceString) Lower(value map[string][]string) RustBuffer {
	return LowerIntoRustBuffer[map[string][]string](c, value)
}

func (_ FfiConverterMapStringSequenceString) Write(writer io.Writer, mapValue map[string][]string) {
	if len(mapValue) > math.MaxInt32 {
		panic("map[string][]string is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(mapValue)))
	for key, value := range mapValue {
		FfiConverterStringINSTANCE.Write(writer, key)
		FfiConverterSequenceStringINSTANCE.Write(writer, value)
	}
}

type FfiDestroyerMapStringSequenceString struct{}

func (_ FfiDestroyerMapStringSequenceString) Destroy(mapValue map[string][]string) {
	for key, value := range mapValue {
		FfiDestroyerString{}.Destroy(key)
		FfiDestroyerSequenceString{}.Destroy(value)
	}
}

const (
	uniffiRustFuturePollReady      C.int8_t = 0
	uniffiRustFuturePollMaybeReady C.int8_t = 1
)

func uniffiRustCallAsync(
	rustFutureFunc func(*C.RustCallStatus) *C.void,
	pollFunc func(*C.void, unsafe.Pointer, *C.RustCallStatus),
	completeFunc func(*C.void, *C.RustCallStatus),
	_liftFunc func(bool),
	freeFunc func(*C.void, *C.RustCallStatus),
) {
	rustFuture, err := uniffiRustCallAsyncInner(nil, rustFutureFunc, pollFunc, freeFunc)
	if err != nil {
		panic(err)
	}
	defer rustCall(func(status *C.RustCallStatus) int {
		freeFunc(rustFuture, status)
		return 0
	})

	rustCall(func(status *C.RustCallStatus) int {
		completeFunc(rustFuture, status)
		return 0
	})
}

func uniffiRustCallAsyncWithResult[T any, U any](
	rustFutureFunc func(*C.RustCallStatus) *C.void,
	pollFunc func(*C.void, unsafe.Pointer, *C.RustCallStatus),
	completeFunc func(*C.void, *C.RustCallStatus) T,
	liftFunc func(T) U,
	freeFunc func(*C.void, *C.RustCallStatus),
) U {
	rustFuture, err := uniffiRustCallAsyncInner(nil, rustFutureFunc, pollFunc, freeFunc)
	if err != nil {
		panic(err)
	}

	defer rustCall(func(status *C.RustCallStatus) int {
		freeFunc(rustFuture, status)
		return 0
	})

	res := rustCall(func(status *C.RustCallStatus) T {
		return completeFunc(rustFuture, status)
	})
	return liftFunc(res)
}

func uniffiRustCallAsyncWithError(
	converter BufLifter[error],
	rustFutureFunc func(*C.RustCallStatus) *C.void,
	pollFunc func(*C.void, unsafe.Pointer, *C.RustCallStatus),
	completeFunc func(*C.void, *C.RustCallStatus),
	_liftFunc func(bool),
	freeFunc func(*C.void, *C.RustCallStatus),
) error {
	rustFuture, err := uniffiRustCallAsyncInner(converter, rustFutureFunc, pollFunc, freeFunc)
	if err != nil {
		return err
	}

	defer rustCall(func(status *C.RustCallStatus) int {
		freeFunc(rustFuture, status)
		return 0
	})

	_, err = rustCallWithError(converter, func(status *C.RustCallStatus) int {
		completeFunc(rustFuture, status)
		return 0
	})
	return err
}

func uniffiRustCallAsyncWithErrorAndResult[T any, U any](
	converter BufLifter[error],
	rustFutureFunc func(*C.RustCallStatus) *C.void,
	pollFunc func(*C.void, unsafe.Pointer, *C.RustCallStatus),
	completeFunc func(*C.void, *C.RustCallStatus) T,
	liftFunc func(T) U,
	freeFunc func(*C.void, *C.RustCallStatus),
) (U, error) {
	var returnValue U
	rustFuture, err := uniffiRustCallAsyncInner(converter, rustFutureFunc, pollFunc, freeFunc)
	if err != nil {
		return returnValue, err
	}

	defer rustCall(func(status *C.RustCallStatus) int {
		freeFunc(rustFuture, status)
		return 0
	})

	res, err := rustCallWithError(converter, func(status *C.RustCallStatus) T {
		return completeFunc(rustFuture, status)
	})
	if err != nil {
		return returnValue, err
	}
	return liftFunc(res), nil
}

func uniffiRustCallAsyncInner(
	converter BufLifter[error],
	rustFutureFunc func(*C.RustCallStatus) *C.void,
	pollFunc func(*C.void, unsafe.Pointer, *C.RustCallStatus),
	freeFunc func(*C.void, *C.RustCallStatus),
) (*C.void, error) {
	pollResult := C.int8_t(-1)
	waiter := make(chan C.int8_t, 1)
	chanHandle := cgo.NewHandle(waiter)

	rustFuture, err := rustCallWithError(converter, func(status *C.RustCallStatus) *C.void {
		return rustFutureFunc(status)
	})
	if err != nil {
		return nil, err
	}

	defer chanHandle.Delete()

	for pollResult != uniffiRustFuturePollReady {
		ptr := unsafe.Pointer(&chanHandle)
		_, err = rustCallWithError(converter, func(status *C.RustCallStatus) int {
			pollFunc(rustFuture, ptr, status)
			return 0
		})
		if err != nil {
			return nil, err
		}
		res := <-waiter
		pollResult = res
	}

	return rustFuture, nil
}

// Callback handlers for an async calls.  These are invoked by Rust when the future is ready.  They
// lift the return value or error and resume the suspended function.

//export uniffiFutureContinuationCallbackmatrix_sdk_ffi
func uniffiFutureContinuationCallbackmatrix_sdk_ffi(ptr unsafe.Pointer, pollResult C.int8_t) {
	doneHandle := *(*cgo.Handle)(ptr)
	done := doneHandle.Value().((chan C.int8_t))
	done <- pollResult
}

func uniffiInitContinuationCallback() {
	rustCall(func(uniffiStatus *C.RustCallStatus) bool {
		C.ffi_matrix_sdk_ffi_rust_future_continuation_callback_set(
			C.RustFutureContinuation(C.uniffiFutureContinuationCallbackmatrix_sdk_ffi),
			uniffiStatus,
		)
		return false
	})
}

func GenTransactionId() string {
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_func_gen_transaction_id(_uniffiStatus)
	}))
}

func GenerateWebviewUrl(widgetSettings WidgetSettings, room *Room, props ClientProperties) (string, error) {
	return uniffiRustCallAsyncWithErrorAndResult(
		FfiConverterTypeParseError{}, func(status *C.RustCallStatus) *C.void {
			// rustFutureFunc
			return (*C.void)(C.uniffi_matrix_sdk_ffi_fn_func_generate_webview_url(FfiConverterTypeWidgetSettingsINSTANCE.Lower(widgetSettings), FfiConverterRoomINSTANCE.Lower(room), FfiConverterTypeClientPropertiesINSTANCE.Lower(props),
				status,
			))
		},
		func(handle *C.void, ptr unsafe.Pointer, status *C.RustCallStatus) {
			// pollFunc
			C.ffi_matrix_sdk_ffi_rust_future_poll_rust_buffer(unsafe.Pointer(handle), ptr, status)
		},
		func(handle *C.void, status *C.RustCallStatus) RustBufferI {
			// completeFunc
			return C.ffi_matrix_sdk_ffi_rust_future_complete_rust_buffer(unsafe.Pointer(handle), status)
		},
		FfiConverterStringINSTANCE.Lift, func(rustFuture *C.void, status *C.RustCallStatus) {
			// freeFunc
			C.ffi_matrix_sdk_ffi_rust_future_free_rust_buffer(unsafe.Pointer(rustFuture), status)
		})
}

func GetElementCallRequiredPermissions() WidgetCapabilities {
	return FfiConverterTypeWidgetCapabilitiesINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_func_get_element_call_required_permissions(_uniffiStatus)
	}))
}

func LogEvent(file string, line *uint32, level LogLevel, target string, message string) {
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_func_log_event(FfiConverterStringINSTANCE.Lower(file), FfiConverterOptionalUint32INSTANCE.Lower(line), FfiConverterTypeLogLevelINSTANCE.Lower(level), FfiConverterStringINSTANCE.Lower(target), FfiConverterStringINSTANCE.Lower(message), _uniffiStatus)
		return false
	})
}

func MakeWidgetDriver(settings WidgetSettings) (WidgetDriverAndHandle, error) {
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeParseError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_func_make_widget_driver(FfiConverterTypeWidgetSettingsINSTANCE.Lower(settings), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue WidgetDriverAndHandle
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeWidgetDriverAndHandleINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func MediaSourceFromUrl(url string) *MediaSource {
	return FfiConverterMediaSourceINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_func_media_source_from_url(FfiConverterStringINSTANCE.Lower(url), _uniffiStatus)
	}))
}

func MessageEventContentFromHtml(body string, htmlBody string) *RoomMessageEventContentWithoutRelation {
	return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_func_message_event_content_from_html(FfiConverterStringINSTANCE.Lower(body), FfiConverterStringINSTANCE.Lower(htmlBody), _uniffiStatus)
	}))
}

func MessageEventContentFromHtmlAsEmote(body string, htmlBody string) *RoomMessageEventContentWithoutRelation {
	return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_func_message_event_content_from_html_as_emote(FfiConverterStringINSTANCE.Lower(body), FfiConverterStringINSTANCE.Lower(htmlBody), _uniffiStatus)
	}))
}

func MessageEventContentFromMarkdown(md string) *RoomMessageEventContentWithoutRelation {
	return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_func_message_event_content_from_markdown(FfiConverterStringINSTANCE.Lower(md), _uniffiStatus)
	}))
}

func MessageEventContentFromMarkdownAsEmote(md string) *RoomMessageEventContentWithoutRelation {
	return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_func_message_event_content_from_markdown_as_emote(FfiConverterStringINSTANCE.Lower(md), _uniffiStatus)
	}))
}

func MessageEventContentNew(msgtype MessageType) (*RoomMessageEventContentWithoutRelation, error) {
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeClientError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_matrix_sdk_ffi_fn_func_message_event_content_new(FfiConverterTypeMessageTypeINSTANCE.Lower(msgtype), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *RoomMessageEventContentWithoutRelation
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterRoomMessageEventContentWithoutRelationINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func NewVirtualElementCallWidget(props VirtualElementCallWidgetOptions) (WidgetSettings, error) {
	_uniffiRV, _uniffiErr := rustCallWithError(FfiConverterTypeParseError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_func_new_virtual_element_call_widget(FfiConverterTypeVirtualElementCallWidgetOptionsINSTANCE.Lower(props), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue WidgetSettings
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeWidgetSettingsINSTANCE.Lift(_uniffiRV), _uniffiErr
	}
}

func SdkGitSha() string {
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return C.uniffi_matrix_sdk_ffi_fn_func_sdk_git_sha(_uniffiStatus)
	}))
}

func SetupOtlpTracing(config OtlpTracingConfiguration) {
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_func_setup_otlp_tracing(FfiConverterTypeOtlpTracingConfigurationINSTANCE.Lower(config), _uniffiStatus)
		return false
	})
}

func SetupTracing(config TracingConfiguration) {
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_matrix_sdk_ffi_fn_func_setup_tracing(FfiConverterTypeTracingConfigurationINSTANCE.Lower(config), _uniffiStatus)
		return false
	})
}
