#!/bin/bash -e
set -o pipefail

JS_SDK_VERSION=$1

if [ -z "$JS_SDK_VERSION" ] || [ "$JS_SDK_VERSION" = "-h" ] || [ "$JS_SDK_VERSION" = "--help" ];
then
    echo "Rebuild the version of JS SDK used. (requires on PATH: yarn)"
    echo "Usage: $0 [version]"
    echo "  [version]: the yarn/npm package to use. This is fed directly into 'yarn add' so branches/commits can be used"
    echo ""
    echo "Examples:"
    echo "  Install a released version: $0 matrix-js-sdk@29.1.0"
    echo "  Install develop branch:     $0 matrix-js-sdk@https://github.com/matrix-org/matrix-js-sdk#develop"
    echo "  Install specific commit:    $0 matrix-js-sdk@https://github.com/matrix-org/matrix-js-sdk#36c958642cda08d32bc19c2303ebdfca470d03c1"
    exit 1
fi

(cd ./internal/api/js/js-sdk && yarn add $1 && yarn install && yarn build)
rm -rf ./internal/api/js/chrome/dist || echo 'no dist directory detected';
cp -r ./internal/api/js/js-sdk/dist/. ./internal/api/js/chrome/dist
