#!/bin/bash -e
set -o pipefail

cargo install uniffi-bindgen-go --tag v0.4.0+v0.28.3 --git https://github.com/NordSecurity/uniffi-bindgen-go
