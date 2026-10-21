package model

// BusinessIdentity is a reporting hint, never an authentication or ACL identity.
// Empty optional scope labels remain empty instead of being guessed.
type BusinessIdentity struct {
	Service        string `json:"service"`
	Role           string `json:"role"`
	ServerID       string `json:"serverId"`
	Environment    string `json:"environment"`
	Channel        string `json:"channel"`
	Instance       string `json:"instance"`
	Implementation string `json:"implementation"`
}

func (i BusinessIdentity) Labels() map[string]string {
	return map[string]string{"service": i.Service, "role": i.Role, "serverId": i.ServerID, "environment": i.Environment, "channel": i.Channel, "instance": i.Instance, "implementation": i.Implementation}
}

type BusinessBinding struct {
	Version     int64  `json:"version"`
	Mode        string `json:"mode"` // default, inherit or independent
	DashboardID string `json:"dashboard_id"`
}

type BusinessInstance struct {
	ID         string           `json:"id"`
	Identity   BusinessIdentity `json:"identity"`
	ReportedIP string           `json:"reported_ip"`
	ObservedIP string           `json:"observed_ip"`
	FirstSeen  int64            `json:"first_seen"`
	LastSeen   int64            `json:"last_seen"`
	Binding    BusinessBinding  `json:"binding"`
}
