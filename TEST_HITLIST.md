## Test Hitlist

This is an attempt to manually enumerate all conceivable failure modes with end-to-end encryption. This hitlist is designed to discern categories of bugs which A) have not been seen in bug reports yet, B) are not known issues yet, C) are potential issues. This testing methodology is responsible for uncovering [numerous](https://github.com/matrix-org/synapse/issues/16680) [issues](https://github.com/matrix-org/synapse/issues/16681) which were previously unknown.

### Test permutations

For each test, multiple permutations can be tested:
 - homogenous clients (e.g Alice and Bob are both on JS)
 - heterogeneous clients (e.g Alice is on JS, Bob is using Rust FFI)
 - both clients are on the same server
 - clients are on different servers (testing federation)

This creates a matrix of test permutations, represented via `COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX`. There exists a total of 4x4=16 permutations, given:
- each client can be 1 of 4 types (j,r,J,R)
- two clients are needed per test

However, in practice there are only 12 permutations because we can ignore duplicate permutations caused by testing on HS1 vs HS2 e.g testing 2x JS clients on HS1 and then re-testing 2x HS clients on HS2 makes no sense:
```
Alice | Bob | Fed? | Same client?
 j       j     N          Y
 j       r     N          N
 r       j     N          N
 r       r     N          Y

 J       j     Y          Y
 J       r     Y          N
 R       j     Y          N
 R       r     Y          Y
 j       J     Y          Y
 j       R     Y          N
 r       J     Y          N
 r       R     Y          Y

Below permutations can be ignored as they
are duplicates of the first 4

 J       J     N          Y
 J       R     N          N
 R       J     N          N
 R       R     N          Y
```

The test hitlist will generally not refer to any specific permutation, preferring the terms Alice and Bob. In some cases, federation may be explicitly mentioned if the test makes no sense without federation.

### Membership ACLs
- [x] Happy case Alice and Bob in an encrypted room can send and receive encrypted messages, and decrypt them all.
- [x] Bob can see messages when he was invited but not joined to the room. Subsequent messages are also decryptable.
- [x] In a public, `shared` history visibility room, a new user Bob cannot decrypt earlier messages prior to his join, despite being able to see the events. Subsequent messages are decryptable.
- [x] Bob leaves the room. Some messages are sent. Bob rejoins and cannot decrypt the messages sent whilst he was gone (ensuring we cycle keys).
- [x] Bob cannot decrypt older messages when logging in on a new device. When the device is logged out and in again, Bob cannot decrypt messages sent whilst he was logged out.
- [x] EXPECTED FAIL: Alice invites Bob, Alice sends a message, Bob changes their device, then Bob joins. Bob should be able to see Alice's message.

### Key backups
These tests only make sense on a single server, for a single user.

- [x] New device for Alice cannot decrypt previous messages. Backups can be made on Alice's first device. Alice's new device can download the backup and decrypt the messages. Check backups work cross-platform (e.g create on rust, restore on JS and vice versa).
- [x] Inputting the wrong recovery key fails to decrypt the backup.

### One-time Keys
- [x] When Alice runs out of OTKs, the fallback key is used.
- [x] Alice cycles her fallback key when she becomes aware that it has been used.
- [x] When a fallback key used by multiple sessions, Alice accepts all of them. TODO: when should Alice stop accepting usage of this key?
- [ ] When a OTK is reused, Alice rejects the 2nd+ use of the OTK.

### Key Verification: (Short Authentication String)
- [ ] Happy case Alice <-> Bob key verification.
- [ ] Happy case Alice <-> Alice key verification (different devices).
- [ ] A MITMed key fails key verification.
- [ ] Repeat all of the above, but for QR code. (render QR code to png then rescan).
- [ ] Repeat all of the above, but for Emoji representations of SAS.
- [ ] Verification can be cancelled.

### Network connectivity
Network connectivity tests are extremely time sensitive as retries are often using timeouts in clients.

- [x] If a client cannot upload OTKs, it retries.
- [x] If a client cannot claim OTKs, it retries.
- [x] If a server cannot send device list updates over federation, it retries. https://github.com/matrix-org/complement/pull/695
- [ ] If a client cannot query device keys for a user, it retries.
- [ ] If a server cannot query device keys on another server, it retries.
- [x] If a client cannot send a to-device msg, it retries.
- [x] If a server cannot send a to-device msg to another server, it retries. https://github.com/matrix-org/complement/pull/694

### State Synchronisation
This refers to cases where the client has some state and wishes to synchronise it with the server but is interrupted from doing so in a fatal (SIGKILL) manner. Clients MUST persist state they wish to synchronise to avoid state being regenerated and hence getting out-of-sync with server state. All of these tests require persistent storage on clients. These tests will typically intercept responses from the server and then SIGKILL the client. Upon restart, only if the client has persisted the new state _prior to uploading_ AND endpoints are idempotent (since the client will retry the operation) will state remain in sync. These tests aren't limited to clients, as servers also need to synchronise state over federation.

- [x] If a client is terminated mid-way through uploading OTKs, it re-uploads the _same set_ of OTKs on startup.
- [ ] If a client is terminated mid-way through uploading device keys, it re-uploads the _same set_ of device keys on startup.
- [ ] If a client is terminated mid-way through uploading cross-signing keys, it re-uploads the _same set_ of keys on startup.
- [ ] If a client is terminated mid-way through sending a to-device message, it retries sending _the same message_ on startup.
- [ ] If a client is terminated mid-way through calculating device list changes via `/keys/changes`, it retries on startup.
- [ ] If a server is terminated mid-way through sending a device list update over federation, it retries on startup.
- [ ] If a server is terminated mid-way through sending a to-device message over federation, it retries on startup.

### Room Keys
- [x] The room key is cycled when a user leaves a room.
- [x] The room key is cycled when one of a user's devices logs out.
- [ ] The room key is cycled when one of a user's devices is blacklisted.
- [ ] The room key is cycled when history visibility changes to something more restrictive TODO: define precisely.
- [ ] The room key is cycled when the encryption algorithm changes.
- [ ] The room key is cycled when `rotation_period_msgs` is met (default: 100).
- [ ] The room key is cycled when `rotation_period_ms` is exceeded (default: 1 week).
- [x] The room key is not cycled when one of a user's devices logs in.
- [x] The room key is not cycled when the client restarts.
- [x] The room key is not cycled when users change their display name.

### Adversarial Attacks

TODO See polyjuice tests, port and add more.


### Regression tests

Tests for known failures.

 - [ ] Receive a to-device event with a room key, then fail requests to `/keys/query`. Ensure you can still see encrypted messages in that room. Regression test for https://github.com/vector-im/element-web/issues/24682
 - [ ] Receive many to-device events followed by a room key, then quickly restart the client. Ensure you can still see encrypted messages in that room. Tests that to-device events are persisted locally or the since token is not advanced before processing to avoid dropped to-device events. Regression test for https://github.com/vector-im/element-web/issues/23113
 - [ ] If you make a new room key, you need to send it to all devices in the room. If you restart the client mid-way through sending, ensure the rest get sent upon restart.
 - [ ] Tests for [MSC3061](https://github.com/matrix-org/matrix-spec-proposals/pull/3061): Sharing room keys for past messages. Rust SDK: https://github.com/matrix-org/matrix-rust-sdk/issues/580
 - [ ] Ensure that we send at least 100 to-device messages per HTTP request when changing the room key: https://github.com/vector-im/element-web/issues/24680
 - [ ] Check that we do not delete OTK private keys when we receive a badly formed pre-key message using that key https://github.com/element-hq/element-ios/issues/7480
 - [ ] If you get a lot of to-device msgs all at once, ensure they are processed in-order https://github.com/element-hq/element-web/issues/25723
 - [ ] Check that to-device msgs are not dropped if you restart the client quickly when it gets a /sync response https://github.com/element-hq/element-meta/issues/762
