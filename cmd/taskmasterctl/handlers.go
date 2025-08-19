package main

import (
	"errors"
	"fmt"
	"log"
	"net/rpc"
	"strings"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

const (
	nameWidth int = 20
)

func newStatusHandler(client *rpc.Client) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 || args[1] == "all" {
			var names []string
			if err := client.Call(taskmaster.RPCServiceList, struct{}{}, &names); err != nil {
				if err == rpc.ErrShutdown {
					fmt.Print("server is closed")
					return term.Exit
				}
				return err
			}

			for _, name := range names {
				var pid int
				if err := client.Call(taskmaster.RPCServiceStatus, name, &pid); err != nil {
					if err == rpc.ErrShutdown {
						fmt.Print("server is closed")
						return term.Exit
					}
					continue
				}
				fill := strings.Repeat(" ", nameWidth-len(name))
				status := "RUNNING"
				if pid == 0 {
					status = "STOPPED"
				}
				fmt.Printf("%s%s %s %d\n", name, fill, status, pid)
			}
			return nil
		}

		var errs []error
		for _, arg := range args[1:] {
			var pid int
			if err := client.Call(taskmaster.RPCServiceStatus, arg, &pid); err != nil {
				if err == rpc.ErrShutdown {
					fmt.Print("server is closed")
					return term.Exit
				}
				if err.Error() == taskmaster.ErrTaskUnknown.Error() {
					fmt.Printf("unknown cmd: %s\n", arg)
					log.Printf("%s: unknown cmd: %s\n", args[0], arg)
				}
				errs = append(errs, err)
				continue
			}
			fill := strings.Repeat(" ", nameWidth-len(arg))
			status := "RUNNING"
			if pid == 0 {
				status = "STOPPED"
			}
			fmt.Printf("%s%s %s %d\n", arg, fill, status, pid)
		}

		if len(errs) > 0 {
			return fmt.Errorf("%s: couldn't get status: %w", args[0], errors.Join(errs...))
		}
		return nil
	}
}

func newReloadHandler(client *rpc.Client) term.CmdHandler {
	return func(args ...string) error {
		var changed bool
		if err := client.Call(taskmaster.RPCServiceReloadConfig, struct{}{}, &changed); err != nil {
			if err == rpc.ErrShutdown {
				fmt.Print("server is closed")
				return term.Exit
			}
			return fmt.Errorf("%s: couldn't reload config: %w", args[0], err)
		}
		log.Printf("config reloaded? %t\n", changed)
		return nil
	}
}

func newStartHandler(client *rpc.Client) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 {
			fmt.Println("missing paramater")
			return fmt.Errorf("%s: missing parameter", args[0])
		}
		for _, arg := range args[1:] {
			var reply struct{}
			err := client.Call(taskmaster.RPCServiceStart, arg, &reply)
			if err != nil {
				if err == rpc.ErrShutdown {
					fmt.Print("server is closed")
					return term.Exit
				}
				if err.Error() == taskmaster.ErrTaskUnknown.Error() {
					fmt.Printf("%s: %s\n", err, arg)
				}
				return fmt.Errorf("%s: %w", args[0], err)
			}
			fmt.Printf("running task %s\n", arg)
		}
		return nil
	}
}

func newStopHandler(client *rpc.Client) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 {
			return fmt.Errorf("%s: missing parameter", args[0])
		}
		for _, arg := range args[1:] {
			var reply struct{}
			if err := client.Call(taskmaster.RPCServiceStop, arg, &reply); err != nil {
				if err == rpc.ErrShutdown {
					fmt.Print("server is closed")
					return term.Exit
				}
				if err.Error() == taskmaster.ErrTaskUnknown.Error() {
					fmt.Printf("unknown task: %s\n", arg)
				} else {
					fmt.Printf("couldn't stop %s\n", arg)
				}
				return fmt.Errorf("%s: couldn't stop %s: %w", args[0], arg, err)
			}
		}
		return nil
	}
}
