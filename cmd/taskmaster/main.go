package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

var exitStatus int = 0
var logFile *os.File

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
}

func main() {
	defer logFile.Close()
	var cfg taskmaster.Config
	if err := cfg.Init(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	service := taskmaster.New(&cfg, taskmaster.WithOutputFile(logFile))
	service.AutoStart()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	t := term.New()
	t.AddCmd("config", "List config's tasks or information about a specific task.", newInfoCfgHandler(&cfg))
	t.AddCmd("reload", "Reload config file.", newReloadHandler(service))
	t.AddCmd("start", "Start a command.", newStartHandler(service))
	t.AddCmd("stop", "Stop a command.", newStopHandler(service))
	t.AddCmd("status", "foo", newStatusHandler(service))

	go func() {
		t.Run()
		if err := service.Close(); err != nil {
			log.Printf("Failed to close the service: %s\n", err)
		}
	}()

	// go readline(service.Cancel)
	go handleSignals(sigChan, service)

	<-service.Ctx.Done()
	err := context.Cause(service.Ctx)
	if err != nil {
		switch err {
		case taskmaster.ServiceClosed:
			// Nothing to do
		case TerminatedBySignal:
			fmt.Println()
		default:
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

		os.Exit(exitStatus)
	}
}

func handleSignals(sigChan chan os.Signal, service *taskmaster.Service) {
	for {
		select {
		case s := <-sigChan:
			switch s {
			case syscall.SIGINT:
				signal.Stop(sigChan)
				log.Printf("%s SIGINT", TerminatedBySignal.Error())
				service.Cancel(TerminatedBySignal)
				exitStatus = 130
			case syscall.SIGTERM:
				signal.Stop(sigChan)
				log.Printf("%s SIGTERM", TerminatedBySignal.Error())
				service.Cancel(TerminatedBySignal)
				exitStatus = 143
			case syscall.SIGHUP:
				if err := newReloadHandler(service)(); err != nil {
					log.Printf("Error: %v\n", err)
				}
			}
		}
	}
}
