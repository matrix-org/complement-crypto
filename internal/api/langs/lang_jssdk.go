//go:build jssdk

package langs

// Can't use tag name 'js' as that is used already for wasm bindings...
import (
	"fmt"
	"os"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/js"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/must"
)

func init() {
	fmt.Println("Adding JS bindings")
	SetLanguageBinding(api.ClientTypeJS, &JSLanguageBindings{})
}

type JSLanguageBindings struct{}

func (b *JSLanguageBindings) PreTestRun(contextID string) {
	// nuke persistent storage from previous run. We do this on startup rather than teardown
	// to allow devs to introspect DBs / Chrome profiles if tests fail.
	if contextID == "" {
		os.RemoveAll("./chromedp")
	}
	if contextID != "" {
		contextID = "_" + contextID
	}
	js.SetupJSLogs(fmt.Sprintf("./logs/js_sdk%s.log", contextID))
}

func (b *JSLanguageBindings) PostTestRun(contextID string) {
	js.WriteJSLogs()
}

func (b *JSLanguageBindings) MustCreateClient(t ct.TestLike, cfg api.ClientCreationOpts) api.Client {
	client, err := js.NewJSClient(t, cfg)
	must.NotError(t, "NewJSClient: %s", err)
	return client
}
