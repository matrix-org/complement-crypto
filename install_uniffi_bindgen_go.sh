#!/bin/bash -e
set -o pipefail

git clone --depth 1 --branch main https://github.com/NordSecurity/uniffi-bindgen-go _temp_uniffi_bindgen_go;
(cd _temp_uniffi_bindgen_go && git submodule init && git submodule update && cargo install uniffi-bindgen-go --path ./bindgen);
rm -rf _temp_uniffi_bindgen_go;
