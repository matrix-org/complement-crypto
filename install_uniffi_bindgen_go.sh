#!/bin/bash -e
set -o pipefail

cargo install uniffi-bindgen-go --rev 5c68e58349035874d534a7c34117062a77a6f86e --git https://github.com/tnull/uniffi-bindgen-go/
