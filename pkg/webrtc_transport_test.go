package goortc

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/stretchr/testify/assert"
)

var router *Router

func newTransportHelper(t assert.TestingT) (*WebRTCTransport, error) {
	if router == nil {
		router = newRouterHelper(t)
	}
	return newWebRTCTransport(1, router)
}

func transportConnectHelper(t assert.TestingT) (*WebRTCTransport, *testORTCStack) {
	server, err := newTransportHelper(t)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	client, err := newORTCStack(false)
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
			DTLSParameters:   caps1.DtlsParameters,
			ICECandidates:    caps1.IceCandidates,
			ICEParameters:    caps1.IceParameters,
			SCTPCapabilities: caps1.SCTPCapabilities,
		}, true)
		errCh <- err
	}()

	assert.NoError(t, <-errCh)

	return server, client
}

func TestNewTransport(t *testing.T) {
	transport, err := newTransportHelper(t)
	assert.Nil(t, err)
	assert.NotNil(t, transport)
}

func TestGetTransportCapabilities(t *testing.T) {
	transport, err := newTransportHelper(t)
	assert.Nil(t, err)
	caps := transport.GetCapabilities()
	assert.NotNil(t, caps)
	log.Println(caps)
}

func TestTransportConnect(t *testing.T) {
	server, client := transportConnectHelper(t)
	err := server.Stop()
	assert.NoError(t, err)
	err = client.close()
	assert.NoError(t, err)
}

func TestProduce(t *testing.T) {
	server, client := transportConnectHelper(t)
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	assert.NoError(t, err)

	rtpSender, err := client.api.NewRTPSender(track, client.dtls)
	assert.NoError(t, err)
	sendingParams := rtpSender.GetParameters()
	fmt.Println("send params:", sendingParams.Encodings)
	assert.NoError(t, rtpSender.Send(sendingParams))
	seenPacket, seenPacketCancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	go func(testing *testing.T) {
		<-time.After(20 * time.Millisecond)
		producer, err := server.Produce(webrtc.RTPCodecTypeVideo, RTPParameters{
			Mid:              "0",
			Encodings:        sendingParams.Encodings,
			HeaderExtensions: sendingParams.HeaderExtensions,
			Codecs:           sendingParams.Codecs,
		}, false)
		errCh <- err
		if producer == nil {
			errCh <- fmt.Errorf("Producer is nil")
			return
		}
		producer.onRTPPacket = func(producerId uint, rtp *buffer.ExtPacket) {
			logger.Info("rtp packet", rtp, "producer id", producerId)
			seenPacketCancel()
		}
	}(t)
	ticker := time.NewTicker(time.Millisecond * 20)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			select {
			case <-seenPacket.Done():
				return
			default:
				assert.NoError(t, track.WriteSample(media.Sample{Data: []byte{0xAA}, Duration: time.Second}))
			}
		}
	}()
	assert.NoError(t, <-errCh)
}
