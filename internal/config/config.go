package config

import (
	"os"
	"strings"

	"github.com/matrix-org/complement-crypto/internal/api"
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
	//  - `R`: Run a Rust SDK FFI client on hs2. TODO: needs additional SS proxy / postgres.
	// ```
	// For example, for a simple "Alice and Bob" test:
	// ```
	//  - `rj,rr`: Run the test twice. Run 1: Alice=rust, Bob=JS. Run 2: Alice=rust, Bob=rust. All on HS1.
	//  - `jJ`: Run the test once. Run 1: Alice=JS on HS1, Bob=JS on HS2. Tests federation.
	// ```
	TestClientMatrix [][2]api.ClientType

	// Name: COMPLEMENT_CRYPTO_TCPDUMP
	// Default: 0
	// Description: If 1, automatically attempts to run `tcpdump` when the containers are running. Stops dumping when
	// tests complete. This will probably require you to run `go test` with `sudo -E`. The `.pcap` file is written to
	// `tests/test.pcap`.
	TCPDump bool
}

func NewComplementCryptoConfigFromEnvVars() *ComplementCrypto {
	matrix := os.Getenv("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX")
	if matrix == "" {
		matrix = "jj,jr,rj,rr"
	}
	segs := strings.Split(matrix, ",")
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
			case 'j':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeJS,
					HS:   "hs1",
				}
			case 'J':
				testCase[i] = api.ClientType{
					Lang: api.ClientTypeJS,
					HS:   "hs2",
				}
			// TODO: case 'R': requires 2x sliding syncs / postgres
			default:
				panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX bad value: " + val)
			}
		}
		testClientMatrix = append(testClientMatrix, testCase)
	}
	if len(testClientMatrix) == 0 {
		panic("COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX: no tests will run as no matrix values are set")
	}
	return &ComplementCrypto{
		TCPDump:          os.Getenv("COMPLEMENT_CRYPTO_TCPDUMP") == "1",
		TestClientMatrix: testClientMatrix,
	}
}
