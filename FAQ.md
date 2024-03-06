## How do I...

### Find logs for CI runs

Ensure your firewall allows containers to talk to the host: https://github.com/matrix-org/complement-crypto/issues/13#issuecomment-1973203807

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

### Add some logs to figure out what is happening

See "Changing the JavaScript directly" and "Changing the JavaScript directly"
under "Modify the client code".

### Use a different version of matrix-js-sdk

Prerequisites:
 - A working Yarn/npm installation (version?)

This repo has a self-hosted copy of `matrix-js-sdk` which it will run in a headless chrome, in order to mimic Element Web (Rust Crypto edition).

In order to regenerate the JS SDK, run `./rebuild_js_sdk.sh` with an appropriate version. Run the script without any args to see possible options.

Internally, we use Vite to bundle JS SDK into a single page app, which has no UI and merely sets `window.matrix = sdk;` so tests can create clients. It also sets some required helper functions (notably a `Buffer` implementation for key backups).

### Modify the client code
*Why: if you want to play with changes to the clients, or add logging
information, you will need to modify the client code and rebuild it to make sure
it is running inside the tests.*

#### Changing the JavaScript directly (the easy way)

If you just need to add some logging or make small changes, you can modify the
bundled JavaScript code directly. Just edit the file:

```
internal/api/js/chrome/dist/assets/index-*.js
```

You can search in this file for the function you are interested in. If you add
`console.log` lines here, the output should show up in `tests/logs/js_sdk.log`.

Once you've changed the JavaScript, you can re-run your tests immediately and
your changed code will run. You don't even need to use `count=1` to force the
tests to re-run, because the framework will detect your changes.

(Notice that `index-*.js` contains the compiled WASM in a variable called
`matrix_sdk_crypto_wasm_bg_wasm`, but because it's compiled, it's not in a form
that we can easily edit.)

#### Changing the native Rust directly (also fairly easy)

If you need to try out changes to Rust code (e.g. to add logging) and you don't
mind your changes only applying to the native Rust (and NOT applying to Rust
within the JavaScript client, which is compiled to WASM) then you can modify it
in-place and then recompile it.

Edit files inside the `rust-sdk` directory (which is just a copy of
a specific version of `matrix-rust-sdk`) and then `cargo build -p
matrix-sdk-ffi` inside that directory.

Make sure you launch the tests with LIBRARY_PATH pointing to
`rust-sdk/target/debug` so that the built code gets used.

Make sure you add `-count=1` on the command line when you re-run the tests,
because changes to the rust here won't trigger the framework to re-execute a
test that it has already run.

#### Using your local matrix-js-sdk

If you want to try out changes within a local `matrix-js-sdk`, you need to
perform several steps:

1. Clone the matrix-js-sdk repo:

    ```
    cd code
    git clone https://github.com/matrix-org/matrix-js-sdk.git
    ```

    and make any changes you want to make.

2. Change `internal/api/js/js-sdk/package.json` to say e.g.:

    ```
    "matrix-js-sdk": "file:/home/andy/code/matrix-js-sdk",
    ```

    in place of the existing `matrix-js-sdk` line in that file.

    Note: `yarn link` did NOT work for me (AndyB) - I had to use a `file:` URL.
    It can be relative if you prefer: this will be relative to the location of
    `package.json`.

    Further note: you can't use `yarn link` within matrix-js-sdk to refer to its
    dependencies either - I had to change to use `file:` URLs there too.

3. Rebuild the JavaScript project:

    ```
    cd internal/api/js/js-sdk
    yarn install && yarn build
    ```

    This creates files inside `internal/api/js/js_sdk/dist`.

4. Copy the built code into `chrome/dist`:

    ```
    cp -r internal/api/js/js-sdk/dist/. internal/api/js/chrome/dist
    ```

Now you can re-run your tests and see the effect of your changes.

#### Using your local matrix-rust-sdk-crypto-wasm and matrix-rust-sdk

If you want to make changes to the Rust code and see the effect in the
JavaScript tests, then you need to rebuild the WASM. Follow these steps:

1. Perform the steps from "Using your local matrix-js-sdk" above.

    So that you have a local matrix-js-sdk that you can use to bundle the WASM
    you build into `internal/api/js/chrome/dist`.

    Make sure this step is working, perhaps by adding some log lines and
    checking they appear in the logs, before moving on.

2. Clone the Rust code and the WASM bindings:

    ```
    cd code
    git clone https://github.com/matrix-org/matrix-rust-sdk.git
    git clone https://github.com/matrix-org/matrix-rust-sdk-crypto-wasm.git
    ```

    Make any changes you want to make to the code here.

3. Modify matrix-rust-sdk-crypto-wasm/.cargo/config to look like:

    ```
    [build]
    target = "wasm32-unknown-unknown"

    [patch.'https://github.com/matrix-org/matrix-rust-sdk']
    matrix-sdk-base = { path = "../matrix-rust-sdk/crates/matrix-sdk-base" }
    matrix-sdk-common = { path = "../matrix-rust-sdk/crates/matrix-sdk-common" }
    matrix-sdk-crypto = { path = "../matrix-rust-sdk/crates/matrix-sdk-crypto" }
    matrix-sdk-indexeddb = { path = "../matrix-rust-sdk/crates/matrix-sdk-indexeddb" }
    matrix-sdk-qrcode = { path = "../matrix-rust-sdk/crates/matrix-sdk-qrcode" }
    ```

    (As described in the
    [README](https://github.com/matrix-org/matrix-rust-sdk-crypto-wasm/#local-development-with-matrix-rust-sdk).)

    This means the WASM bindings will build your local matrix-rust-sdk instead
    of a released version.

4. Modify matrix-js-sdk to refer to your local matrix-rust-sdk-crypto-wasm.
   Open `~/code/matrix-js-sdk/package.json` and modify the line about
   `@matrix-org/matrix-sdk-crypto-wasm` to look something like:

    ```
    "@matrix-org/matrix-sdk-crypto-wasm": "file:/home/andy/code/matrix-rust-sdk-crypto-wasm",
    ```

    (Relative URLs are allowed, relative to the location of `package.json`.)

    Note that `yarn link` does not work (for me - AndyB).

5. Build the bindings:

    ```
    cd code/matrix-rust-sdk-crypto-wasm
    yarn build:dev
    ```

    (Note that you don't need to build matrix-rust-sdk - the above command
    fetches the code from there and builds it all into `pkg/*.js` here.)

6. Rebuild and re-copy the JavaScript by repeating the last two steps from
   "Using your local matrix-js-sdk" above.

Now when you re-run the JavaScript tests they should reflect the changes you
made in your Rust code.

#### TODO: sharing the same Rust code between JavaScript and Rust tests

It should be possible to replace the `rust-sdk` directory with a symlink to your
local `matrix-rust-sdk` directory, repeat all of the steps in both "Changing the
native Rust directly" and "Using your local matrix-rust-sdk-crypto-wasm and
matrix-rust-sdk", and share the Rust code between both JavaScript and native
Rust tests, but I have not actually tried it.

### React to changed interfaces in the Rust

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
- **In the matrix-rust-sdk working directory**: generate the Go bindings to `../complement-crypto/internal/api/rust`: `uniffi-bindgen-go -o ../complement-crypto/internal/api/rust --library ../matrix-rust-sdk/target/debug/libmatrix_sdk_ffi.a`
- Patch up the generated code as it's not quite right:
    * Add `// #cgo LDFLAGS: -lmatrix_sdk_ffi` immediately after `// #include <matrix_sdk_ffi.h>` at the top of `matrix_sdk_ffi.go`.
    * Patch up the import: replace `matrix_sdk_ui` with `github.com/matrix-org/complement-crypto/internal/api/rust/matrix_sdk_ui`. Do this for all the `matrix_sdk` imports.
    * Add type assertions: https://github.com/NordSecurity/uniffi-bindgen-go/issues/36
    * Specify `matrix_sdk` package qualifier for `RustBufferI`: https://github.com/NordSecurity/uniffi-bindgen-go/issues/43
- Sanity check compile `LIBRARY_PATH="$LIBRARY_PATH:/path/to/matrix-rust-sdk/target/debug" go test -c ./tests`

