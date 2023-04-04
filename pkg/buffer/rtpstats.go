package buffer

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"

	"github.com/livekit/mediatransportutil"
)

const (
	GapHistogramNumBins = 101
	NumSequenceNumbers  = 65536
	FirstSnapshotId     = 1
	SnInfoSize          = 2048
	SnInfoMask          = SnInfoSize - 1
	TooLargeOWD         = 400 * time.Millisecond
)

type RTPFlowState struct {
	HasLoss            bool
	LossStartInclusive uint16
	LossEndExclusive   uint16
}

type IntervalStats struct {
	packets            uint32
	bytes              uint64
	headerBytes        uint64
	packetsPadding     uint32
	bytesPadding       uint64
	headerBytesPadding uint64
	packetsLost        uint32
	frames             uint32
}

type RTPDeltaInfo struct {
	StartTime            time.Time
	Duration             time.Duration
	Packets              uint32
	Bytes                uint64
	HeaderBytes          uint64
	PacketsDuplicate     uint32
	BytesDuplicate       uint64
	HeaderBytesDuplicate uint64
	PacketsPadding       uint32
	BytesPadding         uint64
	HeaderBytesPadding   uint64
	PacketsLost          uint32
	PacketsMissing       uint32
	Frames               uint32
	RttMax               uint32
	JitterMax            float64
	Nacks                uint32
	Plis                 uint32
	Firs                 uint32
}

type Snapshot struct {
	startTime             time.Time
	extStartSN            uint32
	packetsDuplicate      uint32
	bytesDuplicate        uint64
	headerBytesDuplicate  uint64
	packetsLostOverridden uint32
	nacks                 uint32
	plis                  uint32
	firs                  uint32
	maxRtt                uint32
	maxJitter             float64
	maxJitterOverridden   float64
}

type SnInfo struct {
	pktTime       int64
	hdrSize       uint16
	pktSize       uint16
	isPaddingOnly bool
	marker        bool
}

type RTCPSenderReportData struct {
	RTPTimestamp uint32
	NTPTimestamp mediatransportutil.NtpTime
	ArrivalTime  time.Time
}

type RTCPSenderReportDataExt struct {
	SenderReportData RTCPSenderReportData
	SmoothedOWD      time.Duration
}

type RTPStatsParams struct {
	ClockRate              uint32
	IsReceiverReportDriven bool
	Logger                 logger.Logger
}

type RTPStats struct {
	params RTPStatsParams
	logger logger.Logger

	lock sync.RWMutex

	initialized        bool
	resyncOnNextPacket bool

	startTime time.Time
	endTime   time.Time

	extStartSN uint32
	highestSN  uint16
	cycles     uint16

	extHighestSNOverridden uint32
	lastRRTime             time.Time
	lastRR                 rtcp.ReceptionReport

	highestTS   uint32
	highestTime int64

	lastTransit uint32

	bytes                uint64
	headerBytes          uint64
	bytesDuplicate       uint64
	headerBytesDuplicate uint64
	bytesPadding         uint64
	headerBytesPadding   uint64
	packetsDuplicate     uint32
	packetsPadding       uint32

	packetsOutOfOrder uint32

	packetsLost           uint32
	packetsLostOverridden uint32

	frames uint32

	jitter              float64
	maxJitter           float64
	jitterOverridden    float64
	maxJitterOverridden float64

	snInfos        [SnInfoSize]SnInfo
	snInfoWritePtr int

	gapHistogram [GapHistogramNumBins]uint32

	nacks        uint32
	nackAcks     uint32
	nackMisses   uint32
	nackRepeated uint32

	plis    uint32
	lastPli time.Time

	layerLockPlis    uint32
	lastLayerLockPli time.Time

	firs    uint32
	lastFir time.Time

	keyFrames    uint32
	lastKeyFrame time.Time

	rtt    uint32
	maxRtt uint32

	srDataExt *RTCPSenderReportDataExt

	nextSnapshotId uint32
	snapshots      map[uint32]*Snapshot
}

type rtpStatsInternal struct {
	StartTime            *time.Time
	EndTime              *time.Time
	Duration             float64
	Packets              uint32
	PacketRate           float64
	Bytes                uint64
	HeaderBytes          uint64
	Bitrate              float64
	PacketsLost          uint32
	PacketLossRate       float64
	PacketLossPercentage float32
	PacketsDuplicate     uint32
	PacketDuplicateRate  float64
	BytesDuplicate       uint64
	HeaderBytesDuplicate uint64
	BitrateDuplicate     float64
	PacketsPadding       uint32
	PacketPaddingRate    float64
	BytesPadding         uint64
	HeaderBytesPadding   uint64
	BitratePadding       float64
	PacketsOutOfOrder    uint32
	Frames               uint32
	FrameRate            float64
	JitterCurrent        float64
	JitterMax            float64
	GapHistogram         map[int32]uint32
	Nacks                uint32
	NackAcks             uint32
	NackMisses           uint32
	NackRepeated         uint32
	Plis                 uint32
	LastPli              *time.Time
	Firs                 uint32
	LastFir              *time.Time
	RttCurrent           uint32
	RttMax               uint32
	KeyFrames            uint32
	LastKeyFrame         *time.Time
	LayerLockPlis        uint32
	LastLayerLockPli     *time.Time
}

