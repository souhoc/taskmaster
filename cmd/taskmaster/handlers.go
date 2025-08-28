package main

import (
	"fmt"
	"log/slog"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

type Handler struct {
	service  *taskmaster.Service
	terminal *term.Term
}

func (h *Handler) SetTerminal() {
	h.terminal.AddCmd("reload", "Reload config file.", h.Reload)
	h.terminal.AddCmd("start", "Start one ore more processes.", h.Start)
	h.terminal.AddCmd("stop", "Stop one ore more processes.", h.Stop)
	h.terminal.AddCmd("status", "Display status of one or more processes", h.Status)

	h.terminal.SetCompletions(h.service.List()...)
}

func (h *Handler) Status(args ...string) error {
	if len(args) == 1 {
		args = append(args, h.service.List()...)
	}

	for _, arg := range args[1:] {
		status := h.service.Status(arg)
		if status == taskmaster.ProcessStatusUnknown {
			fmt.Printf("%s %s\n", arg, status)
			continue
		}

		pid, err := h.service.GetPid(arg)
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
		if err := h.service.Start(arg); err != nil {
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
		if err := h.service.Stop(arg); err != nil {
			fmt.Printf("%s: %s\n", err, arg)
			return fmt.Errorf("%s: %w", args[0], err)
		}

		fmt.Printf("stopped process: %s\n", arg)
	}

	return nil
}

func (h *Handler) Reload(_ ...string) error {
	changed, err := h.service.Reload()
	if err != nil {
		slog.Error("reload",
			slog.Bool("new_config?", changed),
			slog.Any("error", err),
		)
		return err
	}

	h.terminal.SetCompletions(h.service.List()...)

	return nil
}
