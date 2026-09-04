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

// newOutputPump creates the pipes and wrappers the pump owns. stderrLimit
// bounds how many bytes the stderr wrapper retains in memory; 0 means
// unbounded. The stdout wrapper is always unbounded, as it was before the
// pump existed.
func newOutputPump(writer OutputWriter, stderrLimit int) (*outputPump, error) {
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
			wrapper:   newOutputWrapper(writer, StdoutStream, 0),
			done:      make(chan error, 1),
		},
		stderr: &pumpStream{
			childEnd:  stderrWrite,
			parentEnd: stderrRead,
			wrapper:   newOutputWrapper(writer, StderrStream, stderrLimit),
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
// It must not be called before the privilege window has closed: the readers
// run at the process's current effective UID, and every OutputWriter.Write
// they perform runs there too. That is not yet true of this package:
// executeWithUserGroup still wraps prepareCommand, startPrepared and
// superviseCommand together in PrivMgr.WithPrivileges, so for a run-as
// command the readers do run at euid 0. Narrowing the window to the start
// phase is what makes the sentence above a fact rather than a requirement on
// the caller.
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

// release closes every descriptor the pump holds: the pipe write ends (via
// releaseChildEnds) and the read ends. The reader goroutines close their own
// read ends, so on the normal path this closes only the write ends again;
// closing an already-closed file is not an error, which makes release safe
// on paths where start was never reached and after wait alike.
func (p *outputPump) release() error {
	err := p.releaseChildEnds()
	err = errors.Join(err, closeUnlessClosed(p.stdout.parentEnd), closeUnlessClosed(p.stderr.parentEnd))
	return err
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
	// The read end is ours alone from here on; a close failure cannot be
	// reported to the caller, so it is dropped.
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

// boundedBuffer reproduces the rule os/exec's unexported prefixSuffixSaver
// applies to Cmd.Output's stderr, so callers with no OutputWriter keep
// seeing the same output as today. A limit of 0 disables the bound and the
// type degenerates to bytes.Buffer. limit must be non-negative.
//
// Write never fails and never signals the limit to its caller: reaching the
// bound must not stop the reader draining stderr, since a command that
// writes past the bound and then exits successfully must keep succeeding.
// Stopping the child on overflow is a different mechanism, applied
// elsewhere, not by this type.
type boundedBuffer struct {
	limit     int          // 0 = unbounded
	unbounded bytes.Buffer // collects every byte when limit == 0
	prefix    []byte       // first limit bytes
	suffix    []byte       // ring buffer holding the last limit bytes
	suffixW   int          // write position in suffix
	skipped   int64        // bytes dropped between prefix and suffix
}

// newBoundedBuffer builds a buffer bounded to limit bytes of prefix and
// limit bytes of suffix; 0 means unbounded. A negative limit is a
// programming error: Write would slice p past its length on the first call
// larger than the limit, so it is rejected here rather than reached there.
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
	lenp := len(p)
	p = b.fill(&b.prefix, p)
	if overage := len(p) - b.limit; overage > 0 {
		p = p[overage:]
		b.skipped += int64(overage)
	}
	p = b.fill(&b.suffix, p)
	// The suffix is full now if p is non-empty; overwrite it in a circle.
	for len(p) > 0 {
		n := copy(b.suffix[b.suffixW:], p)
		p = p[n:]
		b.skipped += int64(n)
		b.suffixW += n
		if b.suffixW == b.limit {
			b.suffixW = 0
		}
	}
	return lenp, nil
}

// fill appends up to len(p) bytes of p to *dst, such that *dst does not grow
// larger than the limit. It returns the un-appended suffix of p.
func (b *boundedBuffer) fill(dst *[]byte, p []byte) []byte {
	if remain := b.limit - len(*dst); remain > 0 {
		add := min(len(p), remain)
		*dst = append(*dst, p[:add]...)
		p = p[add:]
	}
	return p
}

// Bytes returns prefix + omission marker + suffix, in the same shape
// os/exec reports the retained stderr of a failed Cmd.Output.
func (b *boundedBuffer) Bytes() []byte {
	if b.limit == 0 {
		return b.unbounded.Bytes()
	}
	if b.suffix == nil {
		return b.prefix
	}
	if b.skipped == 0 {
		return append(b.prefix, b.suffix...)
	}
	var buf bytes.Buffer
	buf.Grow(len(b.prefix) + len(b.suffix) + omissionMarkerCapacity)
	buf.Write(b.prefix)
	buf.WriteString("\n... omitting ")
	buf.WriteString(strconv.FormatInt(b.skipped, 10))
	buf.WriteString(" bytes ...\n")
	buf.Write(b.suffix[b.suffixW:])
	buf.Write(b.suffix[:b.suffixW])
	return buf.Bytes()
}
