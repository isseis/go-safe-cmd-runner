package executor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// ErrOutputPipe is returned when the parent cannot create the stdout/stderr
// pipes the child writes into.
var ErrOutputPipe = errors.New("failed to create output pipe")

// omissionMarkerCapacity bounds the rendered size of the omission marker
// ("\n... omitting <n> bytes ...\n") for any value of n, so the buffer can
// be grown once instead of per write.
const omissionMarkerCapacity = 50

// pipeFn creates the stdout/stderr pipes. It is a variable so a test can
// replace it and exercise the pipe-creation failure path.
var pipeFn = os.Pipe

// outputPump hands the pipe write ends to exec.Cmd so os/exec starts no copy
// goroutine of its own, then reads the read ends itself once the start phase
// has returned. The two reading goroutines join through buffered channels,
// not a WaitGroup, so the pump declares no synchronization primitive.
type outputPump struct {
	stdout  *pumpStream
	stderr  *pumpStream
	started bool // guards start against a second call; see start
}

// pumpStream is one direction: the pipe pair plus the wrapper that buffers
// the bytes and forwards them to the OutputWriter.
type pumpStream struct {
	childEnd  *os.File // handed to exec.Cmd; closed by releaseChildEnds
	parentEnd *os.File // read by the pump goroutine, which closes it on exit
	wrapper   *outputWrapper
	done      chan error
}

// newOutputPump creates the pipes and wrappers the pump owns. Both wrappers
// retain at most retainedOutputLimit bytes in memory; writer, when non-nil,
// still receives every byte.
func newOutputPump(writer OutputWriter) (*outputPump, error) {
	stdoutRead, stdoutWrite, err := pipeFn()
	if err != nil {
		return nil, fmt.Errorf("%w: stdout: %w", ErrOutputPipe, err)
	}
	stderrRead, stderrWrite, err := pipeFn()
	if err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		return nil, fmt.Errorf("%w: stderr: %w", ErrOutputPipe, err)
	}
	return &outputPump{
		stdout: &pumpStream{
			childEnd:  stdoutWrite,
			parentEnd: stdoutRead,
			wrapper:   newOutputWrapper(writer, StdoutStream, retainedOutputLimit),
			done:      make(chan error, 1),
		},
		stderr: &pumpStream{
			childEnd:  stderrWrite,
			parentEnd: stderrRead,
			wrapper:   newOutputWrapper(writer, StderrStream, retainedOutputLimit),
			done:      make(chan error, 1),
		},
	}, nil
}

// childFiles returns the pipe write ends to hand to exec.Cmd as Stdout and
// Stderr. Passing *os.File rather than an io.Writer is what keeps os/exec
// from starting a copy goroutine per stream.
func (p *outputPump) childFiles() (stdout, stderr *os.File) {
	return p.stdout.childEnd, p.stderr.childEnd
}

// releaseChildEnds closes the pipe write ends that were handed to the child.
// It must run immediately after the start phase returns -- success or
// failure -- or the read ends never reach EOF and wait blocks until its
// deadline. Idempotent, so release may follow it.
func (p *outputPump) releaseChildEnds() error {
	return errors.Join(
		closeUnlessClosed(p.stdout.childEnd),
		closeUnlessClosed(p.stderr.childEnd),
	)
}

// start launches one reader goroutine per stream.
//
// It must not be called before the start window has closed: the readers run
// at the process's current effective UID, and every OutputWriter.Write they
// perform runs there too. Its one caller, superviseCommand, runs after that
// window has closed, which is what makes it a fact rather than only a
// requirement on the caller.
//
// That covers the start window only. The two windows superviseCommand may
// open afterwards -- the kill window and the staging cleanup window -- do
// open with these readers live, so a Write that lands in one of them runs at
// euid 0. That overlap is a residual risk the design accepts and bounds
// (02_architecture.md section 5.3): both windows are exceptional, and neither
// exists on the path a command that runs to completion takes.
//
// Calling start twice would leave a reader per stream blocked forever on
// the send to done, which has room for one value: a silent goroutine leak,
// since the read ends are closed by then and nothing reports the second
// reader's outcome. The pump has one call site today, so a second call is a
// programming error and is rejected as one.
func (p *outputPump) start() {
	if p.started {
		panic("outputPump.start called twice: the second reader per stream would block on done forever")
	}
	p.started = true
	go p.stdout.run()
	go p.stderr.run()
}

// wait collects the captured output and the streams' write errors. It blocks
// until both reader goroutines have finished, or until the deadline passes,
// whichever comes first.
//
// deadline == 0 means no limit: the normal run uses it. A non-zero deadline
// bounds the collection after a kill, where a grandchild may hold a pipe
// write end open.
//
// A stream whose reader has not finished by the deadline is reported as nil:
// its goroutine may still be inside outputWrapper.Write, so its buffer and
// write error must not be read here. Only a stream whose done channel has
// yielded a value is read. The stdout stream's write error takes precedence
// over stderr's, matching the order the pre-pump implementation checked
// them in.
//
//nolint:revive // the error is not last because timedOut qualifies it: it is the flag that says the streams' values are nil, not errors
func (p *outputPump) wait(deadline time.Duration) (stdout, stderr []byte, writeErr error, timedOut bool) {
	var timeout <-chan time.Time
	if deadline > 0 {
		timer := time.NewTimer(deadline)
		defer timer.Stop()
		timeout = timer.C
	}

	var stdoutErr, stderrErr error
	stdoutFinished, stderrFinished := false, false
loop:
	for !stdoutFinished || !stderrFinished {
		select {
		case stdoutErr = <-p.stdout.done:
			stdoutFinished = true
		case stderrErr = <-p.stderr.done:
			stderrFinished = true
		case <-timeout:
			timedOut = true
			break loop
		}
	}

	if stdoutFinished {
		stdout = p.stdout.wrapper.GetBuffer()
		writeErr = stdoutErr
	}
	if stderrFinished {
		stderr = p.stderr.wrapper.GetBuffer()
		if writeErr == nil {
			writeErr = stderrErr
		}
	}
	return stdout, stderr, writeErr, timedOut
}

