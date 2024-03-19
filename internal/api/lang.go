package api

import "github.com/matrix-org/complement/ct"

type ClientTypeLang string

var (
	ClientTypeRust ClientTypeLang = "rust"
	ClientTypeJS   ClientTypeLang = "js"
)

// LanguageBindings is the interface any new language implementation needs to satisfy to
// work with complement crypto.
type LanguageBindings interface {
	// PreTestRun is a hook which is executed before any tests run. This can be used to
	// clean up old log files from previous runs.
	PreTestRun()
	// PostTestRun is a hook which is executed after all tests have run. This can be used
	// to flush test logs.
	PostTestRun()
	// MustCreateClient is called to create a new client in this language. If the client cannot
	// be created, the test should be failed by calling ct.Fatalf(t, ...).
	MustCreateClient(t ct.TestLike, cfg ClientCreationOpts) Client
}
