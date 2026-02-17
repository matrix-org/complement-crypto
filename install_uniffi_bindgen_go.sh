#!/bin/bash -e
set -o pipefail

cargo install uniffi-bindgen-go --rev c1374e9d303826b941c8d91ba0c3008fc2d34714 --git https://github.com/tnull/uniffi-bindgen-go