// closeReadEnds closes the pipe read ends, which stops the reader goroutines
// waiting on a write end nobody left will close. Idempotent: the readers close
// their own read ends on the normal path.
func (p *outputPump) closeReadEnds() error {
	return errors.Join(
		closeUnlessClosed(p.stdout.parentEnd),
		closeUnlessClosed(p.stderr.parentEnd),
	)
}

// release closes every descriptor the pump holds: the pipe write ends (via
// releaseChildEnds) and the read ends. The reader goroutines close their own
// read ends, so on the normal path this closes only the write ends again;
// closing an already-closed file is not an error, which makes release safe
// on paths where start was never reached and after wait alike.
func (p *outputPump) release() error {
	return errors.Join(p.releaseChildEnds(), p.closeReadEnds())
}

// run reads the pipe read end into the wrapper until the last write end
// closes (EOF) or a read error occurs, closes the read end, then signals
// done. Closing the read end on every exit -- including a write error from
// the OutputWriter -- is what makes the child receive SIGPIPE when the
// output size limit is exceeded.
func (s *pumpStream) run() {
	// A read error other than EOF is not distinguishable from the child
	// dying with the pipe broken; the run's outcome is the wrapper's write
	// error, so the read error is dropped.
	_, _ = io.Copy(s.wrapper, s.parentEnd)
	// Closing again is harmless: closeReadEnds may have closed this end
	// already to unblock the io.Copy above (that is how an abandoned child's
	// drain is cut short), and a close failure cannot be reported to the
	// caller, so it is dropped.
	_ = s.parentEnd.Close()
	s.done <- s.wrapper.GetWriteError()
}

// closeUnlessClosed closes f unless it is already closed, which release and
// releaseChildEnds both treat as their idempotent success case.
func closeUnlessClosed(f *os.File) error {
	err := f.Close()
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

// boundedBuffer retains the leading limit bytes written to it (the leading
// window) and counts the rest without keeping them. A limit of 0 disables the
// bound and the type degenerates to bytes.Buffer; production always passes
// retainedOutputLimit. limit must be non-negative.
//
// Contract of Bytes: without an overflow it returns exactly what was written;
// after an overflow it returns only complete lines -- the leading window cut
// back to its last newline -- followed by the omission marker, whose count
// is every written byte not returned. Never returning a partial line is what
// keeps redaction sound on the retained text: a secret's marker (key=,
// Bearer, a PEM BEGIN line) precedes its value, so a kept value keeps its
// marker, and a value is never cut into a fragment redaction cannot
// recognize.
//
// Write never fails and never signals the limit to its caller: reaching the
// bound must not stop the reader draining the stream, since a command that
// writes past the bound and then exits successfully must keep succeeding.
// Stopping the child on overflow is a different mechanism, applied
// elsewhere, not by this type.
type boundedBuffer struct {
	limit     int          // 0 = unbounded
	unbounded bytes.Buffer // collects every byte when limit == 0
	prefix    []byte       // the leading window: the first limit bytes
	skipped   int64        // bytes written after the leading window filled
}

// newBoundedBuffer builds a buffer retaining the first limit bytes; 0 means
// unbounded. A negative limit is a programming error and is rejected here
// rather than surfacing as a slice bound panic inside Write.
func newBoundedBuffer(limit int) *boundedBuffer {
	if limit < 0 {
		panic(fmt.Sprintf("newBoundedBuffer: limit must not be negative, got %d", limit))
	}
	return &boundedBuffer{limit: limit}
}

// Write appends p to the buffer and never returns an error.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit == 0 {
		_, _ = b.unbounded.Write(p) // bytes.Buffer.Write never fails
		return len(p), nil
	}
	add := min(len(p), b.limit-len(b.prefix))
	b.prefix = append(b.prefix, p[:add]...)
	b.skipped += int64(len(p) - add)
	return len(p), nil
}

// Bytes returns the retained output as described in the type's contract.
// The marker is "\n... omitting N bytes ...\n"; with no newline in the
// leading window it is all that is returned.
func (b *boundedBuffer) Bytes() []byte {
	if b.limit == 0 {
		return b.unbounded.Bytes()
	}
	if b.skipped == 0 {
		return b.prefix
	}
	kept := bytes.LastIndexByte(b.prefix, '\n') + 1 // 0 when there is no newline
	omitted := b.skipped + int64(len(b.prefix)-kept)
	var buf bytes.Buffer
	buf.Grow(kept + omissionMarkerCapacity)
	buf.Write(b.prefix[:kept])
	buf.WriteString("\n... omitting ")
	buf.WriteString(strconv.FormatInt(omitted, 10))
	buf.WriteString(" bytes ...\n")
	return buf.Bytes()
}
