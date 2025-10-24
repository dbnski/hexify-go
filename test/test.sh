#!/bin/bash

SOURCE=$(basename $1)
TEST=${SOURCE%.*}
INPUT="test/${TEST}.in"
CHECK="test/${TEST}.check"
ARGS="test/${TEST}.args"
OUTPUT="test/${TEST}.out"
CODE="test/${TEST}.code"

if [ ! -f "$ARGS" ]
then
  cat "$INPUT" | ./hexify 1> "$OUTPUT" 2>&1
else
  cat "$INPUT" | ./hexify $(<"$ARGS") 1> "$OUTPUT" 2>&1
fi

diff -a -u "$CHECK" "$OUTPUT"
_RVAL=$?

if [ -f "$CODE" ]
then
  [ $_RVAL -eq $(<"$CODE") ] && _RVAL=0
fi

rm -f "$OUTPUT"

exit $_RVAL
