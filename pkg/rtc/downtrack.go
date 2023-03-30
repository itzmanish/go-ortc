package rtc

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/transport/v2/packetio"
	"github.com/pion/webrtc/v3"
)

const (
	maxPadding = 2000
)

// DownTrack  implements TrackLocal, is the track used to write packets
// to SFU Subscriber, the track handle the packets for simple producer
type DownTrack struct {
	id string

	bound         atomic.Bool
	mime          string
	ssrc          uint32
	streamID      string
	mid           string
	maxTrack      int
	payloadType   uint8
	sequencer     *sequencer
	started       bool
	kind          MediaKind
	bufferFactory *buffer.Factory
	rtcpReader    *buffer.RTCPReader
	payload       *[]byte

	enabled      atomic.Bool
	syncRequired atomic.Bool
	snOffset     uint16
	tsOffset     uint32
	lastSN       uint16
	lastTS       uint32
	twccSN       uint32

	codec                 webrtc.RTPCodecCapability
	enabledHeaders        []webrtc.RTPHeaderExtensionParameter
	producer              *Producer
	writeStream           webrtc.TrackLocalWriter
	onCloseHandler        func()
	onBind                func()
	rtpStats              *buffer.RTPStats
	deltaStatsSnapshotId  uint32
	onTransportCCFeedback func(*rtcp.TransportLayerCC)
	onREMBFeedback        func(*rtcp.ReceiverEstimatedMaximumBitrate)

	logger logger.Logger
}

// NewDownTrack returns a DownTrack.
func NewDownTrack(mid uint8, c webrtc.RTPCodecCapability, producer *Producer, bf *buffer.Factory, mt int) (*DownTrack, error) {
	id := GenerateRandomString(10)
	d := &DownTrack{
		id:            id,
		maxTrack:      mt,
		streamID:      producer.streamID,
		mid:           strconv.Itoa(int(mid)),
		bufferFactory: bf,
		producer:      producer,
		codec:         c,
		kind:          producer.kind,
		logger:        logger.NewLogger(fmt.Sprintf("Downtrack[id: %v]", id)).WithField("kind", producer.kind),
	}
	d.rtpStats = buffer.NewRTPStats(buffer.RTPStatsParams{
		ClockRate:              d.codec.ClockRate,
		IsReceiverReportDriven: true,
		Logger:                 d.logger,
	})
	d.deltaStatsSnapshotId = d.rtpStats.NewSnapshotId()

	return d, nil
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
		d.syncRequired.Store(true)
		d.enabled.Store(true)
		if rr := d.bufferFactory.GetOrNew(packetio.RTCPBufferPacket, uint32(t.SSRC())).(*buffer.RTCPReader); rr != nil {
			rr.OnPacket(func(pkt []byte) {
				d.handleRTCP(pkt)
			})
			d.rtcpReader = rr
		}

		if d.Kind() == webrtc.RTPCodecTypeVideo {
			d.sequencer = newSequencer(d.maxTrack, maxPadding, d.logger)
		}
		if d.onBind != nil {
			d.onBind()
		}
		d.bound.Store(true)
		d.enabledHeaders = t.HeaderExtensions()
		return codec, nil
	}
	return webrtc.RTPCodecParameters{}, webrtc.ErrUnsupportedCodec
}

// Unbind implements the teardown logic when the track is no longer needed. This happens
// because a track has been stopped.
func (d *DownTrack) Unbind(_ webrtc.TrackLocalContext) error {
	d.bound.Store(false)
	return nil
}

// ID is the unique identifier for this Track. This should be unique for the
// stream, but doesn't have to globally unique. A common example would be 'audio' or 'video'
// and StreamID would be 'desktop' or 'webcam'
func (d *DownTrack) ID() string { return d.id }

// Codec returns current track codec capability
func (d *DownTrack) Codec() webrtc.RTPCodecCapability { return d.codec }

// Mid returns the mid that this current track has
func (d *DownTrack) Mid() string { return d.mid }

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

// WriteRTP writes a RTP Packet to the DownTrack
func (d *DownTrack) WriteRTP(p *buffer.ExtPacket) error {
	if !d.enabled.Load() || !d.bound.Load() {
		return nil
	}
	return d.writeSimpleRTP(p)
}

func (d *DownTrack) Enabled() bool {
	return d.enabled.Load()
}

// Mute enables or disables media forwarding
func (d *DownTrack) Mute(val bool) {
	if d.enabled.Load() != val {
		return
	}
	d.enabled.Store(!val)
	d.syncRequired.Store(true)

}

