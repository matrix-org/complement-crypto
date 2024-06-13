package tests

import (
	"testing"

	"github.com/matrix-org/complement-crypto/internal/cc"
	"github.com/matrix-org/complement-crypto/internal/config"
)

// globals to ensure we are always referring to the same set of HSes/proxies between tests
var (
	instance *cc.Instance
)

// Main entry point when users run `go test`. Defined in https://pkg.go.dev/testing#hdr-Main
func TestMain(m *testing.M) {
	instance = cc.NewInstance(config.NewComplementCryptoConfigFromEnvVars())
	instance.TestMain(m)
}

// Instance returns the test instance. Guaranteed to be non-nil if called in a test,
// because TestMain would have been called before the test runs.
func Instance() *cc.Instance {
	return instance
}
