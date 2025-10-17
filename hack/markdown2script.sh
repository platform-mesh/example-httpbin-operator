#!/usr/bin/env sh

markdown="$1"
filter="$2"

# print statements as they are executed
echo 'set -x'
# exit on error
echo 'set -e'

awk '
BEGIN {
    in_code_block = 0;
    ignore_filter = "'"$filter"'";
    ignored_code_block = 0;
}
/^```/ {
    in_code_block = !in_code_block;
    if (ignore_filter != "" && $0 ~ ignore_filter) {
        ignored_code_block = 1;
    }
    if (!in_code_block) {
        ignored_code_block = 0;
    }
    next;
}
( in_code_block && !ignored_code_block ) { print }
' "$markdown"
