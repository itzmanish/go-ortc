package goortc

import (
	"github.com/stretchr/testify/assert"
)

func newRouterHelper(t assert.TestingT) *Router {
	sfu, err := NewSFU()
	assert.NoError(t, err)
	return sfu.NewRouter()
}
