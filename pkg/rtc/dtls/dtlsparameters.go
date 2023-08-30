package dtls

// DTLSParameters holds information relating to DTLS configuration.
type DTLSParameters struct {
	Role         DTLSRole          `json:"role"`
	Fingerprints []DTLSFingerprint `json:"fingerprints"`
}