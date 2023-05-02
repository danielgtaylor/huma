package negotiation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccept(t *testing.T) {
	assert.Equal(t, "b", SelectQValue("a; q=0.5, b;q=1.0,c; q=0.3", []string{"a", "b", "d"}))
}

func TestAcceptBest(t *testing.T) {
	assert.Equal(t, "b", SelectQValue("a; q=1.0, b;q=1.0,c; q=0.3", []string{"b", "a"}))
}

func TestAcceptSimple(t *testing.T) {
	assert.Equal(t, "b", SelectQValue("a; q=0.5, b,c; q=0.3", []string{"a", "b", "c"}))
}

func TestAcceptSingle(t *testing.T) {
	assert.Equal(t, "b", SelectQValue("b", []string{"a", "b", "c"}))
}

func TestNoMatch(t *testing.T) {
	assert.Equal(t, "", SelectQValue("a; q=1.0, b;q=1.0,c; q=0.3", []string{"d", "e"}))
}

func TestAcceptFast(t *testing.T) {
	assert.Equal(t, "b", SelectQValueFast("a; q=0.5, b;q=1.0,c; q=0.3", []string{"a", "b", "d"}))
}

func TestAcceptFast2(t *testing.T) {
	assert.Equal(t, "application/cbor", SelectQValueFast("application/ion;q=0.6,application/json;q=0.5,application/yaml;q=0.5,text/*;q=0.2,application/cbor;q=0.9,application/msgpack;q=0.8,*/*", []string{"application/json", "application/cbor"}))
}

func TestAcceptBestFast(t *testing.T) {
	assert.Equal(t, "b", SelectQValueFast("a; q=1.0, b;q=1.0,c; q=0.3", []string{"b", "a"}))
}

func TestNoMatchFast(t *testing.T) {
	assert.Equal(t, "", SelectQValueFast("a; q=1.0, b;q=1.0,c; q=0.3", []string{"d", "e"}))
}

func BenchmarkMatch(b *testing.B) {
	header := "a; q=0.5, b;q=1.0,c; q=0.3"
	allowed := []string{"a", "b", "d"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SelectQValue(header, allowed)
	}
}

func BenchmarkMatchFast(b *testing.B) {
	header := "a; q=0.5, b;q=1.0,c; q=0.3"
	allowed := []string{"a", "b", "d"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SelectQValueFast(header, allowed)
	}
}
