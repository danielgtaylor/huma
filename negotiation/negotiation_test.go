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

func TestNoMatch(t *testing.T) {
	assert.Equal(t, "", SelectQValue("a; q=1.0, b;q=1.0,c; q=0.3", []string{"d", "e"}))
}
