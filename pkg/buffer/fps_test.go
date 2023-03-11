package buffer

import (
	"testing"

	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type testFrameInfo struct {
	header      rtp.Header
	framenumber uint16
	spatial     int
	temporal    int
	frameDiff   []int
}

var fpsLogger = logger.NewLogger("FPS_TEST")

func (f *testFrameInfo) toVP8() *ExtPacket {
	return &ExtPacket{
		Packet: &rtp.Packet{Header: f.header},
		Payload: VP8{
			PictureID: f.framenumber,
		},
		VideoLayer: VideoLayer{Spatial: InvalidLayerSpatial, Temporal: int32(f.temporal)},
	}
}

func createFrames(startFrameNumber uint16, startTs uint32, totalFramesPerSpatial int, fps [][]float32, spatialDependency bool) [][]*testFrameInfo {
	spatials := len(fps)
	temporals := len(fps[0])
	frames := make([][]*testFrameInfo, spatials)
	for s := 0; s < spatials; s++ {
		frames[s] = make([]*testFrameInfo, 0, totalFramesPerSpatial)
	}
	fn := startFrameNumber

	nextTs := make([][]uint32, spatials)
	tsStep := make([][]uint32, spatials)
	for i := 0; i < spatials; i++ {
		nextTs[i] = make([]uint32, temporals)
		tsStep[i] = make([]uint32, temporals)
		for j := 0; j < temporals; j++ {
			nextTs[i][j] = startTs
			tsStep[i][j] = uint32(90000 / fps[i][j])
		}
	}

	currentTs := make([]uint32, spatials)
	for i := 0; i < spatials; i++ {
		currentTs[i] = startTs
	}
	for i := 0; i < totalFramesPerSpatial; i++ {
		for s := 0; s < spatials; s++ {
			frame := &testFrameInfo{
				header:      rtp.Header{Timestamp: currentTs[s]},
				framenumber: fn,
				spatial:     s,
			}
			for t := 0; t < temporals; t++ {
				if currentTs[s] >= nextTs[s][t] {
					frame.temporal = t
					for nt := t; nt < temporals; nt++ {
						nextTs[s][nt] += tsStep[s][nt]
					}
					break
				}
			}
			currentTs[s] += tsStep[s][temporals-1]
			frames[s] = append(frames[s], frame)
			fn++

			for fidx := len(frames[s]) - 1; fidx >= 0; fidx-- {
				cf := frames[s][fidx]
				if cf.header.Timestamp-frame.header.Timestamp > 0x80000000 {
					frame.frameDiff = append(frame.frameDiff, int(frame.framenumber-cf.framenumber))
					break
				}
			}

			if spatialDependency && frame.spatial > 0 {
				for fidx := len(frames[frame.spatial-1]) - 1; fidx >= 0; fidx-- {
					cf := frames[frame.spatial-1][fidx]
					if cf.header.Timestamp == frame.header.Timestamp {
						frame.frameDiff = append(frame.frameDiff, int(frame.framenumber-cf.framenumber))
						break
					}
				}
			}
		}
	}

	return frames
}

func verifyFps(t *testing.T, expect, got []float32) {
	require.Equal(t, len(expect), len(got))
	for i := 0; i < len(expect); i++ {
		require.GreaterOrEqual(t, got[i], expect[i]*0.9, "expect %v, got %v", expect, got)
		require.LessOrEqual(t, got[i], expect[i]*1.1, "expect %v, got %v", expect, got)
	}
}

type testcase struct {
	startTs           uint32
	startFrameNumber  uint16
	fps               [][]float32
	spatialDependency bool
}

func TestFpsVP8(t *testing.T) {
	cases := map[string]testcase{
		"normal": {
			startTs:          12345678,
			startFrameNumber: 100,
			fps:              [][]float32{{5, 10, 15}, {5, 10, 15}, {7.5, 15, 30}},
		},
		"frame number and timestamp wrap": {
			startTs:          (uint32(1) << 31) - 10,
			startFrameNumber: (uint16(1) << 15) - 10,
			fps:              [][]float32{{5, 10, 15}, {5, 10, 15}, {7.5, 15, 30}},
		},
		"2 temporal layers": {
			startTs:          12345678,
			startFrameNumber: 100,
			fps:              [][]float32{{7.5, 15}, {7.5, 15}, {15, 30}},
		},
	}

	for name, c := range cases {
		testCase := c
		t.Run(name, func(t *testing.T) {
			fps := testCase.fps
			frames := [][]*testFrameInfo{}
			vp8calcs := make([]*FrameRateCalculatorVP8, len(fps))
			for i := range vp8calcs {
				vp8calcs[i] = NewFrameRateCalculatorVP8(90000, fpsLogger)
				frames = append(frames, createFrames(c.startFrameNumber, c.startTs, 200, [][]float32{fps[i]}, false)[0])
			}

			var frameratesGot bool
			for s, fs := range frames {
				for _, f := range fs {
					if vp8calcs[s].RecvPacket(f.toVP8()) {
						frameratesGot = true
						for _, calc := range vp8calcs {
							if !calc.Completed() {
								frameratesGot = false
								break
							}
						}
					}
				}
			}
			require.True(t, frameratesGot)
			for i, calc := range vp8calcs {
				fpsExpected := fps[i]
				fpsGot := calc.GetFrameRate()
				verifyFps(t, fpsExpected, fpsGot[:len(fpsExpected)])
			}
		})
	}
	t.Run("packet lost and duplicate", func(t *testing.T) {
		fps := [][]float32{{7.5, 15}, {7.5, 15}, {15, 30}}
		frames := [][]*testFrameInfo{}
		vp8calcs := make([]*FrameRateCalculatorVP8, len(fps))
		for i := range vp8calcs {
			vp8calcs[i] = NewFrameRateCalculatorVP8(90000, fpsLogger)
			frames = append(frames, createFrames(100, 12345678, 300, [][]float32{fps[i]}, false)[0])
			for j := 5; j < 130; j++ {
				if j%2 == 0 {
					frames[i][j] = frames[i][j-1]
				}
			}
		}

		var frameratesGot bool
		for s, fs := range frames {
			for _, f := range fs {
				if vp8calcs[s].RecvPacket(f.toVP8()) {
					frameratesGot = true
					for _, calc := range vp8calcs {
						if !calc.Completed() {
							frameratesGot = false
							break
						}
					}
				}
			}
		}
		require.True(t, frameratesGot)
		for i, calc := range vp8calcs {
			fpsExpected := fps[i]
			fpsGot := calc.GetFrameRate()
			verifyFps(t, fpsExpected, fpsGot[:len(fpsExpected)])
		}
	})
}
