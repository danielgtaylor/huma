// Source from https://github.com/itchyny/json2yaml
// Copyright (c) 2022 itchyny, MIT licensed.

// Package yaml implements a converter from JSON to YAML.
//
//nolint:all
package yaml

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Convert reads JSON from r and writes YAML to w.
func Convert(w io.Writer, r io.Reader) error {
	return (&converter{w, new(bytes.Buffer), []byte{'.'}, 0}).convert(r)
}

type converter struct {
	w      io.Writer
	buf    *bytes.Buffer
	stack  []byte
	indent int
}

func (c *converter) flush() error {
	_, err := c.w.Write(c.buf.Bytes())
	c.buf.Reset()
	return err
}

func (c *converter) convert(r io.Reader) error {
	c.buf.Grow(8 * 1024)
	dec := json.NewDecoder(r)
	dec.UseNumber()
	err := c.convertInternal(dec)
	if err != nil {
		if bs := c.buf.Bytes(); len(bs) > 0 && bs[len(bs)-1] != '\n' {
			c.buf.WriteByte('\n')
		}
	}
	if ferr := c.flush(); ferr != nil && err == nil {
		err = ferr
	}
	return err
}

func (c *converter) convertInternal(dec *json.Decoder) error {
	for {
		token, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				if len(c.stack) == 1 {
					return nil
				}
				err = io.ErrUnexpectedEOF
			}
			return err
		}
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{', '[':
				if len(c.stack) > 1 {
					c.indent += 2
				}
				c.stack = append(c.stack, byte(delim))
				if dec.More() {
					if c.stack[len(c.stack)-2] == ':' {
						c.buf.WriteByte('\n')
						c.writeIndent()
					}
					if c.stack[len(c.stack)-1] == '[' {
						c.buf.WriteString("- ")
					}
				} else {
					if c.stack[len(c.stack)-2] == ':' {
						c.buf.WriteByte(' ')
					}
					if c.stack[len(c.stack)-1] == '{' {
						c.buf.WriteString("{}\n")
					} else {
						c.buf.WriteString("[]\n")
					}
				}
				continue
			case '}', ']':
				c.stack = c.stack[:len(c.stack)-1]
				if len(c.stack) > 1 {
					c.indent -= 2
				}
			}
		} else {
			switch c.stack[len(c.stack)-1] {
			case '{':
				if err := c.writeValue(token); err != nil {
					return err
				}
				c.buf.WriteByte(':')
				c.stack[len(c.stack)-1] = ':'
				continue
			case ':':
				c.buf.WriteByte(' ')
				fallthrough
			default:
				if err := c.writeValue(token); err != nil {
					return err
				}
				c.buf.WriteByte('\n')
			}
		}
		if dec.More() {
			c.writeIndent()
			switch c.stack[len(c.stack)-1] {
			case ':':
				c.stack[len(c.stack)-1] = '{'
			case '[':
				c.buf.WriteString("- ")
			case '.':
				c.buf.WriteString("---\n")
			}
		}
	}
}

func (c *converter) writeIndent() {
	if n := c.indent; n > 0 {
		const spaces = "                                "
		if l := len(spaces); n <= l {
			c.buf.WriteString(spaces[:n])
		} else {
			c.buf.WriteString(spaces)
			for n -= l; n > 0; n, l = n-l, l*2 {
				if n < l {
					l = n
				}
				c.buf.Write(c.buf.Bytes()[c.buf.Len()-l:])
			}
		}
	}
}

func (c *converter) writeValue(v any) error {
	switch v := v.(type) {
	default:
		c.buf.WriteString("null")
	case bool:
		if v {
			c.buf.WriteString("true")
		} else {
			c.buf.WriteString("false")
		}
	case json.Number:
		c.buf.WriteString(string(v))
	case string:
		c.writeString(v)
	}
	if c.buf.Len() > 4*1024 {
		return c.flush()
	}
	return nil
}