func NewRTPStats(params RTPStatsParams) *RTPStats {
	return &RTPStats{
		params:         params,
		logger:         params.Logger,
		nextSnapshotId: FirstSnapshotId,
		snapshots:      make(map[uint32]*Snapshot),
	}
}

func (r *RTPStats) Seed(from *RTPStats) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if from == nil || !from.initialized {
		return
	}

	r.initialized = from.initialized
	r.resyncOnNextPacket = from.resyncOnNextPacket

	r.startTime = from.startTime
	// do not clone endTime as a non-zero endTime indiacates an ended object

	r.extStartSN = from.extStartSN
	r.highestSN = from.highestSN
	r.cycles = from.cycles

	r.extHighestSNOverridden = from.extHighestSNOverridden
	r.lastRRTime = from.lastRRTime
	r.lastRR = from.lastRR

	r.highestTS = from.highestTS
	r.highestTime = from.highestTime

	r.lastTransit = from.lastTransit

	r.bytes = from.bytes
	r.headerBytes = from.headerBytes
	r.bytesDuplicate = from.bytesDuplicate
	r.headerBytesDuplicate = from.headerBytesDuplicate
	r.bytesPadding = from.bytesPadding
	r.headerBytesPadding = from.headerBytesPadding
	r.packetsDuplicate = from.packetsDuplicate
	r.packetsPadding = from.packetsPadding

	r.packetsOutOfOrder = from.packetsOutOfOrder

	r.packetsLost = from.packetsLost
	r.packetsLostOverridden = from.packetsLostOverridden

	r.frames = from.frames

	r.jitter = from.jitter
	r.maxJitter = from.maxJitter
	r.jitterOverridden = from.jitterOverridden
	r.maxJitterOverridden = from.maxJitterOverridden

	r.snInfos = from.snInfos
	r.snInfoWritePtr = from.snInfoWritePtr

	r.gapHistogram = from.gapHistogram

	r.nacks = from.nacks
	r.nackAcks = from.nackAcks
	r.nackMisses = from.nackMisses
	r.nackRepeated = from.nackRepeated

	r.plis = from.plis
	r.lastPli = from.lastPli

	r.layerLockPlis = from.layerLockPlis
	r.lastLayerLockPli = from.lastLayerLockPli

	r.firs = from.firs
	r.lastFir = from.lastFir

	r.keyFrames = from.keyFrames
	r.lastKeyFrame = from.lastKeyFrame

	r.rtt = from.rtt
	r.maxRtt = from.maxRtt

	if from.srDataExt != nil {
		r.srDataExt = &RTCPSenderReportDataExt{
			SenderReportData: from.srDataExt.SenderReportData,
			SmoothedOWD:      from.srDataExt.SmoothedOWD,
		}
	} else {
		r.srDataExt = nil
	}

	r.nextSnapshotId = from.nextSnapshotId
	for id, ss := range from.snapshots {
		ssCopy := *ss
		r.snapshots[id] = &ssCopy
	}
}

func (r *RTPStats) SetLogger(logger logger.Logger) {
	r.logger = logger
}

func (r *RTPStats) Stop() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.endTime = time.Now()
}

func (r *RTPStats) NewSnapshotId() uint32 {
	r.lock.Lock()
	defer r.lock.Unlock()

	id := r.nextSnapshotId
	if r.initialized {
		r.snapshots[id] = &Snapshot{startTime: time.Now(), extStartSN: r.extStartSN}
	}

	r.nextSnapshotId++

	return id
}

func (r *RTPStats) IsActive() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.initialized && r.endTime.IsZero()
}

