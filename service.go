package taskmaster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	SocketName          = "/tmp/taskmaster.sock"
	defaultStopTime     = 3 * time.Second
	defaultStartTime    = 3 * time.Second
	defaultStartRetries = 3

	processNameFormat string = "%s_%02d"
)

type Service struct {
	Ctx       context.Context
	Cancel    context.CancelCauseFunc
	cfg       *Config
	out       *os.File
	mu        sync.Mutex
	processes map[string]*Process
}

func New(cfg *Config, opts ...OptFn) *Service {
	ctx, cancel := context.WithCancelCause(context.Background())
	s := &Service{
		Ctx:    ctx,
		Cancel: cancel,
		cfg:    cfg,
	}
	s.processes = s.makeProcesses(cfg.Tasks)

	for _, fn := range opts {
		fn(s)
	}

	slog.Info("new service")
	return s
}

func (s *Service) makeProcesses(tasks map[string]*Task) map[string]*Process {
	processes := make(map[string]*Process)

	for taskName, task := range tasks {
		if task.NumProcs == 1 {
			process, err := s.newProcess(taskName, task)
			if err != nil {
				slog.Error("failed to make process",
					slog.String("process", taskName),
					slog.Any("error", err),
				)
				continue
			}

			processes[taskName] = process
			slog.Info("init",
				slog.String("process", taskName),
			)
			continue
		}
		for i := range task.NumProcs {
			processName := fmt.Sprintf(processNameFormat, taskName, i)
			process, err := s.newProcess(processName, task)
			if err != nil {
				slog.Error("init process failed",
					slog.String("process", processName),
					slog.Any("error", err),
				)
				continue
			}

			processes[processName] = process
			slog.Info("init",
				slog.String("process", processName),
			)
		}
	}

	return processes
}

// Start starts a process if it exists.
//
// Parameters:
//   - name: the name of the process.
func (s *Service) Start(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	process, exists := s.processes[name]
	if !exists {
		return ErrProcessUnknown
	}
	if process == nil {
		return ErrProcessNil
	}
	if process.cmd.Process != nil {
		return ErrProcessAlreadyStarted
	}

	for range process.task.StartRetries {
		if err := process.Start(); err != nil {
			return ErrProcessAlreadyStarted
		}
		slog.Info("spawned",
			slog.String("process", name),
			slog.Int("pid", process.cmd.Process.Pid),
		)
		tmpCmd := process.cmd
		go s.handleProcessCompletion(name)

		// Wait StartTime to see if the process successfully started.
		time.Sleep(process.task.StartTime)
		if tmpCmd.ProcessState == nil {
			slog.Info("success",
				slog.String("process", name),
				slog.Int("pid", tmpCmd.Process.Pid),
				slog.Int("tries", process.startCount),
			)
			s.processes[name].status = ProcessStatusRunning
			return nil
		}
	}

	process.status = ProcessStatusFailed
	return ErrProcessIsNotRunning
}

// handleProcessCompletion handles the completion of a process. Id needed it
// retry to start it, or restart it.
func (s *Service) handleProcessCompletion(name string) {
	process := s.processes[name]

	err := process.cmd.Wait()
	shouldRetryStart := process.ShouldRetryStart()
	shouldRestart := process.ShouldRestart()
	slog.Warn("exited",
		slog.String("process", name),
		slog.Int("exit_code", process.cmd.ProcessState.ExitCode()),
		slog.Bool("should_retry_start", shouldRetryStart),
		slog.Bool("should_restart", shouldRestart),
		slog.Int("start_count", process.startCount),
	)

	if err == nil {
		// Successful exit.
		process.done <- nil
		if err := s.resetProcess(name, ProcessStatusExited); err != nil {
			slog.Error("failed",
				slog.String("process", name),
				slog.Any("resetProcess", err))
		}
		return
	}

	if process.cmd.ProcessState.ExitCode() == -1 {
		// Got killed. dont go further.
		process.done <- nil
		if err := s.resetProcess(name, ProcessStatusStopped); err != nil {
			slog.Error("failed",
				slog.String("process", name),
				slog.Any("resetProcess", err))
		}
		return
	}

	if shouldRetryStart {
		process.cmd, err = s.newCmd(name, process.task)
		process.status = ProcessStatusFailed
		if err != nil {
			slog.Error("failed",
				slog.String("process", name),
				slog.Any("newCmd", err))
		}
		return
	}

	if shouldRestart {
		if err := s.resetProcess(name, ProcessStatusIdle); err != nil {
			slog.Error("failed",
				slog.String("process", name),
				slog.Any("resetProcess", err))
			return
		}

		if err := s.Start(name); err != nil {
			slog.Error("retry start",
				slog.String("process", name),
				slog.Any("service.Start", err))
		}
		return
	}

	process.done <- err
	if err := s.resetProcess(name, ProcessStatusExited); err != nil {
		slog.Error("failed",
			slog.String("process", name),
			slog.Any("resetProcess", err))
	}
}

