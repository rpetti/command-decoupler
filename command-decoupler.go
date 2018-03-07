package main

import (
	"bufio"
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

var (
	pipe              = "\\\\.\\pipe\\command-decoupler"
	decoupledCommands decoupledCommandsFlag
)

const (
	// STDOUT enum to specify output type
	STDOUT = 1
	// STDERR enum to specify output type
	STDERR = 2
)

type decoupledCommandsFlag []string

func (i *decoupledCommandsFlag) String() string {
	return strings.Join(*i, ",")
}

func (i *decoupledCommandsFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func consumeOutput(channel chan commandResponseLine, reader io.Reader, outputType int) {
	bufferedReader := bufio.NewReader(reader)
	for {
		bytes, _, err := bufferedReader.ReadLine()
		if err != nil {
			break
		}
		channel <- commandResponseLine{
			Text:       string(bytes),
			OutputType: outputType,
			Done:       false,
		}
	}
	channel <- commandResponseLine{
		OutputType: outputType,
		Done:       true,
	}
}

func runCmd(channel chan commandResponseLine, cmd *exec.Cmd) {
	err := cmd.Run()
	exitCode := 0
	text := ""
	if err != nil {
		exitCode = 1
		text = err.Error()
	}
	channel <- commandResponseLine{
		Done:       true,
		ReturnCode: exitCode,
		Text:       text,
		OutputType: STDERR,
	}
}

// connectonHandler: Reads the command to execute, runs it, and writes the results back to the client
func connectionHandler(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	gobReader := gob.NewDecoder(reader)

	var cr commandRequest
	err := gobReader.Decode(&cr)
	if err != nil {
		log.Fatal(err)
	}
	cmd := exec.Command(cr.Command, cr.Args...)
	cmd.Env = os.Environ()
	cmd.Dir = cr.WorkingDir
	channel := make(chan commandResponseLine)
	outpipe, _ := cmd.StdoutPipe()
	errpipe, _ := cmd.StderrPipe()
	go consumeOutput(channel, outpipe, STDOUT)
	go consumeOutput(channel, errpipe, STDERR)
	go runCmd(channel, cmd)

	writer := bufio.NewWriter(conn)
	gobWriter := gob.NewEncoder(writer)
	doneCount := 0
	returnCode := 0
	for {
		cl := <-channel
		if cl.Done {
			doneCount++
			returnCode += cl.ReturnCode
		} else {
			gobWriter.Encode(cl)
		}
		if doneCount == 3 {
			break
		}
	}
	gobWriter.Encode(commandResponseLine{
		Done:       true,
		ReturnCode: returnCode,
	})
	writer.Flush()
}

// pipeServer: server listener for named pipe
func pipeServer(comms chan string) {
	defer close(comms)
	listener, err := winio.ListenPipe(pipe, &winio.PipeConfig{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer listener.Close()
	for {
		comms <- "ready"
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection")
			break
		}
		go connectionHandler(conn)
	}
}

type commandRequest struct {
	Command    string
	Args       []string
	WorkingDir string
}

type commandResponseLine struct {
	Text       string
	OutputType int
	ReturnCode int
	Done       bool
}

// client: masquerades as an executed sub-command, and asks the server to run the actual command instead
func client() {
	// Connect to server
	timeout := 5 * time.Second
	log.Println("Command decoupled: ", strings.Join(os.Args, " "))
	conn, err := winio.DialPipe(pipe, &timeout)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Write the command request
	writer := bufio.NewWriter(conn)
	gobWriter := gob.NewEncoder(writer)
	dir, _ := os.Getwd()
	gobWriter.Encode(commandRequest{
		Command:    os.Args[0],
		Args:       os.Args[1:],
		WorkingDir: dir,
	})
	writer.Flush()

	// Read and print the command results
	reader := bufio.NewReader(conn)
	gobReader := gob.NewDecoder(reader)
	for {
		var cr commandResponseLine
		err := gobReader.Decode(&cr)
		if err != nil {
			break
		}
		fmt.Println(cr.Text)
		if cr.Done {
			os.Exit(cr.ReturnCode)
		}
	}
}

func copyFile(from string, to string) error {
	f, err := os.Open(from)
	defer f.Close()
	if err != nil {
		return err
	}
	t, err := os.OpenFile(to, os.O_RDWR|os.O_CREATE, 0777)
	defer t.Close()
	if err != nil {
		return err
	}
	_, err = io.Copy(t, f)
	if err != nil {
		return err
	}
	return nil
}

func cleanup(tempDir string, execPath string) {
	// Retry because windows file locking is garbage
	for {
		os.RemoveAll(tempDir)
		_, err := os.Stat(tempDir)
		if os.IsNotExist(err) {
			break
		}
		time.Sleep(time.Second)
	}
	for _, decoupledCmd := range decoupledCommands {
		decoupledCmd := filepath.Join(execPath, fmt.Sprintf("%s.exe", strings.TrimSuffix(decoupledCmd, filepath.Ext(decoupledCmd))))
		for {
			os.Remove(decoupledCmd)
			_, err := os.Stat(decoupledCmd)
			if os.IsNotExist(err) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func myUsage() {
	fmt.Printf("Usage: %s [OPTIONS] <command> <args>...\n", os.Args[0])
	flag.PrintDefaults()
}

func decoupler() {
	flag.Usage = myUsage
	tempDir := filepath.Join(os.Getenv("TEMP"), fmt.Sprintf("command-decoupler-%d", os.Getpid()))
	execPath := flag.String("path", "", "Path to store executables to intercept calls to commands defined by -cmd")
	flag.Var(&decoupledCommands, "cmd", "Command that needs to be decoupled. Can be specified multiple times.")
	flag.Parse()
	if *execPath == "" {
		*execPath = tempDir
	}
	absPath, err := filepath.Abs(*execPath)
	if err == nil {
		*execPath = absPath
	}
	if len(decoupledCommands) == 0 || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// Create temp directory and copy command hooks
	err = os.Mkdir(tempDir, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup(tempDir, *execPath)
	executable, _ := os.Executable()
	for _, decoupledCmd := range decoupledCommands {
		decoupledCmd = strings.TrimSuffix(decoupledCmd, filepath.Ext(decoupledCmd))
		err := copyFile(executable, filepath.Join(*execPath, fmt.Sprintf("%s.exe", decoupledCmd)))
		if err != nil {
			log.Fatal(err)
		}
	}

	// Start server
	serverComms := make(chan string)
	go pipeServer(serverComms)
	if <-serverComms == "ready" {
		log.Printf("Decoupler server ready")
	}

	// Execute wrapped command with tempDir in path
	cmd := exec.Command(flag.Args()[0], flag.Args()[1:]...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PATH=%s;%s", *execPath, os.Getenv("PATH")))
	log.Printf("Running: %s", strings.Join(cmd.Args, " "))
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	executable, _ := os.Executable()
	if strings.HasPrefix(filepath.Base(executable), "command-decoupler") {
		// We're the server/wrapper
		decoupler()
	} else {
		// We're the masquerading command
		client()
	}
}
