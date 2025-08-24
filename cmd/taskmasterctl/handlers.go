package main

import (
	"fmt"
	"log/slog"
	"net/rpc"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

const (
	nameWidth int = 20
)

type Handler struct {
	client   *rpc.Client
	terminal *term.Term
}

func (h *Handler) SetTerminal() {
	h.terminal.AddCmd("status", "Display status of one or more processes", h.Status)
	h.terminal.AddCmd("start", "Start one ore more processes.", h.Start)
	h.terminal.AddCmd("stop", "Stop one ore more processes.", h.Stop)
	h.terminal.AddCmd("reload", "Reload config file.", h.Reload)

	var processes []string
	err := h.client.Call(taskmaster.RPCServiceList, struct{}{}, &processes)
	if err == nil {
		h.terminal.SetCompletions(processes...)
	}
}

func (h *Handler) Status(args ...string) error {
	if len(args) == 1 {
		var processes []string
		if err := h.client.Call(taskmaster.RPCServiceList, struct{}{}, &processes); err != nil {
			if err == rpc.ErrShutdown {
				fmt.Print("service is closed")
				return term.Exit
			}
			return err
		}
		args = append(args, processes...)
	}

	for _, arg := range args[1:] {
		var status taskmaster.ProcessStatus
		h.client.Call(taskmaster.RPCServiceStatus, arg, &status)

		var pid int
		err := h.client.Call(taskmaster.RPCServiceGetPid, arg, &pid)
		if err == rpc.ErrShutdown {
			fmt.Print("service is closed")
			return term.Exit
		}

		if status == taskmaster.ProcessStatusRunning && err == nil {
			fmt.Printf("%s %s %d\n", arg, status, pid)
			continue
		}
		fmt.Printf("%s %s XXXX\n", arg, status)
	}

	return nil
}

func (h *Handler) Start(args ...string) error {
	if len(args) == 1 {
		return fmt.Errorf("%s: missing parameter", args[0])
	}

	for _, arg := range args[1:] {
		if err := h.client.Call(taskmaster.RPCServiceStart, arg, nil); err != nil {
			if err == rpc.ErrShutdown {
				fmt.Print("service is closed")
				return term.Exit
			}

			fmt.Printf("%s: %s\n", err, arg)
			return fmt.Errorf("%s: %w", args[0], err)
		}

		fmt.Printf("running task %s\n", arg)
	}

	return nil
}

func (h *Handler) Stop(args ...string) error {
	if len(args) == 1 {
		return fmt.Errorf("%s: missing parameter", args[0])
	}

	for _, arg := range args[1:] {
		if err := h.client.Call(taskmaster.RPCServiceStop, arg, nil); err != nil {
			if err == rpc.ErrShutdown {
				fmt.Print("service is closed")
				return term.Exit
			}

			fmt.Printf("%s: %s\n", err, arg)
			return fmt.Errorf("%s: %w", args[0], err)
		}

		fmt.Printf("stopped process: %s\n", arg)
	}

	return nil
}

func (h *Handler) Reload(_ ...string) error {
	var changed bool
	err := h.client.Call(taskmaster.RPCServiceReloadConfig, struct{}{}, &changed)
	if err != nil {
		if err == rpc.ErrShutdown {
			fmt.Print("service is closed")
			return term.Exit
		}
		slog.Error("reload",
			slog.Bool("new_config?", changed),
			slog.Any("error", err),
		)
		return err
	}

	var processes []string
	err = h.client.Call(taskmaster.RPCServiceList, struct{}{}, &processes)
	if err == nil {
		h.terminal.SetCompletions(processes...)
	}

	return nil
}
