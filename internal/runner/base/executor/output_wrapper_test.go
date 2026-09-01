//go:build test

package executor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamWrite is one recorded Write call. Stream and data are kept together so
// that a stdout/stderr mix-up shows up in the recording instead of cancelling
// out in a total byte count.
type streamWrite struct {
	stream OutputStream
	data   string
}

// streamRecorder is an OutputWriter that records every Write call and can be
// primed to fail: errs is consumed one entry per Write, and Write succeeds once
// it is exhausted.
type streamRecorder struct {
	writes []streamWrite
	errs   []error
}

func (r *streamRecorder) Write(stream OutputStream, data []byte) error {
	r.writes = append(r.writes, streamWrite{stream: stream, data: string(data)})
	if len(r.errs) == 0 {
		return nil
	}
	err := r.errs[0]
	r.errs = r.errs[1:]
	return err
}

func (r *streamRecorder) Close() error { return nil }

// TestOutputWrapper_SeparatesStdoutAndStderr covers the two properties the
// wrapper keeps without a mutex: each wrapper buffers only its own stream and
// tags its writes with that stream, and the first write error is the one
// reported afterwards.
func TestOutputWrapper_SeparatesStdoutAndStderr(t *testing.T) {
	t.Run("stdout_and_stderr_do_not_mix", func(t *testing.T) {
		recorder := &streamRecorder{}
		stdoutWrapper := &outputWrapper{writer: recorder, stream: StdoutStream}
		stderrWrapper := &outputWrapper{writer: recorder, stream: StderrStream}

		n, err := stdoutWrapper.Write([]byte("out-1"))
		require.NoError(t, err)
		assert.Equal(t, len("out-1"), n)

		n, err = stderrWrapper.Write([]byte("err-1"))
		require.NoError(t, err)
		assert.Equal(t, len("err-1"), n)

		n, err = stdoutWrapper.Write([]byte("out-2"))
		require.NoError(t, err)
		assert.Equal(t, len("out-2"), n)

		assert.Equal(t, "out-1out-2", string(stdoutWrapper.GetBuffer()))
		assert.Equal(t, "err-1", string(stderrWrapper.GetBuffer()))
		assert.NoError(t, stdoutWrapper.GetWriteError())
		assert.NoError(t, stderrWrapper.GetWriteError())

		assert.Equal(t, []streamWrite{
			{stream: StdoutStream, data: "out-1"},
			{stream: StderrStream, data: "err-1"},
			{stream: StdoutStream, data: "out-2"},
		}, recorder.writes)
	})

	t.Run("get_write_error_returns_first_error", func(t *testing.T) {
		firstErr := errors.New("first write failure")
		secondErr := errors.New("second write failure")
		recorder := &streamRecorder{errs: []error{firstErr, secondErr}}
		wrapper := &outputWrapper{writer: recorder, stream: StdoutStream}

		n, err := wrapper.Write([]byte("a"))
		require.ErrorIs(t, err, firstErr)
		assert.Zero(t, n)

		n, err = wrapper.Write([]byte("b"))
		require.ErrorIs(t, err, secondErr)
		assert.Zero(t, n)

		assert.ErrorIs(t, wrapper.GetWriteError(), firstErr)
		assert.NotErrorIs(t, wrapper.GetWriteError(), secondErr)
	})
}
