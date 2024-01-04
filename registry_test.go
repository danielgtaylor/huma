package huma

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2/examples/protodemo/protodemo"
	"github.com/stretchr/testify/assert"
)

type Output[T any] struct {
	data T
}

type Embedded[P any] struct {
	data P
}

func TestDefaultSchemaNamer(t *testing.T) {
	testUser := Output[*[]Embedded[protodemo.User]]{}

	name := DefaultSchemaNamer(reflect.TypeOf(testUser), "hint")
	fmt.Println(reflect.TypeOf(testUser))
	fmt.Println(name)
	assert.True(t, name == "Outputgithubcomdanielgtaylorhumav2Embeddedgithubcomdanielgtaylorhumav2examplesprotodemoprotodemoUser")
}
