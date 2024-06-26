package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement/ct"
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
				verifier.Logf(t, "Verifier(SENDER) %s %s", verifierClientType.Lang, verifier.Opts().DeviceID)
				verifiee.Logf(t, "Verifiee(RECEIVER) %s %s", verifieeClientType.Lang, verifiee.Opts().DeviceID)
				verifieeStage := verifiee.ListenForVerificationRequests(t)
				verifierStage := verifier.RequestOwnUserVerification(t)
				for {
					select {
					case receiverStage := <-verifieeStage:
						switch stage := receiverStage.(type) {
						case api.VerificationStageRequestedRequetee:
							t.Logf("[RECEIVER]VerificationStageRequestedRequetee: %+v", stage.Request())
							stage.Ready()
						case api.VerificationStageRequested:
							t.Logf("[RECEIVER]VerificationStageRequested: %+v", stage.Request())
						case api.VerificationStageReady:
							t.Logf("[RECEIVER]VerificationStageReady")
						case api.VerificationStageTransitioned:
							t.Logf("[RECEIVER]VerificationStageTransitioned")
							t.Logf("[RECEIVER] Emoji: %v Decimals: %v", stage.VerificationData().Emojis, stage.VerificationData().Decimals)
						case api.VerificationStageStart:
							t.Logf("[RECEIVER]VerificationStageStart")
							stage.Transition()
						case api.VerificationStageDone:
							t.Logf("[RECEIVER]VerificationStageDone")
							return
						case api.VerificationStageCancelled: // should not be cancelled
							ct.Errorf(t, "[RECEIVER]VerificationStageCancelled")
						}
					case senderStage := <-verifierStage:
						switch stage := senderStage.(type) {
						case api.VerificationStageRequestedRequetee: // the verifier should not get a requestee state
							ct.Errorf(t, "[SENDER]VerificationStageRequestedRequetee: %+v", stage.Request())
						case api.VerificationStageRequested:
							t.Logf("[SENDER]VerificationStageRequested: %+v", stage.Request())
						case api.VerificationStageReady:
							t.Logf("[SENDER]VerificationStageReady")
							t.Logf("[SENDER]VerificationStageReady: starting m.sas.v1")
							stage.Start("m.sas.v1")
						case api.VerificationStageTransitioned:
							t.Logf("[SENDER]VerificationStageTransitioned")
							t.Logf("[SENDER] Emoji: %v Decimals: %v", stage.VerificationData().Emojis, stage.VerificationData().Decimals)
						case api.VerificationStageStart:
							t.Logf("[SENDER]VerificationStageStart")
						case api.VerificationStageDone:
							t.Logf("[SENDER]VerificationStageDone")
							return
						case api.VerificationStageCancelled: // should not be cancelled
							ct.Errorf(t, "[SENDER]VerificationStageCancelled")
						}
					case <-time.After(5 * time.Second):
						ct.Fatalf(t, "timed out after 5s")
						return
					}
				}
			})
		})

	})
}
