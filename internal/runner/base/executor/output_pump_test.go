//go:build test

package executor

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file do not count the process's open descriptors:
// numOpenFDs lives in the external test package and cannot be referenced
// here. Descriptor release is instead asserted by closing the same file a
// second time and expecting os.ErrClosed.
//
// No test in this file calls t.Parallel: TestOutputPump_PipeCreationFailureReleasesDescriptors
// swaps the package-level pipeFn, and the pump tests that call newOutputPump
// must not run while it is swapped.

// TestBoundedBuffer_UnlimitedBehavesLikeBytesBuffer checks that a limit of 0
// disables the bound entirely: every byte written is retained, exactly like
// bytes.Buffer.
func TestBoundedBuffer_UnlimitedBehavesLikeBytesBuffer(t *testing.T) {
	limitless := newBoundedBuffer(0)
	reference := &bytes.Buffer{}

	assert.Empty(t, limitless.Bytes())

	for _, in := range []string{"hello ", "bounded", "\nline\n", "tail"} {
		nRef, errRef := reference.WriteString(in)
		require.NoError(t, errRef)
		n, err := limitless.Write([]byte(in))
		require.NoError(t, err)
		assert.Equal(t, nRef, n)
	}
	assert.Equal(t, reference.String(), string(limitless.Bytes()))
}

// TestBoundedBuffer_KeepsPrefixAndSuffix checks the bounded case: the first
// limit bytes and the last limit bytes survive, the middle is replaced by
// the omission marker, and no marker appears while the total stays within
// 2*limit.
func TestBoundedBuffer_KeepsPrefixAndSuffix(t *testing.T) {
	const limit = 4
	tests := []struct {
		name  string
		input string   // written in one Write; empty when parts is set
		parts []string // written in this order; empty for a single Write
		want  string
	}{
		{
			name:  "exactly limit",
			input: "abcd",
			want:  "abcd",
		},
		{
			name:  "limit plus one",
			input: "abcda",
			want:  "abcda",
		},
		{
			name:  "twice limit",
			input: "abcdabcd",
			want:  "abcdabcd",
		},
		{
			name:  "twice limit plus one",
			input: "abcdabcda",
			want:  "abcd\n... omitting 1 bytes ...\nbcda",
		},
		{
			name:  "beyond twice limit",
			input: strings.Repeat("abcd", 4),
			want:  "abcd\n... omitting 8 bytes ...\nabcd",
		},
		{
			name:  "across multiple writes",
			parts: []string{"abcd", "abcde", "fgh"},
			want:  "abcd\n... omitting 4 bytes ...\nefgh",
		},
		{
			name:  "ring wrap",
			parts: []string{"abcd", "efgh", "ijklm"}, // the ring copy wraps its write position
			want:  "abcd\n... omitting 5 bytes ...\njklm",
		},
		{
			name:  "two-iteration ring",
			parts: []string{"abcd", "efgh", "ij", "klmno"}, // one write overruns a full ring
			want:  "abcd\n... omitting 7 bytes ...\nlmno",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newBoundedBuffer(limit)
			writes := tt.parts
			if len(writes) == 0 {
				writes = []string{tt.input}
			}
			for _, part := range writes {
				n, err := buf.Write([]byte(part))
				require.NoError(t, err)
				assert.Equal(t, len(part), n)
			}
			assert.Equal(t, tt.want, string(buf.Bytes()))
		})
	}
}

// TestBoundedBuffer_WriteNeverFails checks that Write keeps returning
// (len(p), nil) once the bound is exceeded: the reader must keep draining
// stderr, and a command that writes past the bound and then exits
// successfully must keep succeeding.
func TestBoundedBuffer_WriteNeverFails(t *testing.T) {
	buf := newBoundedBuffer(4)

	n, err := buf.Write([]byte("abcdefgh")) // a single write beyond the bound
	require.NoError(t, err)
	assert.Equal(t, len("abcdefgh"), n)

	n, err = buf.Write([]byte("more"))
	require.NoError(t, err)
	assert.Equal(t, len("more"), n)

	assert.Equal(t, "abcd\n... omitting 4 bytes ...\nmore", string(buf.Bytes()))
}

