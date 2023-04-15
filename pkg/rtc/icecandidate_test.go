package rtc

import (
	"testing"

	"github.com/pion/ice/v2"
	"github.com/stretchr/testify/assert"
)

func TestICECandidate_Convert(t *testing.T) {
	testCases := []struct {
		native ICECandidate

		expectedType           ice.CandidateType
		expectedNetwork        string
		expectedAddress        string
		expectedPort           int
		expectedComponent      uint16
		expectedRelatedAddress *ice.CandidateRelatedAddress
	}{
		{
			ICECandidate{
				Foundation: "foundation",
				Priority:   128,
				Address:    "1.0.0.1",
				Protocol:   ICEProtocolUDP,
				Port:       1234,
				Typ:        ICECandidateTypeHost,
				Component:  1,
			},

			ice.CandidateTypeHost,
			"udp",
			"1.0.0.1",
			1234,
			1,
			nil,
		},
	}

	for i, testCase := range testCases {
		var expectedICE ice.Candidate
		var err error
		switch testCase.expectedType { // nolint:exhaustive
		case ice.CandidateTypeHost:
			config := ice.CandidateHostConfig{
				Network:    testCase.expectedNetwork,
				Address:    testCase.expectedAddress,
				Port:       testCase.expectedPort,
				Component:  testCase.expectedComponent,
				Foundation: "foundation",
				Priority:   128,
			}
			expectedICE, err = ice.NewCandidateHost(&config)
		case ice.CandidateTypeServerReflexive:
			config := ice.CandidateServerReflexiveConfig{
				Network:    testCase.expectedNetwork,
				Address:    testCase.expectedAddress,
				Port:       testCase.expectedPort,
				Component:  testCase.expectedComponent,
				Foundation: "foundation",
				Priority:   128,
				RelAddr:    testCase.expectedRelatedAddress.Address,
				RelPort:    testCase.expectedRelatedAddress.Port,
			}
			expectedICE, err = ice.NewCandidateServerReflexive(&config)
		case ice.CandidateTypePeerReflexive:
			config := ice.CandidatePeerReflexiveConfig{
				Network:    testCase.expectedNetwork,
				Address:    testCase.expectedAddress,
				Port:       testCase.expectedPort,
				Component:  testCase.expectedComponent,
				Foundation: "foundation",
				Priority:   128,
				RelAddr:    testCase.expectedRelatedAddress.Address,
				RelPort:    testCase.expectedRelatedAddress.Port,
			}
			expectedICE, err = ice.NewCandidatePeerReflexive(&config)
		}
		assert.NoError(t, err)

		// first copy the candidate ID so it matches the new one
		testCase.native.statsID = expectedICE.ID()
		actualICE, err := testCase.native.toICE()
		assert.NoError(t, err)

		assert.Equal(t, expectedICE, actualICE, "testCase: %d ice not equal %v", i, actualICE)
	}
}

func TestConvertTypeFromICE(t *testing.T) {
	t.Run("host", func(t *testing.T) {
		ct, err := convertTypeFromICE(ice.CandidateTypeHost)
		if err != nil {
			t.Fatal("failed coverting ice.CandidateTypeHost")
		}
		if ct != ICECandidateTypeHost {
			t.Fatal("should be converted to ICECandidateTypeHost")
		}
	})
}

func TestICECandidate_ToJSON(t *testing.T) {
	candidate := ICECandidate{
		Foundation: "foundation",
		Priority:   128,
		Address:    "1.0.0.1",
		Protocol:   ICEProtocolUDP,
		Port:       1234,
		Typ:        ICECandidateTypeHost,
		Component:  1,
	}

	candidateInit := candidate.ToJSON()

	assert.Equal(t, uint16(0), *candidateInit.SDPMLineIndex)
	assert.Equal(t, "candidate:foundation 1 udp 128 1.0.0.1 1234 typ host", candidateInit.Candidate)
}
