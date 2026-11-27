package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationString(t *testing.T) {
	for _, value := range []string{"s", "s/1", "f@s", "f@s/id"} {
		loc, err := ParseLocation(value)
		assert.NoError(t, err)
		assert.Equal(t, value, loc.String())
	}
}
