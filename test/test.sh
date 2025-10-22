#!/bin/bash
set -e

SOURCE=$(basename $1)
TEST=${SOURCE%.*}
INPUT="test/${TEST}.in"
CHECK="test/${TEST}.check"
ARGS="test/${TEST}.args"
OUTPUT="test/${TEST}.out"

if [ ! -f "$ARGS" ]
then
  cat "$INPUT" | ./hexify > "$OUTPUT"
else
  cat "$INPUT" | ./hexify $(<"$ARGS") > "$OUTPUT"
fi

diff -a -u "$CHECK" "$OUTPUT"
_RVAL=$?

rm -f "$OUTPUT"

exit $_RVAL
