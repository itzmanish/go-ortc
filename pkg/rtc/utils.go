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
