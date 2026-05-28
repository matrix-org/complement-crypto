package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/langs"
)

// The config for running Complement Crypto. This is configured using environment variables. The comments
// in this struct are structured so they can be automatically parsed via gendoc. See /cmd/gendoc.
// There are additional configuration options available: see https://github.com/matrix-org/complement/blob/main/ENVIRONMENT.md
type ComplementCrypto struct {
	// Name: COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX
	// Default: jj,jr,rj,rr
	// Description: The client test matrix to run. Every test is run for each given permutation.
	// The default matrix tests all JS/Rust permutations _ignoring federation_.
	// ```
	// Valid values are:
	//  - `j`: Run a JS SDK client on hs1.
	//  - `r`: Run a Rust SDK FFI client on hs1.
	//  - `J`: Run a JS SDK client on hs2.
	//  - `R`: Run a Rust SDK FFI client on hs2.
	// ```
	// For example, for a simple "Alice and Bob" test:
	// ```
	//  - `rj,rr`: Run the test twice. Run 1: Alice=rust, Bob=JS. Run 2: Alice=rust, Bob=rust. All on HS1.
	//  - `jJ`: Run the test once. Run 1: Alice=JS on HS1, Bob=JS on HS2. Tests federation.
	// ```
	// If the matrix only consists of one letter (e.g all j's) then rust-specific tests will not run and vice versa.
	TestClientMatrix [][2]api.ClientType

	// Which languages should be tested in ForEachClientType tests.
	// Derived from TestClientMatrix
	clientLangs map[api.ClientTypeLang]bool

	// Name: COMPLEMENT_CRYPTO_MITMDUMP
	// Default: ""
	// Description: The path to dump the output from `mitmdump`. This file can then be used with mitmweb to view
	// all the HTTP flows in the test.
	MITMDump string

	// Name: COMPLEMENT_CRYPTO_RPC_BINARY
	// Default: ""
	// Description: The absolute path to the pre-built rpc binary file. This binary is generated via `go build -tags=jssdk,rust ./cmd/rpc`.
	// This binary is used when running multiprocess tests. If this environment variable is not supplied, tests which try to use multiprocess
	// clients will be skipped, making this environment variable optional.
	RPCBinaryPath string

	MITMProxyAddonsDir string
}

func (c *ComplementCrypto) ShouldTest(lang api.ClientTypeLang) bool {
	return c.clientLangs[lang]
}

// Bindings returns all the known language bindings for this particular complement-crypto configuration. Panics on
// unknown bindings.
func (c *ComplementCrypto) Bindings() []api.LanguageBindings {
	bindings := make([]api.LanguageBindings, 0, len(c.clientLangs))
	for l := range c.clientLangs {
		b := langs.GetLanguageBindings(l)
		if b == nil {
			panic("unknown language: " + l)
		}
		bindings = append(bindings, b)
	}
	return bindings
}

func NewComplementCryptoConfigFromEnvVars(relativePathToMITMAddonsDir string) *ComplementCrypto {
	matrix := os.Getenv("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX")
	if matrix == "" {
		matrix = "jj,jr,rj,rr"
	}
	segs := strings.Split(matrix, ",")
	clientLangs := make(map[api.ClientTypeLang]bool)
	var testClientMatrix [][2]api.ClientType
	for _, val := range segs { // e.g val == 'rj'
		if len(val) != 2 {
			panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX bad value: " + val)
		}
		testCase := [2]api.ClientType{}
		for i, ch := range val {
			switch ch {
			case 'r':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeRust,
					HS:   "hs1",
				}
				clientLangs[api.ClientTypeRust] = true
			case 'j':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeJS,
					HS:   "hs1",
				}
				clientLangs[api.ClientTypeJS] = true
			case 'J':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeJS,
					HS:   "hs2",
				}
				clientLangs[api.ClientTypeJS] = true
			case 'R':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeRust,
					HS:   "hs2",
				}
				clientLangs[api.ClientTypeRust] = true
			default:
				panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX bad value: " + val)
			}
		}
		testClientMatrix = append(testClientMatrix, testCase)
	}
	if len(testClientMatrix) == 0 {
		panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX: no tests will run as no matrix values are set")
	}
	rpcBinaryPath := os.Getenv("COMPLEMENT_CRYPTO_RPC_BINARY")
	if rpcBinaryPath != "" {
		if _, err := os.Stat(rpcBinaryPath); err != nil {
			panic("COMPLEMENT_CRYPTO_RPC_BINARY must be the absolute path to a binary file: " + err.Error())
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		panic("Cannot get current working directory: " + err.Error())
	}

	return &ComplementCrypto{
		MITMDump:           os.Getenv("COMPLEMENT_CRYPTO_MITMDUMP"),
		RPCBinaryPath:      rpcBinaryPath,
		TestClientMatrix:   testClientMatrix,
		clientLangs:        clientLangs,
		MITMProxyAddonsDir: filepath.Join(wd, relativePathToMITMAddonsDir),
	}
}
