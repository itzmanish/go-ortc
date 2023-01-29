package goortc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newRouterHelper(t assert.TestingT) *Router {
	sfu, err := NewSFU()
	assert.NoError(t, err)
	return sfu.NewRouter()
}

func TestNewRouter(t *testing.T) {
	router := newRouterHelper(t)
	assert.NotNil(t, router)
}