// TestOutputPump_SeparatesStreams checks that the two streams reach the
// shared OutputWriter tagged with their own stream, through the pump's
// reader goroutines.
func TestOutputPump_SeparatesStreams(t *testing.T) {
	recorder := &streamRecorder{}
	pump, err := newOutputPump(recorder, 0)
	require.NoError(t, err)
	stdoutFile, stderrFile := pump.childFiles()
	pump.start()

	// Each stream is drained to completion before the next one is written,
	// so the recordings cannot interleave and the recorder is reached by at
	// most one goroutine at a time.
	_, err = stdoutFile.Write([]byte("out-1"))
	require.NoError(t, err)
	_ = stdoutFile.Close()
	<-pump.stdout.done

	_, err = stderrFile.Write([]byte("err-1"))
	require.NoError(t, err)
	_ = stderrFile.Close()
	<-pump.stderr.done

	// The readers have finished, so the wrapper state is stable to read.
	assert.Equal(t, "out-1", string(pump.stdout.wrapper.GetBuffer()))
	assert.Equal(t, "err-1", string(pump.stderr.wrapper.GetBuffer()))
	assert.NoError(t, pump.stdout.wrapper.GetWriteError())
	assert.NoError(t, pump.stderr.wrapper.GetWriteError())
	assert.Equal(t, []streamWrite{
		{stream: StdoutStream, data: "out-1"},
		{stream: StderrStream, data: "err-1"},
	}, recorder.writes)
}

// streamFailWriter is an OutputWriter that fails every write, with a
// distinct sentinel per stream, so the precedence of the write errors the
// pump reports can be asserted.
type streamFailWriter struct {
	stdoutErr error
	stderrErr error
}

func (w *streamFailWriter) Write(stream OutputStream, _ []byte) error {
	switch stream {
	case StdoutStream:
		return w.stdoutErr
	case StderrStream:
		return w.stderrErr
	default:
		return nil
	}
}

func (w *streamFailWriter) Close() error { return nil }

// TestOutputPump_WriteErrorPrefersStdout checks that when both streams fail,
// wait reports the stdout stream's write error.
func TestOutputPump_WriteErrorPrefersStdout(t *testing.T) {
	stdoutErr := errors.New("stdout size limit exceeded")
	stderrErr := errors.New("stderr size limit exceeded")
	pump, err := newOutputPump(&streamFailWriter{stdoutErr: stdoutErr, stderrErr: stderrErr}, 0)
	require.NoError(t, err)
	stdoutFile, stderrFile := pump.childFiles()
	pump.start()

	_, err = stdoutFile.Write([]byte("o"))
	require.NoError(t, err)
	_, err = stderrFile.Write([]byte("e"))
	require.NoError(t, err)
	require.NoError(t, pump.releaseChildEnds())

	stdout, stderr, writeErr, timedOut := pump.wait(0)
	assert.False(t, timedOut)
	assert.ErrorIs(t, writeErr, stdoutErr, "the stdout write error takes precedence over stderr's")
	assert.NotErrorIs(t, writeErr, stderrErr)
	assert.Equal(t, "o", string(stdout), "the chunk is buffered before the failing write")
	assert.Equal(t, "e", string(stderr))
}

// TestOutputPump_WaitDeadlineReadsFinishedStreamOnly checks the mixed
// deadline case: a stream that finished before the deadline is collected,
// while the unfinished one is reported as nil. It also pins release after a
// timed-out wait: it must close the still-open ends without error, which is
// how a later kill path stops a reader still running.
func TestOutputPump_WaitDeadlineReadsFinishedStreamOnly(t *testing.T) {
	recorder := &streamRecorder{}
	pump, err := newOutputPump(recorder, 0)
	require.NoError(t, err)
	stdoutFile, stderrFile := pump.childFiles()
	pump.start()

	// The stdout stream reaches EOF; the stderr stream is left open, so at
	// the deadline one reader is finished and the other is not.
	_, err = stdoutFile.Write([]byte("done"))
	require.NoError(t, err)
	_ = stdoutFile.Close()

	stdout, stderr, writeErr, timedOut := pump.wait(50 * time.Millisecond)
	require.True(t, timedOut)
	assert.Equal(t, "done", string(stdout), "a finished stream is still collected")
	assert.Nil(t, stderr, "an unfinished stream must be reported as nil")
	assert.NoError(t, writeErr)
	assert.Equal(t, []streamWrite{{stream: StdoutStream, data: "done"}}, recorder.writes)

	// release after the timed-out wait closes what the readers did not:
	// the still-open stderr write end and read end, and the already-closed
	// stdout write end again. None of that may error.
	require.NoError(t, pump.release())

	_ = stderrFile.Close()
	select {
	case <-pump.stderr.done:
	case <-time.After(time.Second):
		t.Fatal("stderr reader did not finish after the write end closed")
	}
}

