#!/bin/sh
#
# Helper for the single_sdk_tests workflow: builds and installs the js-sdk from a given source.
# The aim is to provide the right location of the JS SDK to rebuild_js_sdk.sh.
#
# Usage: `install-js-sdk.sh <source>` where `<source>` is one of:
#
#  * `.`  to use the current directory, or
#  * The name of a branch within ``matrix-js-sdk@https://github.com/matrix-org/matrix-js-sdk`

set -ex

js_sdk_src="$1"

if [ -z "$js_sdk_src" ]; then
    echo "Usage: $0 <jssdk source>" >&2
    exit 1
fi

complement_crypto_dir="$(dirname $0)/../../"

corepack enable
echo "Installing matrix-js-sdk @ $js_sdk_src"

if [ "$js_sdk_src" = "." ]; then
    # If we install from a local directory, we have to build the js-sdk ourselves.
    echo "Building js-sdk @ $(pwd)"

    PM=$(cat package.json | jq -r '.packageManager')
    if [[ $PM == "pnpm@"* ]]; then
        pnpm install
    else
        yarn install
    fi

    yarn_path="file:$(pwd)"
else
    # No need to build the js-sdk when installing from git: yarn will do it for us.
    yarn_path="https://github.com/matrix-org/matrix-js-sdk#$js_sdk_src"
fi

cd "$complement_crypto_dir"
./rebuild_js_sdk.sh "matrix-js-sdk@$yarn_path"
