package rtc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
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

func testProducerHelper(t *testing.T, server *WebRTCTransport, client *testORTCStack) *Producer {
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	assert.NoError(t, err)

	rtpSender, err := client.api.NewRTPSender(track, client.dtls)
	assert.NoError(t, err)
	sendingParams := rtpSender.GetParameters()
	parsedSendingParams := ParseRTPSendParametersToORTC(sendingParams)
	fmt.Println("send params:", sendingParams.Encodings)
	assert.NoError(t, rtpSender.Send(sendingParams))
	seenPacket, seenPacketCancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	wg := &sync.WaitGroup{}
	var producer *Producer
	wg.Add(1)
	go func(testing *testing.T) {
		<-time.After(20 * time.Millisecond)
		producer, err = server.Produce(VideoMediaKind, ConvertRTPSendParametersToRTPReceiveParameters(RTPSendParameters{
			RTPParameters: RTPParameters{
				Mid:              parsedSendingParams.Mid,
				HeaderExtensions: parsedSendingParams.HeaderExtensions,
				Rtcp:             parsedSendingParams.Rtcp,
				Codecs: []RTPCodecParameters{
					parsedSendingParams.Codecs[0],
				},
			},
			Encodings: parsedSendingParams.Encodings,
		}), false)
		errCh <- err
		if producer == nil {
			errCh <- fmt.Errorf("Producer is nil")
			return
		}
		producer.onRTPPacket = func(producerId uint, rtp *buffer.ExtPacket) {
			logger.Info("rtp packet", rtp, "producer id", producerId)
			wg.Done()
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
	wg.Wait()
	return producer
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

func TestProduce(t *testing.T) {
	server, client := transportConnectHelper(t, false, true)
	testProducerHelper(t, server, client)
}

func TestConsume(t *testing.T) {
	producingServer, producingClient := transportConnectHelper(t, false, true)
	consumingServer, consumingClient := transportConnectHelper(t, true, false)
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	assert.NoError(t, err)

	rtpSender, err := producingClient.api.NewRTPSender(track, producingClient.dtls)
	assert.NoError(t, err)
	sendingParams := rtpSender.GetParameters()
	parsedSendingParams := ParseRTPSendParametersToORTC(sendingParams)
	fmt.Println("send params:", sendingParams.Encodings)
	assert.NoError(t, rtpSender.Send(sendingParams))
	seenPacket, seenPacketCancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	wg := &sync.WaitGroup{}
	var producer *Producer
	go func(testing *testing.T) {
		<-time.After(20 * time.Millisecond)
		producer, err = producingServer.Produce(VideoMediaKind, ConvertRTPSendParametersToRTPReceiveParameters(parsedSendingParams), false)
		errCh <- err
		if producer == nil {
			errCh <- fmt.Errorf("Producer is nil")
			return
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
	consumer, err := consumingServer.Consume(producer.Id, false)
	assert.NoError(t, err)

	consumer.onRTPPacket = func(id uint, p *buffer.ExtPacket) {
		logger.Info("onRTPPacket: ", id, p)
		assert.NotNil(t, p)
		seenPacketCancel()
	}
	consumeParams := consumer.GetParameters()

	receiver, err := consumingClient.api.NewRTPReceiver(webrtc.RTPCodecTypeVideo, consumingClient.dtls)
	assert.NoError(t, err)

	assert.NoError(t, receiver.Receive(ParseRTPReciveParametersFromORTC(ConvertRTPSendParametersToRTPReceiveParameters(consumeParams))))

	wg.Add(1)
	go func() {
		track := receiver.Track()
		buff := tempBuff.GetBuffer(uint32(track.SSRC()))
		if buff == nil {
			t.Error("buffer not found")
		}
		buff.Bind(receiver.GetParameters(), buffer.Options{
			MaxBitRate: 1500,
		})
		extP, err := buff.ReadExtended()
		logger.Info("extP: ", extP)
		assert.NoError(t, err)

		wg.Done()
	}()

	wg.Wait()

}