// TestOutputPump_PipeCreationFailureReleasesDescriptors checks that a pipe
// creation failure returns ErrOutputPipe and releases the descriptors that
// were already created.
func TestOutputPump_PipeCreationFailureReleasesDescriptors(t *testing.T) {
	origPipeFn := pipeFn
	var firstRead, firstWrite *os.File
	calls := 0
	pipeFn = func() (*os.File, *os.File, error) {
		calls++
		if calls == 1 {
			r, w, err := origPipeFn()
			firstRead, firstWrite = r, w
			return r, w, err
		}
		return nil, nil, errSecondPipe
	}
	t.Cleanup(func() { pipeFn = origPipeFn })

	pump, err := newOutputPump(nil, 0)
	require.ErrorIs(t, err, ErrOutputPipe)
	require.Nil(t, pump)
	// The underlying failure stays reachable: a caller that wants to tell
	// descriptor exhaustion (syscall.EMFILE) from other pipe failures reads
	// it through errors.Is, so the cause must be wrapped, not formatted in.
	assert.ErrorIs(t, err, errSecondPipe)

	// The first pipe's ends were released on the failure: closing them again
	// reports already-closed.
	assert.ErrorIs(t, firstRead.Close(), os.ErrClosed)
	assert.ErrorIs(t, firstWrite.Close(), os.ErrClosed)
}

// TestOutputPump_ReleaseIsIdempotent checks that release and
// releaseChildEnds can be called repeatedly on a pump whose readers were
// never started, without an error.
func TestOutputPump_ReleaseIsIdempotent(t *testing.T) {
	pump, err := newOutputPump(&streamRecorder{}, 0)
	require.NoError(t, err)

	require.NoError(t, pump.releaseChildEnds())
	require.NoError(t, pump.releaseChildEnds())
	require.NoError(t, pump.release())
	require.NoError(t, pump.release())
}

// TestOutputPump_WaitDeadlineDoesNotReadUnfinishedStream checks that on the
// deadline path wait reports an unfinished stream as nil and timedOut true,
// without reading its buffer. The stdout reader copies the chunk written
// below into the wrapper buffer before the deadline fires; a wait that read
// the unfinished buffer would race that write under -race.
func TestOutputPump_WaitDeadlineDoesNotReadUnfinishedStream(t *testing.T) {
	pump, err := newOutputPump(&streamRecorder{}, 0)
	require.NoError(t, err)
	stdoutFile, stderrFile := pump.childFiles()
	pump.start()

	// Both write ends stay open, so both readers are still running when the
	// deadline fires.
	_, err = stdoutFile.Write([]byte("pending"))
	require.NoError(t, err)

	stdout, stderr, writeErr, timedOut := pump.wait(50 * time.Millisecond)
	require.True(t, timedOut)
	assert.Nil(t, stdout, "an unfinished stream must be reported as nil")
	assert.Nil(t, stderr)
	assert.NoError(t, writeErr)

	// Close both write ends and join the readers so no pump goroutine
	// outlives the test.
	_ = stdoutFile.Close()
	_ = stderrFile.Close()
	for _, s := range []*pumpStream{pump.stdout, pump.stderr} {
		select {
		case <-s.done:
		case <-time.After(time.Second):
			t.Fatalf("reader %s did not finish after the write end closed", s.wrapper.stream)
		}
	}
}

// errSecondPipe stands in for the errno os.Pipe reports; the test asserts it
// is reachable through the ErrOutputPipe wrapper.
var errSecondPipe = errors.New("second pipe creation failed")

// TestNewBoundedBuffer_RejectsNegativeLimit checks that a negative limit is
// rejected at construction. Without the check, Write slices p past its
// length on the first write larger than the limit and panics there instead,
// far from the mistake.
func TestNewBoundedBuffer_RejectsNegativeLimit(t *testing.T) {
	assert.Panics(t, func() { newBoundedBuffer(-1) })

	// The limits the executor actually passes must stay constructible: 0 for
	// the OutputWriter path, nilWriterStderrLimit for the Cmd.Output path.
	// This fails if either is ever edited into an invalid shape.
	for _, limit := range []int{0, nilWriterStderrLimit} {
		assert.NotPanics(t, func() { newBoundedBuffer(limit) })
	}
}

// TestOutputPump_StartRejectsSecondCall checks the guard on start. A second
// call would leave one reader per stream blocked forever on the send to
// done, which has room for a single value -- a goroutine leak no test could
// see, since the read ends are closed by then.
func TestOutputPump_StartRejectsSecondCall(t *testing.T) {
	pump, err := newOutputPump(nil, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pump.release() })

	pump.start()
	assert.Panics(t, func() { pump.start() })

	require.NoError(t, pump.releaseChildEnds())
	_, _, _, timedOut := pump.wait(time.Second)
	assert.False(t, timedOut, "the readers started by the first call must still finish")
}
