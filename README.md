# hexify

A minimal utility that transforms byte strings often found in MySQL log files into hexadecimal literal representation.

## Features

- Detects `'binstring'`, `binary 'binstring'`, and `_binary 'binstring'` patterns and replaces them with `0x...` literals
- Handles escape sequences (`\\`, `\'`, `\"`, `\0`, etc.)

## Notes

- Aborts conversion for byte strings longer than the declared limit leaving the original version (defaults to 256 bytes)
- Full-line comments are preserved unchanged regardless their content

## Build

```bash
make
```

## Usage

```bash
$ echo "SELECT * FROM table WHERE id = BINARY '1234567890';" | ./hexify -l 256
SELECT * FROM table WHERE id = 0x31323334353637383930;
$ echo "SELECT * FROM table WHERE id = BINARY '1234567890';" | ./hexify -l 8
SELECT * FROM table WHERE id = _binary '1234567890';
```
