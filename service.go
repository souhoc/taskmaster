package taskmaster

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
)

const (
	SocketName = "/tmp/taskmaster.sock"
)

type TaskMaster struct {
	Ctx    context.Context
	Cancel context.CancelCauseFunc
	mu     sync.Mutex
	tasks  map[string]*exec.Cmd
}

func New(size int) *TaskMaster {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &TaskMaster{
		Ctx:    ctx,
		Cancel: cancel,
		tasks:  make(map[string]*exec.Cmd, size),
	}
}

func (t *TaskMaster) Shutdown(cause string, rep *struct{}) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Cancel(errors.New(cause))

	return nil
}

func (*TaskMaster) GetPid(arg *struct{}, resp *int) error {
	*resp = os.Getpid()
	return nil
}

func (t *TaskMaster) Start(task TaskConfig, rep *exec.Cmd) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	rep = exec.CommandContext(t.Ctx, task.Cmd, task.Args...)

	rep.Stdout = os.Stdout

	if err := rep.Start(); err != nil {
		return err
	}
	return nil
}

func (*TaskMaster) List(args *struct{}, rep *[]string) error {

	return nil
}
