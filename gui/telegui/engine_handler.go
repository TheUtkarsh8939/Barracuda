package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	defaultEnginePath  = "./Barracuda-V7.exe"
	shutdownGraceDelay = 2 * time.Second
	scannerBufferBytes = 1024 * 1024
)

type EngineRunner struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	writeMu sync.Mutex

	handlerMu sync.RWMutex
	onLine    func(string)

	waitErrMu sync.Mutex
	waitErr   error

	done     chan struct{}
	doneOnce sync.Once
}

// StartEngine starts Barracuda with the default path used by telegui.
func StartEngine() (*EngineRunner, error) {
	return StartEngineAtPath(defaultEnginePath)
}

// StartEngineAtPath starts Barracuda from a custom path.
func StartEngineAtPath(binaryPath string, args ...string) (*EngineRunner, error) {
	cmd := exec.Command(binaryPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open engine stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open engine stdout: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open engine stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start engine process: %w", err)
	}

	runner := &EngineRunner{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
	}

	runner.startReadLoop(runner.stdout, "")
	runner.startReadLoop(runner.stderr, "stderr: ")

	go func() {
		err := runner.cmd.Wait()
		runner.setWaitErr(err)
		runner.closeDone()
	}()

	return runner, nil
}

// SetLineHandler registers a callback that receives each stdout/stderr line.
func (e *EngineRunner) SetLineHandler(handler func(string)) {
	e.handlerMu.Lock()
	e.onLine = handler
	e.handlerMu.Unlock()
}

// Send writes one UCI command line to engine stdin.
func (e *EngineRunner) Send(command string) error {
	if e == nil {
		return errors.New("engine runner is nil")
	}
	if e.stdin == nil {
		return errors.New("engine stdin is not available")
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	_, err := io.WriteString(e.stdin, command+"\n")
	if err != nil {
		return fmt.Errorf("failed to write command to engine: %w", err)
	}
	return nil
}

// Close attempts graceful shutdown via "quit", then kills if needed.
func (e *EngineRunner) Close() error {
	if e == nil {
		return nil
	}

	_ = e.Send("quit")

	e.writeMu.Lock()
	if e.stdin != nil {
		_ = e.stdin.Close()
	}
	e.writeMu.Unlock()

	select {
	case <-e.done:
		return e.getWaitErr()
	case <-time.After(shutdownGraceDelay):
		if e.cmd != nil && e.cmd.Process != nil {
			_ = e.cmd.Process.Kill()
		}
		<-e.done
		return e.getWaitErr()
	}
}

// Done exposes a channel closed when process exits.
func (e *EngineRunner) Done() <-chan struct{} {
	if e == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return e.done
}

func (e *EngineRunner) startReadLoop(pipe io.ReadCloser, prefix string) {
	go func() {
		scanner := bufio.NewScanner(pipe)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, scannerBufferBytes)

		for scanner.Scan() {
			e.emitLine(prefix + scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			e.emitLine(prefix + "read error: " + err.Error())
		}
	}()
}

func (e *EngineRunner) emitLine(line string) {
	e.handlerMu.RLock()
	handler := e.onLine
	e.handlerMu.RUnlock()
	if handler != nil {
		handler(line)
	}
}

func (e *EngineRunner) setWaitErr(err error) {
	e.waitErrMu.Lock()
	e.waitErr = err
	e.waitErrMu.Unlock()
}

func (e *EngineRunner) getWaitErr() error {
	e.waitErrMu.Lock()
	defer e.waitErrMu.Unlock()
	return e.waitErr
}

func (e *EngineRunner) closeDone() {
	e.doneOnce.Do(func() {
		close(e.done)
	})
}
