package messagingtest_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/messagingtest"
)

func TestSamplesCoverEveryTemplate(t *testing.T) {
	t.Parallel()
	samples := messagingtest.Samples()
	require.Equal(t, messaging.TemplateNames(), slices.Sorted(maps.Keys(samples)),
		"every registered template has a sample, and no sample names an unknown template")
	for name, m := range samples {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, name, m.Template)
			_, err := messaging.Render(m)
			require.NoError(t, err)
			_, err = messaging.RenderPlain(m)
			require.NoError(t, err)
		})
	}
}
