package buffer

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gammazero/deque"
	"github.com/itzmanish/go-ortc/v2/pkg/audio"
	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"go.uber.org/atomic"

	"github.com/livekit/mediatransportutil"
	"github.com/livekit/mediatransportutil/pkg/bucket"
	"github.com/livekit/mediatransportutil/pkg/nack"
	"github.com/livekit/mediatransportutil/pkg/twcc"
)

const (
	ReportDelta = 1e9
)

type pendingPacket struct {
	arrivalTime int64
	packet      []byte
}

type ExtPacket struct {
	VideoLayer
	Arrival   int64
	Packet    *rtp.Packet
	Payload   interface{}
	KeyFrame  bool
	RawPacket []byte
}

// Buffer contains all packets
type Buffer struct {
	sync.RWMutex
	bucket        *bucket.Bucket
	nacker        *nack.NackQueue
	videoPool     *sync.Pool
	audioPool     *sync.Pool
	codecType     webrtc.RTPCodecType
	extPackets    deque.Deque
	pPackets      []pendingPacket
	closeOnce     sync.Once
	mediaSSRC     uint32
	clockRate     uint32
	lastReport    int64
	twccExt       uint8
	audioLevelExt uint8
	bound         bool
	closed        atomic.Bool
	mime          string

	// supported feedbacks
	latestTSForAudioLevelInitialized bool
	latestTSForAudioLevel            uint32

	twcc             *twcc.Responder
	audioLevelParams audio.AudioLevelParams
	audioLevel       *audio.AudioLevel

	lastPacketRead int

	pliThrottle int64

	rtpStats             *RTPStats
	rrSnapshotId         uint32
	deltaStatsSnapshotId uint32

	lastFractionLostToReport uint8 // Last fraction lost from subscribers, should report to publisher; Audio only

	// callbacks
	onClose            func()
	onRtcpFeedback     func([]rtcp.Packet)
	onRtcpSenderReport func(*RTCPSenderReportData)
	onFpsChanged       func()

	// logger
	logger logger.Logger

	maxLayerChangedCB func(int32, int32)

	paused              bool
	frameRateCalculator [DefaultMaxLayerSpatial + 1]FrameRateCalculator
	frameRateCalculated bool
}

// NewBuffer constructs a new Buffer
func NewBuffer(ssrc uint32, vp, ap *sync.Pool) *Buffer {
	l := logger.NewLogger(fmt.Sprintf("Buffer [SSRC: %v]", ssrc)) // will be reset with correct context via SetLogger
	b := &Buffer{
		mediaSSRC:   ssrc,
		videoPool:   vp,
		audioPool:   ap,
		pliThrottle: int64(500 * time.Millisecond),
		logger:      l,
	}
	b.extPackets.SetMinCapacity(7)
	return b
}

func (b *Buffer) SetLogger(logger logger.Logger) {
	b.Lock()
	defer b.Unlock()

	b.logger = logger
	if b.rtpStats != nil {
		b.rtpStats.SetLogger(logger)
	}
}

func (b *Buffer) SetPaused(paused bool) {
	b.Lock()
	defer b.Unlock()

	b.paused = paused
}

func (b *Buffer) SetTWCC(twcc *twcc.Responder) {
	b.Lock()
	defer b.Unlock()

	b.twcc = twcc
}

func (b *Buffer) SetAudioLevelParams(audioLevelParams audio.AudioLevelParams) {
	b.Lock()
	defer b.Unlock()

	b.audioLevelParams = audioLevelParams
}

