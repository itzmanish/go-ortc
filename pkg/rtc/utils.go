package rtc

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v3"
)

// setVp8TemporalLayer is a helper to detect and modify accordingly the vp8 payload to reflect
// temporal changes in the SFU.
// VP8 temporal layers implemented according https://tools.ietf.org/html/rfc7741
// func setVP8TemporalLayer(p *buffer.ExtPacket, d *DownTrack) (buf []byte, picID uint16, tlz0Idx uint8, drop bool) {
// 	pkt, ok := p.Payload.(buffer.VP8)
// 	if !ok {
// 		return
// 	}

// 	layer := atomic.LoadInt32(&d.temporalLayer)
// 	currentLayer := uint16(layer)
// 	currentTargetLayer := uint16(layer >> 16)
// 	// Check if temporal getLayer is requested
// 	if currentTargetLayer != currentLayer {
// 		if pkt.TID <= uint8(currentTargetLayer) {
// 			atomic.StoreInt32(&d.temporalLayer, int32(currentTargetLayer)<<16|int32(currentTargetLayer))
// 		}
// 	} else if pkt.TID > uint8(currentLayer) {
// 		drop = true
// 		return
// 	}

// 	buf = *d.payload
// 	buf = buf[:len(p.Packet.Payload)]
// 	copy(buf, p.Packet.Payload)

// 	picID = pkt.PictureID - d.simulcast.refPicID + d.simulcast.pRefPicID + 1
// 	tlz0Idx = pkt.TL0PICIDX - d.simulcast.refTlZIdx + d.simulcast.pRefTlZIdx + 1

// 	if p.Head {
// 		d.simulcast.lPicID = picID
// 		d.simulcast.lTlZIdx = tlz0Idx
// 	}

// 	modifyVP8TemporalPayload(buf, pkt.PicIDIdx, pkt.TlzIdx, picID, tlz0Idx, pkt.MBit)

// 	return
// }

func modifyVP8TemporalPayload(payload []byte, picIDIdx, tlz0Idx int, picID uint16, tlz0ID uint8, mBit bool) {
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, picID)
	payload[picIDIdx] = pid[0]
	if mBit {
		payload[picIDIdx] |= 0x80
		payload[picIDIdx+1] = pid[1]
	}
	payload[tlz0Idx] = tlz0ID
}

// Do a fuzzy find for a codec in the list of codecs
// Used for lookup up a codec in an existing list to find a match
func codecParametersFuzzySearch(needle webrtc.RTPCodecParameters, haystack []webrtc.RTPCodecParameters) (webrtc.RTPCodecParameters, error) {
	// First attempt to match on MimeType + SDPFmtpLine
	for _, c := range haystack {
		if strings.EqualFold(c.RTPCodecCapability.MimeType, needle.RTPCodecCapability.MimeType) &&
			c.RTPCodecCapability.SDPFmtpLine == needle.RTPCodecCapability.SDPFmtpLine {
			return c, nil
		}
	}

	// Fallback to just MimeType
	for _, c := range haystack {
		if strings.EqualFold(c.RTPCodecCapability.MimeType, needle.RTPCodecCapability.MimeType) {
			return c, nil
		}
	}

	return webrtc.RTPCodecParameters{}, webrtc.ErrCodecNotFound
}

var (
	ntpEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
)

type atomicBool int32
type ntpTime uint64

func (a *atomicBool) set(value bool) (swapped bool) {
	if value {
		return atomic.SwapInt32((*int32)(a), 1) == 0
	}
	return atomic.SwapInt32((*int32)(a), 0) == 1
}

func (a *atomicBool) get() bool {
	return atomic.LoadInt32((*int32)(a)) != 0
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateRandomString returns a securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := 0; i < n; i++ {
		b[i] = letters[b[i]%byte(len(letters))]
	}

	return *(*string)(unsafe.Pointer(&b))
}
