#!/bin/bash -eux

CURRENT_BRANCH=${GITHUB_HEAD_REF:-${GITHUB_REF#refs/heads/}}
DEFAULT_BRANCH=$(git ls-remote --symref https://github.com/$1.git HEAD | grep 'ref: ' | sed 's@.*/@@' | cut -f 1)
# check if current branch exists on this repo
response=$(curl -s -o /dev/null -w "%{http_code}" "https://api.github.com/repos/matrix-org/matrix-rust-sdk/branches/$CURRENT_BRANCH")
if [ "$response" = "200" ]; then
    echo "$CURRENT_BRANCH"
else
    echo "$DEFAULT_BRANCH"
fi