package yaml_test

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	json2yaml "github.com/danielgtaylor/huma/v2/yaml"
)

func TestConvert(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
		err  string
	}{
		{
			name: "null",
			src:  "null",
			want: join([]string{"null"}),
		},
		{
			name: "boolean",
			src:  "false true",
			want: join([]string{"false", "true"}),
		},
		{
			name: "number",
			src:  "0 128 -320 3.14 -6.63e-34",
			want: join([]string{"0", "128", "-320", "3.14", "-6.63e-34"}),
		},
		{
			name: "string",
			src:  `"" "foo" "null" "hello, world" "\"\\\b\f\r\t" "１２３４５" " １２３４５ "`,
			want: join([]string{`""`, `foo`, `"null"`, `hello, world`, `"\"\\\b\f\r\t"`, `１２３４５`, `" １２３４５ "`}),
		},
		{
			name: "quote booleans",
			src:  `"true" "False" "YES" "y" "no" "n" "oN" "Off" "truer" "oon" "f"`,
			want: join([]string{`"true"`, `"False"`, `"YES"`, `"y"`, `"no"`, `"n"`, `"oN"`, `"Off"`, `truer`, `oon`, `f`}),
		},
		{
			name: "quote integers",
			src: `"0" "+42" "128" "900" "-1_234_567_890" "+ 1" "11:22" "+1:2" "-3:4" "0:1:02:1:0" "12:50" "12:60"
				"0b1" "0b11_00" "0b" "0b2" "0664" "0_1_2_3" "0_" "0678" "0123.456e789" "0o1_0" "0O0" "0o"
				"0x0" "0x09af" "0xFE_FF" "0x" "0xfg" "0x_F_"`,
			want: join([]string{
				`"0"`, `"+42"`, `"128"`, `"900"`, `"-1_234_567_890"`, `+ 1`, `"11:22"`, `"+1:2"`, `"-3:4"`, `"0:1:02:1:0"`, `"12:50"`, `12:60`,
				`"0b1"`, `"0b11_00"`, `0b`, `0b2`, `"0664"`, `"0_1_2_3"`, `"0_"`, `"0678"`, `"0123.456e789"`, `"0o1_0"`, `"0O0"`, `0o`,
				`"0x0"`, `"0x09af"`, `"0xFE_FF"`, `0x`, `0xfg`, `"0x_F_"`,
			}),
		},
		{
			name: "quote floating point numbers",
			src: `"0.1" "3.14156" "-42.195" "-.3" "+6." "-+1" "1E+9" "6.63e-34" "1e2"
				"1_2.3_4e56" "120:30:40.56" ".inf" "+.inf" "-.inf" ".infr" ".nan" "+.nan" "-.nan" ".nan."`,
			want: join([]string{
				`"0.1"`, `"3.14156"`, `"-42.195"`, `"-.3"`, `"+6."`, `-+1`, `"1E+9"`, `"6.63e-34"`, `"1e2"`,
				`"1_2.3_4e56"`, `"120:30:40.56"`, `".inf"`, `"+.inf"`, `"-.inf"`, `.infr`, `".nan"`, `+.nan`, `-.nan`, `.nan.`,
			}),
		},
		{
			name: "quote date time",
			src: `"2022-08-04" "1000-1-1" "9999-12-31" "1999-99-99" "999-9-9" "2000-08" "2000-08-" "2000-"
				"2022-01-01T12:13:14" "2022-02-02 12:13:14.567" "2022-03-03   1:2:3" "2022-03-04 15:16:17." "2022-03-04 15:16:"
				"2000-12-31T01:02:03-09:00" "2000-12-31t01:02:03Z" "2000-12-31 01:02:03 +7" "2222-22-22  22:22:22  +22:22"`,
			want: join([]string{
				`"2022-08-04"`, `"1000-1-1"`, `"9999-12-31"`, `"1999-99-99"`, `999-9-9`, `2000-08`, `2000-08-`, `2000-`,
				`"2022-01-01T12:13:14"`, `"2022-02-02 12:13:14.567"`, `"2022-03-03   1:2:3"`, `"2022-03-04 15:16:17."`, `"2022-03-04 15:16:"`,
				`"2000-12-31T01:02:03-09:00"`, `"2000-12-31t01:02:03Z"`, `"2000-12-31 01:02:03 +7"`, `"2222-22-22  22:22:22  +22:22"`,
			}),
		},
		{
			name: "quote indicators",
			src: `"!" "\"" "#" "$" "%" "&" "'" "(" ")" "*" "+" ","
				"-" "--" "---" "----" "- " "--- -" "- ---" "- --- -" "-- --" "?-" "-?" "?---" "---?" "--- ?"
				"." ".." "..." "...." "... ." ". ..." ". ... ." ".. .." "?..." "...?" "... ?"
				"." "/" ":" ";" "<" "=" ">" "?" "[" "\\" "]" "^" "_" "{" "|" "}" "~" "@" "\u0060"
				"%TAG" "!!str" "!<>" "&anchor" "*anchor" "https://example.com/?q=text#fragment" "%!/bin/sh"
				"- ." ". -" "-." ".-" "? ." ". ?" "?." ".?" "?\t." ": ." ". :" ":\t." ". :?" "?:" ":?" ". ? :." "[]" "{}"
				". #" "# ." ".#." ". #." ".# ." ". # ." ". ! \" $ % & ' ( ) * + , - / ; < = > ? [ \\ ] ^ _ { | } ~"`,
			want: join([]string{
				`"!"`, `"\""`, `"#"`, `$`, `"%"`, `"&"`, `"'"`, `(`, `)`, `"*"`, `+`, `","`,
				`"-"`, `--`, `"---"`, `----`, `"- "`, `"--- -"`, `"- ---"`, `"- --- -"`, `-- --`, `?-`, `-?`, `?---`, `---?`, `"--- ?"`,
				`.`, `..`, `"..."`, `....`, `"... ."`, `. ...`, `. ... .`, `.. ..`, `?...`, `...?`, `"... ?"`,
				`.`, `/`, `":"`, `;`, `<`, `=`, `">"`, `"?"`, `"["`, `\`, `"]"`, `^`, `_`, `"{"`, `"|"`, `"}"`, `"~"`, `"@"`, "\"`\"",
				`"%TAG"`, `"!!str"`, `"!<>"`, `"&anchor"`, `"*anchor"`, `https://example.com/?q=text#fragment`, `"%!/bin/sh"`,
				`"- ."`, `. -`, `-.`, `.-`, `"? ."`, `. ?`, `?.`, `.?`, `"?\t."`, `": ."`, `". :"`, `":\t."`, `. :?`, `"?:"`, `:?`, `. ? :.`, `"[]"`, `"{}"`,
				`". #"`, `"# ."`, `.#.`, `". #."`, `.# .`, `". # ."`, `. ! " $ % & ' ( ) * + , - / ; < = > ? [ \ ] ^ _ { | } ~`,
			}),
		},
		{
			name: "quote white spaces",
			src:  `" " "\t" " ." " .\n" ". " "\t." ".\t" ". ." ".\t."`,
			want: join([]string{`" "`, `"\t"`, `" ."`, `" .\n"`, `". "`, `"\t."`, `".\t"`, `. .`, `".\t."`}),
		},
		{
			name: "quote and escape special characters",
			src: "\" \\\\ \" \"\\u001F\" \"\\u001F\\n\" \"\u007F\" \"\u007F\\n\" \"\u0080\" \".\u0089.\" \"\u009F\" \"\u009F\\n\"" +
				"\"\uFDCF\" \"\uFDD0\uFDD1\uFDD2\uFDD3\uFDD4\uFDD5\uFDD6\uFDD7\uFDD8\uFDD9\uFDDA\uFDDB\uFDDC\uFDDD\uFDDE\uFDDF\uFDE0\uFDEF\"" +
				"\"\uFDF0\" \"\uFEFE\" \"\uFEFF\" \"\uFFFD\" \"\uFFFE\" \"\uFFFF\" \"\uFFFF\\n\"",
			want: join([]string{
				`" \\ "`, `"\x1F"`, `"\x1F\n"`, `"\x7F"`, `"\x7F\n"`, `"\x80"`, `".\x89."`, `"\x9F"`, `"\x9F\n"`,
				"\uFDCF", `"\uFDD0\uFDD1\uFDD2\uFDD3\uFDD4\uFDD5\uFDD6\uFDD7\uFDD8\uFDD9\uFDDA\uFDDB\uFDDC\uFDDD\uFDDE\uFDDF\uFDE0\uFDEF"`,
				"\uFDF0", "\uFEFE", `"\uFEFF"`, "\uFFFD", `"\uFFFE"`, `"\uFFFF"`, `"\uFFFF\n"`,
			}),
		},
		{
			name: "empty object",
			src:  "{}",
			want: `{}
`,
		},
		{
			name: "simple object",
			src:  `{"foo": 128, "bar": null, "baz": false}`,
			want: `foo: 128
bar: null
baz: false
`,
		},
		{
			name: "nested object",
			src: `{
				"foo": {"bar": {"baz": 128, "bar": null}, "baz": 0},
				"bar": {"foo": {}, "bar": {"bar": {}}, "baz": {}},
				"baz": {}
			}`,
			want: `foo:
  bar:
    baz: 128
    bar: null
  baz: 0
bar:
  foo: {}
  bar:
    bar: {}
  baz: {}
baz: {}
`,
		},
		{
			name: "multiple objects",
			src:  `{}{"foo":128}{}`,
			want: join([]string{"{}", "foo: 128", "{}"}),
		},
		{
			name: "unclosed object with no entries",
			src:  "{",
			want: "{}\n",
			err:  "unexpected EOF",
		},
		{
			name: "unclosed object after object key",
			src:  `{"foo"`,
			want: "foo:\n",
			err:  "unexpected EOF",
		},
		{
			name: "unclosed object after object value",
			src:  `{"foo":128`,
			want: "foo: 128\n",
			err:  "unexpected EOF",
		},
		{
			name: "empty array",
			src:  "[]",
			want: `[]
`,
		},
		{
			name: "simple array",
			src:  `[null,false,true,-128,12345678901234567890,"foo bar baz"]`,
			want: `- null
- false
- true
- -128
- 12345678901234567890
- foo bar baz
`,
		},
		{
			name: "nested array",
			src:  "[0,[1],[2,3],[4,[5,[6,[],7],[]],[8]],[],9]",
			want: `- 0
- - 1
- - 2
  - 3
- - 4
  - - 5
    - - 6
      - []
      - 7
    - []
  - - 8
- []
- 9
`,
		},
		{
			name: "nested object and array",
			src:  `{"foo":[0,{"bar":[],"foo":{}},[{"foo":[{"foo":[]}]}],[[[{}]]]],"bar":[{}]}`,
			want: `foo:
  - 0
  - bar: []
    foo: {}
  - - foo:
        - foo: []
  - - - - {}
bar:
  - {}
`,
		},
		{
			name: "multiple arrays",
			src:  `[][{"foo":128}][]`,
			want: join([]string{"[]", "- foo: 128", "[]"}),
		},
		{
			name: "deeply nested object",
			src:  strings.Repeat(`{"x":`, 100) + "{}" + strings.Repeat("}", 100),
			want: (func() string {
				var sb strings.Builder
				spaces := strings.Repeat("  ", 100)
				for i := 0; i < 100; i++ {
					if i > 0 {
						sb.WriteByte('\n')
					}
					sb.WriteString(spaces[:2*i])
					sb.WriteString("x:")
				}
				sb.WriteString(" {}\n")
				return sb.String()
			})(),
		},
		{
			name: "unclosed empty array",
			src:  "[",
			want: "[]\n",
			err:  "unexpected EOF",
		},
		{
			name: "unclosed array after an element",
			src:  "[1",
			want: "- 1\n",
			err:  "unexpected EOF",
		},
		{
			name: "unexpected closing bracket",
			src:  `{"x":]`,
			want: "x:\n",
			err:  "invalid character ']'",
		},
		{
			name: "unexpected character in array",
			src:  "[1,%",
			want: "- 1\n- \n",
			err:  "invalid character '%'",
		},
		{
			name: "block style string",
			src: `"\n" "\n\n" "a\n" "a\n\n" "a\n\n\n" "a \n" "a\t\n" "a\n " "a\n\t" "a\r\n"
				"a\nb" "a\r\nb" "a\n\nb" "a\nb\n" "a\n  b\nc" "a\n  b\nc\n"
				"\na" "\n a" "\n\na" "\na\n" "\na\nb\n" "\na\nb\n\n" "\n\ta\n"
				"# a\n" "# a\r" "[a]\n" "!\n#\n%\n- a\n" "- a\n- b\n" "---\n" "a: b # c\n"`,
			want: join([]string{
				`"\n"`, `"\n\n"`, "|\n  a", "|+\n  a\n", "|+\n  a\n\n", "|\n  a ", "|\n  a\t", "|-\n  a\n   ", "|-\n  a\n  \t", `"a\r\n"`,
				"|-\n  a\n  b", `"a\r\nb"`, "|-\n  a\n\n  b", "|\n  a\n  b", "|-\n  a\n    b\n  c", "|\n  a\n    b\n  c",
				"|-\n\n  a", `"\n a"`, "|-\n\n\n  a", "|\n\n  a", "|\n\n  a\n  b", "|+\n\n  a\n  b\n", `"\n\ta\n"`,
				"|\n  # a", `"# a\r"`, "|\n  [a]", "|\n  !\n  #\n  %\n  - a", "|\n  - a\n  - b", "|\n  ---", "|\n  a: b # c",
			}),
		},
		{
			name: "block style string in object and array",
			src:  `{"x": "a\nb\n", "y": ["\na","\na\n"], "z": {"a\nb": {"a\nb\n": ["a\nb"]}}}{"w":"a\nb"}`,
			want: `x: |
  a
  b
"y":
  - |-

    a
  - |

    a
z:
  ? |-
    a
    b
  :
    ? |
      a
      b
    :
      - |-
        a
        b
---
w: |-
  a
  b
`,
		},
		{
			name: "large array",
			src:  "[" + strings.Repeat(`"test",`, 999) + `"test"]`,
			want: strings.Repeat("- test\n", 1000),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			err := json2yaml.Convert(&sb, strings.NewReader(tc.src))
			if got, want := diff(sb.String(), tc.want); got != want {
				t.Fatalf("should write\n  %q\nbut got\n  %q", want, got)
			}
			if tc.err == "" {
				if err != nil {
					t.Fatalf("should not raise an error but got: %s", err)
				}
			} else {
				if err == nil {
					t.Fatalf("should raise an error %q but got no error", tc.err)
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("should raise an error %q but got error %q", tc.err, err)
				}
			}
		})
	}
}

