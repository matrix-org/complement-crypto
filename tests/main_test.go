package tests

import (
	"sync"
	"testing"

	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/deploy"
)

var (
	ssDeployment *deploy.SlidingSyncDeployment
	ssMutex      *sync.Mutex
)

func TestMain(m *testing.M) {
	ssMutex = &sync.Mutex{}
	defer func() { // always teardown even if panicking
		ssMutex.Lock()
		if ssDeployment != nil {
			ssDeployment.Teardown()
		}
		ssMutex.Unlock()
	}()
	complement.TestMain(m, "crypto")

}

func Deploy(t *testing.T) *deploy.SlidingSyncDeployment {
	ssMutex.Lock()
	defer ssMutex.Unlock()
	if ssDeployment != nil {
		return ssDeployment
	}
	ssDeployment = deploy.RunNewDeployment(t)
	return ssDeployment
}

/*
type timelineListener struct {
	fn func(diff []*matrix_sdk_ffi.TimelineDiff)
}

func (l *timelineListener) OnUpdate(diff []*matrix_sdk_ffi.TimelineDiff) {
	l.fn(diff)
} */
