//go:build jssdk

package langs

import (
	"fmt"
	"os"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

// Can't use tag name 'js' as that is used already for wasm bindings...
func init() {
	fmt.Println("Adding JS bindings")
	SetLangaugeBinding(api.ClientTypeJS, &JSLanguageBindings{})
}

type JSLanguageBindings struct{}

func (b *JSLanguageBindings) PreTestRun() {
	// nuke persistent storage from previous run. We do this on startup rather than teardown
	// to allow devs to introspect DBs / Chrome profiles if tests fail.
	os.RemoveAll("./chromedp")
	js.SetupJSLogs("./logs/js_sdk.log")
}

func (b *JSLanguageBindings) PostTestRun() {
	js.WriteJSLogs()
}

func (b *JSLanguageBindings) MustCreateClient(t ct.TestLike, cfg api.ClientCreationOpts) api.Client {
	client, err := js.NewJSClient(t, cfg)
	must.NotError(t, "NewJSClient: %s", err)
	return client
}
