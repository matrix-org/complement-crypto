## Test Hitlist

This is an attempt to manually enumerate all conceivable failure modes with end-to-end encryption. This hitlist is designed to discern categories of bugs which A) have not been seen in bug reports yet, B) are not known issues yet, C) are potential issues. This testing methodology is responsible for uncovering [numerous](https://github.com/matrix-org/synapse/issues/16680) [issues](https://github.com/matrix-org/synapse/issues/16681) which were previously unknown.

### Membership ACLs
- [x] Happy case Alice and Bob in an encrypted room can send and receive encrypted messages, and decrypt them all.
- [x] Bob can see messages when he was invited but not joined to the room. Subsequent messages are also decryptable.
- [x] In a public, `shared` history visibility room, a new user Bob cannot decrypt earlier messages prior to his join, despite being able to see the events. Subsequent messages are decryptable.
- [x] Bob leaves the room. Some messages are sent. Bob rejoins and cannot decrypt the messages sent whilst he was gone (ensuring we cycle keys).
- [x] Bob cannot decrypt older messages when logging in on a new device. When the device is logged out and in again, Bob cannot decrypt messages sent whilst he was logged out.
- [x] EXPECTED FAIL: Alice invites Bob, Alice sends a message, Bob changes their device, then Bob joins. Bob should be able to see Alice's message.

### Key backups
- [x] New device for Alice cannot decrypt previous messages. Backups can be made on Alice's first device. Alice's new device can download the backup and decrypt the messages. Check backups work cross-platform (e.g create on rust, restore on JS and vice versa).
- [x] Inputting the wrong recovery key fails to decrypt the backup.

### One-time Keys
- [x] When Alice runs out of OTKs, the fallback key is used.
- [x] Alice cycles her fallback key when she becomes aware that it has been used.
- [ ] When a fallback key used by multiple sessions, Alice accepts all of them. TODO: when should Alice stop accepting usage of this key?
- [ ] When a OTK is reused, Alice rejects the 2nd+ use of the OTK.

### Key Verification: (Short Authentication String)
- [ ] Happy case Alice <-> Bob key verification.
- [ ] Happy case Alice <-> Bob key verification over federation.
- [ ] Happy case Alice <-> Alice key verification (different devices).
- [ ] A MITMed key fails key verification.
- [ ] Repeat all of the above, but for QR code. (render QR code to png then rescan).
- [ ] Repeat all of the above, but for Emoji representations of SAS.
- [ ] Verification can be cancelled.
- [ ] Verification can be cancelled over federation.

### Network connectivity
Network connectivity tests are extremely time sensitive as retries are often using timeouts in clients.

- [x] If a client cannot upload OTKs, it retries.
- [ ] If a client cannot claim local OTKs, it retries.
- [ ] If a client cannot claim remote OTKs, it retries.
- [x] If a server cannot send device list updates over federation, it retries. https://github.com/matrix-org/complement/pull/695
- [ ] If a client cannot query device keys for a user, it retries.
- [ ] If a server cannot query device keys on another server, it retries.
- [x] If a client cannot send a to-device msg, it retries.
- [x] If a server cannot send a to-device msg to another server, it retries. https://github.com/matrix-org/complement/pull/694
- [ ] Repeat all of the above, but restart the client|server after the initial connection failure. This checks that retries aren't just stored in memory but persisted to disk.

### State Synchronisation:
This refers to cases where the client has some state and wishes to synchronise it with the server but is interrupted from doing so in a terminal (SIGKILL) manner. Clients MUST persist state they wish to synchronise to avoid state being regenerated and hence getting out-of-sync with server state. All of these tests require persistent storage on clients. Tests will typically intercept responses from the server and then send back an HTTP 504 to cause state synchronisation bugs, as from the server's perspective everything went through okay, but the client doesn't know that.

- [x] If a client is terminated mid-way through uploading OTKs, it re-uploads the _same set_ of OTKs on startup.
- [ ] If a client is terminated mid-way through uploading device keys, it re-uploads the _same set_ of device keys on startup.
- [ ] If a client is terminated mid-way through uploading cross-signing keys, it re-uploads the _same set_ of keys on startup.
- [ ] If a client is terminated mid-way through sending a to-device message, it retries sending _the same message_ on startup.
- [ ] If a client is terminated mid-way through calculating device list changes via `/keys/changes`, it retries on startup.
- [ ] If a server is terminated mid-way through sending a device list update over federation, it retries on startup.
- [ ] If a server is terminated mid-way through sending a to-device message over federation, it retries on startup.

### Adversarial Attacks

TODO See polyjuice tests, port and add more.


### Regression tests

Tests for known failures.

 - [ ] Receive a to-device event with a room key, then fail requests to `/keys/query`. Ensure you can still see encrypted messages in that room. Regression test for https://github.com/vector-im/element-web/issues/24682
 - [ ] Receive many to-device events followed by a room key, then quickly restart the client. Ensure you can still see encrypted messages in that room. Tests that to-device events are persisted locally or the since token is not advanced before processing to avoid dropped to-device events. Regression test for https://github.com/vector-im/element-web/issues/23113
 - [ ] If you make a new room key, you need to send it to all devices in the room. If you restart the client mid-way through sending, ensure the rest get sent upon restart.
 - [ ] Tests for [MSC3061](https://github.com/matrix-org/matrix-spec-proposals/pull/3061): Sharing room keys for past messages. Rust SDK: https://github.com/matrix-org/matrix-rust-sdk/issues/580
