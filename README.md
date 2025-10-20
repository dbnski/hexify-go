# unhex

A minimal utility that transforms byte strings often found in MySQL log files into hexadecimal literal representation.

## Features

- Detects `'binstring'`, `binary 'binstring'`, and `_binary 'binstring'` patterns and replaces them with `0x...` literals
- Handles escape sequences (`\\`, `\'`, `\"`, `\0`, etc.)

## Notes

- Aborts conversion for byte strings longer than 256 bytes leaving the original version
- Full-line comments are preserved unchanged regardless their content

## Build

```bash
make
```

## Usage

```bash
$ echo "SELECT * FROM table WHERE id = _binary 'abcdefghijklmnop';" | ./hexify
SELECT * FROM table WHERE id = 0x6162636465666768696a6b6c6d6e6f70;
```
