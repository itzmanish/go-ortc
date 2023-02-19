package rtc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSFU(t *testing.T) {
	sfu, err := NewSFU()
	assert.NoError(t, err)
	assert.NotNil(t, sfu)
}