// Close track
func (d *DownTrack) Close() {
	if !d.bound.Swap(false) {
		return
	}
	d.rtpStats.Stop()
	d.logger.Info("Closing downtrack", d.id)
	if d.payload != nil {
		packetFactory.Put(d.payload)
	}
	if d.rtcpReader != nil {
		d.logger.Debug("downtrack close rtcp reader")
		d.rtcpReader.Close()
		d.rtcpReader.OnPacket(nil)
	}
	if d.onCloseHandler != nil {
		d.onCloseHandler()
	}

}

// OnCloseHandler method to be called on remote tracked removed
func (d *DownTrack) OnCloseHandler(fn func()) {
	d.onCloseHandler = fn
}

func (d *DownTrack) OnBind(fn func()) {
	d.onBind = fn
}

func (d *DownTrack) OnTransportCCFeedback(hdlr func(*rtcp.TransportLayerCC)) {
	d.onTransportCCFeedback = hdlr
}

func (d *DownTrack) OnREMBFeedback(hdlr func(*rtcp.ReceiverEstimatedMaximumBitrate)) {
	d.onREMBFeedback = hdlr
}

func (d *DownTrack) CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk {
	if !d.bound.Load() {
		return nil
	}
	return []rtcp.SourceDescriptionChunk{
		{
			Source: d.ssrc,
			Items: []rtcp.SourceDescriptionItem{{
				Type: rtcp.SDESCNAME,
				Text: d.StreamID(),
			}},
		},
	}
}

func (d *DownTrack) CreateSenderReport() *rtcp.SenderReport {
	if !d.bound.Load() {
		return nil
	}

	return d.rtpStats.GetRtcpSenderReport(d.ssrc, d.producer.GetRTCPSenderReportDataExt())
}

func (d *DownTrack) writeSimpleRTP(extPkt *buffer.ExtPacket) error {
	if d.syncRequired.Load() {
		if d.Kind() == webrtc.RTPCodecTypeVideo {
			if !extPkt.KeyFrame {
				d.producer.buffer.SendPLI(true)
				return nil
			}
		}

		if d.lastSN != 0 {
			d.snOffset = extPkt.Packet.SequenceNumber - d.lastSN - 1
			d.tsOffset = extPkt.Packet.Timestamp - d.lastTS - 1
		}
		d.syncRequired.Store(false)
	}

	newSN := extPkt.Packet.SequenceNumber - d.snOffset
	newTS := extPkt.Packet.Timestamp - d.tsOffset
	if d.sequencer != nil {
		d.sequencer.push(extPkt.Packet.SequenceNumber, newSN, newTS, 0)
	}
	if !d.started {
		d.lastSN = newSN
		d.lastTS = newTS
		d.started = true
	}
	hdr := extPkt.Packet.Header
	err := d.prepareRTPHeaderForForwarding(&hdr, newTS, newSN)
	if err != nil {
		d.logger.Warn("unable to write headers", err)
		return err
	}
	_, err = d.writeStream.WriteRTP(&hdr, extPkt.Packet.Payload)
	if err != nil {
		return err
	}
	d.rtpStats.Update(&hdr, len(extPkt.Packet.Payload), 0, time.Now().UnixNano())

	return nil
}

func (d *DownTrack) prepareRTPHeaderForForwarding(hdr *rtp.Header, ts uint32, sn uint16) (err error) {
	hdr.PayloadType = d.payloadType
	hdr.Timestamp = ts
	hdr.SequenceNumber = sn
	hdr.SSRC = d.ssrc
	// If video and have rotation uri set keep that
	value := hdr.GetExtension(VideoOrientationExtensionID)
	// clear out extensions that may have been in the forwarded header
	hdr.Extension = false
	hdr.ExtensionProfile = 0
	hdr.Extensions = []rtp.Extension{}
	for _, hExt := range d.enabledHeaders {
		switch hExt.URI {
		case sdp.SDESMidURI:
			// Here we assume that there is MidMaxLength available bytes, even if now
			// they are padding bytes.
			if len(d.Mid()) > int(MidMaxLength) {
				return fmt.Errorf("no enough space for MID value [MidMaxLength: %v, mid: %s]", len(d.Mid()), d.Mid())
			}
			err = hdr.SetExtension(uint8(hExt.ID), []byte(d.Mid()))
		case sdp.ABSSendTimeURI:
			sendTime := rtp.NewAbsSendTimeExtension(time.Now())
			b, err1 := sendTime.Marshal()
			if err1 != nil {
				break
			}

			err = hdr.SetExtension(uint8(hExt.ID), b)
		case sdp.TransportCCURI:
			sequenceNumber := atomic.AddUint32(&d.twccSN, 1) - 1

			tcc, err1 := (&rtp.TransportCCExtension{TransportSequence: uint16(sequenceNumber)}).Marshal()
			if err1 != nil {
				break
			}
			err = hdr.SetExtension(uint8(hExt.ID), tcc)
		case VideoOrientationURI:
			if len(value) != 0 {
				err = hdr.SetExtension(VideoOrientationExtensionID, value)
			}
		}
		if err != nil {
			return
		}
	}
	return nil
}

