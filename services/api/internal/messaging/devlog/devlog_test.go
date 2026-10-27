package devlog_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
)

func loginCode(to, code string) messaging.Message {
	return messaging.Message{To: to, Template: messaging.TemplateLoginCode, Params: map[string]string{"code": code}}
}

func TestFileGetsOneJSONLinePerMessage(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "devlog.jsonl")
	p := devlog.New(slog.New(slog.NewTextHandler(io.Discard, nil)), file)

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.SendTemplate(context.Background(), loginCode("+62812000"+strconv.Itoa(10000+i), strconv.Itoa(100000+i)))
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	f, err := os.Open(file)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	info, err := f.Stat()
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "login codes are readable by the developer only")

	codes := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var line devlog.Line
		require.NoError(t, json.Unmarshal(sc.Bytes(), &line), "every line is whole JSON: %q", sc.Text())
		require.Equal(t, messaging.TemplateLoginCode, line.Template)
		require.Len(t, line.Params, 1)
		require.Contains(t, line.Text, line.Params[0], "the code is params[0]")
		codes[line.To] = line.Params[0]
	}
	require.NoError(t, sc.Err())
	require.Len(t, codes, n)
	require.Equal(t, "100007", codes["+6281200010007"])
}

func TestUnwritableFileFailsTheSend(t *testing.T) {
	t.Parallel()
	p := devlog.New(slog.New(slog.NewTextHandler(io.Discard, nil)), filepath.Join(t.TempDir(), "missing", "devlog.jsonl"))
	_, err := p.SendTemplate(context.Background(), loginCode("+6281200010001", "123456"))
	require.Error(t, err, "a developer sees the problem instead of waiting for a code that never lands")
}
