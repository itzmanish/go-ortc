package rtc

import (
	"fmt"
	"io"
	"time"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"go.uber.org/atomic"
)

const sdBatchSize = 20

type Consumer struct {
	logger.Logger
	Id uint

	closed      atomic.Bool
	closeCh     chan bool
	paused      atomic.Bool
	priority    uint
	kind        MediaKind
	isSimulcast bool

	parameters RTPSendParameters
	track      *DownTrack
	producer   *Producer
	sender     *webrtc.RTPSender
	transport  *WebRTCTransport

	onRTPPacket func(id uint, packet *buffer.ExtPacket)
}

func newConsumer(id uint, producer *Producer, track *DownTrack, transport *WebRTCTransport, paused bool) (*Consumer, error) {
	sender, err := transport.api.NewRTPSender(track, transport.dtlsConn)
	if err != nil {
		return nil, errFailedToCreateConsumer(err)
	}

	parameters := sender.GetParameters()
	err = sender.Send(parameters)
	if err != nil {
		return nil, err
	}
	ortcParams := ParseRTPSendParametersToORTC(parameters)
	ortcParams.Rtcp = RTCPParameters{
		Cname:       track.StreamID(),
		Mux:         true,
		ReducedSize: true,
	}
	ortcParams.Mid = track.Mid()
	consumer := &Consumer{
		Id:          id,
		producer:    producer,
		sender:      sender,
		transport:   transport,
		track:       track,
		kind:        producer.kind,
		paused:      *atomic.NewBool(paused),
		parameters:  ortcParams,
		closed:      *atomic.NewBool(false),
		closeCh:     make(chan bool),
		onRTPPacket: nil,
		Logger:      logger.NewLogger(fmt.Sprintf("Consumer [id: %v]", id)).WithField("kind", producer.kind),
	}
	go consumer.rtcpWriteWorker()
	go consumer.rtcpReaderWorker()
	return consumer, nil
}

func (c *Consumer) Close() {
	if c.closed.Swap(true) {
		return
	}
	close(c.closeCh)
}

func (c *Consumer) Kind() MediaKind {
	return c.kind
}

func (c *Consumer) GetParameters() RTPSendParameters {
	return c.parameters
}

func (c *Consumer) Resume() {
	c.paused.CompareAndSwap(true, false)
}

func (c *Consumer) Pause() {
	c.paused.CompareAndSwap(false, true)
}

func (c *Consumer) Write(packet *buffer.ExtPacket) error {
	if c.paused.Load() {
		return nil
	}
	err := c.track.WriteRTP(packet)
	if err != nil {
		return err
	}
	if c.onRTPPacket != nil {
		c.onRTPPacket(c.Id, packet)
	}
	return nil
}

func (c *Consumer) SendRTCP(packets []rtcp.Packet) (int, error) {
	c.Logger.Debug("sending rtcp to consumer", packets)
	return c.transport.WriteRTCP(packets)
}

func (c *Consumer) rtcpReaderWorker() {
	rtcpBuf := make([]byte, 1500)
	for {
		select {
		case <-c.closeCh:
			c.Logger.Warn("consumer closed! stopping writing SR...")
			return
		default:
			// NOTE: I hate this but for some reason if we don't keep doing sender.Read()
			// then you won't get any data on rtcp buffer reader.
			_, _, rtcpErr := c.sender.Read(rtcpBuf)
			if rtcpErr != nil {
				return
			}
		}
	}
}

func (c *Consumer) rtcpWriteWorker() {
	for {
		select {
		case <-c.closeCh:
			c.Logger.Warn("consumer closed! stopping writing SR...")
			return
		case <-time.After(5 * time.Second):
			{
				var srs []rtcp.Packet
				var sd []rtcp.SourceDescriptionChunk
				sr := c.track.CreateSenderReport()
				chunks := c.track.CreateSourceDescriptionChunks()
				if sr == nil || chunks == nil {
					continue
				}
				srs = append(srs, sr)
				sd = append(sd, chunks...)

				// now send in batches of sdBatchSize
				var batch []rtcp.SourceDescriptionChunk
				var pkts []rtcp.Packet
				batchSize := 0
				for len(sd) > 0 || len(srs) > 0 {
					numSRs := len(srs)
					if numSRs > 0 {
						if numSRs > sdBatchSize {
							numSRs = sdBatchSize
						}
						pkts = append(pkts, srs[:numSRs]...)
						srs = srs[numSRs:]
					}

					size := len(sd)
					spaceRemain := sdBatchSize - batchSize
					if spaceRemain > 0 && size > 0 {
						if size > spaceRemain {
							size = spaceRemain
						}
						batch = sd[:size]
						sd = sd[size:]
						pkts = append(pkts, &rtcp.SourceDescription{Chunks: batch})
						if _, err := c.SendRTCP(pkts); err != nil {
							if err == io.EOF || err == io.ErrClosedPipe {
								return
							}
							c.Logger.Error("could not send down track reports", err)
						}
					}

					pkts = pkts[:0]
					batchSize = 0
				}
			}
		}
	}
}
