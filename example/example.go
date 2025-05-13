package main

import (
	"fmt"
	"io"
	"net"
	"os"

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
			"port": 8080,
		}

		option := godaemonizer.DaemonizeOption{}

		// set emtpy stdio, stdout, stderr
		//option.UseNullIO()

		// pass the echo server config to the daemon process
		err = daemonizer.Daemonize(configMap, option)
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

func startEchoServer(config map[string]interface{}) {
	// Extract the host and port from the config.
	host := config["host"].(string)
	port := config["port"].(float64)

	address := fmt.Sprintf("%s:%d", host, int(port))
	network := "tcp"

	fmt.Printf("Starting echo server on %s://%s\n", network, address)

	// Listen for incoming connections on the specified network and address.
	listener, err := net.Listen(network, address)
	if err != nil {
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Server is listening...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		fmt.Printf("Accepted connection from %s\n", conn.RemoteAddr())
		go handleConnection(conn)
	}
}

// handleConnection handles a single client connection.
func handleConnection(conn net.Conn) {
	defer conn.Close()

	// echo
	io.Copy(conn, conn)
}
