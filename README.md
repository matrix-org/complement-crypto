## Complement-Crypto

Complement for Rust SDK crypto.

**EXPERIMENTAL: As of Nov 2023 this repo is under active development currently so things will break constantly.**


### What is it? Why?

Complement-Crypto extends the existing Complement test suite to support full end-to-end testing of the Rust SDK. End-to-end testing is defined at the FFI / JS SDK layer through to a real homeserver, a real sliding sync proxy, real federation, to another rust SDK on FFI / JS SDK.

Why:
- To detect "unable to decrypt" failures and add regression tests for them.
- To date, there exists no test suite which meets the scope of Complement-Crypto.

### How do I run it?

You need to build Rust SDK FFI bindings _and_ JS SDK before you can get this to run. You also need a Complement homeserver image. When that is setup:

```
COMPLEMENT_BASE_IMAGE=homeserver:latest go test -v ./tests
```

TODO: consider checking in working builds so you can git clone and run. Git LFS for `libmatrix_sdk_ffi.so` given it's 60MB?

### JS SDK

Prerequisites:
 - A working Yarn/npm installation (version?)

This repo has a self-hosted copy of `matrix-js-sdk` which it will run in a headless chrome, in order to mimic Element Web (Rust Crypto edition).

In order to regenerate the JS SDK, run `./rebuild_js_sdk.sh` with an appropriate version.

### FFI Bindings (TODO)

Prerequisites:
 - A working Rust installation (min version?)
 - A working Go installation (1.19+?)

This repo has bindings to the `matrix_sdk` crate in rust SDK, in order to mimic Element X.

In order to generate these bindings, follow these instructions:
- Check out https://github.com/matrix-org/matrix-rust-sdk/tree/kegan/complement-test-fork (TODO: go back to main when async fns work with bindgen)
- Get the bindings generator: (TODO: recheck if https://github.com/NordSecurity/uniffi-bindgen-go/pull/13 lands)
```
git clone https://github.com/dignifiedquire/uniffi-bindgen-go.git
cd uniffi-bindgen-go
git checkout upgarde-uniffi-24
git submodule init
git submodule update
cd ..
cargo install uniffi-bindgen-go --path ./uniffi-bindgen-go/bindgen
```
- Compile the rust SDK: `cargo build -p matrix-sdk-crypto-ffi -p matrix-sdk-ffi`. Check that `target/debug/libmatrix_sdk_ffi.a` exists.
- Generate the Go bindings to `./rust`: `uniffi-bindgen-go -l ../matrix-rust-sdk/target/debug/libmatrix_sdk_ffi.a -o ./rust ../matrix-rust-sdk/bindings/matrix-sdk-ffi/src/api.udl`
- Patch up the generated code as it's not quite right:
    * `sed -i '' 's/bindingsContractVersion := 23/bindingsContractVersion := 24/' rust/matrix_sdk_ffi/matrix_sdk_ffi.go`
    * Add `// #cgo LDFLAGS: -lmatrix_sdk_ffi` immediately after `// #include <matrix_sdk_ffi.h>` at the top of `matrix_sdk_ffi.go`.
    * Replace field names `Error` with `Error2` to fix `unknown field Error in struct literal`.
- Sanity check compile `LIBRARY_PATH="$LIBRARY_PATH:/path/to/matrix-rust-sdk/target/debug" go test -c ./tests`

