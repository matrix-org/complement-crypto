## Complement-Crypto
*As of May 2024 this project is being used in CI for existing SDKs. As such, it has a stable API exposed via a Github Action.*

Complement Crypto is an end-to-end test suite for next generation Matrix _clients_, designed to test the full spectrum of E2EE APIs.
It currently tests [rust SDK FFI bindings](https://github.com/matrix-org/matrix-rust-sdk/tree/main/bindings/matrix-sdk-ffi) and
[JS SDK](https://github.com/matrix-org/matrix-js-sdk/).

### Installing

*Please ensure you have met Complement's [Dependencies](https://github.com/matrix-org/complement?tab=readme-ov-file#dependencies) first.
In practice, this means you must have `go`, `docker` and `libolm` installed.*

Complement Crypto can be compiled and run in different modes depending on which SDK is being tested. For example, if you only want
to test JS SDK then you do not need to compile rust code or run rust tests, and vice versa. Conversely, if you want to test
interoperability between the two SDKs then you need to compile both SDKs.

#### JS SDK

Run `./rebuild_js_sdk.sh` according to its help page:
```
Rebuild the version of JS SDK used. (requires on PATH: yarn)
Usage: ./rebuild_js_sdk.sh [version]
  [version]: the yarn/npm package to use. This is fed directly into 'yarn add' so branches/commits can be used

Examples:
  Install a released version: ./rebuild_js_sdk.sh matrix-js-sdk@29.1.0
  Install develop branch:     ./rebuild_js_sdk.sh matrix-js-sdk@https://github.com/matrix-org/matrix-js-sdk#develop
  Install specific commit:    ./rebuild_js_sdk.sh matrix-js-sdk@https://github.com/matrix-org/matrix-js-sdk#36c958642cda08d32bc19c2303ebdfca470d03c1
```

#### Rust SDK

Pre-requisites:
 - `cargo` installed and on your PATH
 - Install uniffi bindings for Go: `./install_uniffi_bindgen_go.sh` - ensure `uniffi-bindgen-go` is on your PATH.

Run `./rebuild_rust_sdk.sh` according to its help page:
```
Rebuild the version of rust SDK used. Execute this inside the complement-crypto directory. (requires on PATH: uniffi-bindgen-go, cargo, git)
Usage: ./rebuild_rust_sdk.sh [version|directory]
  [version]: the rust SDK git repo and branch|tag to use. Syntax: '$HTTPS_URL@$TAG|$BRANCH'
             Stores repository in $PWD/_temp_rust_sdk
  [directory]: the local rust SDK checkout to use.

Examples:
  Install main branch:  ./rebuild_rust_sdk.sh https://github.com/matrix-org/matrix-rust-sdk@main
  Install 0.7.1 tag:    ./rebuild_rust_sdk.sh https://github.com/matrix-org/matrix-rust-sdk@0.7.1
  Install ./rust-sdk    ./rebuild_rust_sdk.sh ./rust-sdk

[directory] is determined if the first character is a '.' or '/'. If neither, it is assumed to be a [version]
The [version] is split into the URL and TAG|BRANCH then fed directly into 'git clone --depth 1 --branch <tag_name> <repo_url>'
```

### Running

Find a complement-compatible homeserver image. If you don't care which image is used, use `ghcr.io/matrix-org/synapse-service:v1.94.0` 
which will Just Work out-of-the-box.

To run only rust tests:
```
COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX=rr \
COMPLEMENT_BASE_IMAGE=ghcr.io/matrix-org/synapse-service:v1.94.0 \
LIBRARY_PATH=$LIBRARY_PATH:/path/to/matrix-rust-sdk/target/debug \
go test -v -count=1 -tags=rust -timeout 15m ./tests
```

To run only JS tests:
```
COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX=jj \
COMPLEMENT_BASE_IMAGE=ghcr.io/matrix-org/synapse-service:v1.94.0 \
go test -v -count=1 -tags=jssdk -timeout 15m ./tests
```

`COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX` controls which SDK is used to create test clients, and the `-tags` option
controls conditional compilation so other SDKs don't need to be compiled for the tests to run.

To test interoperability between the SDKs, `tcpdump` the traffic, run extra multiprocess tests and more,
see [ENVIRONMENT.md](ENVIRONMENT.md) for the full configuration options.

*See [FAQ.md](FAQ.md) for more information around debugging.*

### Test hitlist
There is an exhaustive set of tests that this repository aims to exercise. See [TEST_HITLIST.md](TEST_HITLIST.md).

### Architecture

Tests sometimes require reverse proxy interception to let some requests pass through but not others. For this, we use [mitmproxy](https://mitmproxy.org/).

```
     Host        |       dockerd           
                 |                          +-----------+      
                 |                     .--> | ss proxy1 | <------.
 +----------+    |    +-----------+    |    +-----+-----+        V
 | Go tests | <--|--> | mitmproxy | <--+--> | hs1 |          +----------+
 +----------+    |    +-----------+    |    +-----+          | postgres |
                 |                     +--> | hs2 |          +----------+
                 |                     |    +-----+-----+        ^
                 |                     `--> | ss proxy2 | <------`
                 |                          +-----------+      
```

TODO: flesh out mitm controller API

### Rationale

Complement-Crypto extends the existing Complement test suite to support full end-to-end testing of the Matrix Rust SDK. End-to-end testing is defined at the FFI / JS SDK layer through to a real homeserver, a real sliding sync proxy, real federation, to another rust SDK on FFI / JS SDK.

Why:
- To detect "unable to decrypt" failures and *add regression tests* for them.
- To ensure cross-client compatibility (e.g mobile clients work with web clients and vice versa).
- To enable new kinds of security tests (active attacker tests)

Goals:
- Must work under Github Actions / Gitlab CI/CD.
- Must be fast (where fast is no slower than the slowest action in CI, in other words it shouldn't be slowing down existing workflows).
- Must be able to test next-gen clients Element X and Element-Web R (Rust crypto).
- Must be able to test Synapse.
- Must be able to test the full spectrum of E2EE tasks (key backups, x-signing, etc)
- Should be able to test over real federation.
- Should be able to manipulate network conditions.
- Should be able to manipulate program state (e.g restart, sigkill, clear storage).
- Could test other homeservers than Synapse.
- Could test other clients than rust SDK backed ones.
- Could provide benchmarking/performance testing.

Anti-Goals:
- Esoteric test edge cases e.g Synapse worker race conditions, FFI concurrency control issues. For these, a unit test in the respective project would be more appropriate.
- UI testing. This is not a goal because it slows down tests, is less portable e.g needs emulators and is usually significantly more flakey than no-UI tests.


### Github Action

To run tests for a single SDK, insert this action:

```yaml
    complement-crypto:
        name: "Run Complement Crypto tests"
        uses: matrix-org/complement-crypto/.github/workflows/single_sdk_tests.yml@main
        with:
            use_js_sdk: "." # example is for JS SDK, use the current checkout
```

The complete options for `with:` are as follows:
 - `use_js_sdk`: Controls the source location of the JS SDK. Provide either a tag, commit or branch on `matrix-org/matrix-js-sdk`. Alternatively, if you have a local checkout somewhere, specify the path of the checkout e.g `.` for the working directory, or `/full/path/to/js/sdk`. Relative paths aren't supported.
 - `use_rust_sdk`: same as `use_js_sdk` but for the rust SDK.
 - `use_complement_crypto`: same as `use_js_sdk` but for Complement Crypto itself. Defaults to `main`.