func (s *Service) Stop(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	process, exits := s.processes[name]
	if !exits {
		return ErrProcessUnknown
	}
	if process == nil {
		return ErrProcessNil
	}
	if process.cmd == nil || process.cmd.Process == nil {
		return ErrProcessIsNotRunning
	}

	var sig syscall.Signal
	switch process.task.StopSignal {
	case "", "TERM", "SIGTERM":
		sig = syscall.SIGTERM
	case "INT", "SIGINT":
		sig = syscall.SIGINT
	case "KILL", "SIGKILL":
		sig = syscall.SIGKILL
	case "HUP", "SIGHUP":
		sig = syscall.SIGHUP
	case "USR1", "SIGUSR1":
		sig = syscall.SIGUSR1
	case "USR2", "SIGUSR2":
		sig = syscall.SIGUSR2
	default:
		return fmt.Errorf("unsupported stop signal: %s", process.task.StopSignal)
	}

	if err := process.cmd.Process.Signal(sig); err != nil {
		return fmt.Errorf("failed to send signal %s to task %s: %w", sig, name, err)
	}

	select {
	case err := <-process.done:
		if err != nil {
			return fmt.Errorf("task %s exited with error: %w", name, err)
		}
		return nil
	case <-time.After(process.task.StopTime):
		// Timeout reached, force kill
		if err := process.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill task %s: %w", name, err)
		}
		return fmt.Errorf("task %s was forcibly killed after timeout", name)
	}
}

func (s *Service) Status(name string) ProcessStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	process, exists := s.processes[name]
	if !exists || process == nil {
		return ProcessStatusUnknown
	}

	return process.status
}

// AutoStart starts processes that is set to auto start.
//
// Returns:
//   - wait: A function to wait the completion of AutoStart
func (s *Service) AutoStart() (wait func()) {
	c := make(chan struct{})
	wait = func() {
		<-c
	}

	go func() {
		defer close(c)
		var keys []string
		for name, process := range s.processes {
			if process == nil {
				continue
			}

			if !process.task.AutoStart {
				continue
			}
			keys = append(keys, name)
		}
		if err := s.Batch(s.Start, keys); err != nil {
			slog.Error("auto start", slog.Any("Batch", err))
		}
	}()

	return
}

// Close shuts down the service and cleans up resources.
func (s *Service) Close() error {
	defer s.Cancel(ServiceClosed)
	var keys []string
	for name, process := range s.processes {
		if process.status == ProcessStatusRunning {
			keys = append(keys, name)
		}
	}
	if err := s.Batch(s.Stop, keys); err != nil {
		return fmt.Errorf("close errors: %w", err)
	}

	slog.Info("service closed")
	return nil
}

func (s *Service) newCmd(name string, task *Task) (*exec.Cmd, error) {
	cmd := exec.CommandContext(s.Ctx, task.Cmd, task.Args...)
	cmd.Args[0] = name

	if task.WorkingDir != "" {
		cmd.Dir = task.WorkingDir
	}

	if len(task.Env) > 0 {
		env := os.Environ()
		for k, v := range task.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	if task.Umask != 0 {
		oldUmask := syscall.Umask(task.Umask)
		defer syscall.Umask(oldUmask)
	}

	if task.Stdout == "" {
		cmd.Stdout = nil
	} else {
		file, err := os.OpenFile(task.Stdout, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open stdout file %s: %w", task.Stdout, err)
		}
		cmd.Stdout = file
	}

	if task.Stderr == "" {
		cmd.Stderr = nil
	} else {
		file, err := os.OpenFile(task.Stderr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open stderr file %s: %w", task.Stderr, err)
		}
		cmd.Stderr = file
	}

	return cmd, nil
}

func (s *Service) newProcess(name string, task *Task) (*Process, error) {
	cmd, err := s.newCmd(name, task)
	if err != nil {
		return nil, fmt.Errorf("s.newCmd: %w", err)
	}
	return &Process{
		cmd:        cmd,
		task:       task,
		startCount: 0,
		startAt:    time.Time{},
		status:     ProcessStatusIdle,
		done:       make(chan error, 1),
	}, nil
}

func (s *Service) resetProcess(name string, status ProcessStatus) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	process, exists := s.processes[name]
	if !exists {
		return ErrProcessUnknown
	}

	s.processes[name], err = s.newProcess(name, process.task)
	if err != nil {
		return err
	}
	s.processes[name].status = status

	return
}

