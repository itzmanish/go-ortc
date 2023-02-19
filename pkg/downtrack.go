package goortc

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/rtcp"
	"github.com/pion/transport/packetio"
	"github.com/pion/webrtc/v3"
)

// DownTrackType determines the type of track
type DownTrackType int

const (
	SimpleDownTrack DownTrackType = iota + 1
	SimulcastDownTrack
)

// DownTrack  implements TrackLocal, is the track used to write packets
// to SFU Subscriber, the track handle the packets for simple, simulcast
// and SVC Publisher.
type DownTrack struct {
	id string

	bound       atomicBool
	mime        string
	ssrc        uint32
	streamID    string
	maxTrack    int
	payloadType uint8
	// sequencer     *sequencer
	trackType     DownTrackType
	bufferFactory *buffer.Factory
	payload       *[]byte

	currentSpatialLayer int32
	targetSpatialLayer  int32
	temporalLayer       int32

	enabled  atomicBool
	reSync   atomicBool
	snOffset uint16
	tsOffset uint32
	lastSSRC uint32
	lastSN   uint16
	lastTS   uint32

	simulcast        simulcastTrackHelpers
	maxSpatialLayer  int32
	maxTemporalLayer int32

	codec          webrtc.RTPCodecCapability
	producer       *Producer
	writeStream    webrtc.TrackLocalWriter
	onCloseHandler func()
	onBind         func()
	closeOnce      sync.Once

	// Report helpers
	octetCount  uint32
	packetCount uint32
	maxPacketTs uint32
}

// NewDownTrack returns a DownTrack.
func NewDownTrack(c webrtc.RTPCodecCapability, producer *Producer, bf *buffer.Factory, mt int) (*DownTrack, error) {
	return &DownTrack{
		id:            producer.trackID,
		maxTrack:      mt,
		streamID:      producer.streamID,
		bufferFactory: bf,
		producer:      producer,
		codec:         c,
	}, nil
}

// Bind is called by the PeerConnection after negotiation is complete
// This asserts that the code requested is supported by the remote peer.
// If so it setups all the state (SSRC and PayloadType) to have a call
func (d *DownTrack) Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	parameters := webrtc.RTPCodecParameters{RTPCodecCapability: d.codec}
	if codec, err := codecParametersFuzzySearch(parameters, t.CodecParameters()); err == nil {
		d.ssrc = uint32(t.SSRC())
		d.payloadType = uint8(codec.PayloadType)
		d.writeStream = t.WriteStream()
		d.mime = strings.ToLower(codec.MimeType)
		d.reSync.set(true)
		d.enabled.set(true)
		if rr := d.bufferFactory.GetOrNew(packetio.RTCPBufferPacket, uint32(t.SSRC())).(*buffer.RTCPReader); rr != nil {
			rr.OnPacket(func(pkt []byte) {
				d.handleRTCP(pkt)
			})
		}
		if strings.HasPrefix(d.codec.MimeType, "video/") {
			// FIXME: I don't understand this so commenting out
			// d.sequencer = newSequencer(d.maxTrack)
		}
		if d.onBind != nil {
			d.onBind()
		}
		d.bound.set(true)
		return codec, nil
	}
	return webrtc.RTPCodecParameters{}, webrtc.ErrUnsupportedCodec
}

// Unbind implements the teardown logic when the track is no longer needed. This happens
// because a track has been stopped.
func (d *DownTrack) Unbind(_ webrtc.TrackLocalContext) error {
	d.bound.set(false)
	return nil
}

// ID is the unique identifier for this Track. This should be unique for the
// stream, but doesn't have to globally unique. A common example would be 'audio' or 'video'
// and StreamID would be 'desktop' or 'webcam'
func (d *DownTrack) ID() string { return d.id }

// Codec returns current track codec capability
func (d *DownTrack) Codec() webrtc.RTPCodecCapability { return d.codec }

// StreamID is the group this track belongs too. This must be unique
func (d *DownTrack) StreamID() string { return d.streamID }

// RID is the RTP Stream ID for this track. This is Simulcast specific and not used.
func (d *DownTrack) RID() string { return "" }

// Kind controls if this TrackLocal is audio or video
func (d *DownTrack) Kind() webrtc.RTPCodecType {
	switch {
	case strings.HasPrefix(d.codec.MimeType, "audio/"):
		return webrtc.RTPCodecTypeAudio
	case strings.HasPrefix(d.codec.MimeType, "video/"):
		return webrtc.RTPCodecTypeVideo
	default:
		return webrtc.RTPCodecType(0)
	}
}

