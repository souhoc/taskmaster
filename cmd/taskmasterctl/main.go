package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"syscall"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
	"github.com/souhoc/taskmaster/util"
)

var (
	logFile *os.File
)

func init() {
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

	log.SetPrefix("taskmasterctl ")
	logger := util.NewLogger("", logFile)
	slog.SetDefault(slog.New(logger))
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
				slog.Warn("exiting...", slog.String("signal", s.String()))
				program.Stop()
				return
			}
		}
	}
}