func (b *Buffer) Bind(params webrtc.RTPParameters) {
	b.Lock()
	defer b.Unlock()
	if b.bound {
		return
	}
	codec := params.Codecs[0]

	b.rtpStats = NewRTPStats(RTPStatsParams{
		ClockRate: codec.ClockRate,
		Logger:    b.logger,
	})
	b.rrSnapshotId = b.rtpStats.NewSnapshotId()
	b.deltaStatsSnapshotId = b.rtpStats.NewSnapshotId()

	b.clockRate = codec.ClockRate
	b.lastReport = time.Now().UnixNano()
	b.mime = strings.ToLower(codec.MimeType)

	for _, ext := range params.HeaderExtensions {
		switch ext.URI {

		case sdp.AudioLevelURI:
			b.audioLevelExt = uint8(ext.ID)
			b.audioLevel = audio.NewAudioLevel(b.audioLevelParams)
		}
	}

	switch {
	case strings.HasPrefix(b.mime, "audio/"):
		b.codecType = webrtc.RTPCodecTypeAudio
		b.bucket = bucket.NewBucket(b.audioPool.Get().(*[]byte))
	case strings.HasPrefix(b.mime, "video/"):
		b.codecType = webrtc.RTPCodecTypeVideo
		b.bucket = bucket.NewBucket(b.videoPool.Get().(*[]byte))
		if b.frameRateCalculator[0] == nil && strings.EqualFold(codec.MimeType, webrtc.MimeTypeVP8) {
			b.frameRateCalculator[0] = NewFrameRateCalculatorVP8(b.clockRate, b.logger)
		}

	default:
		b.codecType = webrtc.RTPCodecType(0)
	}

	for _, fb := range codec.RTCPFeedback {
		switch fb.Type {
		case webrtc.TypeRTCPFBGoogREMB:
			b.logger.Debug("Setting feedback", "type", webrtc.TypeRTCPFBGoogREMB)
			b.logger.Debug("REMB not supported, RTCP feedback will not be generated")
		case webrtc.TypeRTCPFBTransportCC:
			if b.codecType == webrtc.RTPCodecTypeVideo {
				b.logger.Debug("Setting feedback", "type", webrtc.TypeRTCPFBTransportCC)
				for _, ext := range params.HeaderExtensions {
					if ext.URI == sdp.TransportCCURI {
						b.twccExt = uint8(ext.ID)
						break
					}
				}
			}
		case webrtc.TypeRTCPFBNACK:
			// pion use a single mediaengine to manage negotiated codecs of peerconnection, that means we can't have different
			// codec settings at track level for same codec type, so enable nack for all audio receivers but don't create nack queue
			// for red codec.
			// We should also not enable nack for audio altogether
			if strings.EqualFold(b.mime, "audio/red") {
				return
			}
			b.logger.Debug("Setting feedback", "type", webrtc.TypeRTCPFBNACK)
			b.nacker = nack.NewNACKQueue()
		}
	}

	for _, pp := range b.pPackets {
		b.calc(pp.packet, pp.arrivalTime)
	}
	b.pPackets = nil
	b.bound = true
}

// Write adds an RTP Packet, out of order, new packet may be arrived later
func (b *Buffer) Write(pkt []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()

	if b.closed.Load() {
		err = io.EOF
		return
	}

	if !b.bound {
		packet := make([]byte, len(pkt))
		copy(packet, pkt)
		b.pPackets = append(b.pPackets, pendingPacket{
			packet:      packet,
			arrivalTime: time.Now().UnixNano(),
		})
		return
	}

	b.calc(pkt, time.Now().UnixNano())
	return
}

func (b *Buffer) Read(buff []byte) (n int, err error) {
	for {
		if b.closed.Load() {
			err = io.EOF
			return
		}
		b.Lock()
		if b.pPackets != nil && len(b.pPackets) > b.lastPacketRead {
			if len(buff) < len(b.pPackets[b.lastPacketRead].packet) {
				err = bucket.ErrBufferTooSmall
				b.Unlock()
				return
			}
			n = len(b.pPackets[b.lastPacketRead].packet)
			copy(buff, b.pPackets[b.lastPacketRead].packet)
			b.lastPacketRead++
			b.Unlock()
			return
		}
		b.Unlock()
		time.Sleep(25 * time.Millisecond)
	}
}