func (r *RTPStats) Update(rtph *rtp.Header, payloadSize int, paddingSize int, packetTime int64) (flowState RTPFlowState) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	first := false
	if !r.initialized {
		r.initialized = true

		r.startTime = time.Now()

		r.highestSN = rtph.SequenceNumber - 1
		r.highestTS = rtph.Timestamp
		r.highestTime = packetTime

		r.extStartSN = uint32(rtph.SequenceNumber)
		r.cycles = 0

		first = true

		// initialize snapshots if any
		for i := uint32(FirstSnapshotId); i < r.nextSnapshotId; i++ {
			r.snapshots[i] = &Snapshot{startTime: r.startTime, extStartSN: r.extStartSN}
		}
	}

	if r.resyncOnNextPacket {
		r.resyncOnNextPacket = false

		r.highestSN = rtph.SequenceNumber - 1
		r.highestTS = rtph.Timestamp
		r.highestTime = packetTime
	}

	hdrSize := uint64(rtph.MarshalSize())
	pktSize := hdrSize + uint64(payloadSize+paddingSize)
	isDuplicate := false
	diff := rtph.SequenceNumber - r.highestSN
	switch {
	// duplicate or out-of-order
	case diff == 0 || diff > (1<<15):
		if diff != 0 {
			r.packetsOutOfOrder++
		}

		// adjust start to account for out-of-order packets before a cycle completes
		if !r.maybeAdjustStartSN(rtph, packetTime, pktSize, hdrSize, payloadSize) {
			if !r.isSnInfoLost(rtph.SequenceNumber) {
				r.bytesDuplicate += pktSize
				r.headerBytesDuplicate += hdrSize
				r.packetsDuplicate++
				isDuplicate = true
			} else {
				r.packetsLost--
				r.setSnInfo(rtph.SequenceNumber, uint16(pktSize), uint16(hdrSize), uint16(payloadSize), rtph.Marker)
			}
		}

	// in-order
	default:
		if diff > 1 {
			flowState.HasLoss = true
			flowState.LossStartInclusive = r.highestSN + 1
			flowState.LossEndExclusive = rtph.SequenceNumber
		}

		// update gap histogram
		r.updateGapHistogram(int(diff))

		// update missing sequence numbers
		r.clearSnInfos(r.highestSN+1, rtph.SequenceNumber)
		r.packetsLost += uint32(diff - 1)

		r.setSnInfo(rtph.SequenceNumber, uint16(pktSize), uint16(hdrSize), uint16(payloadSize), rtph.Marker)

		if rtph.SequenceNumber < r.highestSN && !first {
			r.cycles++
		}
		r.highestSN = rtph.SequenceNumber
		r.highestTS = rtph.Timestamp
		r.highestTime = packetTime
	}

	if !isDuplicate {
		if payloadSize == 0 {
			r.packetsPadding++
			r.bytesPadding += pktSize
			r.headerBytesPadding += hdrSize
		} else {
			r.bytes += pktSize
			r.headerBytes += hdrSize

			if rtph.Marker {
				r.frames++
			}

			r.updateJitter(rtph, packetTime)
		}
	}

	return
}

func (r *RTPStats) ResyncOnNextPacket() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.resyncOnNextPacket = true
}

func (r *RTPStats) maybeAdjustStartSN(rtph *rtp.Header, packetTime int64, pktSize uint64, hdrSize uint64, payloadSize int) bool {
	if (r.getExtHighestSN() - r.extStartSN + 1) >= (NumSequenceNumbers / 2) {
		return false
	}

	if (rtph.SequenceNumber - uint16(r.extStartSN)) < (1 << 15) {
		return false
	}

	r.packetsLost += uint32(uint16(r.extStartSN)-rtph.SequenceNumber) - 1
	beforeAdjust := r.extStartSN
	r.extStartSN = uint32(rtph.SequenceNumber)

	r.setSnInfo(rtph.SequenceNumber, uint16(pktSize), uint16(hdrSize), uint16(payloadSize), rtph.Marker)

	for _, s := range r.snapshots {
		if s.extStartSN == beforeAdjust {
			s.extStartSN = r.extStartSN
		}
	}

	return true
}

func (r *RTPStats) GetTotalPacketsPrimary() uint32 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.getTotalPacketsPrimary()
}

func (r *RTPStats) getTotalPacketsPrimary() uint32 {
	packetsExpected := r.getExtHighestSN() - r.extStartSN + 1
	if r.packetsLost > packetsExpected {
		// should not happen
		return 0
	}

	packetsSeen := packetsExpected - r.packetsLost
	if r.packetsPadding > packetsSeen {
		return 0
	}

	return packetsSeen - r.packetsPadding
}

func (r *RTPStats) UpdateFromReceiverReport(rr rtcp.ReceptionReport) (rtt uint32, isRttChanged bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() || !r.params.IsReceiverReportDriven {
		return
	}

	rtt, err := mediatransportutil.GetRttMsFromReceiverReportOnly(&rr)
	if err == nil {
		isRttChanged = rtt != r.rtt
	} else {
		if err != mediatransportutil.ErrRttNoLastSenderReport {
			r.logger.Warn("error getting rtt", err)
		}
	}

	if r.lastRRTime.IsZero() || r.extHighestSNOverridden <= rr.LastSequenceNumber {
		r.extHighestSNOverridden = rr.LastSequenceNumber
		r.packetsLostOverridden = rr.TotalLost

		if isRttChanged {
			r.rtt = rtt
			if rtt > r.maxRtt {
				r.maxRtt = rtt
			}
		}

		r.jitterOverridden = float64(rr.Jitter)
		if r.jitterOverridden > r.maxJitterOverridden {
			r.maxJitterOverridden = r.jitterOverridden
		}

		// update snapshots
		for _, s := range r.snapshots {
			if isRttChanged && rtt > s.maxRtt {
				s.maxRtt = rtt
			}

			if r.jitterOverridden > s.maxJitterOverridden {
				s.maxJitterOverridden = r.jitterOverridden
			}
		}

		r.lastRRTime = time.Now()
		r.lastRR = rr
	} else {
		r.logger.Debug(
			fmt.Sprintf("receiver report potentially out of order, highestSN: existing: %d, received: %d", r.extHighestSNOverridden, rr.LastSequenceNumber),
			"lastRRTime", r.lastRRTime,
			"lastRR", r.lastRR,
			"sinceLastRR", time.Since(r.lastRRTime),
			"receivedRR", rr,
		)
	}
	return
}

