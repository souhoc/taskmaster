package main

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

const (
	nameWidth int = 20
)

type Handler struct {
	service *taskmaster.Service
}

func (h *Handler) AddCmds(t *term.Term) {
	t.AddCmd("reload", "Reload config file.", h.Reload)
	t.AddCmd("start", "Start one ore more processes.", h.Start)
	t.AddCmd("stop", "Stop one ore more processes.", h.Stop)
	t.AddCmd("status", "Display status of one or more processes", h.Status)

}

func (h *Handler) Status(args ...string) error {
	if len(args) == 1 {
		args = append(args, h.service.List()...)
	}

	slog.Debug("handler.Status", slog.Any("args", args))

	for _, arg := range args[1:] {
		status := h.service.Status(arg)
		if pid, err := h.service.GetPid(arg); status == taskmaster.ProcessStatusRunning && err == nil {
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
			if errors.Is(err, taskmaster.ErrProcessUnknown) {
				fmt.Printf("%s: %s\n", err, arg)
				continue
			}
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
	slog.Info("reload succesful")

	return nil
}
