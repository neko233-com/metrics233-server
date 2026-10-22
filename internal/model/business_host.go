package model

// BusinessHostBinding is an administrator-confirmed relationship. Reporting IP
// labels and NAT peers must never create or change this relationship.
type BusinessHostBinding struct {
	Version int64  `json:"version"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
}