func (r *RTPStats) UpdateNack(nackCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.nacks += nackCount
}

func (r *RTPStats) UpdateNackProcessed(nackAckCount uint32, nackMissCount uint32, nackRepeatedCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.nackAcks += nackAckCount
	r.nackMisses += nackMissCount
	r.nackRepeated += nackRepeatedCount
}

func (r *RTPStats) UpdatePliAndTime(pliCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.updatePliLocked(pliCount)
	r.updatePliTimeLocked()
}

func (r *RTPStats) UpdatePli(pliCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.updatePliLocked(pliCount)
}

func (r *RTPStats) updatePliLocked(pliCount uint32) {
	r.plis += pliCount
}

func (r *RTPStats) UpdatePliTime() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.updatePliTimeLocked()
}

func (r *RTPStats) updatePliTimeLocked() {
	r.lastPli = time.Now()
}

func (r *RTPStats) LastPli() time.Time {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.lastPli
}

func (r *RTPStats) TimeSinceLastPli() int64 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return time.Now().UnixNano() - r.lastPli.UnixNano()
}

func (r *RTPStats) UpdateLayerLockPliAndTime(pliCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.layerLockPlis += pliCount
	r.lastLayerLockPli = time.Now()
}

func (r *RTPStats) UpdateFir(firCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.firs += firCount
}

func (r *RTPStats) UpdateFirTime() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.lastFir = time.Now()
}

func (r *RTPStats) UpdateKeyFrame(kfCount uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.keyFrames += kfCount
	r.lastKeyFrame = time.Now()
}

func (r *RTPStats) UpdateRtt(rtt uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.endTime.IsZero() {
		return
	}

	r.rtt = rtt
	if rtt > r.maxRtt {
		r.maxRtt = rtt
	}

	for _, s := range r.snapshots {
		if rtt > s.maxRtt {
			s.maxRtt = rtt
		}
	}
}

func (r *RTPStats) GetRtt() uint32 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.rtt
}

func (r *RTPStats) SetRtcpSenderReportData(srData *RTCPSenderReportData) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if srData == nil {
		r.srDataExt = nil
		return
	}

	// prevent against extreme case of anachronous sender reports
	if r.srDataExt != nil && r.srDataExt.SenderReportData.NTPTimestamp > srData.NTPTimestamp {
		return
	}

	// Low pass filter one-way-delay (owd) to normalize time stamp to local time base when sending RTCP Sender Report.
	// Forwarding RTCP Sender Report would be ideal. But, there are a couple of issues with that
	//   1. Senders could have different clocks.
	//   2. Adjusting to current time as required by RTCP spec.
	// By normalizing to local clock, these issues can be addressed. However, normalization is not straightforward
	// as it is not possible to know the propagation delay and processing delay at both ends (send side processing
	// after time stamping the RTCP packet and receive side processing after reading packet off the wire).
	// Smoothed version of OWD is used to alleviate irregularities somewhat.
	owd := srData.ArrivalTime.Sub(srData.NTPTimestamp.Time())
	if r.srDataExt != nil {
		prevOwd := r.srDataExt.SenderReportData.ArrivalTime.Sub(r.srDataExt.SenderReportData.NTPTimestamp.Time())
		if time.Duration(math.Abs(float64(owd)-float64(prevOwd))) > TooLargeOWD {
			r.logger.Info("large one-way-delay", "owd", owd, "prevOwd", prevOwd)
		}
	}

	smoothedOwd := owd
	if r.srDataExt != nil {
		smoothedOwd = r.srDataExt.SmoothedOWD
	}
	r.srDataExt = &RTCPSenderReportDataExt{
		SenderReportData: *srData,
		SmoothedOWD:      (owd + smoothedOwd) / 2,
	}
}

func (r *RTPStats) GetRtcpSenderReportDataExt() *RTCPSenderReportDataExt {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.srDataExt == nil {
		return nil
	}

	return &RTCPSenderReportDataExt{
		SenderReportData: r.srDataExt.SenderReportData,
		SmoothedOWD:      r.srDataExt.SmoothedOWD,
	}
}

