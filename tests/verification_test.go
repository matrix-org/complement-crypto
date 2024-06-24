package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/cc"
)

// happy case test of Alice verifying one of her devices.
func TestVerificationSAS(t *testing.T) {
	Instance().ClientTypeMatrix(t, func(t *testing.T, verifierClientType, verifieeClientType api.ClientType) {
		if verifieeClientType.Lang == api.ClientTypeRust {
			t.Skipf("rust cannot be a verifiee yet, see https://github.com/matrix-org/matrix-rust-sdk/issues/3595")
		}
		tc := Instance().CreateTestContext(t, verifierClientType)
		verifieeUser := &cc.User{
			CSAPI:      tc.Alice.CSAPI,
			ClientType: verifieeClientType,
		}

		tc.WithAliceSyncing(t, func(verifier api.Client) {
			tc.WithClientSyncing(t, &cc.ClientCreationRequest{
				User: verifieeUser,
				Opts: api.ClientCreationOpts{
					DeviceID: "OTHER_DEVICE",
				},
			}, func(verifiee api.Client) {
				verifier.RequestOwnUserVerification(t, &VerificationListener{})
			})
		})
		time.Sleep(time.Second)
	})
}

type VerificationListener struct {
	onVerificationStateChange    func(vState api.VerificationState)
	didAcceptVerificationRequest func()
	didReceiveVerificationData   func(vData api.VerificationData)
	didFail                      func()
	didCancel                    func()
	didFinish                    func()
}

func (v *VerificationListener) Close() {}
func (v *VerificationListener) OnVerificationStateChange(vState api.VerificationState) {
	if v.onVerificationStateChange != nil {
		v.onVerificationStateChange(vState)
	}
}
func (v *VerificationListener) DidAcceptVerificationRequest() {
	if v.didAcceptVerificationRequest != nil {
		v.didAcceptVerificationRequest()
	}
}
func (v *VerificationListener) DidReceiveVerificationData(vData api.VerificationData) {
	if v.didReceiveVerificationData != nil {
		v.didReceiveVerificationData(vData)
	}
}
func (v *VerificationListener) DidFail() {
	if v.didFail != nil {
		v.didFail()
	}
}
func (v *VerificationListener) DidCancel() {
	if v.didCancel != nil {
		v.didCancel()
	}
}
func (v *VerificationListener) DidFinish() {
	if v.didFinish != nil {
		v.didFinish()
	}
}
