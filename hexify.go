package main

import (
	"bufio"
	"fmt"
	"flag"
	"io"
	"os"
	"strconv"
	"unicode"
)

type State int

const (
	TEXT            State = iota
	QUOTED_STRING
	BINARY
	RAW
	ESCAPE_SEQUENCE
)

var keyword = []byte("binary")

type Task struct {
	r             io.Reader
	w             *bufio.Writer
	binlog        bool
	limit         int
	keep          bool
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

func NewTask(r io.Reader, w io.Writer, limit int, keep bool, binlog bool) *Task {
	return &Task{
		r:       r,
		w:       bufio.NewWriter(w),
		binlog:  binlog,
		limit:   limit,
		keep:    keep,
		state:   TEXT,
		newLine: true,
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
			case 0, '0':
				c = 0x00
				i++
			case 'b':
				c = '\b'
				i++
			case 'n':
				c = '\n'
				i++
			case 'r':
				c = '\r'
				i++
			case 't':
				c = '\t'
				i++
			case 'Z':
				c = 0x1a
				i++
			case 'x':
				// mysqlbinlog converts non-printable chars as \xHH
				if t.binlog {
					if i > len(t.buffer) - 4 {
						return fmt.Errorf(
							"bad sequence '%s': %x %x %x %x; expected \\x followed by hexadecimal byte value",
							string(t.buffer[i:i+4]), t.buffer[i], t.buffer[i+1], t.buffer[i+2], t.buffer[i+3],
						)
					}
					c = 0
					// decode the byte code
					for j := 2; j < 4; j++ {
						c = c << 4
						b := t.buffer[i+j]
						if b >= '0' && b <= '9' {
							b -= '0'
							c |= b
							continue
						}
						b |= 0x20 // uppercase to lowercase
						if b >= 'a' && b <= 'f' {
							b -= 'a'
							c |= b + 10
							continue
						}
						// not any of 0-9, A-F, a-f
						return fmt.Errorf(
							"bad sequence '%s': %x %x %x %x; expected \\x followed by hexadecimal byte value",
							string(t.buffer[i:i+4]), t.buffer[i], t.buffer[i+1], t.buffer[i+2], t.buffer[i+3],
						)
					}
					i += 3
					break
				}
				return fmt.Errorf("unexpected escape sequence '%s': %02x %02x",
							string(t.buffer[i:i+2]), t.buffer[i], t.buffer[i+1])
			case 'f':
				if t.binlog {
					c = '\f'
					i++
					break
				}
				return fmt.Errorf("unexpected escape sequence '%s': %02x %02x",
							string(t.buffer[i:i+2]), t.buffer[i], t.buffer[i+1])
			default:
				return fmt.Errorf("unexpected escape sequence '%s': %02x %02x",
							string(t.buffer[i:i+2]), t.buffer[i], t.buffer[i+1])
			}
		}

		fmt.Fprintf(t.w, "%02x", c)
	}

	return nil
}

func (t *Task) printEatenChars() {
	for ; t.underscores > 0; t.underscores-- {
		t.w.WriteByte('_')
	}
	t.w.Write(keyword[:t.matched])
	t.matched = 0
	for ; t.whitespaces > 0; t.whitespaces-- {
		t.w.WriteByte(' ')
	}
}

func (t *Task) Run() error {
	defer t.w.Flush()

	var (
		pos       int
		keep      bool
		hashCount int
	)

	offset := 0
	bufLen := t.limit * 2

	if bufLen < 65536 {
		bufLen = 65536
	}

	buf     := make([]byte, bufLen)
	t.buffer = make([]byte, 0, t.limit)

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
					// search for pseudo queries in binlog dumps
					if t.binlog {
						hashCount = 1
					}
					t.w.WriteByte(c)
					break
				}

				// copy and quickly move on
				if t.inComment {
					if hashCount > 0 {
						if c == '#' {
							hashCount++
						} else {
							// it may be a pseudo query line
							if hashCount == 3 {
								t.inComment = false
							}
							hashCount = 0
						}
					}
					t.w.WriteByte(c)
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
						pos = offset + i + 1
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
					keyword[t.matched] = c
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
					pos = offset + i + 1
					break
				}

				t.w.WriteByte(c)

			case RAW:
				// search for closing quote char
				if c == '\\' {
					t.backslashes++
				} else {
					// non-escaped quote char ends the current string
					if c == t.quoteChar && t.backslashes % 2 == 0 {
						if !keep {
							t.w.WriteString("<byte string: ")
							t.w.WriteString(strconv.Itoa(offset + i - pos))
							t.w.WriteString(" bytes>")
							t.w.WriteByte(t.quoteChar)
						}
						t.quoteChar = 0
						t.state = TEXT
					}
					t.backslashes = 0
				}

				if keep {
					t.w.WriteByte(c)
				}

			case ESCAPE_SEQUENCE:
				switch c {
				case 'x':
					t.state = BINARY
				default:
					t.state = QUOTED_STRING
				}
				fallthrough

			case QUOTED_STRING:
				// non-printable ascii implies byte string
				if c < 0x20 || c == 0x7f {
					t.state = BINARY;
				}
				if t.binlog {
					if c == '\\' {
						t.state = ESCAPE_SEQUENCE
					}
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
									return fmt.Errorf("Parse error at position %d: %s\n", offset + i, err)
								}
							} else {
								t.w.WriteByte(t.quoteChar)
								t.w.Write(t.buffer)
								t.w.WriteByte(t.quoteChar)
							}
						} else {
							if t.inBinary {
								t.printEatenChars()
							}
							t.w.WriteByte(t.quoteChar)
							t.w.WriteByte(t.quoteChar)
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
				if len(t.buffer) == t.limit {
					// dump everything without conversion
					t.printEatenChars()
					t.w.WriteByte(t.quoteChar)
					keep = false
					if t.keep || t.state == QUOTED_STRING {
						keep = true
						t.w.Write(t.buffer)
						t.w.WriteByte(c) // not in the buffer yet
					}

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
	binlog := flag.Bool("b", false, "parse output from mysqlbinlog")
	limit  := flag.Int("l", 256, "byte strings longer than this value will be replaced with placeholder text")
	keep   := flag.Bool("k", false, "keep long byte strings unmodified")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: hexify [-h] [-b] [-k] [-l <size>]\n")
		fmt.Fprintf(os.Stderr, "Convert byte strings to hexadecimal literals\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	flag.Parse()

	task := NewTask(os.Stdin, os.Stdout, *limit, *keep, *binlog)
	if err := task.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}
