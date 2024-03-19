## Complement-Crypto
*EXPERIMENTAL: As of Jan 2024 this repo is under active development currently so things will break constantly.*

Complement Crypto is an end-to-end test suite for next generation Matrix _clients_, designed to test the full spectrum of E2EE APIs. 

### How do I run it?

*See [FAQ.md](FAQ.md) for more information.*

Please ensure you have met Complement's [Dependencies](https://github.com/matrix-org/complement?tab=readme-ov-file#dependencies) first.

It's currently pretty awful to run, as you need toolchains for both Rust and JS. Working on improving this. All tests are run in Github Actions, so see https://github.com/matrix-org/complement-crypto/blob/main/.github/workflows/tests.yaml for a step-by-step process.

You need to build Rust SDK FFI bindings _and_ JS SDK before you can get this to run. You also need a Complement homeserver image. When that is setup:

```
COMPLEMENT_BASE_IMAGE=homeserver:latest go test -tags='rust,jssdk' -v ./tests
```

TODO: consider checking in working builds so you can git clone and run. Git LFS for `libmatrix_sdk_ffi.so` given it's 60MB?
TODO: Dockerify JS SDK so developers don't _need_ an active npm install?

#### Environment Variables
See [ENVIRONMENT.md](ENVIRONMENT.md).

### Test hitlist
There is an exhaustive set of tests that this repository aims to exercise. See [TEST_HITLIST.md](TEST_HITLIST.md).

### Architecture

Tests sometimes require reverse proxy interception to let some requests pass through but not others. For this, we use [mitmproxy](https://mitmproxy.org/).

```
     Host        |       dockerd           
                 |                          +----------+      +----------+
                 |                     .--> | ss proxy | <--> | postgres |
 +----------+    |    +-----------+    |    +-----+----+      +----------+
 | Go tests | <--|--> | mitmproxy | <--+--> | hs1 |
 +----------+    |    +-----------+    |    +-----+
                 |                     `--> | hs2 |
                 |                          +-----+
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


### Github Action (TODO)

Inputs:
 - version/commit/branch of JS SDK
 - version/commit/branch of Rust SDK
 - version/commit/branch of synapse?
 - version/commit/branch of sliding sync proxy?
 - Test only JS, only Rust, mixed.
