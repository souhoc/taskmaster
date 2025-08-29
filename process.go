package taskmaster

import (
	"os/exec"
	"syscall"
	"time"
)

//go:generate stringer --type ProcessStatus --trimprefix ProcessStatus
type ProcessStatus int

const (
	ProcessStatusUnknown ProcessStatus = iota
	ProcessStatusRunning
	ProcessStatusExited
	ProcessStatusStopped
	ProcessStatusIdle
	ProcessStatusFailed
)

type Process struct {
	cmd        *exec.Cmd
	task       *Task
	startCount int
	startAt    time.Time
	status     ProcessStatus
	done       chan error
}

func (p *Process) Start() error {
	p.startAt = time.Now()
	p.startCount++
	if p.task.Umask != 0 {
		oldUmask := syscall.Umask(p.task.Umask)
		defer syscall.Umask(oldUmask)
	}
	return p.cmd.Start()
}

// ShouldRestart returns true if the process as successfully started and the
// task is configured so.
func (p Process) ShouldRestart() bool {
	return p.cmd.ProcessState != nil &&
		time.Since(p.startAt) > p.task.StartTime &&
		p.task.shouldRestart(p.cmd.ProcessState.ExitCode()) && p.status != ProcessStatusStopped
}

// ShouldRetryStart returns true if the process didn't start successfully and
// the task is configured so.
func (p Process) ShouldRetryStart() bool {
	return p.startCount < p.task.StartRetries &&
		p.cmd.ProcessState != nil &&
		time.Since(p.startAt) < p.task.StartTime
}
