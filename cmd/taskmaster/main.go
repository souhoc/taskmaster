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
	"github.com/souhoc/taskmaster/util"
)

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
		logFile.Close()
		os.Exit(1)
	}

	if cfg.DropToUser != "" {
		if err := util.DropToUser(cfg.DropToUser); err != nil {
			fmt.Fprintln(os.Stderr, err)
			logFile.Close()
			os.Exit(1)
		}
	}

	service := taskmaster.New(&cfg, taskmaster.WithOutputFile(logFile))

	spinner := util.NewSinner(nil)
	go spinner.Spin("Auto starting tasks...")
	service.AutoStart()()
	spinner.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	t := term.New()
	addCmds(t, service)

	go handleSignals(sigChan, service, t)

	t.Run()
	if err := service.Close(); err != nil {
		log.Printf("Failed to close the service: %s\n", err)
	}

	switch err := context.Cause(service.Ctx); err {
	case taskmaster.ServiceClosed:
		// Nothing to do
	default:
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func addCmds(t *term.Term, service *taskmaster.Service) {
	t.AddCmd("reload", "Reload config file.", newReloadHandler(service))
	t.AddCmd("start", "Start a task.", newStartHandler(service))
	t.AddCmd("stop", "Stop a task.", newStopHandler(service))
	t.AddCmd("status", "list running processes", newStatusHandler(service))
}

func handleSignals(sigChan chan os.Signal, service *taskmaster.Service, program *term.Term) {
	defer signal.Stop(sigChan)
	for {
		select {
		case s := <-sigChan:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("%s: %s\n", TerminatedBySignal, s)
				program.Stop()
				return
			case syscall.SIGHUP:
				if err := newReloadHandler(service)(); err != nil {
					log.Printf("Error: %v\n", err)
				}
			}
		}
	}
}