func (s *Service) GetPid(name string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	process, exists := s.processes[name]
	if !exists {
		return 0, ErrProcessUnknown
	}
	if process == nil {
		return 0, ErrProcessNil
	}
	if process.cmd == nil || process.cmd.Process == nil || process.cmd.ProcessState != nil {
		return 0, ErrProcessIsNotRunning
	}

	return process.cmd.Process.Pid, nil
}

func (s *Service) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.processes))
	for k := range s.processes {
		keys = append(keys, k)
	}
	return keys
}

func (s *Service) Batch(fn func(name string) error, names []string) error {
	errC := make(chan error, len(names))
	var wg sync.WaitGroup

	wg.Add(len(names))
	for _, name := range names {
		go func() {
			defer wg.Done()
			err := fn(name)
			if err != nil {
				errC <- fmt.Errorf("%s: %w", name, err)
				return
			}
			errC <- nil
		}()
	}
	wg.Wait()
	close(errC)

	var errs []error
	for err := range errC {
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil

}

func (s *Service) Reload() (changed bool, err error) {
	s.mu.Lock()
	var newCfg Config
	if err := newCfg.Load(); err != nil {
		return false, fmt.Errorf("service: failed to load config: %w", err)
	}
	if newCfg.Compare(*s.cfg) {
		s.mu.Unlock()
		return false, nil
	}

	newProcesses := s.makeProcesses(newCfg.Tasks)

	// Stop all processes not in the new processes
	var keys []string
	for _, name := range diffProcesses(s.processes, newProcesses) {
		if s.processes[name].status == ProcessStatusRunning {
			keys = append(keys, name)
		}
	}
	slog.Debug("batch stop",
		slog.Any("names", keys),
	)
	s.mu.Unlock()
	if err := s.Batch(s.Stop, keys); err != nil {
		return false, fmt.Errorf("error while stopping old processes: %w", err)
	}
	s.mu.Lock()

	// If a process is running but doesnt need to be restarted, just add
	// it to the new processes. Else, restart it.
	keys = keys[:0]
	for _, name := range commonProcesses(newProcesses, s.processes) {
		slog.Debug("commonProcesses",
			slog.String("process", name),
			slog.String("status", s.processes[name].status.String()),
			slog.Bool("DiffNeedRestart", s.processes[name].task.DiffNeedRestart(*newProcesses[name].task)),
			slog.Int("old_task_proces", s.processes[name].task.NumProcs),
			slog.String("old_task_proces", s.processes[name].task.Stdout),
			slog.Int("new_task_proces", newProcesses[name].task.NumProcs),
			slog.String("new_task_proces", newProcesses[name].task.Stdout),
		)
		if s.processes[name].status != ProcessStatusRunning {
			continue
		}
		if s.processes[name].task.DiffNeedRestart(*newProcesses[name].task) {
			keys = append(keys, name)
		} else {
			newProcesses[name] = s.processes[name]
		}
	}
	slog.Debug("batch stop for restart",
		slog.Any("names", keys),
	)
	s.mu.Unlock()
	if err := s.Batch(s.Stop, keys); err != nil {
		return false, fmt.Errorf("error while stopping for restart processes: %w", err)
	}
	time.Sleep(time.Millisecond * 500)
	s.mu.Lock()

	slog.Debug("processes switch",
		slog.Any("old", s.processes),
		slog.Any("new", newProcesses),
	)
	oldProcesses := s.processes
	s.processes = newProcesses
	*s.cfg = newCfg

	slog.Debug("batch start for restart",
		slog.Any("names", keys),
	)
	s.mu.Unlock()
	if err := s.Batch(s.Start, keys); err != nil {
		return true, fmt.Errorf("error while restarting processes: %w", err)
	}
	s.mu.Lock()

	// Start all new processes
	keys = keys[:0]
	for _, name := range diffProcesses(newProcesses, oldProcesses) {
		if newProcesses[name].task.AutoStart {
			keys = append(keys, name)
		}
	}
	slog.Debug("batch start new processes",
		slog.Any("names", keys),
	)
	s.mu.Unlock()
	if err := s.Batch(s.Start, keys); err != nil {
		return false, fmt.Errorf("error while starting new processes: %w", err)
	}

	slog.Info("reload succesful")
	return true, nil
}

// diffProcesses lists processes that are in a but not in b.
func diffProcesses(a, b map[string]*Process) []string {
	var keys []string
	for name := range a {
		_, exists := b[name]
		if !exists {
			keys = append(keys, name)
		}
	}

	return keys
}

// commonProcesses lists processes that are in a and b.
func commonProcesses(a, b map[string]*Process) []string {
	var keys []string
	for name := range a {
		_, exists := b[name]
		if exists {
			keys = append(keys, name)
		}
	}

	return keys
}

// =============================
type OptFn func(*Service)

func WithOutputFile(f *os.File) OptFn {
	return func(s *Service) {
		s.out = f
	}
}
