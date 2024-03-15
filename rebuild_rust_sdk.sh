#!/bin/bash -e -o pipefail

RUST_SDK_VERSION=$1;

if [ -z "$RUST_SDK_VERSION" ] || [ "$RUST_SDK_VERSION" = "-h" ] || [ "$RUST_SDK_VERSION" = "--help" ];
then
    echo "Rebuild the version of rust SDK used. (requires on PATH: uniffi-bindgen-go, cargo, git)"
    echo "Usage: $0 [version]"
    echo "  [version]: the rust SDK git repo and branch|tag to use. Syntax: '\$HTTPS_URL@\$TAG|\$BRANCH'"
    echo ""
    echo "Examples:"
    echo "  Install main branch:  $0 https://github.com/matrix-org/matrix-rust-sdk@main"
    echo "  Install 0.7.1 tag:    $0 https://github.com/matrix-org/matrix-rust-sdk@0.7.1"
    echo ""
    echo "The [version] is split into the URL and TAG|BRANCH then fed directly into 'git clone --depth 1 --branch <tag_name> <repo_url>'"
    echo "Ensure LIBRARY_PATH is set to $(pwd)/_temp_rust_sdk/target/debug so the .a/.dylib file is picked up when 'go test' is run."
    exit 1
fi

rm -rf _temp_rust_sdk || echo 'no temp directory found, cloning';
SEGMENTS=(${RUST_SDK_VERSION//@/ });
git clone --depth 1 --branch ${SEGMENTS[1]} ${SEGMENTS[0]} _temp_rust_sdk;
# replace uniffi version to one that works with uniffi-bindgen-go
cd _temp_rust_sdk
sed -i.bak 's/uniffi =.*/uniffi = "0\.25\.3"/' Cargo.toml
sed -i.bak 's^uniffi_bindgen =.*^uniffi_bindgen = { git = "https:\/\/github.com\/mozilla\/uniffi-rs", rev = "0a03b713306d6ce3de033157fc2ce92a238c2e24" }^' Cargo.toml
cargo build -p matrix-sdk-ffi
# generate the bindings
uniffi-bindgen-go -o ../internal/api/rust --config ../uniffi.toml --library ./target/debug/libmatrix_sdk_ffi.a
# add LDFLAGS
cd ..
sed -i.bak 's^// #include <matrix_sdk_ffi.h>^// #include <matrix_sdk_ffi.h>\n// #cgo LDFLAGS: -lmatrix_sdk_ffi^' internal/api/rust/matrix_sdk_ffi/matrix_sdk_ffi.go

echo "OK! Ensure LIBRARY_PATH is set to $(pwd)/_temp_rust_sdk/target/debug so the .a/.dylib file is picked up when 'go test' is run."