func (r *RTPStats) GetRtcpSenderReport(ssrc uint32, srDataExt *RTCPSenderReportDataExt) *rtcp.SenderReport {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.initialized {
		return nil
	}

	if srDataExt == nil || srDataExt.SenderReportData.NTPTimestamp == 0 || srDataExt.SenderReportData.ArrivalTime.IsZero() {
		// no sender report from publisher
		return nil
	}

	// NTP timestamp in sender report from publisher side could have a different base,
	// i. e. although it should be wall clock at time of send, have observed instances of older timer.
	// It is not possible to accurately calculate current time in the NTP time base of the publisher side.
	// So, using a smoothed version of one way delay for use in sender reports.
	now := time.Now()
	nowNTP := mediatransportutil.ToNtpTime(now)
	nowRTP := r.highestTS

	smoothedLocalTimeOfLatestSenderReportNTP := srDataExt.SenderReportData.NTPTimestamp.Time().Add(srDataExt.SmoothedOWD)
	if smoothedLocalTimeOfLatestSenderReportNTP.After(now) {
		r.logger.Info("smoothed time of NTP is ahead",
			"now", now,
			"smoothed", smoothedLocalTimeOfLatestSenderReportNTP,
			"diff", smoothedLocalTimeOfLatestSenderReportNTP.Sub(now),
		)
		nowRTP += uint32(now.Sub(time.Unix(0, r.highestTime)).Milliseconds() * int64(r.params.ClockRate) / 1000)
	} else {
		nowRTP = srDataExt.SenderReportData.RTPTimestamp + uint32(now.Sub(smoothedLocalTimeOfLatestSenderReportNTP).Milliseconds()*int64(r.params.ClockRate)/1000)
	}

	return &rtcp.SenderReport{
		SSRC:        ssrc,
		NTPTime:     uint64(nowNTP),
		RTPTime:     nowRTP,
		PacketCount: r.getTotalPacketsPrimary() + r.packetsDuplicate + r.packetsPadding,
		OctetCount:  uint32(r.bytes + r.bytesDuplicate + r.bytesPadding),
	}
}

func (r *RTPStats) SnapshotRtcpReceptionReport(ssrc uint32, proxyFracLost uint8, snapshotId uint32) *rtcp.ReceptionReport {
	r.lock.Lock()
	then, now := r.getAndResetSnapshot(snapshotId)
	r.lock.Unlock()

	if now == nil || then == nil {
		return nil
	}

	r.lock.RLock()
	defer r.lock.RUnlock()

	packetsExpected := now.extStartSN - then.extStartSN
	if packetsExpected > NumSequenceNumbers {
		r.logger.Warn(
			"too many packets expected in receiver report",
			fmt.Errorf("start: %d, end: %d, expected: %d", then.extStartSN, now.extStartSN, packetsExpected),
		)
		return nil
	}
	if packetsExpected == 0 {
		return nil
	}

	packetsLost := uint32(0)
	if r.params.IsReceiverReportDriven {
		// should not be set for streams that need to generate reception report
		packetsLost = now.packetsLostOverridden - then.packetsLostOverridden
		if int32(packetsLost) < 0 {
			packetsLost = 0
		}
	} else {
		intervalStats := r.getIntervalStats(uint16(then.extStartSN), uint16(now.extStartSN))
		packetsLost = intervalStats.packetsLost
	}
	lossRate := float32(packetsLost) / float32(packetsExpected)
	fracLost := uint8(lossRate * 256.0)
	if proxyFracLost > fracLost {
		fracLost = proxyFracLost
	}

	var dlsr uint32
	if r.srDataExt != nil && !r.srDataExt.SenderReportData.ArrivalTime.IsZero() {
		delayMS := uint32(time.Since(r.srDataExt.SenderReportData.ArrivalTime).Milliseconds())
		dlsr = (delayMS / 1e3) << 16
		dlsr |= (delayMS % 1e3) * 65536 / 1000
	}

	jitter := r.jitter
	if r.params.IsReceiverReportDriven {
		// should not be set for streams that need to generate reception report
		jitter = r.jitterOverridden
	}

	lastSR := uint32(0)
	if r.srDataExt != nil {
		lastSR = uint32(r.srDataExt.SenderReportData.NTPTimestamp >> 16)
	}
	return &rtcp.ReceptionReport{
		SSRC:               ssrc,
		FractionLost:       fracLost,
		TotalLost:          r.packetsLost,
		LastSequenceNumber: now.extStartSN,
		Jitter:             uint32(jitter),
		LastSenderReport:   lastSR,
		Delay:              dlsr,
	}
}

