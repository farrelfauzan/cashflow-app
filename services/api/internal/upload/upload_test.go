package upload_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

func TestActionsAreInTheCatalog(t *testing.T) {
	t.Parallel()
	for _, a := range upload.Actions {
		require.True(t, authz.IsAction(a), a)
	}
}

func TestValidCreate(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	tests := []struct {
		name      string
		in        upload.CreateInput
		wantErr   error
		wantField string
		wantType  string
	}{
		{name: "a receipt photo", in: upload.CreateInput{Purpose: upload.PurposeCashReceipt, TenantID: tenant, ContentType: "image/jpeg", Size: 2_000_000}, wantType: "image/jpeg"},
		{name: "a deposit photo", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/webp", Size: 1}, wantType: "image/webp"},
		{name: "the type is normalized", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: " Image/PNG ", Size: 10}, wantType: "image/png"},
		{name: "exactly the limit", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/png", Size: upload.MaxSize}, wantType: "image/png"},
		{name: "an unknown purpose", in: upload.CreateInput{Purpose: "selfie", ContentType: "image/jpeg", Size: 10}, wantErr: upload.ErrPurposeUnknown},
		{name: "no purpose", in: upload.CreateInput{ContentType: "image/jpeg", Size: 10}, wantErr: upload.ErrPurposeUnknown},
		{name: "a receipt without a tenant", in: upload.CreateInput{Purpose: upload.PurposeCashReceipt, ContentType: "image/jpeg", Size: 10}, wantField: "tenantId"},
		{name: "a deposit with a tenant", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, TenantID: tenant, ContentType: "image/jpeg", Size: 10}, wantField: "tenantId"},
		{name: "HEIC is not accepted", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/heic", Size: 10}, wantErr: upload.ErrTypeNotAllowed},
		{name: "a PDF is not accepted", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "application/pdf", Size: 10}, wantErr: upload.ErrTypeNotAllowed},
		{name: "a type with parameters is not accepted", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/jpeg; q=1", Size: 10}, wantErr: upload.ErrTypeNotAllowed},
		{name: "one byte over the limit", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/jpeg", Size: upload.MaxSize + 1}, wantErr: upload.ErrTooLarge},
		{name: "an empty file", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/jpeg", Size: 0}, wantField: "size"},
		{name: "a negative size", in: upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/jpeg", Size: -5}, wantField: "size"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := upload.ValidCreate(tc.in)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantField != "":
				var input *upload.InputError
				require.ErrorAs(t, err, &input)
				require.Equal(t, tc.wantField, input.Field)
				require.NotEmpty(t, input.Message)
			default:
				require.NoError(t, err)
				require.Equal(t, tc.wantType, got.ContentType)
			}
		})
	}
}

func TestSniff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content []byte
		want    string
		allowed bool
	}{
		{name: "JPEG", content: uploadtest.Image("image/jpeg", 600), want: "image/jpeg", allowed: true},
		{name: "PNG", content: uploadtest.Image("image/png", 600), want: "image/png", allowed: true},
		{name: "WebP", content: uploadtest.Image("image/webp", 600), want: "image/webp", allowed: true},
		{name: "a PDF", content: []byte("%PDF-1.7\n%âãÏÓ\n1 0 obj"), want: "application/pdf"},
		{name: "text", content: []byte("Ini bukan gambar."), want: "text/plain"},
		{name: "HTML", content: []byte("<!DOCTYPE html><script>alert(1)</script>"), want: "text/html"},
		{name: "GIF is an image but not accepted", content: []byte("GIF89a\x01\x00\x01\x00"), want: "image/gif"},
		{name: "HEIC is not recognized", content: []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), want: "application/octet-stream"},
		{name: "empty", content: nil, want: "text/plain"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := upload.Sniff(tc.content)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.allowed, upload.AllowedType(got))
		})
	}
}

func TestAllowedTypes(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{"image/jpeg", "image/png", "image/webp"}, upload.AllowedTypes())
}

