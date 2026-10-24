package mkcp

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type failWriter struct {
	attempts int
}

func (w *failWriter) Write([]byte) (int, error) {
	w.attempts++
	return 0, errors.New("mkcp: write failed")
}

func TestWriteSegmentsDoesNotRetryOrSleepAfterWriteError(t *testing.T) {
	writer := &failWriter{}
	conn := &Conn{writer: packetWriter{writer: writer}}

	start := time.Now()
	conn.writeSegments([]segment{&cmdOnlySegment{}})
	elapsed := time.Since(start)

	require.Equal(t, 1, writer.attempts)
	require.Less(t, elapsed, 100*time.Millisecond)
}