func (b *Buffer) ReadExtended(buf []byte) (*ExtPacket, error) {
	for {
		if b.closed.Load() {
			return nil, io.EOF
		}
		b.Lock()
		if b.extPackets.Len() > 0 {
			ep := b.extPackets.PopFront().(*ExtPacket)
			ep = b.patchExtPacket(ep, buf)
			if ep == nil {
				b.Unlock()
				continue
			}

			b.Unlock()
			return ep, nil
		}
		b.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

func (b *Buffer) Close() error {
	b.Lock()
	defer b.Unlock()

	b.closeOnce.Do(func() {
		if b.bucket != nil && b.codecType == webrtc.RTPCodecTypeVideo {
			b.videoPool.Put(b.bucket.Src())
		}
		if b.bucket != nil && b.codecType == webrtc.RTPCodecTypeAudio {
			b.audioPool.Put(b.bucket.Src())
		}

		b.closed.Store(true)

		if b.rtpStats != nil {
			b.rtpStats.Stop()
			b.logger.Info("rtp stats", "direction", "upstream", "stats", b.rtpStats.ToString())
		}

		if b.onClose != nil {
			b.onClose()
		}
	})
	return nil
}

func (b *Buffer) OnClose(fn func()) {
	b.onClose = fn
}

func (b *Buffer) SetPLIThrottle(duration int64) {
	b.Lock()
	defer b.Unlock()

	b.pliThrottle = duration
}

func (b *Buffer) SendPLI(force bool) {
	b.RLock()
	if (b.rtpStats == nil || b.rtpStats.TimeSinceLastPli() < b.pliThrottle) && !force {
		b.RUnlock()
		return
	}

	b.rtpStats.UpdatePliAndTime(1)
	b.RUnlock()

	b.logger.Debug("send pli", "ssrc", b.mediaSSRC, "force", force)
	pli := []rtcp.Packet{
		&rtcp.PictureLossIndication{SenderSSRC: b.mediaSSRC, MediaSSRC: b.mediaSSRC},
	}

	if b.onRtcpFeedback != nil {
		b.onRtcpFeedback(pli)
	}
}

func (b *Buffer) SetRTT(rtt uint32) {
	b.Lock()
	defer b.Unlock()

	if rtt == 0 {
		return
	}

	if b.nacker != nil {
		b.nacker.SetRTT(rtt)
	}

	if b.rtpStats != nil {
		b.rtpStats.UpdateRtt(rtt)
	}
}

func (b *Buffer) calc(pkt []byte, arrivalTime int64) {
	pktBuf, err := b.bucket.AddPacket(pkt)
	if err != nil {
		//
		// Even when erroring, do
		//  1. state update
		//  2. TWCC just in case remote side is retransmitting an old packet for probing
		//
		// But, do not forward those packets
		//
		var rtpPacket rtp.Packet
		if uerr := rtpPacket.Unmarshal(pkt); uerr == nil {
			b.updateStreamState(&rtpPacket, arrivalTime)
			b.processHeaderExtensions(&rtpPacket, arrivalTime)
		}

		if err != bucket.ErrRTXPacket {
			b.logger.Warn("could not add RTP packet to bucket", err)
		}
		return
	}

	var p rtp.Packet
	err = p.Unmarshal(pktBuf)
	if err != nil {
		b.logger.Warn("error unmarshaling RTP packet", err)
		return
	}

	b.updateStreamState(&p, arrivalTime)
	b.processHeaderExtensions(&p, arrivalTime)

	b.doNACKs()

	b.doReports(arrivalTime)

	ep := b.getExtPacket(&p, arrivalTime)
	if ep == nil {
		return
	}
	b.extPackets.PushBack(ep)

	b.doFpsCalc(ep)
}

func (b *Buffer) patchExtPacket(ep *ExtPacket, buf []byte) *ExtPacket {
	n, err := b.getPacket(buf, ep.Packet.SequenceNumber)
	if err != nil {
		b.logger.Warn("could not get packet", err, "sn", ep.Packet.SequenceNumber)
		return nil
	}
	ep.RawPacket = buf[:n]

	// patch RTP packet to point payload to new buffer
	rtp := *ep.Packet
	payloadStart := ep.Packet.Header.MarshalSize()
	payloadEnd := payloadStart + len(ep.Packet.Payload)
	if payloadEnd > n {
		b.logger.Warn("unexpected marshal size", nil, "max", n, "need", payloadEnd)
		return nil
	}
	rtp.Payload = buf[payloadStart:payloadEnd]
	ep.Packet = &rtp

	return ep
}

func (b *Buffer) doFpsCalc(ep *ExtPacket) {
	if b.paused || b.frameRateCalculated || len(ep.Packet.Payload) == 0 {
		return
	}
	spatial := ep.Spatial
	if spatial < 0 || int(spatial) >= len(b.frameRateCalculator) {
		spatial = 0
	}
	if fr := b.frameRateCalculator[spatial]; fr != nil {
		if fr.RecvPacket(ep) {
			complete := true
			for _, fr2 := range b.frameRateCalculator {
				if fr2 != nil && !fr2.Completed() {
					complete = false
					break
				}
			}
			if complete {
				b.frameRateCalculated = true
				if f := b.onFpsChanged; f != nil {
					go f()
				}
			}
		}
	}
}

func (b *Buffer) updateStreamState(p *rtp.Packet, arrivalTime int64) {
	flowState := b.rtpStats.Update(&p.Header, len(p.Payload), int(p.PaddingSize), arrivalTime)
	if b.nacker != nil {
		b.nacker.Remove(p.SequenceNumber)

		if flowState.HasLoss {
			for lost := flowState.LossStartInclusive; lost != flowState.LossEndExclusive; lost++ {
				b.nacker.Push(lost)
			}
		}
	}
}

func (b *Buffer) processHeaderExtensions(p *rtp.Packet, arrivalTime int64) {
	// submit to TWCC even if it is a padding only packet. Clients use padding only packets as probes
	// for bandwidth estimation
	if b.twcc != nil && b.twccExt != 0 {
		if ext := p.GetExtension(b.twccExt); ext != nil {
			b.twcc.Push(binary.BigEndian.Uint16(ext[0:2]), arrivalTime, p.Marker)
		}
	}

	if b.audioLevelExt != 0 {
		if !b.latestTSForAudioLevelInitialized {
			b.latestTSForAudioLevelInitialized = true
			b.latestTSForAudioLevel = p.Timestamp
		}
		if e := p.GetExtension(b.audioLevelExt); e != nil {
			ext := rtp.AudioLevelExtension{}
			if err := ext.Unmarshal(e); err == nil {
				if (p.Timestamp - b.latestTSForAudioLevel) < (1 << 31) {
					duration := (int64(p.Timestamp) - int64(b.latestTSForAudioLevel)) * 1e3 / int64(b.clockRate)
					if duration > 0 {
						b.audioLevel.Observe(ext.Level, uint32(duration))
					}

					b.latestTSForAudioLevel = p.Timestamp
				}
			}
		}
	}
}

func (b *Buffer) getExtPacket(rtpPacket *rtp.Packet, arrivalTime int64) *ExtPacket {
	ep := &ExtPacket{
		Packet:  rtpPacket,
		Arrival: arrivalTime,
		VideoLayer: VideoLayer{
			Spatial:  InvalidLayerSpatial,
			Temporal: InvalidLayerTemporal,
		},
	}

	if len(rtpPacket.Payload) == 0 {
		// padding only packet, nothing else to do
		return ep
	}

	ep.Temporal = 0

	switch b.mime {
	case "video/vp8":
		vp8Packet := VP8{}
		if err := vp8Packet.Unmarshal(rtpPacket.Payload); err != nil {
			b.logger.Warn("could not unmarshal VP8 packet", err)
			return nil
		}
		ep.KeyFrame = vp8Packet.IsKeyFrame

		// vp8 with DependencyDescriptor enabled, use the TID from the descriptor
		vp8Packet.TID = uint8(ep.Temporal)
		ep.Spatial = InvalidLayerSpatial // vp8 don't have spatial scalability, reset to -1

		ep.Payload = vp8Packet
	case "video/h264":
		ep.KeyFrame = IsH264Keyframe(rtpPacket.Payload)
	case "video/av1":
		ep.KeyFrame = IsAV1Keyframe(rtpPacket.Payload)
	}

	if ep.KeyFrame {
		if b.rtpStats != nil {
			b.rtpStats.UpdateKeyFrame(1)
		}
	}

	return ep
}

func (b *Buffer) doNACKs() {
	if b.nacker == nil {
		return
	}

	if r, numSeqNumsNacked := b.buildNACKPacket(); r != nil {
		if b.onRtcpFeedback != nil {
			b.onRtcpFeedback(r)
		}
		if b.rtpStats != nil {
			b.rtpStats.UpdateNack(uint32(numSeqNumsNacked))
		}
	}
}

func (b *Buffer) doReports(arrivalTime int64) {
	timeDiff := arrivalTime - b.lastReport
	if timeDiff < ReportDelta {
		return
	}

	b.lastReport = arrivalTime

	// RTCP reports
	pkts := b.getRTCP()

	if pkts != nil && b.onRtcpFeedback != nil {
		b.onRtcpFeedback(pkts)
	}
}

func (b *Buffer) buildNACKPacket() ([]rtcp.Packet, int) {
	if nacks, numSeqNumsNacked := b.nacker.Pairs(); len(nacks) > 0 {
		pkts := []rtcp.Packet{&rtcp.TransportLayerNack{
			SenderSSRC: b.mediaSSRC,
			MediaSSRC:  b.mediaSSRC,
			Nacks:      nacks,
		}}
		return pkts, numSeqNumsNacked
	}
	return nil, 0
}

func (b *Buffer) buildReceptionReport() *rtcp.ReceptionReport {
	if b.rtpStats == nil {
		return nil
	}

	return b.rtpStats.SnapshotRtcpReceptionReport(b.mediaSSRC, b.lastFractionLostToReport, b.rrSnapshotId)
}

func (b *Buffer) SetSenderReportData(rtpTime uint32, ntpTime uint64) {
	srData := &RTCPSenderReportData{
		RTPTimestamp: rtpTime,
		NTPTimestamp: mediatransportutil.NtpTime(ntpTime),
		ArrivalTime:  time.Now(),
	}

	b.RLock()
	if b.rtpStats != nil {
		b.rtpStats.SetRtcpSenderReportData(srData)
	}
	b.RUnlock()

	if b.onRtcpSenderReport != nil {
		b.onRtcpSenderReport(srData)
	}
}

func (b *Buffer) GetSenderReportDataExt() *RTCPSenderReportDataExt {
	b.RLock()
	defer b.RUnlock()

	if b.rtpStats != nil {
		return b.rtpStats.GetRtcpSenderReportDataExt()
	}

	return nil
}

func (b *Buffer) SetLastFractionLostReport(lost uint8) {
	b.Lock()
	defer b.Unlock()

	b.lastFractionLostToReport = lost
}

func (b *Buffer) getRTCP() []rtcp.Packet {
	var pkts []rtcp.Packet

	rr := b.buildReceptionReport()
	if rr != nil {
		pkts = append(pkts, &rtcp.ReceiverReport{
			SSRC:    b.mediaSSRC,
			Reports: []rtcp.ReceptionReport{*rr},
		})
	}
	b.logger.Debugf("producer rtcp: %+v", rr)
	return pkts
}

func (b *Buffer) GetPacket(buff []byte, sn uint16) (int, error) {
	b.Lock()
	defer b.Unlock()

	return b.getPacket(buff, sn)
}

func (b *Buffer) getPacket(buff []byte, sn uint16) (int, error) {
	if b.closed.Load() {
		return 0, io.EOF
	}
	return b.bucket.GetPacket(buff, sn)
}

func (b *Buffer) OnRtcpFeedback(fn func(fb []rtcp.Packet)) {
	b.onRtcpFeedback = fn
}

func (b *Buffer) OnRtcpSenderReport(fn func(srData *RTCPSenderReportData)) {
	b.onRtcpSenderReport = fn
}

// GetMediaSSRC returns the associated SSRC of the RTP stream
func (b *Buffer) GetMediaSSRC() uint32 {
	return b.mediaSSRC
}

// GetClockRate returns the RTP clock rate
func (b *Buffer) GetClockRate() uint32 {
	return b.clockRate
}

func (b *Buffer) GetStats() *RTPStats {
	b.RLock()
	defer b.RUnlock()

	return b.rtpStats
}

func (b *Buffer) GetDeltaStats() *StreamStatsWithLayers {
	b.RLock()
	defer b.RUnlock()

	if b.rtpStats == nil {
		return nil
	}

	deltaStats := b.rtpStats.DeltaInfo(b.deltaStatsSnapshotId)
	if deltaStats == nil {
		return nil
	}

	return &StreamStatsWithLayers{
		RTPStats: deltaStats,
		Layers: map[int32]*RTPDeltaInfo{
			0: deltaStats,
		},
	}
}

func (b *Buffer) GetAudioLevel() (float64, bool) {
	b.RLock()
	defer b.RUnlock()

	if b.audioLevel == nil {
		return 0, false
	}

	return b.audioLevel.GetLevel()
}

// TODO : now we rely on stream tracker for layer change, dependency still
// work for that too. Do we keep it unchange or use both methods?
func (b *Buffer) OnMaxLayerChanged(fn func(int32, int32)) {
	b.maxLayerChangedCB = fn
}

func (b *Buffer) OnFpsChanged(f func()) {
	b.Lock()
	b.onFpsChanged = f
	b.Unlock()
}

func (b *Buffer) GetTemporalLayerFpsForSpatial(layer int32) []float32 {
	if int(layer) >= len(b.frameRateCalculator) {
		return nil
	}

	if fc := b.frameRateCalculator[layer]; fc != nil {
		return fc.GetFrameRate()
	}
	return nil
}
