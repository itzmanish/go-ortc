package goortc

import (
	"log"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTransportHelper(t assert.TestingT) (*WebRTCTransport, error) {
	return newWebRTCTransport(1, newRouterHelper(t))
}

func TestNewTransport(t *testing.T) {
	transport, err := newTransportHelper(t)
	assert.Nil(t, err)
	assert.NotNil(t, transport)
}

func TestGetTransportCapabilities(t *testing.T) {
	transport, err := newTransportHelper(t)
	assert.Nil(t, err)
	caps, err := transport.GetCapabilities()
	assert.NoError(t, err)
	assert.NotNil(t, caps)
	log.Println(caps)
}

func TestTransportConnect(t *testing.T) {
	transport1, err := newTransportHelper(t)
	assert.NoError(t, err)
	assert.NotNil(t, transport1)
	transport2, err := newTransportHelper(t)
	assert.NoError(t, err)
	assert.NotNil(t, transport2)

	caps1, err := transport1.GetCapabilities()
	assert.NoError(t, err)
	assert.NotNil(t, caps1)

	caps2, err := transport2.GetCapabilities()
	assert.NoError(t, err)
	assert.NotNil(t, caps2)

	wg := sync.WaitGroup{}
	go func() {
		err := transport1.Connect(caps2.DtlsParameters)
		assert.NoError(t, err)
		wg.Add(1)
	}()

	go func() {
		err := transport2.Connect(caps1.DtlsParameters)
		assert.NoError(t, err)
		wg.Add(1)
	}()

	wg.Wait()

	err = transport1.Stop()
	assert.NoError(t, err)
	err = transport2.Stop()
	assert.NoError(t, err)
}
