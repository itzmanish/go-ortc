package rtc

import (
	"fmt"

	"github.com/pion/ice/v2"
)

// ICECandidateType represents the type of the ICE candidate used.
type ICECandidateType int

const (
	// ICECandidateTypeHost indicates that the candidate is of Host type as
	// described in https://tools.ietf.org/html/rfc8445#section-5.1.1.1. A
	// candidate obtained by binding to a specific port from an IP address on
	// the host. This includes IP addresses on physical interfaces and logical
	// ones, such as ones obtained through VPNs.
	ICECandidateTypeHost ICECandidateType = iota + 1
)

// This is done this way because of a linter.
const (
	iceCandidateTypeHostStr = "host"
)

// NewICECandidateType takes a string and converts it into ICECandidateType
func NewICECandidateType(raw string) (ICECandidateType, error) {
	switch raw {
	case iceCandidateTypeHostStr:
		return ICECandidateTypeHost, nil
	default:
		return ICECandidateType(Unknown), fmt.Errorf("%w: %s", errICECandidateTypeUnknown, raw)
	}
}

func (t ICECandidateType) String() string {
	switch t {
	case ICECandidateTypeHost:
		return iceCandidateTypeHostStr
	default:
		return ErrUnknownType.Error()
	}
}

func getCandidateType(candidateType ice.CandidateType) (ICECandidateType, error) {
	switch candidateType {
	case ice.CandidateTypeHost:
		return ICECandidateTypeHost, nil
	default:
		// NOTE: this should never happen[tm]
		err := fmt.Errorf("%w: %s", errICEInvalidConvertCandidateType, candidateType.String())
		return ICECandidateType(Unknown), err
	}
}
