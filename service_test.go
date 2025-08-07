package taskmaster

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestNewService(t *testing.T) {
	cfg := &Config{}
	s := New(cfg)

	if s.Ctx == nil {
		t.Error("Expected context to be set")
	}
	if s.Cancel == nil {
		t.Error("Expected cancel function to be set")
	}
	if s.cfg != cfg {
		t.Error("Expected config to be set")
	}
	if s.cmds == nil {
		t.Error("Expected cmds map to be initialized")
	}
}

func TestServiceHandleTaskCompletion(t *testing.T) {
	cfg := &Config{}
	s := New(cfg)

	task := &Task{
		Cmd:      "echo",
		Args:     []string{"Hello, World!"},
		NumProcs: 1,
	}

	cmd := exec.Command(task.Cmd, task.Args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Wait for the command to finish
	go func() {
		time.Sleep(1 * time.Second)
		s.handleTaskCompletion("test", task, cmd)
	}()

	// Check if the command is removed from the cmds map
	if _, ok := s.cmds["test"]; ok {
		t.Error("Expected command to be removed from cmds map")
	}
}

func TestDiffTasks(t *testing.T) {
	task1 := &Task{
		Cmd:      "echo",
		Args:     []string{"Hello, World!"},
		NumProcs: 1,
	}
	task2 := &Task{
		Cmd:      "echo",
		Args:     []string{"Hello, Universe!"},
		NumProcs: 1,
	}

	tasksA := map[string]*Task{
		"task1": task1,
		"task2": task2,
	}
	tasksB := map[string]*Task{
		"task1": task1,
	}

	diff := diffTasks(tasksA, tasksB)
	if len(diff) != 1 || diff[0] != "task2" {
		t.Errorf("Expected diff to contain 'task2', got %v", diff)
	}
}
