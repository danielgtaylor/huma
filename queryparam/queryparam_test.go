package queryparam

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Borrowed from net/url/url_test.go
type parseTest struct {
	query string
	out   url.Values
	ok    bool
}

var parseTests = []parseTest{
	{
		query: "a=1",
		out:   url.Values{"a": []string{"1"}},
		ok:    true,
	},
	{
		query: "a=1&b=2",
		out:   url.Values{"a": []string{"1"}, "b": []string{"2"}},
		ok:    true,
	},
	{
		query: "a=1&a=2&a=banana",
		out:   url.Values{"a": []string{"1", "2", "banana"}},
		ok:    true,
	},
	{
		query: "ascii=%3Ckey%3A+0x90%3E",
		out:   url.Values{"ascii": []string{"<key: 0x90>"}},
		ok:    true,
	}, {
		query: "a=1;b=2",
		out:   url.Values{},
		ok:    false,
	}, {
		query: "a;b=1",
		out:   url.Values{},
		ok:    false,
	}, {
		query: "a=%3B", // hex encoding for semicolon
		out:   url.Values{"a": []string{";"}},
		ok:    true,
	},
	{
		query: "a%3Bb=1",
		out:   url.Values{"a;b": []string{"1"}},
		ok:    true,
	},
	{
		query: "a=1&a=2;a=banana",
		out:   url.Values{"a": []string{"1"}},
		ok:    false,
	},
	{
		query: "a;b&c=1",
		out:   url.Values{"c": []string{"1"}},
		ok:    false,
	},
	{
		query: "a=1&b=2;a=3&c=4",
		out:   url.Values{"a": []string{"1"}, "c": []string{"4"}},
		ok:    false,
	},
	{
		query: "a=1&b=2;c=3",
		out:   url.Values{"a": []string{"1"}},
		ok:    false,
	},
	{
		query: ";",
		out:   url.Values{},
		ok:    false,
	},
	{
		query: "a=1;",
		out:   url.Values{},
		ok:    false,
	},
	{
		query: "a=1&;",
		out:   url.Values{"a": []string{"1"}},
		ok:    false,
	},
	{
		query: ";a=1&b=2",
		out:   url.Values{"b": []string{"2"}},
		ok:    false,
	},
	{
		query: "a=1&b=2;",
		out:   url.Values{"a": []string{"1"}},
		ok:    false,
	},
}

func TestParseQuery(t *testing.T) {
	for _, test := range parseTests {
		t.Run(test.query, func(t *testing.T) {
			if test.ok {
				for k, v := range test.out {
					result := Get(test.query, k)
					assert.Equal(t, v[0], result)
				}
			} else {
				// Doesn't get stuck, doesn't crash.
				assert.Empty(t, Get(test.query, "missingvalue"))
			}
		})
	}
}

func TestQuery(t *testing.T) {
	for _, item := range []struct {
		query    string
		name     string
		expected string
	}{
		{"foo=bar", "foo", "bar"},
		{"foo=bar&baz=123", "foo", "bar"},
		{"foo=bar&baz=123", "baz", "123"},
		{"foo=bar&baz=123", "missing", ""},
		{"foo=bar&baz=123&bool&another", "bool", "true"},
	} {
		t.Run(item.query+"/"+item.name, func(t *testing.T) {
			assert.Equal(t, item.expected, Get(item.query, item.name))
		})
	}
}

var Result string

func BenchmarkNewQuery(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Result = Get("foo=bar&baz=123&bool", "baz")
	}
}

func BenchmarkStdQuery(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		values, _ := url.ParseQuery("foo=bar&baz=123&bool")
		Result = values.Get("baz")
	}
}

var Foo, Baz, Num, Float, Boolean string
var Values url.Values

func BenchmarkNewQueryMulti(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Foo = Get("foo=bar&baz=123&num=5&float=1.0&bool", "foo")
		Baz = Get("foo=bar&baz=123&num=5&float=1.0&bool", "baz")
		Num = Get("foo=bar&baz=123&num=5&float=1.0&bool", "num")
		Float = Get("foo=bar&baz=123&num=5&float=1.0&bool", "float")
		Boolean = Get("foo=bar&baz=123&num=5&float=1.0&bool", "bool")
	}
}

func BenchmarkStdQueryMulti(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Values, _ = url.ParseQuery("foo=bar&baz=123&bool")
		Foo = Values.Get("foo")
		Baz = Values.Get("baz")
		Num = Values.Get("num")
		Float = Values.Get("float")
		Boolean = Values.Get("bool")
	}
}
