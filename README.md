# Go-Daemonizer

`Go-Daemonizer` is a library (represented by the `Daemonizer` struct) that allows you to run your Go application as a background daemon process. It achieves this by creating a detached child process using the same application binary and can pass initial configuration to the child via pipes.

## How it Works

When you run an application that uses `Go-Daemonizer`, it performs the following steps:

1.  The initial process (the **parent**) checks if it is already running as a daemon (using `IsDaemon()`).
2.  If it's not a daemon, the parent process calls `Daemonize()`.
3.  `Daemonize()` internally executes the *same* application binary again, creating a new **child** process.
4.  The `Daemonizer` sets up a pipe to communicate with the child process.
5.  It passes the provided configuration data to the child via this pipe.
6.  It detaches the child process from the parent's controlling terminal and environment, making it a true daemon.
7.  The parent process then exits.
8.  The child process, upon starting, detects that it *is* the daemonized process (`IsDaemon()` returns true). It receives the configuration via the pipe and continues executing the main application logic in the background.

This method avoids the complexities of `fork()` in Go's runtime and provides a clear separation between the initial setup/daemonization phase and the long-running daemon phase.


```go
import (
	godaemonizer "github.com/cyverse/go-daemonizer"
)

func main() {
	// create a daemonizer
	daemonizer, err := godaemonizer.NewDaemonizer()
	if err != nil {
		panic(err)
	}

	if !daemonizer.IsDaemon() {
		// parent process
		// daemonize the process

		// echo server configuration
		configMap := map[string]interface{}{
			"host": "localhost",
			"port": 8080, // number type is treated as float64 in json
		}

		option := godaemonizer.DaemonizeOption{}

		// empty option inherits stdio, stdout, stderr, working dir, environment from parent process
		
		// to set null stdio, stdout, stderr, use UseNullIO
		//err := option.UseNullIO()
		//if err != nil {
		//	panic(err)
		//}

		// pass the echo server config to the daemon process
		err = daemonizer.Daemonize(config, option)
		if err != nil {
			panic(err)
		}

		fmt.Println("Daemonized")

		// exit the parent process
		// daemon process will continue to run
		return
	}

	// Daemon process
	// get the echo server config from the parent process
	configMap := daemonizer.GetParams()
	startEchoServer(configMap)
}
```

Checkout [example.go](./example/example.go) for full example code.

