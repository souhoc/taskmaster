package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

var (
	logFile *os.File
)

func init() {
	dir, err := os.UserCacheDir()
	if err == nil {
		dir = filepath.Join(dir, "taskmaster")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Printf("can't create user cache dir: %v", err)
	}

	file := filepath.Join(dir, "taskmaster.log")
	logFile, err = os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Printf("can't create user log file: %v", err)
		os.Exit(1)
	}
	log.SetOutput(logFile)
	if os.Getenv("DEBUG") == "true" {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.SetPrefix("taskmasterctl ")
}

func main() {
	client, err := rpc.Dial("unix", taskmaster.SocketName)
	if err != nil {
		opErr, ok := err.(*net.OpError)
		if ok && os.IsNotExist(opErr.Unwrap()) {
			fmt.Fprintln(os.Stderr, "Error: no server seems to run")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error dialing socket: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	t := term.New()
	handler := Handler{client: client, terminal: t}
	handler.SetTerminal()

	go handleSignals(sigChan, t)

	t.Run()
}

func handleSignals(sigChan chan os.Signal, program *term.Term) {
	defer signal.Stop(sigChan)
	for {
		select {
		case s := <-sigChan:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("Terminated by signal: %s\n", s)
				program.Stop()
				return
			}
		}
	}
}
