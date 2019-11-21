#!/bin/sh
set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
ntsdir="$workspace/src/gitee.com/nerthus"
if [ ! -L "$ntsdir/nerthus" ]; then
    mkdir -p "$ntsdir"
    cd "$ntsdir"
    ln -s ../../../../../. nerthus
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$ntsdir/nerthus"
PWD="$ntsdir/nerthus"

# Launch the arguments with the configured environment.
exec "$@"
