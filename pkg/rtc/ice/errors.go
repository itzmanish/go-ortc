package ice

import (
	"errors"
	"fmt"
)

// UnknownError indicates the operation failed for an unknown transient reason.
type UnknownError struct {
	Err error
}

func (e *UnknownError) Error() string {
	return fmt.Sprintf("UnknownError: %v", e.Err)
}

// Unwrap returns the result of calling the Unwrap method on err, if err's type contains
// an Unwrap method returning error. Otherwise, Unwrap returns nil.
func (e *UnknownError) Unwrap() error {
	return e.Err
}

// NotSupportedError indicates the operation is not supported.
type NotSupportedError struct {
	Err error
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("NotSupportedError: %v", e.Err)
}

// Unwrap returns the result of calling the Unwrap method on err, if err's type contains
// an Unwrap method returning error. Otherwise, Unwrap returns nil.
func (e *NotSupportedError) Unwrap() error {
	return e.Err
}

var (

	// ErrUnknownType indicates an error with Unknown info.
	ErrUnknownType = errors.New("unknown")

	errICEProtocolUnknown             = errors.New("unknown protocol")
	errICECandidateTypeUnknown        = errors.New("unknown candidate type")
	errICEInvalidConvertCandidateType = errors.New("cannot convert ice.CandidateType into webrtc.ICECandidateType, invalid type")
)