func (d *DownTrack) retransmitPackets(nacks []uint16) {
	if d.sequencer == nil {
		return
	}

	var pool *[]byte
	defer func() {
		if pool != nil {
			packetFactory.Put(pool)
			pool = nil
		}
	}()
	src := packetFactory.Get().(*[]byte)
	defer packetFactory.Put(src)

	nackAcks := uint32(0)
	nackMisses := uint32(0)
	numRepeatedNACKs := uint32(0)
	for _, meta := range d.sequencer.getPacketsMeta(nacks) {
		nackAcks++

		if pool != nil {
			packetFactory.Put(pool)
			pool = nil
		}

		pktBuff := *src
		n, err := d.producer.ReadRTP(pktBuff, meta.sourceSeqNo)
		if err != nil {
			if err == io.EOF {
				break
			}
			nackMisses++
			continue
		}

		if meta.nacked > 1 {
			numRepeatedNACKs++
		}

		var pkt rtp.Packet
		if err = pkt.Unmarshal(pktBuff[:n]); err != nil {
			continue
		}
		err = d.prepareRTPHeaderForForwarding(&pkt.Header, meta.timestamp, meta.targetSeqNo)
		if err != nil {
			d.logger.Error("writing rtp header extensions err", err)
			continue
		}

		if _, err = d.writeStream.WriteRTP(&pkt.Header, pkt.Payload); err != nil {
			d.logger.Error("writing rtx packet err", err)
		} else {
			d.rtpStats.Update(&pkt.Header, len(pkt.Payload), 0, time.Now().UnixNano())
		}
	}
	d.rtpStats.UpdateNackProcessed(nackAcks, nackMisses, numRepeatedNACKs)
}

func (d *DownTrack) handleRTCP(bytes []byte) {
	pkts, err := rtcp.Unmarshal(bytes)
	if err != nil {
		d.logger.Error("unmarshal rtcp receiver packets err", err)
		return
	}

	pliOnce := true
	sendPliOnce := func() {
		if pliOnce && d.enabled.Load() {
			d.producer.buffer.SendPLI(false)
			d.rtpStats.UpdatePliTime()
			pliOnce = false
		}
	}

	rttToReport := uint32(0)

	var numNACKs uint32
	var numPLIs uint32
	var numFIRs uint32
	for _, pkt := range pkts {
		switch p := pkt.(type) {
		case *rtcp.PictureLossIndication:
			numPLIs++
			sendPliOnce()

		case *rtcp.FullIntraRequest:
			numFIRs++
			sendPliOnce()

		case *rtcp.ReceiverEstimatedMaximumBitrate:
			if d.onREMBFeedback != nil {
				d.onREMBFeedback(p)
			}

		case *rtcp.ReceiverReport:
			// create new receiver report w/ only valid reception reports
			for _, r := range p.Reports {
				if r.SSRC != d.ssrc {
					continue
				}

				rtt, isRttChanged := d.rtpStats.UpdateFromReceiverReport(r)
				if isRttChanged {
					rttToReport = rtt
				}
			}

		case *rtcp.TransportLayerNack:
			var nacks []uint16
			for _, pair := range p.Nacks {
				packetList := pair.PacketList()
				numNACKs += uint32(len(packetList))
				nacks = append(nacks, packetList...)
			}
			d.logger.Info("Got nack packets", nacks)
			go d.retransmitPackets(nacks)

		case *rtcp.TransportLayerCC:
			if p.MediaSSRC == d.ssrc && d.onTransportCCFeedback != nil {
				d.onTransportCCFeedback(p)
			}
		}

	}
	d.rtpStats.UpdatePli(numPLIs)
	d.rtpStats.UpdateFir(numFIRs)

	if rttToReport != 0 {
		if d.sequencer != nil {
			d.sequencer.setRTT(rttToReport)
		}
	}
}
