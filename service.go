package taskmaster

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	SocketName     = "/tmp/taskmaster.sock"
	defaultTimeout = 10 * time.Second

	processNameFormat string = "%s_%02d"
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

	log.Println("service: new instance")
	return s
}

func (s *Service) Start(name string) ([]*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.cfg.Tasks[name]
	if !exists {
		return nil, ErrTaskUnknown
	}

	var cmds []*exec.Cmd
	for i := range task.NumProcs {
		var cmdName string
		if task.NumProcs > 1 {
			cmdName = fmt.Sprintf(processNameFormat, name, i)
		} else {
			cmdName = name
		}

		if cmd, exists := s.cmds[cmdName]; exists && cmd.Process != nil {
			return nil, fmt.Errorf("service: %w", ErrTaskAlreadyRunning)

		}

		cmd, err := s.newCmd(cmdName, task)
		if err != nil {
			return nil, fmt.Errorf("service: couldn't create cmd %s: %w", cmdName, err)
		}

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("service: couldn't start cmd %s: %w", cmdName, err)
		}

		s.cmds[cmdName] = cmd
		go s.handleTaskCompletion(cmdName, task, cmd)
		log.Printf("service: cmd started: %s %d\n", cmdName, cmd.Process.Pid)
		cmds = append(cmds, cmd)
	}

	return cmds, nil
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
		cmd.Stdout = s.out
	} else {
		file, err := os.OpenFile(task.Stdout, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open stdout file %s: %w", task.Stdout, err)
		}
		cmd.Stdout = file
	}

	if task.Stderr == "" {
		cmd.Stderr = s.out
	} else {
		file, err := os.OpenFile(task.Stderr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open stderr file %s: %w", task.Stderr, err)
		}
		cmd.Stderr = file
	}

	return cmd, nil
}

func (s *Service) handleTaskCompletion(name string, task *Task, cmd *exec.Cmd) {
	err := cmd.Wait()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if exitCode == -1 {
		task.done <- nil
	} else {
		task.done <- err
	}

	s.mu.Lock()
	delete(s.cmds, name)
	s.mu.Unlock()

	// Log task completion
	if err == nil {
		log.Printf("service: cmd %s completed successfully (exit code %d)\n", name, exitCode)
		return
	} else if exitCode == -1 {
		log.Printf("service: cmd %s: %s\n", name, err)
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
		return ErrTaskUnknown
	}

	if task.NumProcs > 1 {
		// For multiple process
		var errs []error
		for i := range task.NumProcs {
			cmdName := fmt.Sprintf(processNameFormat, name, i)
			if err := s.stopCmd(cmdName, task); err != nil {
				errs = append(errs, err)
			} else {
				log.Printf("service: cmd stopped: %s\n", cmdName)
			}
		}
		if len(errs) != 0 {
			return fmt.Errorf("service: errors stopping tasks: %w", errors.Join(errs...))
		}
	} else {
		// For one process
		if err := s.stopCmd(name, task); err != nil {
			return fmt.Errorf("service: error stopping task: %w", errors.Join(err))
		}
		log.Printf("service: cmd stopped: %s\n", name)
	}

	return nil
}

func (s *Service) stopCmd(name string, task *Task) error {
	cmd, exists := s.cmds[name]
	if !exists || cmd.Process == nil {
		return ErrTaskNotRunning
	}

	sig := syscall.SIGTERM
	if task.StopSignal != "" {
		switch task.StopSignal {
		case "TERM", "SIGTERM":
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
			return fmt.Errorf("unsupported stop signal: %s", task.StopSignal)
		}
	}

	if err := cmd.Process.Signal(sig); err != nil {
		return fmt.Errorf("failed to send signal to task %s: %w", name, err)
	}

	// Wait for the process to exit or force kill after timeout
	stopTimeout := time.Duration(task.StopTime) * time.Second
	if stopTimeout <= 0 {
		stopTimeout = defaultTimeout
	}

	select {
	case err := <-task.done:
		if err != nil {
			return fmt.Errorf("task %s exited with error: %w", name, err)
		}
		return nil
	case <-time.After(stopTimeout):
		// Timeout reached, force kill
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill task %s: %w", name, err)
		}
		delete(s.cmds, name)
		return fmt.Errorf("task %s was forcibly killed after timeout", name)
	}
}

// Get a cmd, with mutex security
func (s *Service) Get(name string) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd, exists := s.cmds[name]
	if !exists {
		return nil, ErrTaskUnknown
	}

	return cmd, nil
}

