package goortc

import (
	"crypto/rand"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v3"
)

func GetRTPReceivingParameters(parameters RTPParameters) webrtc.RTPReceiveParameters {
	receivingParameters := webrtc.RTPReceiveParameters{
		Encodings: make([]webrtc.RTPDecodingParameters, len(parameters.Encodings)),
	}
	for i, params := range parameters.Encodings {
		receivingParameters.Encodings[i] = webrtc.RTPDecodingParameters{
			RTPCodingParameters: webrtc.RTPCodingParameters{
				RID:         params.RID,
				SSRC:        params.SSRC,
				PayloadType: params.PayloadType,
				RTX:         params.RTX,
			},
		}
	}
	return receivingParameters
}

func GetExtendedParameters(parameters RTPParameters, me *webrtc.MediaEngine) (webrtc.RTPParameters, error) {
	extendedParameters := webrtc.RTPParameters{
		HeaderExtensions: make([]webrtc.RTPHeaderExtensionParameter, len(parameters.HeaderExtensions)),
	}
	for i, params := range parameters.HeaderExtensions {
		extendedParameters.HeaderExtensions[i] = webrtc.RTPHeaderExtensionParameter{
			URI: params.URI,
			ID:  params.ID,
		}
	}
	return extendedParameters, nil
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