func TestObjectKeys(t *testing.T) {
	t.Parallel()
	const key = "uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/00112233445566778899aabbccddeeff"
	require.Equal(t, "incoming/"+key, upload.IncomingKey(key))
	require.Equal(t, "files/"+key, upload.StoredKey(key))
	require.Equal(t, "quarantine/"+key, upload.QuarantineKey(key))
}

// TestInspect covers what the scan decides about one file and where the file ends up.
func TestInspect(t *testing.T) {
	t.Parallel()
	const key = "uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/00112233445566778899aabbccddeeff"
	jpeg := uploadtest.Image("image/jpeg", 4096)
	eicar := []byte(uploadtest.EICAR)
	// A JPEG header with EICAR inside, so it passes the type check and reaches the scanner.
	infectedJPEG := append(uploadtest.Image("image/jpeg", 1024), eicar...)
	pdf := append([]byte("%PDF-1.7\n"), make([]byte, 1000)...)

	tests := []struct {
		name         string
		objects      map[string][]byte // what the bucket holds before the run
		declared     int64
		storageErr   error
		scanErr      error
		want         upload.Finding
		wantErr      bool
		wantKeys     []string // where the file is after the run
		wantScanned  int
		wantNotMoved bool // the run made no move
	}{
		{
			name: "a clean JPEG is taken from incoming and kept", objects: map[string][]byte{upload.IncomingKey(key): jpeg},
			declared: int64(len(jpeg)), want: upload.Finding{Status: upload.StatusClean, ContentType: "image/jpeg"},
			wantKeys: []string{upload.StoredKey(key)}, wantScanned: 1,
		},
		{
			name: "EICAR in a JPEG is infected and quarantined", objects: map[string][]byte{upload.IncomingKey(key): infectedJPEG},
			declared: int64(len(infectedJPEG)),
			want:     upload.Finding{Status: upload.StatusInfected, ContentType: "image/jpeg", Signature: uploadtest.EICARSignature},
			wantKeys: []string{upload.QuarantineKey(key)}, wantScanned: 1,
		},
		{
			name: "the bare EICAR file sent as .jpg is infected, not merely not an image", objects: map[string][]byte{upload.IncomingKey(key): eicar},
			declared: int64(len(eicar)),
			want:     upload.Finding{Status: upload.StatusInfected, ContentType: "text/plain", Signature: uploadtest.EICARSignature},
			wantKeys: []string{upload.QuarantineKey(key)}, wantScanned: 1,
		},
		{
			name: "a PDF named .jpg is rejected and quarantined", objects: map[string][]byte{upload.IncomingKey(key): pdf},
			declared: int64(len(pdf)), want: upload.Finding{Status: upload.StatusRejected, ContentType: "application/pdf", Reason: "not_image"},
			wantKeys: []string{upload.QuarantineKey(key)}, wantScanned: 1,
		},
		{
			name: "a file of another size than declared is rejected and quarantined", objects: map[string][]byte{upload.IncomingKey(key): jpeg},
			declared: int64(len(jpeg)) - 1, want: upload.Finding{Status: upload.StatusRejected, Reason: "size_mismatch"},
			wantKeys: []string{upload.QuarantineKey(key)},
		},
		{
			name: "a file above the limit is rejected and quarantined", objects: map[string][]byte{upload.IncomingKey(key): uploadtest.Image("image/jpeg", int(upload.MaxSize)+1)},
			declared: upload.MaxSize + 1, want: upload.Finding{Status: upload.StatusRejected, Reason: "too_large"},
			wantKeys: []string{upload.QuarantineKey(key)},
		},
		{
			name: "no file yet decides nothing", declared: 10, want: upload.Finding{}, wantKeys: []string{},
		},
		{
			name: "a run after the move to files/ reads it there", objects: map[string][]byte{upload.StoredKey(key): jpeg},
			declared: int64(len(jpeg)), want: upload.Finding{Status: upload.StatusClean, ContentType: "image/jpeg"},
			wantKeys: []string{upload.StoredKey(key)}, wantScanned: 1, wantNotMoved: true,
		},
		{
			name: "a run after the quarantine move keeps it there and decides the same", objects: map[string][]byte{upload.QuarantineKey(key): infectedJPEG},
			declared: int64(len(infectedJPEG)),
			want:     upload.Finding{Status: upload.StatusInfected, ContentType: "image/jpeg", Signature: uploadtest.EICARSignature},
			wantKeys: []string{upload.QuarantineKey(key)}, wantScanned: 1, wantNotMoved: true,
		},
		{
			name: "a quarantined file the scanner now passes is never served", objects: map[string][]byte{upload.QuarantineKey(key): jpeg},
			declared: int64(len(jpeg)), want: upload.Finding{Status: upload.StatusRejected, ContentType: "image/jpeg", Reason: "quarantined_earlier"},
			wantKeys: []string{upload.QuarantineKey(key)}, wantScanned: 1, wantNotMoved: true,
		},
		{
			name: "a second PUT after the move is never read", objects: map[string][]byte{upload.StoredKey(key): jpeg, upload.IncomingKey(key): infectedJPEG},
			declared: int64(len(jpeg)), want: upload.Finding{Status: upload.StatusClean, ContentType: "image/jpeg"},
			wantKeys: []string{upload.StoredKey(key), upload.IncomingKey(key)}, wantScanned: 1, wantNotMoved: true,
		},
		{
			name: "a storage outage is an error", objects: map[string][]byte{upload.IncomingKey(key): jpeg}, declared: int64(len(jpeg)),
			storageErr: uploadtest.ErrDown, wantErr: true, wantKeys: []string{upload.IncomingKey(key)}, wantNotMoved: true,
		},
		{
			name: "a scanner outage is an error and quarantines nothing", objects: map[string][]byte{upload.IncomingKey(key): jpeg}, declared: int64(len(jpeg)),
			scanErr: uploadtest.ErrDown, wantErr: true, wantKeys: []string{upload.StoredKey(key)}, wantScanned: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := uploadtest.NewStorage()
			for k, v := range tc.objects {
				st.Put(k, v)
			}
			st.Err = tc.storageErr
			sc := &uploadtest.Scanner{Err: tc.scanErr}
			got, err := upload.Inspect(context.Background(), st, sc, key, tc.declared)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			keys := st.Keys()
			want := append([]string{}, tc.wantKeys...)
			require.ElementsMatch(t, want, keys)
			require.Equal(t, tc.wantScanned, sc.Scanned())
			if tc.wantNotMoved {
				for _, c := range st.Calls() {
					require.False(t, strings.HasPrefix(c, "move "), "unexpected %s", c)
				}
			}
		})
	}
}

func TestInspectKeepsTheFileIntact(t *testing.T) {
	t.Parallel()
	const key = "uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/ffeeddccbbaa99887766554433221100"
	content := uploadtest.Image("image/png", 70_000)
	content[69_999] = 0x42
	st := uploadtest.NewStorage()
	st.Put(upload.IncomingKey(key), content)
	rec := &recordingScanner{}
	got, err := upload.Inspect(context.Background(), st, rec, key, int64(len(content)))
	require.NoError(t, err)
	require.Equal(t, upload.StatusClean, got.Status)
	require.Equal(t, content, rec.got, "the scanner sees the whole file, the sniffed head included")
	stored, ok := st.Object(upload.StoredKey(key))
	require.True(t, ok)
	require.Equal(t, content, stored)
}

// recordingScanner keeps what it was given and calls it clean.
type recordingScanner struct{ got []byte }

func (r *recordingScanner) Scan(_ context.Context, rd io.Reader) (upload.Verdict, error) {
	b, err := io.ReadAll(rd)
	if err != nil {
		return upload.Verdict{}, err
	}
	r.got = b
	return upload.Verdict{Clean: true}, nil
}
