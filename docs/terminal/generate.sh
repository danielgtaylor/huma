#!/bin/bash
# set -x

# If asciinema-run is not in the path, compile it:
if ! command -v asciinema-run &> /dev/null
then
		echo "asciinema-run could not be found, compiling it..."
		go install ../asciinema-run
fi

export SHELL=/bin/bash
export ARGS="-i 1 --overwrite --cols=72"

FULL_PATH_TO_SCRIPT="$(realpath "$0")"
SCRIPT_DIR="$(dirname "$FULL_PATH_TO_SCRIPT")"

# Cleanup
rm -rf ./my-api

# Run the thing
asciinema-run $1 $1.tmp $ARGS
$SCRIPT_DIR/fix.py $1.tmp > $2
