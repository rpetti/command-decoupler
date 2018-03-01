package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

var (
	pipe = "\\\\.\\pipe\\command-decoupler"
)

func pipeServer() {
	listener, err := winio.ListenPipe(pipe, &winio.PipeConfig{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for {
		fmt.Println("Listening for connections...")
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection.")
			break
		}
		fmt.Println("Connection accepted. Reading data...")
		reader := bufio.NewReader(conn)
		for {
			message, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			fmt.Printf("message: %s", message)
		}
	}
	fmt.Println("Done serving. Closing pipe listener.")
	listener.Close()
}

func main() {
	go pipeServer()
	time.Sleep(1 * time.Second)
	fmt.Println("Connecting to pipe...")
	timeout := 5 * time.Second
	conn, err := winio.DialPipe(pipe, &timeout)
	if err != nil {
		fmt.Printf("Could not connect to to pipe: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected. Writing data...")
	writer := bufio.NewWriter(conn)
	writer.WriteString("This is a test message.\n")
	writer.Flush()
	fmt.Println("Data written. Closing pipe...")
	conn.Close()
	fmt.Println("Done.")
	time.Sleep(1 * time.Second)
}
