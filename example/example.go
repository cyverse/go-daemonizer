package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	godaemonizer "github.com/cyverse/go-daemonizer"
)

type EchoServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func main() {
	d := godaemonizer.New()

	if !d.IsDaemon() {
		cfg := &EchoServerConfig{
			Host: "localhost",
			Port: 8080,
		}

		err := d.Daemonize(context.Background(), cfg, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to daemonize: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("daemon started successfully")
		return
	}

	// daemon process
	var cfg EchoServerConfig
	ready, err := d.WaitForParent(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to receive params: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		ready(err)
		os.Exit(1)
	}

	// signal parent that we're ready
	ready(nil)

	fmt.Printf("echo server listening on %s\n", addr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go io.Copy(conn, conn)
	}
}
