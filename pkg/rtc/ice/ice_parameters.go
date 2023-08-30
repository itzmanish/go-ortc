package ice

type ICEParameters struct {
	UsernameFragment string `json:"usernameFragment"`
	Password         string `json:"password"`
	ICELite          bool   `json:"iceLite"`
}
