package localauth

import (
	"testing"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/stretchr/testify/require"
)

func TestFeishuPreviewRequiresAdminAndTemplateMutationsStayProtected(t *testing.T) {
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor, identity.RoleAdmin} {
		for _, route := range []string{"/api/v1/alerting/contact-points/preview", "/api/v1/alerting/contact-points", "/api/v1/alerting/notification-templates"} {
			err := authorize(principal(role), "POST", route)
			if role == identity.RoleAdmin {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, identity.ErrForbidden)
			}
		}
	}
}
