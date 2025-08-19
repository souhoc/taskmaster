package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/util"
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
	log.SetPrefix("taskmasterd ")
}

func main() {
	defer logFile.Close()
	var cfg taskmaster.Config
	if err := cfg.Init(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	service := taskmaster.New(&cfg)
	rpcService := taskmaster.NewRPCService(service)

	if err := rpc.Register(rpcService); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	lis, err := net.Listen("unix", taskmaster.SocketName)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	defer os.Remove(taskmaster.SocketName)

	// Run service only if the server can listen
	spinner := util.NewSinner(nil)
	go spinner.Spin("Waiting AutoStart to complete...")
	service.AutoStart()()
	spinner.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	go handleEvents(sigChan, lis, service, done)

	fmt.Printf("Server listening %s...\n", taskmaster.SocketName)
	go handleConns(lis)

	<-done
	if err := service.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to close service properly: %v\n", err)
		os.Remove(taskmaster.SocketName)
		os.Exit(1)
	}

	switch err := context.Cause(service.Ctx); err {
	case taskmaster.ServiceClosed:
		// Nothing to do
	default:
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Remove(taskmaster.SocketName)
		os.Exit(1)
	}

	return
}

func handleEvents(sigChan chan os.Signal, lis net.Listener, service *taskmaster.Service, done chan<- struct{}) {
	defer signal.Stop(sigChan)
	for {
		select {
		case s := <-sigChan:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("got %s, exiting...", s)
				close(done)
				if err := lis.Close(); err != nil {
					log.Printf("Error while closing server: %v\n", err)
				}
				return
			case syscall.SIGHUP:
				log.Printf("Got SIGHUP, reloading...")
				if changed, err := service.ReloadConfig(); err != nil {
					log.Printf("Error: couldn't reload config: %s\n", err)
				} else {
					log.Printf("config reloaded? %t\n", changed)
				}
			}
		}
	}
}

func handleConns(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				fmt.Println("Server closed")
				return
			}
			fmt.Printf("Error while accepting a connection: %s", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