func (r *RTPStats) DeltaInfo(snapshotId uint32) *RTPDeltaInfo {
	r.lock.Lock()
	then, now := r.getAndResetSnapshot(snapshotId)
	r.lock.Unlock()

	if now == nil || then == nil {
		return nil
	}

	r.lock.RLock()
	defer r.lock.RUnlock()

	startTime := then.startTime
	endTime := now.startTime

	packetsExpected := now.extStartSN - then.extStartSN
	if packetsExpected > NumSequenceNumbers {
		r.logger.Warn(
			"too many packets expected in delta",
			fmt.Errorf("start: %d, end: %d, expected: %d", then.extStartSN, now.extStartSN, packetsExpected),
		)
		return nil
	}
	if packetsExpected == 0 {
		if r.params.IsReceiverReportDriven {
			// not received RTCP RR
			return nil
		}

		return &RTPDeltaInfo{
			StartTime: startTime,
			Duration:  endTime.Sub(startTime),
		}
	}

	packetsLost := uint32(0)
	packetsMissing := uint32(0)
	intervalStats := r.getIntervalStats(uint16(then.extStartSN), uint16(now.extStartSN))
	if r.params.IsReceiverReportDriven {
		packetsMissing = intervalStats.packetsLost

		packetsLost = now.packetsLostOverridden - then.packetsLostOverridden
		if int32(packetsLost) < 0 {
			packetsLost = 0
		}

		if packetsLost > packetsExpected {
			r.logger.Warn(
				"unexpected number of packets lost",
				fmt.Errorf(
					"start: %d, end: %d, expected: %d, lost: report: %d, interval: %d",
					then.extStartSN,
					now.extStartSN,
					packetsExpected,
					now.packetsLostOverridden-then.packetsLostOverridden,
					intervalStats.packetsLost,
				),
			)
			packetsLost = packetsExpected
		}
	} else {
		packetsLost = intervalStats.packetsLost
	}

	maxJitter := then.maxJitter
	if r.params.IsReceiverReportDriven {
		maxJitter = then.maxJitterOverridden
	}
	maxJitterTime := maxJitter / float64(r.params.ClockRate) * 1e6

	return &RTPDeltaInfo{
		StartTime:            startTime,
		Duration:             endTime.Sub(startTime),
		Packets:              packetsExpected - intervalStats.packetsPadding,
		Bytes:                intervalStats.bytes,
		HeaderBytes:          intervalStats.headerBytes,
		PacketsDuplicate:     now.packetsDuplicate - then.packetsDuplicate,
		BytesDuplicate:       now.bytesDuplicate - then.bytesDuplicate,
		HeaderBytesDuplicate: now.headerBytesDuplicate - then.headerBytesDuplicate,
		PacketsPadding:       intervalStats.packetsPadding,
		BytesPadding:         intervalStats.bytesPadding,
		HeaderBytesPadding:   intervalStats.headerBytesPadding,
		PacketsLost:          packetsLost,
		PacketsMissing:       packetsMissing,
		Frames:               intervalStats.frames,
		RttMax:               then.maxRtt,
		JitterMax:            maxJitterTime,
		Nacks:                now.nacks - then.nacks,
		Plis:                 now.plis - then.plis,
		Firs:                 now.firs - then.firs,
	}
}

func (r *RTPStats) ToString() string {
	p := r.ToProto()
	if p == nil {
		return ""
	}

	r.lock.RLock()
	defer r.lock.RUnlock()

	expectedPackets := r.getExtHighestSN() - r.extStartSN + 1
	expectedPacketRate := float64(expectedPackets) / p.Duration

	str := fmt.Sprintf("t: %+v|%+v|%.2fs", p.StartTime.Format(time.UnixDate), p.EndTime.Format(time.UnixDate), p.Duration)

	str += fmt.Sprintf(" sn: %d|%d", r.extStartSN, r.getExtHighestSN())
	str += fmt.Sprintf(", ep: %d|%.2f/s", expectedPackets, expectedPacketRate)

	str += fmt.Sprintf(", p: %d|%.2f/s", p.Packets, p.PacketRate)
	str += fmt.Sprintf(", l: %d|%.1f/s|%.2f%%", p.PacketsLost, p.PacketLossRate, p.PacketLossPercentage)
	str += fmt.Sprintf(", b: %d|%.1fbps|%d", p.Bytes, p.Bitrate, p.HeaderBytes)
	str += fmt.Sprintf(", f: %d|%.1f/s / %d|%+v", p.Frames, p.FrameRate, p.KeyFrames, p.LastKeyFrame.Format(time.UnixDate))

	str += fmt.Sprintf(", d: %d|%.2f/s", p.PacketsDuplicate, p.PacketDuplicateRate)
	str += fmt.Sprintf(", bd: %d|%.1fbps|%d", p.BytesDuplicate, p.BitrateDuplicate, p.HeaderBytesDuplicate)

	str += fmt.Sprintf(", pp: %d|%.2f/s", p.PacketsPadding, p.PacketPaddingRate)
	str += fmt.Sprintf(", bp: %d|%.1fbps|%d", p.BytesPadding, p.BitratePadding, p.HeaderBytesPadding)

	str += fmt.Sprintf(", o: %d", p.PacketsOutOfOrder)

	jitter := r.jitter
	maxJitter := r.maxJitter
	if r.params.IsReceiverReportDriven {
		jitter = r.jitterOverridden
		maxJitter = r.maxJitterOverridden
	}
	str += fmt.Sprintf(", c: %d, j: %d(%.1fus)|%d(%.1fus)", r.params.ClockRate, uint32(jitter), p.JitterCurrent, uint32(maxJitter), p.JitterMax)

	if len(p.GapHistogram) != 0 {
		first := true
		str += ", gh:["
		for burst, count := range p.GapHistogram {
			if !first {
				str += ", "
			}
			first = false
			str += fmt.Sprintf("%d:%d", burst, count)
		}
		str += "]"
	}

	str += ", n:"
	str += fmt.Sprintf("%d|%d|%d|%d", p.Nacks, p.NackAcks, p.NackMisses, p.NackRepeated)

	str += ", pli:"
	str += fmt.Sprintf("%d|%+v / %d|%+v",
		p.Plis, p.LastPli.Format(time.UnixDate),
		p.LayerLockPlis, p.LastLayerLockPli.Format(time.UnixDate),
	)

	str += ", fir:"
	str += fmt.Sprintf("%d|%+v", p.Firs, p.LastFir.Format(time.UnixDate))

	str += ", rtt(ms):"
	str += fmt.Sprintf("%d|%d", p.RttCurrent, p.RttMax)

	return str
}