// These patterns match more than the specifications,
// but it is okay to quote for parsers just in case.
var (
	quoteSingleLineStringPattern = regexp.MustCompile(
		`^(?:` +
			`(?i:` +
			// tag:yaml.org,2002:null
			`|~|null` +
			// tag:yaml.org,2002:bool
			`|true|false|y(?:es)?|no?|o(?:n|ff)` +
			// tag:yaml.org,2002:int, tag:yaml.org,2002:float
			`|[-+]?(?:0(?:b[01_]+|o[0-7_]+|x[0-9a-f_]+)` + // base 2, 8, 16
			`|(?:[0-9][0-9_]*(?::[0-5]?[0-9])*(?:\.[0-9_]*)?` +
			`|\.[0-9_]+)(?:E[-+]?[0-9]+)?` + // base 10, 60
			`|\.inf)|\.nan` + // infinities, not-a-number
			// tag:yaml.org,2002:timestamp
			`|\d\d\d\d-\d\d?-\d\d?` + // date
			`(?:(?:T|\s+)\d\d?:\d\d?:\d\d?(?:\.\d*)?` + // time
			`(?:\s*(?:Z|[-+]\d\d?(?::\d\d?)?))?)?` + // time zone
			`)$` +
			// c-indicator - '-' - '?' - ':', leading white space
			"|[,\\[\\]{}#&*!|>'\"%@` \\t]" +
			// sequence entry, document markers, mapping key
			`|(?:-(?:--)?|\.\.\.|\?)(?:[ \t]|$)` +
			`)` +
			// mapping value
			`|:(?:[ \t]|$)` +
			// trailing white space, comment
			`|[ \t](?:#|$)` +
			// C0 control codes - '\n', DEL
			"|[\u0000-\u0009\u000B-\u001F\u007F" +
			// C1 control codes, BOM, noncharacters
			"\u0080-\u009F\uFEFF\uFDD0-\uFDEF\uFFFE\uFFFF]",
	)
	quoteMultiLineStringPattern = regexp.MustCompile(
		`` +
			// leading white space
			`^\n*(?:[ \t]|$)` +
			// C0 control codes - '\t' - '\n', DEL
			"|[\u0000-\u0008\u000B-\u001F\u007F" +
			// C1 control codes, BOM, noncharacters
			"\u0080-\u009F\uFEFF\uFDD0-\uFDEF\uFFFE\uFFFF]",
	)
)

func (c *converter) writeString(v string) {
	switch {
	default:
		c.buf.WriteString(v)
	case strings.ContainsRune(v, '\n'):
		if !quoteMultiLineStringPattern.MatchString(v) {
			c.writeBlockStyleString(v)
			break
		}
		fallthrough
	case quoteSingleLineStringPattern.MatchString(v):
		c.writeDoubleQuotedString(v)
	}
}

func (c *converter) writeBlockStyleString(v string) {
	if c.stack[len(c.stack)-1] == '{' {
		c.buf.WriteString("? ")
	}
	c.buf.WriteByte('|')
	if !strings.HasSuffix(v, "\n") {
		c.buf.WriteByte('-')
	} else if strings.HasSuffix(v, "\n\n") {
		c.buf.WriteByte('+')
	}
	c.indent += 2
	for s := ""; v != ""; {
		s, v, _ = strings.Cut(v, "\n")
		c.buf.WriteByte('\n')
		if s != "" {
			c.writeIndent()
			c.buf.WriteString(s)
		}
	}
	c.indent -= 2
	if c.stack[len(c.stack)-1] == '{' {
		c.buf.WriteByte('\n')
		c.writeIndent()
	}
}

// ref: encodeState#string in encoding/json
func (c *converter) writeDoubleQuotedString(s string) {
	const hex = "0123456789ABCDEF"
	c.buf.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if ' ' <= b && b <= '~' && b != '"' && b != '\\' {
				i++
				continue
			}
			if start < i {
				c.buf.WriteString(s[start:i])
			}
			switch b {
			case '"':
				c.buf.WriteString(`\"`)
			case '\\':
				c.buf.WriteString(`\\`)
			case '\b':
				c.buf.WriteString(`\b`)
			case '\f':
				c.buf.WriteString(`\f`)
			case '\n':
				c.buf.WriteString(`\n`)
			case '\r':
				c.buf.WriteString(`\r`)
			case '\t':
				c.buf.WriteString(`\t`)
			default:
				c.buf.Write([]byte{'\\', 'x', hex[b>>4], hex[b&0xF]})
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r <= '\u009F' || '\uFDD0' <= r && (r == '\uFEFF' ||
			r <= '\uFDEF' || r == '\uFFFE' || r == '\uFFFF') {
			if start < i {
				c.buf.WriteString(s[start:i])
			}
			if r <= '\u009F' {
				c.buf.Write([]byte{'\\', 'x', hex[r>>4], hex[r&0xF]})
			} else {
				c.buf.Write([]byte{
					'\\', 'u', hex[r>>12], hex[r>>8&0xF], hex[r>>4&0xF], hex[r&0xF],
				})
			}
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		c.buf.WriteString(s[start:])
	}
	c.buf.WriteByte('"')
}
