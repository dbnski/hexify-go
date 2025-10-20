#!/bin/bash
set -e

SOURCE=$(basename $1)
TEST=${SOURCE%.*}
INPUT="test/${TEST}.in"
CHECK="test/${TEST}.check"
OUTPUT="test/${TEST}.out"

cat "$INPUT" | ./hexify > "$OUTPUT"

diff -a -u "$CHECK" "$OUTPUT"
_RVAL=$?

rm -f "$OUTPUT"

exit $_RVAL
