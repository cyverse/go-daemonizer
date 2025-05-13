package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	// echo server configuration
	config := map[string]interface{}{
		"host": "localhost",
		"port": 8080,
	}

	// create a daemonizer
	daemonizer, err := NewDaemonizer()
	if err != nil {
		panic(err)
	}

	if !daemonizer.IsDaemon() {
		// parent process
		// daemonize the process
		option := DaemonizeOption{}

		// set emtpy stdio, stdout, stderr
		err := option.UseNullIO()
		if err != nil {
			panic(err)
		}

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
	startEchoServer(config)
}

func startEchoServer(config map[string]interface{}) {
	// Extract the host and port from the config.
	host := config["host"].(string)
	port := config["port"].(int)

	address := fmt.Sprintf("%s:%d", host, port)
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