func (r *RTPStats) ToProto() *rtpStatsInternal {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.startTime.IsZero() {
		return nil
	}

	endTime := r.endTime
	if endTime.IsZero() {
		endTime = time.Now()
	}
	elapsed := endTime.Sub(r.startTime).Seconds()
	if elapsed == 0.0 {
		return nil
	}

	packets := r.getTotalPacketsPrimary()
	packetRate := float64(packets) / elapsed
	bitrate := float64(r.bytes) * 8.0 / elapsed

	frameRate := float64(r.frames) / elapsed

	packetsExpected := r.getExtHighestSN() - r.extStartSN + 1
	packetsLost := r.getPacketsLost()
	packetLostRate := float64(packetsLost) / elapsed
	packetLostPercentage := float32(packetsLost) / float32(packetsExpected) * 100.0

	packetDuplicateRate := float64(r.packetsDuplicate) / elapsed
	bitrateDuplicate := float64(r.bytesDuplicate) * 8.0 / elapsed

	packetPaddingRate := float64(r.packetsPadding) / elapsed
	bitratePadding := float64(r.bytesPadding) * 8.0 / elapsed

	jitter := r.jitter
	maxJitter := r.maxJitter
	if r.params.IsReceiverReportDriven {
		jitter = r.jitterOverridden
		maxJitter = r.maxJitterOverridden
	}
	jitterTime := jitter / float64(r.params.ClockRate) * 1e6
	maxJitterTime := maxJitter / float64(r.params.ClockRate) * 1e6

	p := &rtpStatsInternal{
		StartTime:            &r.startTime,
		EndTime:              &endTime,
		Duration:             elapsed,
		Packets:              packets,
		PacketRate:           packetRate,
		Bytes:                r.bytes,
		HeaderBytes:          r.headerBytes,
		Bitrate:              bitrate,
		PacketsLost:          packetsLost,
		PacketLossRate:       packetLostRate,
		PacketLossPercentage: packetLostPercentage,
		PacketsDuplicate:     r.packetsDuplicate,
		PacketDuplicateRate:  packetDuplicateRate,
		BytesDuplicate:       r.bytesDuplicate,
		HeaderBytesDuplicate: r.headerBytesDuplicate,
		BitrateDuplicate:     bitrateDuplicate,
		PacketsPadding:       r.packetsPadding,
		PacketPaddingRate:    packetPaddingRate,
		BytesPadding:         r.bytesPadding,
		HeaderBytesPadding:   r.headerBytesPadding,
		BitratePadding:       bitratePadding,
		PacketsOutOfOrder:    r.packetsOutOfOrder,
		Frames:               r.frames,
		FrameRate:            frameRate,
		KeyFrames:            r.keyFrames,
		LastKeyFrame:         &r.lastKeyFrame,
		JitterCurrent:        jitterTime,
		JitterMax:            maxJitterTime,
		Nacks:                r.nacks,
		NackAcks:             r.nackAcks,
		NackMisses:           r.nackMisses,
		NackRepeated:         r.nackRepeated,
		Plis:                 r.plis,
		LastPli:              &r.lastPli,
		LayerLockPlis:        r.layerLockPlis,
		LastLayerLockPli:     &r.lastLayerLockPli,
		Firs:                 r.firs,
		LastFir:              &r.lastFir,
		RttCurrent:           r.rtt,
		RttMax:               r.maxRtt,
	}

	gapsPresent := false
	for i := 0; i < len(r.gapHistogram); i++ {
		if r.gapHistogram[i] == 0 {
			continue
		}

		gapsPresent = true
		break
	}

	if gapsPresent {
		p.GapHistogram = make(map[int32]uint32, GapHistogramNumBins)
		for i := 0; i < len(r.gapHistogram); i++ {
			if r.gapHistogram[i] == 0 {
				continue
			}

			p.GapHistogram[int32(i+1)] = r.gapHistogram[i]
		}
	}

	return p
}

func (r *RTPStats) getExtHighestSN() uint32 {
	return (uint32(r.cycles) << 16) | uint32(r.highestSN)
}

func (r *RTPStats) getExtHighestSNAdjusted() uint32 {
	if r.params.IsReceiverReportDriven && !r.lastRRTime.IsZero() {
		return r.extHighestSNOverridden
	}

	return r.getExtHighestSN()
}

func (r *RTPStats) getPacketsLost() uint32 {
	if r.params.IsReceiverReportDriven && !r.lastRRTime.IsZero() {
		return r.packetsLostOverridden
	}

	return r.packetsLost
}

func (r *RTPStats) getSnInfoOutOfOrderPtr(sn uint16) int {
	offset := sn - r.highestSN
	if offset > 0 && offset < (1<<15) {
		return -1 // in-order, not expected, maybe too new
	}

	offset = r.highestSN - sn
	if int(offset) >= SnInfoSize {
		// too old, ignore
		return -1
	}

	return (r.snInfoWritePtr - int(offset) - 1) & SnInfoMask
}

