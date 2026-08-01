# Build and run complement-crypto tests

set dotenv-load

BASE_IMAGE := "ghcr.io/matrix-org/synapse-service:v1.117.0"
UNIFFI_GO_VERSION := "v0.7.1+v0.31.0"
COMPLEMENT_DIR := justfile_directory()

# List the available recipes.
default:
    just --list

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
    cargo install uniffi-bindgen-go --tag {{ UNIFFI_GO_VERSION }} --git https://github.com/NordSecurity/uniffi-bindgen-go

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
