package daemonizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const daemonFlag = "--__daemon__"

var (
	ErrAlreadyDaemon = errors.New("already running as daemon")
	ErrDaemonFailed  = errors.New("daemon process failed to start")
)

type Config struct {
	Dir    string
	Env    []string
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

type Daemon struct {
	args     []string
	isDaemon bool
}

func New() *Daemon {
	d := &Daemon{}
	for _, arg := range os.Args {
		if arg == daemonFlag {
			d.isDaemon = true
		} else {
			d.args = append(d.args, arg)
		}
	}
	if d.isDaemon {
		os.Args = d.args
	}
	return d
}

func (d *Daemon) IsDaemon() bool {
	return d.isDaemon
}

func (d *Daemon) Args() []string {
	return d.args
}

// Daemonize launches the daemon process and waits for it to report readiness.
// params must be JSON-serializable (e.g., a struct with json tags).
// Called by the parent process.
func (d *Daemon) Daemonize(ctx context.Context, params any, cfg *Config) error {
	if d.isDaemon {
		return ErrAlreadyDaemon
	}

	paramR, paramW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create param pipe: %w", err)
	}

	statusR, statusW, err := os.Pipe()
	if err != nil {
		paramR.Close()
		paramW.Close()
		return fmt.Errorf("create status pipe: %w", err)
	}

	cmd := exec.CommandContext(ctx, d.args[0], append([]string{daemonFlag}, d.args[1:]...)...)
	cmd.ExtraFiles = []*os.File{paramR, statusW}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if cfg != nil {
		cmd.Dir = cfg.Dir
		cmd.Env = cfg.Env
		cmd.Stdin = cfg.Stdin
		cmd.Stdout = cfg.Stdout
		cmd.Stderr = cfg.Stderr
	}

	if err := cmd.Start(); err != nil {
		paramR.Close()
		paramW.Close()
		statusR.Close()
		statusW.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	// close child-side ends now that the child has inherited them
	paramR.Close()
	statusW.Close()

	// send params
	if err := json.NewEncoder(paramW).Encode(params); err != nil {
		paramW.Close()
		statusR.Close()
		cmd.Process.Release()
		return fmt.Errorf("send params: %w", err)
	}
	paramW.Close()

	// wait for daemon to report status
	var status struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}

	if err := json.NewDecoder(statusR).Decode(&status); err != nil {
		statusR.Close()
		cmd.Process.Release()
		return fmt.Errorf("read daemon status: %w", err)
	}
	statusR.Close()

	cmd.Process.Release()

	if !status.OK {
		return fmt.Errorf("%w: %s", ErrDaemonFailed, status.Error)
	}
	return nil
}

// WaitForParent receives params from the parent process and deserializes into dest.
// dest must be a pointer to the type that was passed to Start.
// The returned function should be called to signal readiness (nil) or failure (error).
func (d *Daemon) WaitForParent(dest any) (ready func(error), err error) {
	if !d.isDaemon {
		return nil, errors.New("not a daemon process")
	}

	paramR := os.NewFile(3, "param_pipe")
	statusW := os.NewFile(4, "status_pipe")

	if err := json.NewDecoder(paramR).Decode(dest); err != nil {
		paramR.Close()
		statusW.Close()
		return nil, fmt.Errorf("read params: %w", err)
	}
	paramR.Close()

	called := false
	ready = func(initErr error) {
		if called {
			return
		}
		called = true
		defer statusW.Close()

		status := struct {
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		}{OK: initErr == nil}

		if initErr != nil {
			status.Error = initErr.Error()
		}
		json.NewEncoder(statusW).Encode(status)
	}

	return ready, nil
}