func (d *DownTrack) Stop() {
	// TODO: close the consumer because the track is stopped
}

// WriteRTP writes a RTP Packet to the DownTrack
func (d *DownTrack) WriteRTP(p *buffer.ExtPacket, layer int) error {
	if !d.enabled.get() || !d.bound.get() {
		return nil
	}
	switch d.trackType {
	case SimpleDownTrack:
		return d.writeSimpleRTP(p)
	case SimulcastDownTrack:
		// TODO: add simulcat forwarder in future
		return fmt.Errorf("Simulacast is not implemented yet")
	}
	return nil
}

func (d *DownTrack) Enabled() bool {
	return d.enabled.get()
}

// Mute enables or disables media forwarding
func (d *DownTrack) Mute(val bool) {
	if d.enabled.get() != val {
		return
	}
	d.enabled.set(!val)
	if val {
		d.reSync.set(val)
	}
}

// Close track
func (d *DownTrack) Close() {
	d.closeOnce.Do(func() {
		// FIXME: downtrack closed means it will impact consumer not producer
		// need to fix this
		logger.Info("Closing sender", d.id)
		if d.payload != nil {
			packetFactory.Put(d.payload)
		}
		if d.onCloseHandler != nil {
			d.onCloseHandler()
		}
	})
}

func (d *DownTrack) SetInitialLayers(spatialLayer, temporalLayer int32) {
	atomic.StoreInt32(&d.currentSpatialLayer, spatialLayer)
	atomic.StoreInt32(&d.targetSpatialLayer, spatialLayer)
	atomic.StoreInt32(&d.temporalLayer, temporalLayer<<16|temporalLayer)
}

func (d *DownTrack) CurrentSpatialLayer() int {
	return int(atomic.LoadInt32(&d.currentSpatialLayer))
}

func (d *DownTrack) SwitchSpatialLayer(targetLayer int32, setAsMax bool) error {
	return nil
}

func (d *DownTrack) SwitchSpatialLayerDone(layer int32) {
	atomic.StoreInt32(&d.currentSpatialLayer, layer)
}

func (d *DownTrack) UptrackLayersChange(availableLayers []uint16) (int64, error) {
	if d.trackType == SimulcastDownTrack {
		currentLayer := uint16(d.currentSpatialLayer)
		maxLayer := uint16(atomic.LoadInt32(&d.maxSpatialLayer))

		var maxFound uint16 = 0
		layerFound := false
		var minFound uint16 = 0
		for _, target := range availableLayers {
			if target <= maxLayer {
				if target > maxFound {
					maxFound = target
					layerFound = true
				}
			} else {
				if minFound > target {
					minFound = target
				}
			}
		}
		var targetLayer uint16
		if layerFound {
			targetLayer = maxFound
		} else {
			targetLayer = minFound
		}
		if currentLayer != targetLayer {
			if err := d.SwitchSpatialLayer(int32(targetLayer), false); err != nil {
				return int64(targetLayer), err
			}
		}
		return int64(targetLayer), nil
	}
	return -1, fmt.Errorf("downtrack %s does not support simulcast", d.id)
}

func (d *DownTrack) SwitchTemporalLayer(targetLayer int32, setAsMax bool) {
	if d.trackType == SimulcastDownTrack {
		layer := atomic.LoadInt32(&d.temporalLayer)
		currentLayer := uint16(layer)
		currentTargetLayer := uint16(layer >> 16)

		// Don't switch until previous switch is done or canceled
		if currentLayer != currentTargetLayer {
			return
		}
		atomic.StoreInt32(&d.temporalLayer, targetLayer<<16|int32(currentLayer))
		if setAsMax {
			atomic.StoreInt32(&d.maxTemporalLayer, targetLayer)
		}
	}
}

// OnCloseHandler method to be called on remote tracked removed
func (d *DownTrack) OnCloseHandler(fn func()) {
	d.onCloseHandler = fn
}

func (d *DownTrack) OnBind(fn func()) {
	d.onBind = fn
}

func (d *DownTrack) CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk {
	if !d.bound.get() {
		return nil
	}
	return []rtcp.SourceDescriptionChunk{
		{
			Source: d.ssrc,
			Items: []rtcp.SourceDescriptionItem{{
				Type: rtcp.SDESCNAME,
				Text: d.streamID,
			}},
		}, {
			Source: d.ssrc,
			Items: []rtcp.SourceDescriptionItem{{
				Type: rtcp.SDESType(15),
				Text: d.producer.parameters.Mid,
			}},
		},
	}
}

