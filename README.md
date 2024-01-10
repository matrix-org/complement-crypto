## Complement-Crypto

Complement for Rust SDK crypto.

**EXPERIMENTAL: As of Jan 2024 this repo is under active development currently so things will break constantly.**

### What is it? Why?

Complement-Crypto extends the existing Complement test suite to support full end-to-end testing of the Rust SDK. End-to-end testing is defined at the FFI / JS SDK layer through to a real homeserver, a real sliding sync proxy, real federation, to another rust SDK on FFI / JS SDK.

Why:
- To detect "unable to decrypt" failures and add regression tests for them.
- To ensure cross-client compatibility (e.g mobile clients work with web clients and vice versa).
- To enable new kinds of security tests (active attacker tests)

### How do I run it?
It's currently pretty awful to run, as you need toolchains for both Rust and JS. Working on improving this. All tests are run in Github Actions, so see https://github.com/matrix-org/complement-crypto/blob/main/.github/workflows/tests.yaml for a step-by-step process.

You need to build Rust SDK FFI bindings _and_ JS SDK before you can get this to run. You also need a Complement homeserver image. When that is setup:

```
COMPLEMENT_BASE_IMAGE=homeserver:latest go test -v ./tests
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

### Github Action (TODO)

Inputs:
 - version/commit/branch of JS SDK
 - version/commit/branch of Rust SDK
 - version/commit/branch of synapse?
 - version/commit/branch of sliding sync proxy?
 - Test only JS, only Rust, mixed.