func (s *Service) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.cmds))
	for k, cmd := range s.cmds {
		if cmd.Process != nil {
			keys = append(keys, k)
		}
	}

	return keys
}

// Close wait all running cmds and close cleanfully
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	defer s.Cancel(ServiceClosed)

	if len(s.cmds) == 0 {
		return nil
	}

	// Channel to collect errors from shutdown operations
	errChan := make(chan error, len(s.cmds))
	var wg sync.WaitGroup

	for cmdName, cmd := range s.cmds {
		if cmd.Process == nil {
			continue
		}

		taskName := cmdName
		if idx := strings.Index(cmdName, "_"); idx != -1 {
			taskName = cmdName[:idx]
		}
		task, exists := s.cfg.Tasks[taskName]
		if !exists {
			log.Printf("service: cmd has no task: %s\n", cmdName)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.stopCmd(cmdName, task); err != nil {
				errChan <- err
			} else {
				log.Printf("service: cmd stopped: %s\n", cmdName)
			}
		}()
	}

	// Wait for all shutdown operations to complete
	wg.Wait()
	close(errChan)

	// Collect and return any errs
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %w", errors.Join(errs...))
	}

	log.Println("service: closed")
	return nil
}

func (s *Service) ReloadConfig() (changed bool, err error) {
	s.mu.Lock()

	var newCfg Config
	if err := newCfg.Load(); err != nil {
		return false, fmt.Errorf("service: failed to load config: %w", err)
	}
	if newCfg.Compare(*s.cfg) {
		s.mu.Unlock()
		return false, nil
	}

	var errs []error
	// Stop all tasks not in the new config
	for _, taskName := range diffTasks(s.cfg.Tasks, newCfg.Tasks) {
		s.mu.Unlock()

		if err := s.Stop(taskName); err != nil {
			errs = append(
				errs,
				fmt.Errorf("error while stopping task %s: %w", taskName, err),
			)
		}

		s.mu.Lock()
	}
	if len(errs) > 0 {
		return false, fmt.Errorf("reload errors: %w", errors.Join(errs...))
	}

	oldCfg := *s.cfg
	*s.cfg = newCfg

	// Start all new tasks
	for _, taskName := range diffTasks(newCfg.Tasks, oldCfg.Tasks) {
		s.mu.Unlock()

		if err := s.Start(taskName); err != nil {
			errs = append(
				errs,
				fmt.Errorf("error while starting task %s: %w", taskName, err),
			)
		}

		s.mu.Lock()
	}
	if len(errs) > 0 {
		return true, fmt.Errorf("reload errors: %w", errors.Join(errs...))
	}

	// Update tasks that where in the old and still in the new
	for _, taskName := range commonTasks(oldCfg.Tasks, newCfg.Tasks) {
		s.mu.Unlock()

		oldTask := oldCfg.Tasks[taskName]
		newTask := newCfg.Tasks[taskName]

		if oldTask.DiffNeedRestart(*newTask) {
			if err := s.Stop(taskName); err != nil {
				errs = append(
					errs,
					fmt.Errorf("error while stopping task %s: %w", taskName, err),
				)
			}
			time.After(time.Millisecond * 500)
			if err := s.Start(taskName); err != nil {
				errs = append(
					errs,
					fmt.Errorf("error while starting task %s: %w", taskName, err),
				)
			}
		} else {
			// NOTE: I have to get back the done chan of the OG task
			// as it is passed in handleTaskCompletion
			newTask.done = oldTask.done
		}

		s.mu.Lock()
	}
	if len(errs) > 0 {
		return true, fmt.Errorf("reload errors: %w", errors.Join(errs...))
	}

	return true, nil
}

// diffTasks lists tasks that are in a but not in b.
func diffTasks(a, b map[string]*Task) []string {
	var tasks []string
	for name := range a {
		_, exists := b[name]
		if !exists {
			tasks = append(tasks, name)
		}
	}

	return tasks
}

// commonTasks lists tasks that are in a and b.
func commonTasks(a, b map[string]*Task) []string {
	var tasks []string
	for name := range a {
		_, exists := b[name]
		if exists {
			tasks = append(tasks, name)
		}
	}

	return tasks
}

// =============================
type OptFn func(*Service)

func WithOutputFile(f *os.File) OptFn {
	return func(s *Service) {
		s.out = f
	}
}
