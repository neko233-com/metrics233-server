package model

import "strings"

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

// BusinessPlatform is a directory label derived from the reported channel.
// Custom channels retain their original spelling and never collide with a built-in key.
func BusinessPlatform(channel string) string {
	key := strings.ToLower(strings.TrimSpace(channel))
	key = strings.ReplaceAll(key, "_", "-")
	key = strings.TrimPrefix(key, "channel-")
	key = strings.TrimSuffix(strings.TrimPrefix(key, "minigame-"), "-minigame")
	switch key {
	case "":
		if strings.TrimSpace(channel) == "" {
			return "unassigned"
		}
	case "wechat", "weixin", "wx", "微信", "微信小游戏":
		return "wechat"
	case "taptap", "tap-tap", "tap", "tap小游戏":
		return "taptap"
	case "douyin", "抖音", "抖音小游戏":
		return "douyin"
	case "qq", "qq小游戏":
		return "qq"
	case "kuaishou", "快手", "快手小游戏":
		return "kuaishou"
	case "bilibili", "b站", "哔哩哔哩":
		return "bilibili"
	case "ios", "appstore", "app-store":
		return "ios"
	case "steam":
		return "steam"
	case "web", "h5":
		return "web"
	case "shared", "common", "共享":
		return "shared"
	}
	if key == "android" || strings.HasPrefix(key, "android-") {
		return "android"
	}
	return "custom:" + channel
}

type BusinessBinding struct {
	Version     int64  `json:"version"`
	Mode        string `json:"mode"` // default, inherit or independent
	DashboardID string `json:"dashboard_id"`
}

type BusinessInstance struct {
	ID         string           `json:"id"`
	Identity   BusinessIdentity `json:"identity"`
	Platform   string           `json:"platform"`
	ReportedIP string           `json:"reported_ip"`
	ObservedIP string           `json:"observed_ip"`
	FirstSeen  int64            `json:"first_seen"`
	LastSeen   int64            `json:"last_seen"`
	Binding    BusinessBinding  `json:"binding"`
	Note       BusinessNote     `json:"note"`
}

// BusinessNote belongs to the operator, never to a reporting identity.
type BusinessNote struct {
	Version     int64  `json:"version"`
	Description string `json:"description"`
	UpdatedAt   int64  `json:"updated_at"`
}
