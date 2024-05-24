package rust

import "github.com/matrix-org/complement-crypto/internal/api/rust/matrix_sdk_ffi"

type MemoryClientSessionDelegate struct {
	userIDToSession map[string]matrix_sdk_ffi.Session
}

func NewMemoryClientSessionDelegate() *MemoryClientSessionDelegate {
	return &MemoryClientSessionDelegate{
		userIDToSession: make(map[string]matrix_sdk_ffi.Session),
	}
}

func (d *MemoryClientSessionDelegate) RetrieveSessionFromKeychain(userID string) (matrix_sdk_ffi.Session, *matrix_sdk_ffi.ClientError) {
	s, exists := d.userIDToSession[userID]
	if !exists {
		return matrix_sdk_ffi.Session{}, matrix_sdk_ffi.NewClientErrorGeneric("Failed to find RestorationToken in the Keychain.")
	}
	return s, nil
}

func (d *MemoryClientSessionDelegate) SaveSessionInKeychain(session matrix_sdk_ffi.Session) {
	d.userIDToSession[session.UserId] = session
}
