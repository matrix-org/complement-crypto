#!/bin/bash -e
set -o pipefail

cargo install uniffi-bindgen-go --tag v0.2.2+v0.25.0 --git https://github.com/NordSecurity/uniffi-bindgen-go
