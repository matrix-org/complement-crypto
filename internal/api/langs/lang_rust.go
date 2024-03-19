//go:build rust

package langs

import (
	"fmt"
	"os"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

func init() {
	fmt.Println("Adding Rust bindings")
	SetLangaugeBinding(api.ClientTypeRust, &RustLanguageBindings{})
}

type RustLanguageBindings struct{}

func (b *RustLanguageBindings) PreTestRun() {
	// nuke persistent storage from previous run. We do this on startup rather than teardown
	// to allow devs to introspect DBs / Chrome profiles if tests fail.
	os.RemoveAll("./rust_storage")
	rust.DeleteOldLogs("rust_sdk_logs")
	rust.DeleteOldLogs("rust_sdk_inline_script")
	rust.SetupLogs("rust_sdk_logs")
}

func (b *RustLanguageBindings) PostTestRun() {
}

func (b *RustLanguageBindings) MustCreateClient(t ct.TestLike, cfg api.ClientCreationOpts) api.Client {
	client, err := rust.NewRustClient(t, cfg)
	must.NotError(t, "NewRustClient: %s", err)
	return client
}
