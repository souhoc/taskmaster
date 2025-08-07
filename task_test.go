package taskmaster

import (
	"testing"
)

func TestTaskDiffNeedRestart(t *testing.T) {
	task1 := Task{
		Cmd:      "echo",
		Args:     []string{"Hello, World!"},
		NumProcs: 1,
	}
	task2 := Task{
		Cmd:      "echo",
		Args:     []string{"Hello, Universe!"},
		NumProcs: 1,
	}

	if !task1.DiffNeedRestart(task2) {
		t.Error("Expected tasks to need restart")
	}
}

func TestTaskShouldRestart(t *testing.T) {
	task := Task{
		AutoRestart: "always",
	}

	if !task.shouldRestart(0) {
		t.Error("Expected task to restart")
	}
}

func TestTaskIsExpectedExitCode(t *testing.T) {
	task := Task{
		ExitCodes: []int{0, 1},
	}

	if !task.isExpectedExitCode(0) {
		t.Error("Expected exit code to be expected")
	}
	if task.isExpectedExitCode(2) {
		t.Error("Expected exit code not to be expected")
	}
}
