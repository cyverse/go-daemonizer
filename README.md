# Go-Daemonizer

`Go-Daemonizer` is a Go library that runs your application as a background daemon process. It re-executes the same binary as a detached child process, passes configuration via pipes, and reports back whether the daemon started successfully — all without requiring `fork()`.

## How it Works

1. The parent process calls `Daemonize()` with a JSON-serializable params struct.
2. `Daemonize()` re-executes the same binary with an internal flag, creating a child process in a new session (`setsid`).
3. Params are sent to the child via a pipe (fd 3). The child deserializes them into the user-provided struct.
4. The child performs initialization (e.g., binding a port) and signals readiness (or failure) back to the parent via a status pipe (fd 4).
5. The parent receives the status and returns — success or error.
6. The child continues running as a daemon.

This avoids the complexities of `fork()` in Go's multi-threaded runtime and gives the parent reliable feedback on whether the daemon started successfully.

## Usage

```go
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	godaemonizer "github.com/cyverse/go-daemonizer"
)

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func main() {
	d := godaemonizer.New()

	if !d.IsDaemon() {
		// Parent process
		cfg := &ServerConfig{Host: "localhost", Port: 8080}

		err := d.Daemonize(context.Background(), cfg, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to daemonize: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("daemon started successfully")
		return
	}

	// Daemon process
	var cfg ServerConfig
	ready, err := d.WaitForParent(&cfg)
	if err != nil {
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		ready(err) // report failure to parent
		os.Exit(1)
	}

	ready(nil) // report success to parent

	// Run server...
	for {
		conn, _ := listener.Accept()
		go func() { /* handle conn */ }()
	}
}
```

## API

### `New() *Daemon`

Creates a new Daemon instance. Detects whether the current process is the parent or the daemon based on an internal command-line flag.

### `(*Daemon) IsDaemon() bool`

Returns true if the current process is the daemon (child) process.

### `(*Daemon) Daemonize(ctx context.Context, params any, cfg *Config) error`

Called by the parent. Launches the daemon process, sends params, and waits for readiness. The `params` value must be JSON-serializable. The `cfg` argument controls the daemon's working directory, environment, and stdio (nil uses sensible defaults).

### `(*Daemon) WaitForParent(dest any) (ready func(error), err error)`

Called by the daemon. Receives params from the parent and deserializes them into `dest` (must be a pointer). Returns a `ready` callback — call `ready(nil)` on success or `ready(err)` on failure to notify the parent.

### `Config`

```go
type Config struct {
	Dir    string   // working directory (empty = inherit)
	Env    []string // environment variables (nil = inherit)
	Stdin  *os.File // nil = /dev/null
	Stdout *os.File // nil = /dev/null
	Stderr *os.File // nil = /dev/null
}
```

## Example

See [example/example.go](./example/example.go) for a complete echo server example.
