package rtc

import (
	"log"
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
)

var router *Router

func newTransportHelper(t assert.TestingT, publisher bool) (*WebRTCTransport, error) {
	if router == nil {
		router = newRouterHelper(t)
	}
	return newWebRTCTransport(1, router, publisher)
}

func transportConnectHelper(t assert.TestingT, useLocalBuffer bool, publisher bool) (*WebRTCTransport, *testORTCStack) {
	server, err := newTransportHelper(t, publisher)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	client, err := newORTCStack(useLocalBuffer)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	sig, err := client.getSignal()
	assert.NoError(t, err)
	errCh := make(chan error)

	caps1 := server.GetCapabilities()
	assert.NotNil(t, caps1)

	go func() {
		err = server.Connect(
			sig.DTLSParameters,
			sig.ICEParameters,
		)
		errCh <- err
	}()

	go func() {
		err = client.setSignal(&testORTCSignal{
			DTLSParameters: caps1.DtlsParameters,
			ICECandidates:  caps1.IceCandidates,
			ICEParameters:  caps1.IceParameters,
			SCTPCapabilities: webrtc.SCTPCapabilities{
				MaxMessageSize: caps1.SCTPCapabilities.MaxMessageSize,
			},
		}, true)
		errCh <- err
	}()

	assert.NoError(t, <-errCh)

	return server, client
}

func TestNewTransport(t *testing.T) {
	transport, err := newTransportHelper(t, true)
	assert.Nil(t, err)
	assert.NotNil(t, transport)
}

func TestGetTransportCapabilities(t *testing.T) {
	transport, err := newTransportHelper(t, true)
	assert.Nil(t, err)
	caps := transport.GetCapabilities()
	assert.NotNil(t, caps)
	log.Println(caps)
}

func TestTransportConnect(t *testing.T) {
	server, client := transportConnectHelper(t, false, true)
	err := server.Stop()
	assert.NoError(t, err)
	err = client.close()
	assert.NoError(t, err)
}
