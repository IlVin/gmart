package domain

type Balance struct {
	Current   Amount `json:"current,omitzero"`
	Withdrawn Amount `json:"withdrawn,omitzero"`
}
