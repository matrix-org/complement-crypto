# FAQ

## Github Actions CI

### Complement-Crypto failed in CI. What do I do?

- Find the failing test(s) in red. Expand the test and find the failed test assertion, which is usually in red too.
  Example: `room_keys_test.go:282: @user-140-bob:hs1 (js): Wait[!SvCtzhROcldgNMyPkB:hs1]: timed out: bob did not see alice's message`
- Download the log tarball for the failed run. On the failed job: Summary -> scroll to bottom -> Artifacts -> Logs.
- Then see question "How do I find the failing test / line?".

> I don't see a red line, just lots of stack traces.

Either:
 - the entire test suite timed out (tests run with `go test -timeout 15m` by default) and you're seeing stack traces of every single running goroutine at the time of the timeout,
 - there's a bug in the test or Complement-Crypto and you're seeing a panic stack trace.

Either way, one of the stack traces will point to a file:line number in `/home/runner/work/complement-crypto/complement-crypto/tests/xxxxx_test.go` which you should use instead.

### How do I add Complement-Crypto to Github Actions CI?

Rust:
```yaml
  complement-crypto:
    name: "Run Complement Crypto tests"
    uses: matrix-org/complement-crypto/.github/workflows/single_sdk_tests.yml@main
    with:
        use_rust_sdk: "." # use local checkout
        use_complement_crypto: "MATCHING_BRANCH" # checkout the same branch name in complement-crypto
```
JS:
```yaml
    complement-crypto:
        name: "Run Complement Crypto tests"
        if: github.event_name == 'merge_group'
        uses: matrix-org/complement-crypto/.github/workflows/single_sdk_tests.yml@main
        with:
            use_js_sdk: "."
```

You cannot currently:
 - mix JS/Rust tests with this github action,
 - test SDKs other than JS/Rust.

## Debugging

### How do I access client SDK logs for the test and correlate it with the failing test line?

*You should have a file name and line number by this point.*

