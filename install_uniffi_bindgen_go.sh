#!/bin/bash -e
set -o pipefail

cargo install uniffi-bindgen-go --rev c9edfe9ec5c6013c68e62ca0801dfbe096e89bf0 --git https://github.com/tnull/uniffi-bindgen-go
