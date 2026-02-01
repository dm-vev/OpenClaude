package main

import (
	"sync"
	"time"

	"github.com/openclaude/openclaude/internal/streamjson"
)

// keepAliveEmitter emits keep_alive events on a fixed interval.
type keepAliveEmitter struct {
	// writer emits stream-json events to the output stream.
	writer *streamjson.Writer
	// interval controls how frequently keep_alive events are sent.
	interval time.Duration
	// stopOnce ensures the stop signal is only closed once.
	stopOnce sync.Once
	// stopCh signals the goroutine to stop emitting events.
	stopCh chan struct{}
	// doneCh reports when the goroutine has exited.
	doneCh chan struct{}
	// errMu guards access to the first error observed.
	errMu sync.Mutex
	// err stores the first error encountered while writing.
	err error
}

// startKeepAlive begins emitting keep_alive events until Stop is called.
func startKeepAlive(writer *streamjson.Writer, interval time.Duration) *keepAliveEmitter {
	// No writer or interval means keep-alives are disabled.
	if writer == nil || interval <= 0 {
		return nil
	}

	emitter := &keepAliveEmitter{
		writer:   writer,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}

	go emitter.loop()
	return emitter
}

// loop emits keep_alive events on a ticker until Stop is called.
func (e *keepAliveEmitter) loop() {
	// Ensure done is closed exactly once when the loop exits.
	defer close(e.doneCh)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Emit the keep_alive heartbeat event.
			if err := e.writer.Write(streamjson.KeepAliveEvent{Type: "keep_alive"}); err != nil {
				// Record the error and exit without calling Stop to avoid self-deadlock.
				e.setErr(err)
				return
			}
		case <-e.stopCh:
			return
		}
	}
}

// Stop stops the keep-alive goroutine and returns the first write error, if any.
func (e *keepAliveEmitter) Stop() error {
	if e == nil {
		return nil
	}

	e.stopOnce.Do(func() {
		close(e.stopCh)
	})
	<-e.doneCh

	e.errMu.Lock()
	defer e.errMu.Unlock()
	return e.err
}

// setErr stores the first error encountered by the emitter.
func (e *keepAliveEmitter) setErr(err error) {
	if err == nil {
		return
	}
	e.errMu.Lock()
	defer e.errMu.Unlock()
	if e.err == nil {
		e.err = err
	}
}
