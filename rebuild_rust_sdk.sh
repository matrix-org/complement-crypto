#!/bin/bash -e
set -o pipefail

ARG=$1;
RUST_SDK_DIR="$(pwd)/_temp_rust_sdk";
COMPLEMENT_DIR="$(pwd)";

if [ -z "$ARG" ] || [ "$ARG" = "-h" ] || [ "$ARG" = "--help" ];
then
    echo "Rebuild the version of rust SDK used. Execute this inside the complement-crypto directory. (requires on PATH: uniffi-bindgen-go, cargo, git, sed)"
    echo "Usage: $0 [version|directory]"
    echo "  [version]: the rust SDK git repo and branch|tag to use. Syntax: '\$HTTPS_URL@\$TAG|\$BRANCH'"
    echo "             Stores repository in $RUST_SDK_DIR"
    echo "  [directory]: the local rust SDK checkout to use."
    echo ""
    echo "Examples:"
    echo "  Install main branch:  $0 https://github.com/matrix-org/matrix-rust-sdk@main"
    echo "  Install 0.7.1 tag:    $0 https://github.com/matrix-org/matrix-rust-sdk@0.7.1"
    echo "  Install ./rust-sdk    $0 ./rust-sdk"
    echo ""
    echo "[directory] is determined if the first character is a '.' or '/'. If neither, it is assumed to be a [version]"
    echo "The [version] is split into the URL and TAG|BRANCH then fed directly into 'git clone --depth 1 --branch <tag_name> <repo_url>'"
    exit 1
fi

if [[ $ARG == /* ]]; then # starts with / => absolute path
  RUST_SDK_DIR="$ARG";
elif [[ $ARG == .* ]]; then # starts with . => relative path
  set +e
  RUST_SDK_DIR="$(readlink -f $ARG)";
  set -e
  if [ -z "$RUST_SDK_DIR" ]; then
    echo "path not found: $ARG";
    exit 1
  fi
else # HTTPS URL => git clone into temp dir  
  rm -rf $RUST_SDK_DIR || echo 'no temp directory found, cloning';
  SEGMENTS=(${ARG//@/ });
  git clone --depth 1 --branch ${SEGMENTS[1]} ${SEGMENTS[0]} $RUST_SDK_DIR;
fi

function restore_backups {
    for i in Cargo.toml Cargo.lock bindings/matrix-sdk-ffi/Cargo.toml; do
        mv "$RUST_SDK_DIR/$i.backup" "$RUST_SDK_DIR/$i"
    done
}

# replace uniffi version to one that works with uniffi-bindgen-go
echo 'building matrix-sdk-ffi...';
cd $RUST_SDK_DIR;
cp Cargo.toml Cargo.toml.backup
cp Cargo.lock Cargo.lock.backup
cp bindings/matrix-sdk-ffi/Cargo.toml bindings/matrix-sdk-ffi/Cargo.toml.backup
trap "restore_backups" EXIT INT TERM
sed -i.bak 's/uniffi =.*/uniffi = "0\.29\.5"/' Cargo.toml
sed -i.bak 's/"wasm-unstable-single-threaded"//' bindings/matrix-sdk-ffi/Cargo.toml
sed -i.bak 's^uniffi_bindgen =.*^uniffi_bindgen = { git = "https:\/\/github.com\/mozilla\/uniffi-rs", rev = "f7a0ba703b4c06fff8fffa98078f2e5d7588a695" }^' Cargo.toml
sed -i.bak 's#matrix-sdk-crypto = {#matrix-sdk-crypto = {features = ["_disable-minimum-rotation-period-ms"],#' Cargo.toml
cargo build -p matrix-sdk-ffi --features 'native-tls,sentry'
# generate the bindings
echo "generating bindings to $COMPLEMENT_DIR/internal/api/rust...";
uniffi-bindgen-go -o $COMPLEMENT_DIR/internal/api/rust --config $COMPLEMENT_DIR/uniffi.toml --library ./target/debug/libmatrix_sdk_ffi.a
# add LDFLAGS
cd $COMPLEMENT_DIR
sed -i.bak 's^// #include <matrix_sdk_ffi.h>^// #include <matrix_sdk_ffi.h>\n// #cgo LDFLAGS: -lmatrix_sdk_ffi^' internal/api/rust/matrix_sdk_ffi/matrix_sdk_ffi.go

echo "OK! Ensure LIBRARY_PATH and LD_LIBRARY_PATH are set to $RUST_SDK_DIR/target/debug so the .so/.dylib file is picked up when 'go test' is run."
echo "e.g COMPLEMENT_BASE_IMAGE=homeserver:latest LIBRARY_PATH=\$LIBRARY_PATH:$RUST_SDK_DIR/target/debug LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$RUST_SDK_DIR/target/debug go test ./tests"
