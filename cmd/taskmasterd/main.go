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
	"syscall"

	"github.com/souhoc/taskmaster"
)

func init() {
	log.SetPrefix("[taskmasterd] ")
	log.SetOutput(os.Stdout)
}

func main() {
	var cfg taskmaster.Config
	if err := cfg.Init(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	service := taskmaster.New(&cfg)

	startConfigTasks(cfg, service)

	rpc.Register(service)

	lis, err := net.Listen("unix", taskmaster.SocketName)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Channel to listen for OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	defer func() {
		lis.Close()
		os.Remove(taskmaster.SocketName)
		signal.Stop(sigChan)
	}()

	go handleEvents(sigChan, service, lis)

	log.Printf("Server listening %s...\n", taskmaster.SocketName)
	handleConns(lis)
}

func startConfigTasks(cfg taskmaster.Config, service *taskmaster.Service) {
	// for taskName, task := range cfg.Tasks {
	// 	log.Printf("running %s...\n", taskName)
	// 	if err := service.Start(task); err != nil {
	// 		log.Printf("error running task: %v\n", err)
	// 	}
	// }
}

func handleEvents(sigChan chan os.Signal, service *taskmaster.Service, lis net.Listener) {
	for {
		select {
		case s := <-sigChan:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM:
				signal.Stop(sigChan)
				log.Printf("got %s, exiting...", s)
				os.Remove(taskmaster.SocketName)
				service.Cancel(nil)
				lis.Close()
				os.Exit(1)
			case syscall.SIGHUP:
				log.Printf("Got SIGHUP, reloading.")
				// TODO: reaload config
			}
		case <-service.Ctx.Done():
			log.Printf("Server shutting down: %v\n", context.Cause(service.Ctx))
			lis.Close()
			return
		}
	}
}

func handleConns(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			fmt.Printf("error: %s", err)
			os.Exit(1)
		}
		go rpc.ServeConn(conn)
	}
}
