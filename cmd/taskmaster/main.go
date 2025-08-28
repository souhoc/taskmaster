package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
	"github.com/souhoc/taskmaster/util"
)

var (
	logFile    *os.File
	configPath string
)

func init() {
	flag.StringVar(&configPath, "config", "", "Config yaml file path. (On Unix systems, it returns $XDG_CONFIG_HOME as specified by https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html if non-empty, else $HOME/.config. On Darwin, it returns $HOME/Library/Application Support. On Windows, it returns %AppData%. On Plan 9, it returns $home/lib).")
	flag.Parse()

	var err error
	logFile, err = util.GetLogfile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get logfile: %v\n", err)
		os.Exit(1)
	}

	log.SetOutput(logFile)
	if os.Getenv("DEBUG") == "true" {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
}

func main() {
	defer logFile.Close()
	var cfg taskmaster.Config
	if err := cfg.Init(configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		logFile.Close()
		os.Exit(1)
	}

	logger := util.NewLogger(cfg.Webhook, logFile)
	slog.SetDefault(slog.New(logger))

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
	handler := Handler{service: service, terminal: t}
	handler.SetTerminal()
	go handleSignals(sigChan, &handler, t)

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

func handleSignals(sigChan chan os.Signal, handler *Handler, program *term.Term) {
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
				if err := handler.Reload(); err != nil {
					log.Printf("Error: %v\n", err)
				}
			}
		}
	}
}
