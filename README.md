## Complement-Crypto

Complement for Rust SDK crypto.


### What is it? Why?

Complement-Crypto extends the existing Complement test suite to support full end-to-end testing of the Rust SDK. End-to-end testing is defined at the FFI / JS SDK layer through to a real homeserver, a real sliding sync proxy, real federation, to another rust SDK on FFI / JS SDK.

Why:
- To detect "unable to decrypt" failures and add regression tests for them.
- To date, there exists no test suite which meets the scope of Complement-Crypto.

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
- Compile the rust SDK: `cargo xtask ci bindings`. Check that `target/debug/libmatrix_sdk_ffi.a` exists.
- Generate the Go bindings to `./sdk`: `uniffi-bindgen-go -l ../matrix-rust-sdk/target/debug/libmatrix_sdk_ffi.a -o ./sdk ../matrix-rust-sdk/bindings/matrix-sdk-ffi/src/api.udl`
- Patch up the generated code as it's not quite right:
```
TODO
```
- Sanity check compile `LIBRARY_PATH=/path/to/matrix-rust-sdk/target/debug go test -c ./tests`