type errWriter struct{}

func (w errWriter) Write(bs []byte) (int, error) {
	return 0, errors.New(fmt.Sprint(len(bs)))
}

func TestConvertError(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		err  string
	}{
		{
			name: "null",
			src:  "null",
			err:  fmt.Sprint(len("null\n")),
		},
		{
			name: "large object key",
			src:  `{"` + strings.Repeat("test", 1200) + `":0}`,
			err:  fmt.Sprint(len("test") * 1200),
		},
		{
			name: "large object value",
			src:  `{"x":"` + strings.Repeat("test", 1200) + `"}`,
			err:  fmt.Sprint(len("x: ") + len("test")*1200),
		},
		{
			name: "large array",
			src:  "[" + strings.Repeat(`"test",`, 1000) + `"test"]`,
			err:  fmt.Sprint(len("- test\n")*(4*1024/len("- test\n")+1) - 1),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := json2yaml.Convert(errWriter{}, strings.NewReader(tc.src))
			if err == nil {
				t.Fatalf("should raise an error %q but got no error", tc.err)
			}
			if err.Error() != tc.err {
				t.Fatalf("should raise an error %q but got error %q", tc.err, err)
			}
		})
	}
}

func join(xs []string) string {
	var sb strings.Builder
	n := 5*(len(xs)-1) + 1
	for _, x := range xs {
		n += len(x)
	}
	sb.Grow(n)
	for i, x := range xs {
		if i > 0 {
			sb.WriteString("---\n")
		}
		sb.WriteString(x)
		sb.WriteString("\n")
	}
	return sb.String()
}

func diff(xs, ys string) (string, string) {
	if xs == ys {
		return "", ""
	}
	for {
		i := strings.IndexByte(xs, '\n')
		j := strings.IndexByte(ys, '\n')
		if i < 0 || j < 0 || xs[:i] != ys[:j] {
			break
		}
		xs, ys = xs[i+1:], ys[j+1:]
	}
	for {
		i := strings.LastIndexByte(xs, '\n')
		j := strings.LastIndexByte(ys, '\n')
		if i < 0 || j < 0 || xs[i:] != ys[j:] {
			break
		}
		xs, ys = xs[:i], ys[:j]
	}
	return xs, ys
}

func ExampleConvert() {
	input := strings.NewReader(`{"Hello": "world!"}`)
	var output strings.Builder
	if err := json2yaml.Convert(&output, input); err != nil {
		log.Fatalln(err)
	}
	fmt.Print(output.String())
	// Output:
	// Hello: world!
}
