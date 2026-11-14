//go:build clamav

// The test against a real clamd has its own build tag, clamav, and its own make target
// (make api-test-clamav), because clamd needs about 1 GB of memory and a minute or two to load
// its signatures, too slow for every integration run. CI runs it as a separate step.
package clamd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/scan/clamd"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// clamavImage is the ClamAV the test and infra/compose.dev.yaml run. It ships with a signature
// database, so the test needs no download from ClamAV's mirrors.
const clamavImage = "clamav/clamav:1.4.3"

func TestAgainstClamAV(t *testing.T) {
	ctx := context.Background()
	ctr, err := testcontainers.Run(ctx, clamavImage,
		testcontainers.WithExposedPorts("3310/tcp"),
		// The image's freshclam would fetch updates on start; the bundled database is enough.
		testcontainers.WithEnv(map[string]string{"CLAMAV_NO_FRESHCLAMD": "true"}),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("3310/tcp").WithStartupTimeout(5*time.Minute)),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)
	addr, err := ctr.PortEndpoint(ctx, "3310/tcp", "")
	require.NoError(t, err)
	c, err := clamd.New(strings.TrimPrefix(addr, "://"))
	require.NoError(t, err)

	// The port opens before the database is loaded on some versions; wait for PONG.
	deadline := time.Now().Add(5 * time.Minute)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.Ping(pingCtx)
		cancel()
		if err == nil {
			break
		}
		require.True(t, time.Now().Before(deadline), "clamd never answered PONG: %v", err)
		time.Sleep(2 * time.Second)
	}

	tests := []struct {
		name    string
		content []byte
		want    upload.Verdict
	}{
		{name: "a clean JPEG", content: uploadtest.Image("image/jpeg", 200_000), want: upload.Verdict{Clean: true}},
		{name: "the EICAR test file", content: []byte(uploadtest.EICAR), want: upload.Verdict{Signature: "Eicar-Test-Signature"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.Scan(ctx, bytes.NewReader(tc.content))
			require.NoError(t, err)
			require.Equal(t, tc.want.Clean, got.Clean)
			if !tc.want.Clean {
				require.Contains(t, got.Signature, "Eicar")
			}
		})
	}
}
