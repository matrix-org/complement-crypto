## Complement-Crypto

Complement for Rust SDK crypto.

**EXPERIMENTAL: As of Nov 2023 this repo is under active development currently so things will break constantly.**


### What is it? Why?

Complement-Crypto extends the existing Complement test suite to support full end-to-end testing of the Rust SDK. End-to-end testing is defined at the FFI / JS SDK layer through to a real homeserver, a real sliding sync proxy, real federation, to another rust SDK on FFI / JS SDK.

Why:
- To detect "unable to decrypt" failures and add regression tests for them.
- To date, there exists no test suite which meets the scope of Complement-Crypto.

### How do I run it?
It's currently pretty awful to run, as you need toolchains for both Rust and JS. Working on improving this.

You need to build Rust SDK FFI bindings _and_ JS SDK before you can get this to run. You also need a Complement homeserver image. When that is setup:

```
COMPLEMENT_BASE_IMAGE=homeserver:latest go test -v ./tests
```

TODO: consider checking in working builds so you can git clone and run. Git LFS for `libmatrix_sdk_ffi.so` given it's 60MB?

If you get failing tests, JS SDK logs to file as does Rust SDK in `./tests`, which are uploaded to GHA as artifacts. You can then grep for the test name to find the right place in the file e.g `grep 'TestAliceBobEncryptionWorks/js|js' tests/js_sdk.log`.

#### Environment Variables

- `COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX` : Comma separated clients to run. Default: `jj,jr,rj,rr`
   Control which kinds of clients to make for tests. `r` creates Rust client. `j` creates JS clients. The default runs all permutations.
- `COMPLEMENT_CRYPTO_TCPDUMP`: If `1`, will execute `tcpdump` automatically when the servers are running on the dynamic ports being used. To run this, `go test` must be run with `sudo -E` (preserve env vars for configuration) e.g `COMPLEMENT_BASE_IMAGE=homeserver:latest sudo -E go test ./tests`. Listens for synapse and sliding sync traffic.


### Test hitlist
There is an exhaustive set of tests that this repository aims to exercise which are below:

Membership ACLs:
- [x] Happy case Alice and Bob in an encrypted room can send and receive encrypted messages, and decrypt them all.
- [x] Bob can see messages when he was invited but not joined to the room. Subsequent messages are also decryptable.
- [x] In a public, `shared` history visibility room, a new user Bob cannot decrypt earlier messages prior to his join, despite being able to see the events. Subsequent messages are decryptable.
- [x] Bob leaves the room. Some messages are sent. Bob rejoins and cannot decrypt the messages sent whilst he was gone (ensuring we cycle keys).
- [x] Bob cannot decrypt older messages when logging in on a new device. When the device is logged out and in again, Bob cannot decrypt messages sent whilst he was logged out.
- [x] EXPECTED FAIL: Alice invites Bob, Alice sends a message, Bob changes their device, then Bob joins. Bob should be able to see Alice's message.

Key backups:
- [ ] New device for Alice cannot decrypt previous messages.
- [ ] Backups can be made on Alice's first device.
- [ ] Alice's new device can download the backup and decrypt the messages.

One-time Keys:
- [ ] When Alice runs out of OTKs, other users use the fallback key.
- [ ] Ensure things don't explode if OTKs are reused (TODO: what should happen here?)

Key Verification: (Short Authentication String)
- [ ] Happy case Alice <-> Bob key verification.
- [ ] Happy case Alice <-> Bob key verification over federation.
- [ ] Happy case Alice <-> Alice key verification (different devices).
- [ ] A MITMed key fails key verification.
- [ ] Repeat all of the above, but for QR code. (render QR code to png then rescan).
- [ ] Repeat all of the above, but for Emoji representations of SAS.
- [ ] Verification can be cancelled.
- [ ] Verification can be cancelled over federation.

Network connectivity:
- [ ] If a client cannot upload OTKs, it retries.
- [ ] If a client cannot claim OTKs, it retries.
- [ ] If a server cannot send device list updates over federation, it retries.
- [ ] If a client cannot query device keys for a user, it retries.
- [ ] If a server cannot query device keys on another server, it retries.
- [ ] If a client cannot send a to-device msg, it retries.
- [ ] If a server cannot send a to-device msg to another server, it retries.
- [ ] Repeat all of the above, but restart the client|server after the initial connection failure. This checks that retries aren't just stored in memory but persisted to disk.

Network connectivity tests are extremely time sensitive as retries are often using timeouts in clients.

### Regenerate JS SDK

Prerequisites:
 - A working Yarn/npm installation (version?)

This repo has a self-hosted copy of `matrix-js-sdk` which it will run in a headless chrome, in order to mimic Element Web (Rust Crypto edition).

In order to regenerate the JS SDK, run `./rebuild_js_sdk.sh` with an appropriate version.

TODO: Dockerify this so developers don't _need_ an active npm install?

### Regenerate FFI Bindings

Prerequisites:
 - A working Rust installation (min version?)
 - A working Go installation (1.19+?)

This repo has bindings to the `matrix_sdk` crate in rust SDK, in order to mimic Element X.

In order to generate these bindings, follow these instructions:
- Check out https://github.com/matrix-org/matrix-rust-sdk/tree/kegan/complement-test-fork (TODO: go back to main when `_ffi_matrix_sdk_ffi_rust_future_continuation_callback_set` is defined)
- Get the bindings generator: (TODO: recheck if https://github.com/NordSecurity/uniffi-bindgen-go/pull/26 lands)
```
git clone https://github.com/dignifiedquire/uniffi-bindgen-go.git
cd uniffi-bindgen-go
git checkout upgrade-uniffi-25
git submodule init
git submodule update
cd ..
cargo install uniffi-bindgen-go --path ./uniffi-bindgen-go/bindgen
```
- Compile the rust SDK: `cargo build -p matrix-sdk-crypto-ffi -p matrix-sdk-ffi`. Check that `target/debug/libmatrix_sdk_ffi.a` exists.
- Generate the Go bindings to `./rust`: `uniffi-bindgen-go -l ../matrix-rust-sdk/target/debug/libmatrix_sdk_ffi.a -o ./rust ../matrix-rust-sdk/bindings/matrix-sdk-ffi/src/api.udl`
- Patch up the generated code as it's not quite right:
    * Add `// #cgo LDFLAGS: -lmatrix_sdk_ffi` immediately after `// #include <matrix_sdk_ffi.h>` at the top of `matrix_sdk_ffi.go`.
    * Replace field names `Error` with `Error2` to fix `unknown field Error in struct literal`.
- Sanity check compile `LIBRARY_PATH="$LIBRARY_PATH:/path/to/matrix-rust-sdk/target/debug" go test -c ./tests`


### Github Action (TODO)

Inputs:
 - version/commit/branch of JS SDK
 - version/commit/branch of Rust SDK
 - version/commit/branch of synapse?
 - Test only JS, only Rust, mixed.
