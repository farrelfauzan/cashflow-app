//go:build integration

package testenv

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
)

func normalize(t *testing.T, raw string) string {
	t.Helper()
	n, err := phone.Normalize(raw)
	require.NoError(t, err)
	return n
}