func (d *DownTrack) UpdateStats(packetLen uint32) {
	atomic.AddUint32(&d.octetCount, packetLen)
	atomic.AddUint32(&d.packetCount, 1)
}

func (d *DownTrack) writeSimpleRTP(extPkt *buffer.ExtPacket) error {
	if d.reSync.get() {
		if d.Kind() == webrtc.RTPCodecTypeVideo {
			if !extPkt.KeyFrame {
				// FIXME: implement this SendRTCP first on producer
				d.producer.SendRTCP([]rtcp.Packet{
					&rtcp.PictureLossIndication{SenderSSRC: d.ssrc, MediaSSRC: extPkt.Packet.SSRC},
				})
				return nil
			}
		}

		if d.lastSN != 0 {
			d.snOffset = extPkt.Packet.SequenceNumber - d.lastSN - 1
			d.tsOffset = extPkt.Packet.Timestamp - d.lastTS - 1
		}
		atomic.StoreUint32(&d.lastSSRC, extPkt.Packet.SSRC)
		d.reSync.set(false)
	}

	d.UpdateStats(uint32(len(extPkt.Packet.Payload)))

	newSN := extPkt.Packet.SequenceNumber - d.snOffset
	newTS := extPkt.Packet.Timestamp - d.tsOffset
	// FIXME:  no sequencer now
	// if d.sequencer != nil {
	// 	d.sequencer.push(extPkt.Packet.SequenceNumber, newSN, newTS, 0, extPkt.Head)
	// }
	if extPkt.Head {
		d.lastSN = newSN
		d.lastTS = newTS
	}
	hdr := extPkt.Packet.Header
	hdr.PayloadType = d.payloadType
	hdr.Timestamp = newTS
	hdr.SequenceNumber = newSN
	hdr.SSRC = d.ssrc

	_, err := d.writeStream.WriteRTP(&hdr, extPkt.Packet.Payload)
	return err
}

func (d *DownTrack) handleRTCP(bytes []byte) {
	if !d.enabled.get() {
		return
	}

	pkts, err := rtcp.Unmarshal(bytes)
	if err != nil {
		logger.Error(err, "Unmarshal rtcp receiver packets err")
	}

	var fwdPkts []rtcp.Packet
	pliOnce := true
	firOnce := true

	var (
		maxRatePacketLoss  uint8
		expectedMinBitrate uint64
	)

	ssrc := atomic.LoadUint32(&d.lastSSRC)
	if ssrc == 0 {
		return
	}
	for _, pkt := range pkts {
		switch p := pkt.(type) {
		case *rtcp.PictureLossIndication:
			if pliOnce {
				p.MediaSSRC = ssrc
				p.SenderSSRC = d.ssrc
				fwdPkts = append(fwdPkts, p)
				pliOnce = false
			}
		case *rtcp.FullIntraRequest:
			if firOnce {
				p.MediaSSRC = ssrc
				p.SenderSSRC = d.ssrc
				fwdPkts = append(fwdPkts, p)
				firOnce = false
			}
		case *rtcp.ReceiverEstimatedMaximumBitrate:
			if expectedMinBitrate == 0 || expectedMinBitrate > uint64(p.Bitrate) {
				expectedMinBitrate = uint64(p.Bitrate)
			}
		case *rtcp.ReceiverReport:
			for _, r := range p.Reports {
				if maxRatePacketLoss == 0 || maxRatePacketLoss < r.FractionLost {
					maxRatePacketLoss = r.FractionLost
				}
			}
			// FIXME: nack not implemented and will be implemented later
			// case *rtcp.TransportLayerNack:
			// 	if d.sequencer != nil {
			// 		var nackedPackets []packetMeta
			// 		for _, pair := range p.Nacks {
			// 			nackedPackets = append(nackedPackets, d.sequencer.getSeqNoPairs(pair.PacketList())...)
			// 		}
			// 		if err = d.producer.RetransmitPackets(d, nackedPackets); err != nil {
			// 			return
			// 		}
			// 	}
			// }
		}
		if d.trackType == SimulcastDownTrack && (maxRatePacketLoss != 0 || expectedMinBitrate != 0) {
			// d.handleLayerChange(maxRatePacketLoss, expectedMinBitrate)
		}

		if len(fwdPkts) > 0 {
			// FIXME: not implemented
			d.producer.SendRTCP(fwdPkts)
		}
	}

}

func (d *DownTrack) getSRStats() (octets, packets uint32) {
	octets = atomic.LoadUint32(&d.octetCount)
	packets = atomic.LoadUint32(&d.packetCount)
	return
}
