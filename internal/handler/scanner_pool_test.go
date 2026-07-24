package handler

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// OPT: scanner buffer pooling. Each streaming request currently allocates a
// 64KB buffer (make([]byte, 0, 64*1024)) for its bufio.Scanner. acquireScanner
// returns a scanner backed by a sync.Pool buffer; releaseScanner returns it for
// reuse. The API returns the buffer pointer (not a closure) so the release path
// stays allocation-free.

// TestAcquireScanner_ScansAllLines: the pooled scanner must behave exactly like
// a plain bufio.Scanner — every line is scanned in order.
func TestAcquireScanner_ScansAllLines(t *testing.T) {
	s, bufp := acquireScanner(strings.NewReader("a\nb\nc\n"))
	defer releaseScanner(bufp)

	var lines []string
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	assert.Equal(t, []string{"a", "b", "c"}, lines)
	assert.NoError(t, s.Err())
}

// TestAcquireScanner_HandlesLineExceedingInitialBuffer: a single line larger
// than the pooled 64KB initial buffer but within the 1MB cap must still scan,
// proving the Buffer max is preserved (scanner grows; the original 64KB slot is
// recycled unchanged).
func TestAcquireScanner_HandlesLineExceedingInitialBuffer(t *testing.T) {
	big := strings.Repeat("x", 100*1024) // 100KB > 64KB initial, < 1MB max
	s, bufp := acquireScanner(strings.NewReader(big + "\n"))
	defer releaseScanner(bufp)

	assert.True(t, s.Scan())
	assert.Equal(t, big, s.Text())
	assert.False(t, s.Scan())
	assert.NoError(t, s.Err())
}

// TestAcquireScanner_PoolReusesBuffer: after release, the next acquire must
// return the very same buffer slot — the pool recycles the 64KB slice instead of
// allocating a fresh one each call. GC is disabled so the released slot is not
// cleared from sync.Pool before the second acquire.
//
// (An earlier version tried to prove pooling via allocation counts against an
// un-pooled baseline, but the compiler stack-allocates the baseline's
// make([]byte, 0, 64*1024) inside the measurement closure, so both paths tied
// at 2 allocs and the counts could not distinguish pooled from un-pooled.
// Pointer identity distinguishes them directly.)
func TestAcquireScanner_PoolReusesBuffer(t *testing.T) {
	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)

	s1, bufp1 := acquireScanner(strings.NewReader("a\n"))
	for s1.Scan() {
	}
	releaseScanner(bufp1)

	_, bufp2 := acquireScanner(strings.NewReader("b\n"))
	defer releaseScanner(bufp2)

	assert.Same(t, bufp1, bufp2, "pool must reuse the released buffer slot")
}
