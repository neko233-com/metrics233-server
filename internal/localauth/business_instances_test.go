package localauth

import (
	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBusinessInstanceRoleMatrix(t *testing.T) {
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor, identity.RoleAdmin} {
		p := principal(role)
		require.NoError(t, authorize(p, "GET", "/api/v1/business/instances"))
		require.NoError(t, authorize(p, "GET", "/api/v1/business/instances/id/data"))
		err := authorize(p, "PUT", "/api/v1/business/instances/id/dashboard")
		if role == identity.RoleViewer {
			require.ErrorIs(t, err, identity.ErrForbidden)
		} else {
			require.NoError(t, err)
		}
	}
	for _, path := range []string{"/api/v1/business/instances/id/unknown", "/api/v1/business/instances/id/nested/dashboard"} {
		require.ErrorIs(t, authorize(principal(identity.RoleEditor), "PUT", path), identity.ErrForbidden)
	}
	require.ErrorIs(t, authorize(principal(identity.RoleEditor), "DELETE", "/api/v1/business/instances/id/dashboard"), identity.ErrForbidden)
}

func TestBusinessNoteRoleMatrix(t *testing.T) {
	for _, role := range []identity.Role{identity.RoleViewer, identity.RoleEditor, identity.RoleAdmin} {
		p := principal(role)
		require.NoError(t, authorize(p, "GET", "/api/v1/business/directory"))
		require.NoError(t, authorize(p, "GET", "/api/v1/business/instances/id/network"))
		err := authorize(p, "PUT", "/api/v1/business/instances/id/note")
		if role == identity.RoleViewer {
			require.ErrorIs(t, err, identity.ErrForbidden)
		} else {
			require.NoError(t, err)
		}
	}
	require.ErrorIs(t, authorize(principal(identity.RoleEditor), "PUT", "/api/v1/business/instances/id/nested/note"), identity.ErrForbidden)
}
