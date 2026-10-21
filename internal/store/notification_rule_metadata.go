package store

import (
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/notificationtemplate"
)

func notificationRuleMetadata(rule model.AlertRule) notificationtemplate.RuleMetadata {
	return notificationtemplate.RuleMetadata{UID: rule.UID, Title: rule.Title, Expr: rule.Expr, ForSeconds: rule.ForSeconds,
		KeepFiringForSeconds: rule.KeepFiringForSeconds, NotificationCooldownSeconds: rule.NotificationCooldownSeconds}
}
