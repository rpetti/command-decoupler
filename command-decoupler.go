package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

var (
	pipe = "\\\\.\\pipe\\command-decoupler"
)

func connectionHandler(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		fmt.Printf("s: Message Received: %s", message)
	}
	fmt.Println("s: Connection closed.")
}

func pipeServer() {
	listener, err := winio.ListenPipe(pipe, &winio.PipeConfig{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for {
		fmt.Println("s: Listening for connections...")
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("s: Error accepting connection.")
			break
		}
		fmt.Println("s: Connection accepted.")
		go connectionHandler(conn)
	}
	fmt.Println("s: Done serving. Closing pipe listener.")
	listener.Close()
}

func main() {
	go pipeServer()
	time.Sleep(1 * time.Second)
	fmt.Println("c: Connecting to pipe...")
	timeout := 5 * time.Second
	conn, err := winio.DialPipe(pipe, &timeout)
	if err != nil {
		fmt.Printf("c: Could not connect to to pipe: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("c: Connected. Writing data...")
	writer := bufio.NewWriter(conn)
	writer.WriteString("This is a test message.\n")
	writer.Flush()
	fmt.Println("c: Data written. Closing pipe...")
	conn.Close()
	fmt.Println("c: Done.")
	time.Sleep(1 * time.Second)
}
