package buffer

import (
	"github.com/itzmanish/go-ortc/pkg/logger"
)

var minFramesForCalculation = [DefaultMaxLayerTemporal + 1]int{8, 15, 40}

type frameInfo struct {
	seq       uint16
	ts        uint32
	fn        uint16
	spatial   int32
	temporal  int32
	frameDiff []int
}

type FrameRateCalculator interface {
	RecvPacket(ep *ExtPacket) bool
	GetFrameRate() []float32
	Completed() bool
}

// -----------------------------
// FrameRateCalculator based on PictureID in VP8
type FrameRateCalculatorVP8 struct {
	frameRates   [DefaultMaxLayerTemporal + 1]float32
	clockRate    uint32
	logger       logger.Logger
	firstFrames  [DefaultMaxLayerTemporal + 1]*frameInfo
	secondFrames [DefaultMaxLayerTemporal + 1]*frameInfo
	fnReceived   [64]*frameInfo
	baseFrame    *frameInfo
	completed    bool
}

func NewFrameRateCalculatorVP8(clockRate uint32, logger logger.Logger) *FrameRateCalculatorVP8 {
	return &FrameRateCalculatorVP8{
		clockRate: clockRate,
		logger:    logger,
	}
}

func (f *FrameRateCalculatorVP8) Completed() bool {
	return f.completed
}

func (f *FrameRateCalculatorVP8) RecvPacket(ep *ExtPacket) bool {
	if f.completed {
		return true
	}
	vp8, ok := ep.Payload.(VP8)
	if !ok {
		f.logger.Debug("no vp8 payload", "sn", ep.Packet.SequenceNumber)
		return false
	}
	fn := vp8.PictureID

	if ep.Temporal >= int32(len(f.frameRates)) {
		f.logger.Warn("invalid temporal layer", nil, "temporal", ep.Temporal)
		return false
	}

	temporal := ep.Temporal
	if temporal < 0 {
		temporal = 0
	}

	if f.baseFrame == nil {
		f.baseFrame = &frameInfo{seq: ep.Packet.SequenceNumber, ts: ep.Packet.Timestamp, fn: fn}
		f.fnReceived[0] = f.baseFrame
		f.firstFrames[temporal] = f.baseFrame
		return false
	}

	baseDiff := fn - f.baseFrame.fn
	if baseDiff == 0 || baseDiff > 0x4000 {
		return false
	}

	if baseDiff >= uint16(len(f.fnReceived)) {
		// frame number is not continuous, reset
		f.reset()

		return false
	}

	if f.fnReceived[baseDiff] != nil {
		return false
	}

	fi := &frameInfo{
		seq:      ep.Packet.SequenceNumber,
		ts:       ep.Packet.Timestamp,
		fn:       fn,
		temporal: temporal,
	}
	f.fnReceived[baseDiff] = fi

	firstFrame := f.firstFrames[temporal]
	secondFrame := f.secondFrames[temporal]
	if firstFrame == nil {
		f.firstFrames[temporal] = fi
		firstFrame = fi
	} else {
		if (secondFrame == nil || secondFrame.fn < fn) && fn != firstFrame.fn && (fn-firstFrame.fn) < 0x4000 {
			f.secondFrames[temporal] = fi
		}
	}

	return f.calc()
}

func (f *FrameRateCalculatorVP8) calc() bool {
	var rateCounter int
	for currentTemporal := int32(0); currentTemporal <= int32(DefaultMaxLayerTemporal); currentTemporal++ {
		if f.frameRates[currentTemporal] > 0 {
			rateCounter++
			continue
		}

		ff := f.firstFrames[currentTemporal]
		sf := f.secondFrames[currentTemporal]
		// lower temporal layer has been calculated, but higher layer has not received any frames, it should not exist
		if rateCounter > 0 && ff == nil {
			rateCounter++
			continue
		}
		if ff == nil || sf == nil {
			continue
		}

		var frameCount int
		lastTs := ff.ts
		for j := ff.fn - f.baseFrame.fn + 1; j < sf.fn-f.baseFrame.fn+1; j++ {
			if f := f.fnReceived[j]; f == nil {
				break
			} else if f.temporal <= currentTemporal {
				frameCount++
				lastTs = f.ts
			}
		}
		if frameCount >= minFramesForCalculation[currentTemporal] {
			f.frameRates[currentTemporal] = float32(f.clockRate) / float32(lastTs-ff.ts) * float32(frameCount)
			rateCounter++
		}
	}

	if rateCounter == len(f.frameRates) {
		f.completed = true

		// normalize frame rates, Microsoft Edge use 3 temporal layers for vp8 but the middle layer has chance to
		// get a very low frame rate, so we need to normalize the frame rate(use fixed ration 1:2 of highest layer for that layer)
		if f.frameRates[2] > 0 && f.frameRates[2] > f.frameRates[1]*3 {
			f.frameRates[1] = f.frameRates[2] / 2
		}
		f.logger.Debug("frame rate calculated", "rate", f.frameRates)
		f.reset()
		return true
	}
	return false
}

func (f *FrameRateCalculatorVP8) reset() {
	for i := range f.firstFrames {
		f.firstFrames[i] = nil
		f.secondFrames[i] = nil
	}

	for i := range f.fnReceived {
		f.fnReceived[i] = nil
	}
	f.baseFrame = nil
}

func (f *FrameRateCalculatorVP8) GetFrameRate() []float32 {
	return f.frameRates[:]
}