func (r *RTPStats) setSnInfo(sn uint16, pktSize uint16, hdrSize uint16, payloadSize uint16, marker bool) {
	writePtr := 0
	ooo := (sn - r.highestSN) > (1 << 15)
	if !ooo {
		writePtr = r.snInfoWritePtr
		r.snInfoWritePtr = (writePtr + 1) & SnInfoMask
	} else {
		writePtr = r.getSnInfoOutOfOrderPtr(sn)
		if writePtr < 0 {
			return
		}
	}

	snInfo := &r.snInfos[writePtr]
	snInfo.pktSize = pktSize
	snInfo.hdrSize = hdrSize
	snInfo.isPaddingOnly = payloadSize == 0
	snInfo.marker = marker
}

func (r *RTPStats) clearSnInfos(startInclusive uint16, endExclusive uint16) {
	for sn := startInclusive; sn != endExclusive; sn++ {
		snInfo := &r.snInfos[r.snInfoWritePtr]
		snInfo.pktSize = 0
		snInfo.hdrSize = 0
		snInfo.isPaddingOnly = false
		snInfo.marker = false

		r.snInfoWritePtr = (r.snInfoWritePtr + 1) & SnInfoMask
	}
}

func (r *RTPStats) isSnInfoLost(sn uint16) bool {
	readPtr := r.getSnInfoOutOfOrderPtr(sn)
	if readPtr < 0 {
		return false
	}

	snInfo := &r.snInfos[readPtr]
	return snInfo.pktSize == 0
}

func (r *RTPStats) getIntervalStats(startInclusive uint16, endExclusive uint16) (intervalStats IntervalStats) {
	packetsNotFound := uint32(0)
	processSN := func(sn uint16) {
		readPtr := r.getSnInfoOutOfOrderPtr(sn)
		if readPtr < 0 {
			packetsNotFound++
			return
		}

		snInfo := &r.snInfos[readPtr]
		switch {
		case snInfo.pktSize == 0:
			intervalStats.packetsLost++

		case snInfo.isPaddingOnly:
			intervalStats.packetsPadding++
			intervalStats.bytesPadding += uint64(snInfo.pktSize)
			intervalStats.headerBytesPadding += uint64(snInfo.hdrSize)

		default:
			intervalStats.packets++
			intervalStats.bytes += uint64(snInfo.pktSize)
			intervalStats.headerBytes += uint64(snInfo.hdrSize)
		}

		if snInfo.marker {
			intervalStats.frames++
		}
	}

	if startInclusive == endExclusive {
		// do a full cycle
		for sn := uint32(0); sn < NumSequenceNumbers; sn++ {
			processSN(uint16(sn))
		}
	} else {
		for sn := startInclusive; sn != endExclusive; sn++ {
			processSN(sn)
		}
	}

	if packetsNotFound != 0 {
		r.logger.Warn(
			"could not find some packets", nil,
			"start", startInclusive,
			"end", endExclusive,
			"count", packetsNotFound,
		)
	}
	return
}

func (r *RTPStats) updateJitter(rtph *rtp.Header, packetTime int64) {
	packetTimeRTP := uint32(packetTime / 1e6 * int64(r.params.ClockRate/1e3))
	transit := packetTimeRTP - rtph.Timestamp

	if r.lastTransit != 0 {
		d := int32(transit - r.lastTransit)
		if d < 0 {
			d = -d
		}
		r.jitter += (float64(d) - r.jitter) / 16
		if r.jitter > r.maxJitter {
			r.maxJitter = r.jitter
		}

		for _, s := range r.snapshots {
			if r.jitter > s.maxJitter {
				r.maxJitter = r.jitter
			}
		}
	}

	r.lastTransit = transit
}

func (r *RTPStats) updateGapHistogram(gap int) {
	if gap < 2 {
		return
	}

	missing := gap - 1
	if missing > len(r.gapHistogram) {
		r.gapHistogram[len(r.gapHistogram)-1]++
	} else {
		r.gapHistogram[missing-1]++
	}
}

func (r *RTPStats) getAndResetSnapshot(snapshotId uint32) (*Snapshot, *Snapshot) {
	if !r.initialized || (r.params.IsReceiverReportDriven && r.lastRRTime.IsZero()) {
		return nil, nil
	}

	then := r.snapshots[snapshotId]
	if then == nil {
		then = &Snapshot{
			startTime:  r.startTime,
			extStartSN: r.extStartSN,
		}
		r.snapshots[snapshotId] = then
	}

	// snapshot now
	r.snapshots[snapshotId] = &Snapshot{
		startTime:             time.Now(),
		extStartSN:            r.getExtHighestSNAdjusted() + 1,
		packetsDuplicate:      r.packetsDuplicate,
		bytesDuplicate:        r.bytesDuplicate,
		headerBytesDuplicate:  r.headerBytesDuplicate,
		packetsLostOverridden: r.packetsLostOverridden,
		nacks:                 r.nacks,
		plis:                  r.plis,
		firs:                  r.firs,
		maxJitter:             0.0,
		maxJitterOverridden:   0.0,
		maxRtt:                0,
	}
	// make a copy so that it can be used independently
	now := *r.snapshots[snapshotId]

	return then, &now
}
