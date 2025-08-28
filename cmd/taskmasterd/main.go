package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/util"
)

var (
	deamon     bool
	configPath string
)

func init() {
	flag.BoolVar(&deamon, "deamon", true, "Whether or not to run as a daemon.")
	flag.StringVar(&configPath, "config", "", "Config yaml file path. (On Unix systems, it returns $XDG_CONFIG_HOME as specified by https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html if non-empty, else $HOME/.config. On Darwin, it returns $HOME/Library/Application Support. On Windows, it returns %AppData%. On Plan 9, it returns $home/lib).")
	flag.Parse()

	if os.Getenv("DEBUG") == "true" {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Warn("DEBUG=true")
	}
	log.SetPrefix("taskmasterd ")
}

func main() {
	if deamon {
		deamonize()
		return
	}

	var cfg taskmaster.Config
	if err := cfg.Init(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init config: %v\n", err)
		os.Exit(1)
	}

	logger := util.NewLogger(cfg.Webhook, os.Stdout)
	slog.SetDefault(slog.New(logger))

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
	service.AutoStart()()

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
				slog.Warn("exiting...", slog.String("signal", s.String()))
				close(done)
				if err := lis.Close(); err != nil {
					slog.Error("failed to close listener gracefully", slog.Any("error", err))
				}
				return
			case syscall.SIGHUP:
				slog.Info("reloading on SIGHUB")
				if changed, err := service.Reload(); err != nil {
					slog.Error("failed to reload", slog.Any("error", err))
				} else {
					slog.Info("config reloaded", slog.Bool("changed?", changed))
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

func deamonize() {
	// Fork and replace the current process with a daemonized version
	execPath, err := exec.LookPath(os.Args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find executable path: %v\n", err)
		os.Exit(1)
	}

	args := append([]string{"-deamon=false"}, os.Args[1:]...)

	// Set up syscall attributes for the new process
	sysAttr := &syscall.SysProcAttr{
		Setsid: true,
	}

	logFile, err := util.GetLogfile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get logfile: %v\n", err)
		os.Exit(1)

	}
	defer logFile.Close()

	cmd := exec.Command(execPath, args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = sysAttr
	cmd.Stderr = logFile
	cmd.Stdout = logFile

	err = cmd.Start()
	if err != nil {
		fmt.Printf("Failed to daemonize: %v\n", err)
		os.Exit(1)
	}

	// Exit the parent process
	fmt.Println("Daemon started with PID:", cmd.Process.Pid)
}
