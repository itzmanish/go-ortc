package rtc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestICECandidateType(t *testing.T) {
	testCases := []struct {
		typeString   string
		shouldFail   bool
		expectedType ICECandidateType
	}{
		{unknownStr, true, ICECandidateType(Unknown)},
		{"host", false, ICECandidateTypeHost},
	}

	for i, testCase := range testCases {
		actual, err := NewICECandidateType(testCase.typeString)
		if (err != nil) != testCase.shouldFail {
			t.Error(err)
		}
		assert.Equal(t,
			testCase.expectedType,
			actual,
			"testCase: %d %v", i, testCase,
		)
	}
}

func TestICECandidateType_String(t *testing.T) {
	testCases := []struct {
		cType          ICECandidateType
		expectedString string
	}{
		{ICECandidateType(Unknown), unknownStr},
		{ICECandidateTypeHost, "host"},
	}

	for i, testCase := range testCases {
		assert.Equal(t,
			testCase.expectedString,
			testCase.cType.String(),
			"testCase: %d %v", i, testCase,
		)
	}
}
