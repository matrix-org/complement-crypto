# Build and run complement-crypto tests

set dotenv-load := true

BASE_IMAGE := "ghcr.io/matrix-org/synapse-service:v1.117.0"
UNIFFI_GO_VERSION := "v0.4.0+v0.28.3"

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

# Build the Rust bindings, necessary to be built before running the tests.
build-rust-bindings rust-sdk-path:
    ./rebuild_rust_sdk.sh $(realpath {{ rust-sdk-path }})

# Install the uniffi-bindgen-go command line utility, necessary to build the bindings.
install-uniffi-bindgen:
    cargo install uniffi-bindgen-go --tag {{ UNIFFI_GO_VERSION }} --git https://github.com/kegsay/uniffi-bindgen-go
