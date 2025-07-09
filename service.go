package taskmaster

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	SocketName     = "/tmp/taskmaster.sock"
	defaultTimeout = 10 * time.Second
)

type Service struct {
	Ctx    context.Context
	Cancel context.CancelCauseFunc
	cfg    *Config
	out    *os.File
	mu     sync.Mutex
	cmds   map[string]*exec.Cmd
}

func New(cfg *Config, opts ...OptFn) *Service {
	ctx, cancel := context.WithCancelCause(context.Background())
	s := &Service{
		Ctx:    ctx,
		Cancel: cancel,
		cfg:    cfg,
		cmds:   make(map[string]*exec.Cmd),
	}

	for _, fn := range opts {
		fn(s)
	}

	return s
}

func (s *Service) Start(name string) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.cfg.Tasks[name]
	if !exists {
		return nil, UnknowTask
	}

	if cmd, exists := s.cmds[name]; exists && cmd.Process != nil {
		return nil, fmt.Errorf("service: task %s is already running\n", name)
	}

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
		cmd.Stdout = s.out
	} else {
		file, err := os.OpenFile(task.Stderr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("service: failed to open stdout file %s: %w", task.Stderr, err)
		}
		cmd.Stdout = file
	}

	if task.Stderr == "" {
		cmd.Stderr = s.out
	} else {
		file, err := os.OpenFile(task.Stderr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("service: failed to open stderr file %s: %w", task.Stderr, err)
		}
		cmd.Stderr = file
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("service: couldn't start cmd %s: %w", name, err)
	}

	s.cmds[name] = cmd

	go s.handleTaskCompletion(name, task, cmd)

	return cmd, nil
}

func (s *Service) handleTaskCompletion(name string, task *Task, cmd *exec.Cmd) {
	err := cmd.Wait()

	s.mu.Lock()
	delete(s.cmds, name)
	s.mu.Unlock()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Log task completion
	if err == nil {
		log.Printf("service: task %s completed successfully (exit code %d)\n", name, exitCode)
		return
	}

	log.Printf("service: task %s exited with code %d: %v\n", name, exitCode, err)

	if task.shouldRestart(exitCode) {
		// Add a small delay before restarting to prevent rapid restart loops
		time.Sleep(1 * time.Second)

		log.Printf("service: restarting task: %s\n", name)

		// Restart the task
		if _, restartErr := s.Start(name); restartErr != nil && s.out != nil {
			log.Printf("service: failed to restart task %s: %v\n", name, restartErr)
		}
	}
}

func (s *Service) Stop(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.cfg.Tasks[name]
	if !exists {
		return UnknowTask
	}

	cmd, exists := s.cmds[name]
	if !exists || cmd.Process == nil {
		return fmt.Errorf("sercice: task %s is not running", name)
	}

	signal := syscall.SIGTERM
	if task.StopSignal != "" {
		switch task.StopSignal {
		case "TERM", "SIGTERM":
			signal = syscall.SIGTERM
		case "INT", "SIGINT":
			signal = syscall.SIGINT
		case "KILL", "SIGKILL":
			signal = syscall.SIGKILL
		case "HUP", "SIGHUP":
			signal = syscall.SIGHUP
		case "USR1", "SIGUSR1":
			signal = syscall.SIGUSR1
		case "USR2", "SIGUSR2":
			signal = syscall.SIGUSR2
		default:
			return fmt.Errorf("service: unsupported stop signal: %s", task.StopSignal)
		}
	}

	if err := cmd.Process.Signal(signal); err != nil {
		return fmt.Errorf("service: failed to send signal to task %s: %w", name, err)
	}

	// Wait for the process to exit or force kill after timeout
	stopTimeout := time.Duration(task.StopTime) * time.Second
	if stopTimeout <= 0 {
		stopTimeout = defaultTimeout
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited
		delete(s.cmds, name)
		if err != nil {
			return fmt.Errorf("service: task %s exited with error: %w", name, err)
		}
		return nil
	case <-time.After(stopTimeout):
		// Timeout reached, force kill
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("service: failed to kill task %s: %w", name, err)
		}
		delete(s.cmds, name)
		return fmt.Errorf("service: task %s was forcibly killed after timeout", name)
	}
}

func (s *Service) Get(name string) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd, exists := s.cmds[name]
	if !exists {
		return nil, UnknowTask
	}

	return cmd, nil
}

func (s *Service) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.cmds))
	for k := range s.cmds {
		keys = append(keys, k)
	}

	return keys
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.cmds) == 0 {
		return nil
	}

	// Channel to collect errors from shutdown operations
	errChan := make(chan error, len(s.cmds))
	var wg sync.WaitGroup

	for taskName, cmd := range s.cmds {
		if cmd.Process == nil {
			continue
		}

		wg.Add(1)
		go func(name string, command *exec.Cmd) {
			defer wg.Done()

			// Send SIGTERM first
			if err := command.Process.Signal(syscall.SIGTERM); err != nil {
				errChan <- fmt.Errorf("failed to send SIGTERM to task %s: %w", name, err)
				return
			}

			// Wait for graceful shutdown with timeout
			done := make(chan error, 1)
			go func() {
				done <- command.Wait()
			}()

			select {
			case err := <-done:
				// Process exited gracefully
				if err != nil {
					errChan <- fmt.Errorf("task %s exited with error: %w", name, err)
				}
			case <-time.After(defaultTimeout):
				// Timeout reached, force kill
				if err := command.Process.Kill(); err != nil {
					errChan <- fmt.Errorf("failed to kill task %s: %w", name, err)
				} else {
					errChan <- fmt.Errorf("task %s was forcibly killed after timeout", name)
				}
			}
		}(taskName, cmd)
	}

	// Wait for all shutdown operations to complete
	wg.Wait()
	close(errChan)

	// Cancel the context to signal shutdown
	if s.Cancel != nil {
		s.Cancel(fmt.Errorf("service shutdown"))
	}

	// Collect and return any errs
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %w", errors.Join(errs...))
	}

	return nil
}

func (s *Service) ReloadConfig() (changed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.cfg.Reload()
	// TODO: se what need to be done in service on change
	// - [ ] if a task doesnt exist, the cmd has to be closed

	return old != nil, err
}

// =============================
type OptFn func(*Service)

func WithOutFile(f *os.File) OptFn {
	return func(s *Service) {
		s.out = f
	}
}
