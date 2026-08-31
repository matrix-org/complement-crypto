# Build and run complement-crypto tests

set dotenv-load

BASE_IMAGE := "ghcr.io/matrix-org/synapse-service:v1.117.0"
UNIFFI_GO_VERSION := "v0.7.1+v0.31.0"
COMPLEMENT_DIR := justfile_directory()

# List the available recipes.
default:
    just --list

# Opens a browser with mitmweb. Then you can open a dump file made via COMPLEMENT_CRYPTO_MITMDUMP. (requires on PATH: docker)
open-mitmweb:
    # use python3 instead of xdg-open because it's more portable (xdg-open doesn't work on MacOS). Sleep 1s and do it in the background.
    (sleep 1 && python3 -m webbrowser http://localhost:1445) &
    # use same version as tests so we don't need to pull any new image. When the user CTRL+Cs this, the container quits.
    docker run --rm -p 1445:8081 mitmproxy/mitmproxy:10.1.5  mitmweb --web-host 0.0.0.0

# Run the Rust tests of complement crypto.
test rust-sdk-path pattern="":
    @echo "Using RUST_PATH: $(realpath {{ rust-sdk-path }})"

    COMPLEMENT_CRYPTO_TEST_CLIENT_MATRIX=rr \
    COMPLEMENT_BASE_IMAGE={{ BASE_IMAGE }} \
    LIBRARY_PATH="${LIBRARY_PATH:-}:$(realpath {{ rust-sdk-path }}/target/debug)" \
    LD_LIBRARY_PATH="${LD_LIBRARY_PATH:-}:$(realpath {{ rust-sdk-path }}/target/debug)" \
    go test -v -count=1 -tags=rust -timeout 15m ./tests {{ if pattern != "" { "-run " + pattern } else { "" } }}

# Install the uniffi-bindgen-go command line utility, necessary to build the bindings.
install-uniffi-bindgen:
    cargo install uniffi-bindgen-go --rev 4f79e52bd8f518e5fa4d7acff9e586aee21e12a0 --git https://github.com/NordSecurity/uniffi-bindgen-go

# Rebuild the version of matrix-rust-sdk used and regenerate its Go bindings.
rebuild-rust-sdk rust-sdk-path:
    {{ just_executable() }} _build-rust-sdk {{ quote(rust-sdk-path) }}
    {{ just_executable() }} _patch-ldflags

[private]
_build-rust-sdk dir:
    #!/usr/bin/env bash
    set -euxo pipefail

    cd "{{ dir }}" 

    cargo build -p matrix-sdk-ffi --features 'sentry, _only-for-testing-disable-megolm-minimum-rotation-period-ms'
    uniffi-bindgen-go -o {{ COMPLEMENT_DIR }}/internal/api/rust --config {{ COMPLEMENT_DIR }}/uniffi.toml --library ./target/debug/libmatrix_sdk_ffi.a

# Add the cgo LDFLAGS directive to the generated bindings.
[private]
_patch-ldflags:
    sed -i.bak 's^// #include <matrix_sdk_ffi.h>^// #include <matrix_sdk_ffi.h>\n// #cgo LDFLAGS: -lmatrix_sdk_ffi^' internal/api/rust/matrix_sdk_ffi/matrix_sdk_ffi.go
