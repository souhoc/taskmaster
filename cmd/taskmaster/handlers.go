package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/souhoc/taskmaster"
	"github.com/souhoc/taskmaster/term"
)

// Commands:
// [x] reload
// [x] status
// [x] start
// [x] stop
// [x] restart
// [x] help

const (
	nameWidth int = 20
)

/* ********* */
/* Mandatory */
/* ********* */

func newStatusHandler(s *taskmaster.Service) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 || args[1] == "all" {
			for _, name := range s.List() {
				cmd, err := s.Get(name)
				if err != nil {
					continue
				}
				fill := strings.Repeat(" ", nameWidth-len(name))
				status := "RUNNING"
				fmt.Printf("%s%s %s %d\n", name, fill, status, cmd.Process.Pid)
			}
			return nil
		}

		var errs []error

		for _, arg := range args[1:] {
			cmd, err := s.Get(arg)
			if err != nil {
				if err == taskmaster.UnknowTask {
					fmt.Printf("unknow cmd: %s\n", arg)
					log.Printf("%s: unknow cmd: %s\n", args[0], arg)
				}
				errs = append(errs, err)
			}
			fill := strings.Repeat(" ", nameWidth-len(arg))
			status := "RUNNING"
			fmt.Printf("%s%s %s %d\n", arg, fill, status, cmd.Process.Pid)
		}

		if len(errs) > 0 {
			return fmt.Errorf("%s: couldn't reload config: %v", args[0], errors.Join(errs...))
		}

		return nil
	}
}

func newReloadHandler(service *taskmaster.Service) term.CmdHandler {
	return func(args ...string) error {
		changed, err := service.ReloadConfig()
		if err != nil {
			return fmt.Errorf("%s: couldn't reload config: %v", args[0], err)
		}
		log.Printf("config reloaded? %t\n", changed)
		return nil
	}
}

func newStartHandler(s *taskmaster.Service) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 {
			return fmt.Errorf("%s: missing parameter", args[0])
		}

		cmd, err := s.Start(args[1])
		if err != nil {
			return fmt.Errorf("%s: %v", args[0], err)
		}
		log.Printf("New cmd running: %s %d\n", args[1], cmd.Process.Pid)

		return nil
	}
}

func newStopHandler(s *taskmaster.Service) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 {
			return fmt.Errorf("%s: missing parameter", args[0])
		}

		for _, arg := range args[1:] {
			if err := s.Stop(arg); err != nil {
				if err == taskmaster.UnknowTask {
					fmt.Printf("unknow task: %s\n", arg)
				} else {
					fmt.Printf("couldnt stop %s\n", arg)
				}
				return fmt.Errorf("%s: couldnt stop %s: %v", args[0], arg, err)
			}
			log.Printf("%s: stopped cmd: %s", args[0], arg)
		}

		return nil
	}
}

/* ***** */
/* Bonus */
/* ***** */

func newInfoCfgHandler(cfg *taskmaster.Config) term.CmdHandler {
	return func(args ...string) error {
		if len(args) <= 1 || args[1] == "all" {
			for name := range cfg.Tasks {
				fmt.Println("  -", name)
			}

			return nil
		}

		for _, arg := range args[1:] {
			task, exists := cfg.Tasks[arg]
			if !exists {
				fmt.Printf("task '%s' not found\n", arg)
			} else {
				fmt.Println(task.String())
			}
			fmt.Println()
		}

		return nil
	}
}
