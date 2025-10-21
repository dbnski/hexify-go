package main

import (
	"bufio"
	"fmt"
	"flag"
	"io"
	"os"
	"unicode"
)

type State int

const (
	READ_BUFFER_SIZE = 4096
	BYTE_STRING_SIZE = 256

	TEXT 		  State = iota
	QUOTED_STRING
	BINARY
	RAW
)

var keyword = []byte("binary")

type Task struct {
	r             io.Reader
	w             io.Writer
	state         State
	buffer        []byte
	quoteChar     byte
	inComment     bool
	inBinary      bool
	newLine       bool
	backslashes   int
	underscores   int
	whitespaces   int
	matched       int
}

func NewTask(r io.Reader, w io.Writer) *Task {
	return &Task{
		r:       r,
		w:       w,
		state:   TEXT,
		newLine: true,
		buffer:  make([]byte, 0, BYTE_STRING_SIZE),
	}
}

func (t *Task) printAsHexString() error {
	if len(t.buffer) == 0 {
		return nil
	}

	fmt.Fprint(t.w, "0x")

	for i := 0; i < len(t.buffer); i++ {
		c := t.buffer[i]
		if c == '\\' {
			if i == len(t.buffer) - 1 {
				return fmt.Errorf("incomplete escape sequence")
			}
			switch t.buffer[i+1] {
			case '\'', '"', '\\':
				c = t.buffer[i+1]
				i++
			case 0x00, '0':
				c = 0x00
				i++
			case 'b':
				c = 0x08
				i++
			case 'n':
				c = 0x0a
				i++
			case 'r':
				c = 0x0d
				i++
			case 't':
				c = 0x09
				i++
			case 'Z':
				c = 0x1a
				i++
			default:
				return fmt.Errorf("bad sequence: %02x %02x\n", c, t.buffer[i+1])
			}
		}

		fmt.Fprintf(t.w, "%02x", c)
	}

	return nil
}

func (t *Task) printEatenChars() {
	for ; t.underscores > 0; t.underscores-- {
		t.w.Write([]byte{'_'})
	}
	t.w.Write(keyword[:t.matched])
	t.matched = 0
	for ; t.whitespaces > 0; t.whitespaces-- {
		t.w.Write([]byte{' '})
	}
}

func (t *Task) Run() error {
	offset  := 0
	buf     := make([]byte, READ_BUFFER_SIZE)

	for {
		n, err := t.r.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		for i := 0; i < n; i++ {
			c := buf[i]

			switch t.state {
			case TEXT:
				// check if full-line comment
				if t.newLine && c == '#' {
					t.inComment = true
				}

				// copy and move on quickly
				if t.inComment {
					t.w.Write([]byte{c})
					break
				}

				// if the binary modifier appeared, search for the string argument
				if t.inBinary {
					// eat any whitespace
					if unicode.IsSpace(rune(c)) {
						t.whitespaces++
						break
					} else if c == '\'' || c == '"' {
						t.state = BINARY
						t.quoteChar = c
						break
					}
					// string argument (quote char) not found
					t.inBinary = false
					t.printEatenChars()
				}

				// eat underscore(s) in case it belongs to _binary modifier
				if c == '_' {
					t.underscores++
					break
				}

				if (c | 0x20) == (keyword[t.matched] | 0x20) {
					t.matched++
					// keep eating chars until we can decide if this is a valid
					// binary modifier or not
					if t.matched < len(keyword) {
						break
					}
					// modifier may be prefixed by a single underscore character
					if t.underscores <= 1 {
						t.inBinary = true
						break
					}
					// not a valid modifier - too many underscores
					t.printEatenChars()
					break
				}

				// no match on the keyword, dump any eaten input
				t.printEatenChars()

				if c == '\'' || c == '"' {
					t.state = QUOTED_STRING
					t.quoteChar = c
					break
				}

				t.w.Write([]byte{c})

			case RAW:
				// search for closing quote char
				if c == '\\' {
					t.backslashes++
				} else {
					// non-escaped quote char ends the current string
					if c == t.quoteChar && t.backslashes % 2 == 0 {
						t.quoteChar = 0
						t.state = TEXT
					}
					t.backslashes = 0
				}

				t.w.Write([]byte{c})

			case QUOTED_STRING:
				// non-printable ascii implies byte string
				if c < 0x20 || c == 0x7f {
					t.state = BINARY;
				}
				fallthrough

			case BINARY:
				// search for the closing quote char
				if c == '\\' {
					t.backslashes++
				} else {
					// non-escaped quote char ends the current string
					if c == t.quoteChar && t.backslashes % 2 == 0 {
						if len(t.buffer) > 0 {
							// convert to hex string or leave as is
							if t.state == BINARY {
								err := t.printAsHexString()
								if err != nil {
									fmt.Fprintf(os.Stderr, "Error at position %d+%d: %s\n", offset, i, err)
									os.Exit(1)
								}
							} else {
								t.w.Write([]byte{t.quoteChar})
								t.w.Write(t.buffer)
								t.w.Write([]byte{t.quoteChar})
							}
						} else {
							if t.inBinary {
								t.printEatenChars()
							}
							t.w.Write([]byte{t.quoteChar, t.quoteChar})
						}

						// reset parser
						t.state       = TEXT
						t.buffer      = t.buffer[:0]
						t.quoteChar   = 0
						t.inBinary    = false
						t.backslashes = 0
						t.underscores = 0
						t.whitespaces = 0
						t.matched  = 0

						break
					}
					t.backslashes = 0
				}

				// if the byte string is too long
				if len(t.buffer) == BYTE_STRING_SIZE {
					// dump everything without conversion
					t.printEatenChars()
					t.w.Write([]byte{t.quoteChar})
					t.w.Write(t.buffer)
					t.w.Write([]byte{c}) // not in the buffer yet

					// continue in raw mode until the end of string
					t.state       = RAW
					t.buffer      = t.buffer[:0]
					t.inBinary    = false
					t.underscores = 0
					t.whitespaces = 0
					t.matched  = 0

					break
				}

				t.buffer = append(t.buffer, c)
			}

			switch {
			case c == '\n':
				t.newLine = true
				t.inComment = false
			default:
				t.newLine = false
			}
		}

		offset += n
	}

	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: hexify [-h]\n")
		fmt.Fprintf(os.Stderr, "Convert byte strings to hexadecimal literals\n")
		os.Exit(0)
	}
	flag.Parse()

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	task := NewTask(os.Stdin, w)
	if err := task.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %w\n", err)
		os.Exit(1)
	}
}