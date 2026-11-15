package devscan_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/scan/devscan"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

func TestScan(t *testing.T) {
	t.Parallel()
	infected := upload.Verdict{Signature: "Eicar-Test-Signature"}
	clean := upload.Verdict{Clean: true}
	jpeg := uploadtest.Image("image/jpeg", 200_000)
	eicar := []byte(devscan.EICAR)
	tests := []struct {
		name    string
		content io.Reader
		want    upload.Verdict
	}{
		{name: "the EICAR test file", content: bytes.NewReader(eicar), want: infected},
		{name: "EICAR at the end of a photo", content: bytes.NewReader(append(bytes.Clone(jpeg), eicar...)), want: infected},
		{name: "EICAR across two reads", content: iotest.HalfReader(bytes.NewReader(append(bytes.Clone(jpeg[:65_500]), eicar...))), want: infected},
		{name: "EICAR one byte at a time", content: iotest.OneByteReader(bytes.NewReader(eicar)), want: infected},
		{name: "a photo", content: bytes.NewReader(jpeg), want: clean},
		{name: "nothing", content: bytes.NewReader(nil), want: clean},
		{name: "EICAR cut short", content: bytes.NewReader(eicar[:len(eicar)-1]), want: clean},
		{name: "the words alone", content: strings.NewReader("EICAR-STANDARD-ANTIVIRUS-TEST-FILE eicar.jpg virus.exe"), want: clean},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := devscan.Scanner{}.Scan(context.Background(), tc.content)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestScanMatchesTheTestFake(t *testing.T) {
	t.Parallel()
	// The worker's development scanner and the tests' fake agree on the test file.
	require.Equal(t, uploadtest.EICAR, devscan.EICAR)
	require.Equal(t, uploadtest.EICARSignature, devscan.Signature)
}

func TestScanFails(t *testing.T) {
	t.Parallel()
	boom := errors.New("connection reset")
	_, err := devscan.Scanner{}.Scan(context.Background(), iotest.ErrReader(boom))
	require.ErrorIs(t, err, boom)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = devscan.Scanner{}.Scan(ctx, bytes.NewReader([]byte(devscan.EICAR)))
	require.ErrorIs(t, err, context.Canceled)
}
