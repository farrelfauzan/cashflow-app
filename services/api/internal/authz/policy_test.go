package authz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePolicy(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			name: "valid custom role",
			doc: `{"Version":"2026-10-25","Statement":[
				{"Sid":"Collect","Effect":"Allow","Action":["payment:record","cash:*"],
				 "Resource":["property/*"],"Condition":{"property":"assigned","amountAtMost":5000000}}]}`,
		},
		{name: "unknown field", doc: `{"Version":"2026-10-25","Statement":[],"Extra":1}`, wantErr: "unknown field"},
		{name: "wrong version", doc: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["invoice:read"]}]}`, wantErr: "version"},
		{name: "no statements", doc: `{"Version":"2026-10-25","Statement":[]}`, wantErr: "no statements"},
		{name: "bad effect", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Maybe","Action":["invoice:read"]}]}`, wantErr: "effect"},
		{name: "no actions", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":[]}]}`, wantErr: "no actions"},
		{name: "typo in action", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoce:read"]}]}`, wantErr: "matches no known action"},
		{name: "action without verb", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoice"]}]}`, wantErr: "resource:verb"},
		{name: "uppercase action", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["Invoice:read"]}]}`, wantErr: "resource:verb"},
		{name: "bad resource", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoice:read"],"Resource":["a b"]}]}`, wantErr: "resource"},
		{name: "bad property condition", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoice:read"],"Condition":{"property":"any"}}]}`, wantErr: "assigned"},
		{name: "unknown condition key", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoice:read"],"Condition":{"ip":"1.2.3.4"}}]}`, wantErr: "unknown field"},
		{name: "negative amount", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoice:read"],"Condition":{"amountAtMost":-1}}]}`, wantErr: "negative"},
		{name: "trailing data", doc: `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["invoice:read"]}]} {}`, wantErr: "trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePolicy([]byte(tc.doc))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestBuiltinPoliciesAndGuardrailsAreValid(t *testing.T) {
	require.NoError(t, Guardrails.Validate())
	for _, key := range BuiltinRoleKeys() {
		r, ok := BuiltinRole(key)
		require.True(t, ok, key)
		require.NoError(t, r.Policy.Validate(), key)
		require.Equal(t, key, r.Key)
	}
}

func TestCatalogConstantsAndSensitiveActionsExist(t *testing.T) {
	for _, a := range []string{ActionMemberRead, ActionMemberInvite, ActionMemberManage, ActionRoleAssign} {
		require.True(t, IsAction(a), a)
	}
	for a := range sensitive {
		require.True(t, IsAction(a), a)
	}
	require.False(t, IsAction("invoice:*"), "wildcards are patterns, not actions")
}
