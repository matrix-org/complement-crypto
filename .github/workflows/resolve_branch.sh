#!/bin/bash -eux

# ./resolve_branch.sh org-name/repo-name
# This script echoes the branch name that should be used when using 'matching branch' logic.
# It defaults to the default branch for the repo provided (as the default isn't always 'main')
# It uses Github Actions env vars to determine what the current branch is, and attempts to
# find that same branch name on the repo provided. If it finds a matching branch, the branch
# name is echoed.
#
# Usage: BRANCH=$(./resolve_branch matrix-org/matrix-rust-sdk)

CURRENT_BRANCH=${GITHUB_HEAD_REF:-${GITHUB_REF#refs/heads/}}
DEFAULT_BRANCH=$(git ls-remote --symref https://github.com/$1.git HEAD | grep 'ref: ' | sed 's@.*/@@' | cut -f 1)
# check if current branch exists on this repo
response=$(curl -s -o /dev/null -w "%{http_code}" "https://api.github.com/repos/$1/branches/$CURRENT_BRANCH")
if [ "$response" = "200" ]; then
    echo "$CURRENT_BRANCH"
else
    echo "$DEFAULT_BRANCH"
fi