Open the test file in [the test directory](https://github.com/matrix-org/complement-crypto/tree/main/tests) and find the line which failed. Note the test name e.g `TestRoomKeyIsCycledOnMemberLeaving`. Search the log files downloaded for this test name. SDK log files include logs for all tests run in one big file. The first thing to do is to identify the section of log lines which relate to the failing test.  There will be at least 1 log line for it as all client operations issued by the test rig gets logged in client SDK log files. Log lines around this will relate to that test. For example:
```
$ grep -rn TestAliceBobEncryptionWorks tests/logs

tests/logs/js_sdk.log:50:13:37:44.589545+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: NewJSClient[@user-2-bob:hs1,IVIYEHMSKT] created client storage=false
tests/logs/js_sdk.log:51:13:37:44.589903+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-2-bob:hs1](js) Login {BaseURL:http://127.0.0.1:49302 UserID:@user-2-bob:hs1 Password:complement-crypto-password SlidingSyncURL:http://127.0.0.1:49304 DeviceID:IVIYEHMSKT PersistentStorage:false EnableCrossProcessRefreshLockProcessName: AccessToken:}
tests/logs/js_sdk.log:80:13:37:44.729508+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-2-bob:hs1](js) MustStartSyncing starting to sync
tests/logs/js_sdk.log:124:13:37:44.777951+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log CC:{"t":1,"d":{"RoomID":"!IyBUfZTYkhXfhfwYUf:hs1","Event":{"type":"m.room.name","sender":"@user-1-alice:hs1","content":{"name":"TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}"},"state_key":"","origin_server_ts":1721047063658,"unsigned":{"age":1110},"event_id":"$4Cljxp1RI7gk6H9WHTZ_hkGSTRt3mopsxu_CoU7GiB0","room_id":"!IyBUfZTYkhXfhfwYUf:hs1"}}}
tests/logs/js_sdk.log:157:13:37:45.284823+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-2-bob:hs1](js) MustStartSyncing now syncing
tests/logs/js_sdk.log:158:13:37:45.294162+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-2-bob:hs1](js) IsRoomEncrypted !IyBUfZTYkhXfhfwYUf:hs1
tests/logs/js_sdk.log:159:13:37:45.294782+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-2-bob:hs1](js) WaitUntilEventInRoom !IyBUfZTYkhXfhfwYUf:hs1
tests/logs/js_sdk.log:203:13:37:45.358253+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log CC:{"t":1,"d":{"RoomID":"!IyBUfZTYkhXfhfwYUf:hs1","Event":{"type":"m.room.name","sender":"@user-1-alice:hs1","content":{"name":"TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}"},"state_key":"","origin_server_ts":1721047063658,"unsigned":{"age":1110},"event_id":"$4Cljxp1RI7gk6H9WHTZ_hkGSTRt3mopsxu_CoU7GiB0","room_id":"!IyBUfZTYkhXfhfwYUf:hs1"}}}
tests/logs/js_sdk.log:222:13:37:45.382435+01:00 [@user-2-bob:hs1,IVIYEHMSKT] console.log TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-2-bob:hs1](js) Close
tests/logs/rust_sdk_logs.2024-07-15-12:2:2024-07-15T12:37:43.724731Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: NewRustClient[@user-1-alice:hs1][MRHCUBQNDO] creating... | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:16:2024-07-15T12:37:43.817457Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: NewRustClient[@user-1-alice:hs1] created client storage=./rust_storage/user-1-alice_hs1_MRHCUBQNDO | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:19:2024-07-15T12:37:43.823183Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) Login {BaseURL:http://127.0.0.1:49302 UserID:@user-1-alice:hs1 Password:complement-crypto-password SlidingSyncURL:http://127.0.0.1:49304 DeviceID:MRHCUBQNDO PersistentStorage:false EnableCrossProcessRefreshLockProcessName: AccessToken:} | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:58:2024-07-15T12:37:44.648606Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) MustStartSyncing starting to sync | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:152:2024-07-15T12:37:44.728925Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) MustStartSyncing now syncing | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:172:2024-07-15T12:37:45.285067Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) IsRoomEncrypted !IyBUfZTYkhXfhfwYUf:hs1 | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:183:2024-07-15T12:37:45.294864Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) SendMessage !IyBUfZTYkhXfhfwYUf:hs1 => Hello world | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:185:2024-07-15T12:37:45.294930Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]AddTimelineListener[!IyBUfZTYkhXfhfwYUf:hs1] | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:190:2024-07-15T12:37:45.296482Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]AddTimelineListener[!IyBUfZTYkhXfhfwYUf:hs1] set up | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:193:2024-07-15T12:37:45.296525Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]AddTimelineListener[!IyBUfZTYkhXfhfwYUf:hs1] TimelineDiff len=1 | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:200:2024-07-15T12:37:45.296857Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]_______ RESET <nil>
tests/logs/rust_sdk_logs.2024-07-15-12:211:2024-07-15T12:37:45.297283Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]_______ RESET &{ID:$QPJN-MOR6hGFPKuCB5IN-kpNFEnVeBXjYfFrIKUcWpo Text: Sender:@user-2-bob:hs1 Target:@user-2-bob:hs1 Membership:join FailedToDecrypt:false}
tests/logs/rust_sdk_logs.2024-07-15-12:214:2024-07-15T12:37:45.297306Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]TimelineDiff change: <nil> | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:216:2024-07-15T12:37:45.297337Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]TimelineDiff change: &{ID:$QPJN-MOR6hGFPKuCB5IN-kpNFEnVeBXjYfFrIKUcWpo Text: Sender:@user-2-bob:hs1 Target:@user-2-bob:hs1 Membership:join FailedToDecrypt:false} | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:223:2024-07-15T12:37:45.300742Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]AddTimelineListener[!IyBUfZTYkhXfhfwYUf:hs1] TimelineDiff len=1 | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:235:2024-07-15T12:37:45.300958Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]_______ PUSH BACK &{ID: Text:Hello world Sender:@user-1-alice:hs1 Target: Membership: FailedToDecrypt:false}
tests/logs/rust_sdk_logs.2024-07-15-12:238:2024-07-15T12:37:45.300979Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]TimelineDiff change: &{ID: Text:Hello world Sender:@user-1-alice:hs1 Target: Membership: FailedToDecrypt:false} | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:256:2024-07-15T12:37:45.356868Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]AddTimelineListener[!IyBUfZTYkhXfhfwYUf:hs1] TimelineDiff len=1 | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:269:2024-07-15T12:37:45.357436Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]_______ SET &{ID:$CkmVfPmnrVbrYf-f3ld7gtiiffoPKy4wmFKu9kzdMUg Text:Hello world Sender:@user-1-alice:hs1 Target: Membership: FailedToDecrypt:false}
tests/logs/rust_sdk_logs.2024-07-15-12:272:2024-07-15T12:37:45.357464Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]TimelineDiff change: &{ID:$CkmVfPmnrVbrYf-f3ld7gtiiffoPKy4wmFKu9kzdMUg Text:Hello world Sender:@user-1-alice:hs1 Target: Membership: FailedToDecrypt:false} | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:274:2024-07-15T12:37:45.357495Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) SendMessage !IyBUfZTYkhXfhfwYUf:hs1 => $CkmVfPmnrVbrYf-f3ld7gtiiffoPKy4wmFKu9kzdMUg | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:300:2024-07-15T12:37:45.374871Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]AddTimelineListener[!IyBUfZTYkhXfhfwYUf:hs1] TimelineDiff len=1 | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:313:2024-07-15T12:37:45.375069Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]_______ SET &{ID:$CkmVfPmnrVbrYf-f3ld7gtiiffoPKy4wmFKu9kzdMUg Text:Hello world Sender:@user-1-alice:hs1 Target: Membership: FailedToDecrypt:false}
tests/logs/rust_sdk_logs.2024-07-15-12:316:2024-07-15T12:37:45.375097Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1]TimelineDiff change: &{ID:$CkmVfPmnrVbrYf-f3ld7gtiiffoPKy4wmFKu9kzdMUg Text:Hello world Sender:@user-1-alice:hs1 Target: Membership: FailedToDecrypt:false} | rust.go:0
tests/logs/rust_sdk_logs.2024-07-15-12:323:2024-07-15T12:37:45.403190Z  INFO TestAliceBobEncryptionWorks/{rust_hs1}|{js_hs1}: [@user-1-alice:hs1](rust) Close | rust.go:0
```

Sometimes the bug cannot be found via client log files alone. Server logs are automatically written to the same directory.

### How do I view HTTP flows in a web UI?

Perhaps server logs aren't giving enough information and you want to see all HTTP requests/responses done. In that case, [enable mitmdump](https://github.com/matrix-org/complement-crypto/blob/main/ENVIRONMENT.md#complement_crypto_mitmdump) (done automatically in CI) and open the dump file in mitmweb to see the raw HTTP request/responses made by all clients. If you don't have mitmweb, run [`open_mitmweb.sh`](https://github.com/matrix-org/complement-crypto/blob/main/open_mitmweb.sh) which will use the mitmproxy image. Once the web UI pops up, File -> Open -> find the dump file.
 - You cannot search HTTP bodies currently: see https://github.com/mitmproxy/mitmproxy/issues/3609
 - Very large dump files take a while to load, you may need to stop a script from running. However, the UI still functions.


## Modifying Client SDK code

*Why: if you want to play with changes to the clients, or add logging
information, you will need to modify the client code and rebuild it to make sure
it is running inside the tests.*

### JS SDK
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

#### Using your local matrix-js-sdk

If you want to try out changes within a local `matrix-js-sdk`:

1. Clone the matrix-js-sdk repo:

    ```
    cd code
    git clone https://github.com/matrix-org/matrix-js-sdk.git
    ```

    and make any changes you want to make.

2. `./rebuild-js-sdk.sh ../path/to/matrix-js-sdk`

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

### Rust

#### Changing the native Rust directly (also fairly easy)

If you need to try out changes to Rust code (e.g. to add logging) and you don't
mind your changes only applying to the native Rust (and NOT applying to Rust
within the JavaScript client, which is compiled to WASM) then you can modify it
in-place and then recompile it.

Ensure you have `uniffi-bindgen-go` installed and on your `PATH`:
```
./install_uniffi_bindgen_go.sh
```

Check out matrix-rust-sdk, edit your files and then run:
```
./rebuild_rust_sdk.sh /path/to/your/matrix-rust-sdk
```

Make sure you launch the tests with `LIBRARY_PATH` pointing to
`$matrix-rust-sdk/target/debug` so that the built code gets used.

Make sure you add `-count=1` on the command line when you re-run the tests,
because changes to the rust here won't trigger the framework to re-execute a
test that it has already run.

#### TODO: sharing the same Rust code between JavaScript and Rust tests

It should be possible to replace the `rust-sdk` directory with a symlink to your
local `matrix-rust-sdk` directory, repeat all of the steps in both "Changing the
native Rust directly" and "Using your local matrix-rust-sdk-crypto-wasm and
matrix-rust-sdk", and share the Rust code between both JavaScript and native
Rust tests, but I have not actually tried it.

### React to changed interfaces in the Rust

This should now be done for you by following the steps in "Changing the native Rust directly". In the past, this involved
lots of manual steps installing `uniffi-bindgen-go` and manually editing the generated code to patch it up so it compiled.
