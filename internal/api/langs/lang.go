// package langs contains language binding implementations
//
// This package would ideally be part of the `api` package but we can't do that
// without creating circular imports, so here we are.
package langs

import (
	"github.com/matrix-org/complement-crypto/internal/api"
)

// this map is populated _at runtime_ with known languages. It is custom to do this inside a func init()
// for the language in question. Each language MUST be specified in a separate file with a custom build
// tag to allow for conditional builds (we don't want to build your language unless the test runner needs it!)
//
// See the existing bindings for examples on how to do this.
var knownLanguages map[api.ClientTypeLang]api.LanguageBindings = map[api.ClientTypeLang]api.LanguageBindings{}

// SetLanguageBinding sets language bindings for the given language. Last write wins
// if the same language is given more than once.
func SetLanguageBinding(l api.ClientTypeLang, b api.LanguageBindings) {
	knownLanguages[l] = b
}

// GetLanguageBindings returns the language bindings for the given language, or nil if it doesn't exist.
func GetLanguageBindings(l api.ClientTypeLang) api.LanguageBindings {
	return knownLanguages[l]
}
