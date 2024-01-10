## How do I...

### Debug failing tests

If tests fail in CI, all log files are uploaded as GHA artifacts.

Most of the time, failing tests will have the failure reason in red on the console output, along with the line number which is where the test got up to before failing:
```
    key_backup_test.go:80: Equal bob's new device failed to decrypt the event: bad backup?: got 'true' want 'false'
    key_backup_test.go:81: Equal bob's new device failed to see the clear text message: got '' want 'An encrypted message'
```
Find the test name this relates to (and permutation). In this case `TestCanBackupKeys`.

SDK log files include logs for all tests run in one big file. The first thing to do is to identify the section of log lines which relate to the failing test. `grep` for the test name in JS SDK and Rust SDK logs:
```
grep 'TestCanBackupKeys' tests/js_sdk.log 
15:12:57.618132Z [http://127.0.0.1:50171,@user-11-alice:hs1] console.log TestCanBackupKeys/{rust_hs1}|{js_hs1}: [@user-11-alice:hs1](js) MustLoadBackup key=EsUG i4V4 71Js 6rBH XMwG CqJu rDQJ qLog bNJg uwHz v2TT r2Pj
...
```
Now you can look around those log lines for any warnings/errors or unexpected behaviour.

Sometimes the bug cannot be found via log files alone. You may want to see server logs. To do this, [enable writing container logs](https://github.com/matrix-org/complement-crypto/blob/main/ENVIRONMENT.md#complement_crypto_write_container_logs) then re-run the test. 

Sometimes, even that isn't enough. Perhaps server logs aren't giving enough information. In that case, [enable tcpdump](https://github.com/matrix-org/complement-crypto/blob/main/ENVIRONMENT.md#complement_crypto_tcpdump) and open the `.pcap` file in Wireshark to see the raw HTTP request/responses made by all clients.

If you need to add console logging to clients, see below.


### JS SDK

#### Regenerate bindings
*Why: if you want to test a different version of the JS SDK you need to rebuild the HTML/JS files.*

Prerequisites:
 - A working Yarn/npm installation (version?)

This repo has a self-hosted copy of `matrix-js-sdk` which it will run in a headless chrome, in order to mimic Element Web (Rust Crypto edition).

In order to regenerate the JS SDK, run `./rebuild_js_sdk.sh` with an appropriate version. Run the script without any args to see possible options.

Internally, we use Vite to bundle JS SDK into a single page app, which has no UI and merely sets `window.matrix = sdk;` so tests can create clients. It also sets some required helper functions (notably a `Buffer` implementation for key backups).

#### Add console logs

If you want to add console logging to the JS SDK, it is easiest to _modify the bundled output_ as it is not minified. To do this, `grep` for function names in `internal/api/dist/assests/index.....js` then use an editor to add `console.log` lines. These lines will appear in JS SDK log files.

### Rust SDK FFI

#### Regenerate FFI Bindings
*Why: if you want to test a different version of the rust SDK you need to regenerate FFI bindings.*

Prerequisites:
 - A working Rust installation (1.72+)
 - A working Go installation (1.19+?)

This repo has bindings to the `matrix_sdk` crate in rust SDK, in order to mimic Element X.

In order to generate these bindings, follow these instructions:
- Check out https://github.com/matrix-org/matrix-rust-sdk/tree/kegan/complement-crypto (TODO: go back to main when
main uses a versioned uniffi release e.g 0.25.2)
- Get the bindings generator:
```
git clone https://github.com/NordSecurity/uniffi-bindgen-go.git
cd uniffi-bindgen-go
git submodule init
git submodule update
cd ..
cargo install uniffi-bindgen-go --path ./uniffi-bindgen-go/bindgen
```
- Compile the rust SDK: `cargo build -p matrix-sdk-ffi`. Check that `target/debug/libmatrix_sdk_ffi.a` exists.
- Generate the Go bindings to `./rust`: `uniffi-bindgen-go -l ../matrix-rust-sdk/target/debug/libmatrix_sdk_ffi.a -o ./rust ../matrix-rust-sdk/bindings/matrix-sdk-ffi/src/api.udl`
- Patch up the generated code as it's not quite right:
    * Add `// #cgo LDFLAGS: -lmatrix_sdk_ffi` immediately after `// #include <matrix_sdk_ffi.h>` at the top of `matrix_sdk_ffi.go`.
    * https://github.com/NordSecurity/uniffi-bindgen-go/issues/36
- Sanity check compile `LIBRARY_PATH="$LIBRARY_PATH:/path/to/matrix-rust-sdk/target/debug" go test -c ./tests`

#### Add console logs

You need a local checkout of `matrix-rust-sdk` and be on the correct branch. You then need to be able to regenerate FFI bindings (see above). Modify the rust source and then regenerate bindings each time, ensuring the resulting `.a` files are on the `LIBRARY_PATH`.