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
	SocketName          = "/tmp/taskmaster.sock"
	defaultTimeout      = 3 * time.Second
	defaultStartRetries = 3

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
	hostname, err := os.Hostname()
	if err != nil {
		s.webhook.Username = "Taskmaster"
	} else {
		s.webhook.Username = hostname
	}
	s.webhook.Send("service: new instance")

	return s
}

// GetTask retrives a task by name.
//
// Parameters:
//   - Name: the task's name.
//
// Returns:
//   - *Task: A pointer to the task.
//   - error: An error if the task in unknown.
func (s *Service) GetTask(name string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.cfg.Tasks[name]
	if !exists {
		return nil, ErrTaskUnknown
	}

	return task, nil
}

// AutoStart starts tasks that is set to auto start.
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
		for taskName, task := range s.cfg.Tasks {
			if !task.AutoStart {
				continue
			}

			if err := s.Start(taskName); err != nil {
				log.Printf("service: error while autostart: %s: %v", taskName, err)
			}
		}
	}()

	return
}

// Start initiates a service by name.
//
// Parameters:
//   - name: The name of the service to start.
//
// Returns:
//   - An error if the service cannot be started.
func (s *Service) Start(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.cfg.Tasks[name]
	if !exists {
		return ErrTaskUnknown
	}

	for i := range task.NumProcs {
		var cmdName string
		if task.NumProcs > 1 {
			cmdName = fmt.Sprintf(processNameFormat, name, i)
		} else {
			cmdName = name
		}

		if cmd, exists := s.cmds[cmdName]; exists && cmd.Process != nil {
			return fmt.Errorf("service: %w", ErrTaskAlreadyRunning)

		}

		cmd, err := s.newCmd(cmdName, task)
		if err != nil {
			return fmt.Errorf("service: couldn't create cmd %s: %w", cmdName, err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("service: couldn't start cmd %s: %w", cmdName, err)
		}

		s.cmds[cmdName] = cmd
		go s.handleTaskCompletion(cmdName, task, cmd)

		// Check that the cmd has successfuly start
		time.Sleep(task.StartTime)
		if cmd.ProcessState != nil {
			log.Printf("service: unsuccessful start: %s", cmdName)
			return fmt.Errorf("service: unsuccessful start: %s", cmdName)
		}
		msg := fmt.Sprintf("service: cmd started successfully: %s %d\n", cmdName, cmd.Process.Pid)
		log.Print(msg)
		s.webhook.Send(msg)
	}

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
		msg := fmt.Sprintf("service: cmd %s completed successfully (exit code %d)\n", name, exitCode)
		log.Print(msg)
		s.webhook.Send(msg)
		return
	} else if exitCode == -1 {
		msg := fmt.Sprintf("service: cmd %s exited: %s\n", name, err)
		log.Print(msg)
		s.webhook.Send(msg)
		return
	}

	msg := fmt.Sprintf("service: task %s exited with code %d: %v\n", name, exitCode, err)
	log.Print(msg)
	s.webhook.Send(msg)

	if task.shouldRestart(exitCode) {
		// Add a small delay before restarting to prevent rapid restart loops
		time.Sleep(1 * time.Second)

		log.Printf("service: restarting task: %s\n", name)

		// Restart the task
		if restartErr := s.Start(name); restartErr != nil && s.out != nil {
			log.Printf("service: failed to restart task %s: %v\n", name, restartErr)
		}
	}
}

// Stop halts a running service by name.
//
// Parameters:
//   - name: The name of the service to stop.
//
// Returns:
//   - An error if the service cannot be stopped.
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
			return fmt.Errorf("service: error stopping task: %w", err)
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

	select {
	case err := <-task.done:
		if err != nil {
			return fmt.Errorf("task %s exited with error: %w", name, err)
		}
		return nil
	case <-time.After(task.StopTime):
		// Timeout reached, force kill
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill task %s: %w", name, err)
		}
		delete(s.cmds, name)
		return fmt.Errorf("task %s was forcibly killed after timeout", name)
	}
}

// Get retrieves a command by name.
//
// Parameters:
//   - name: The name of the command to retrieve.
//
// Returns:
//   - *exec.Cmd: A pointer to the retrieved command.
//   - error: An error if the command cannot be retrieved.
func (s *Service) Get(name string) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd, exists := s.cmds[name]
	if !exists {
		return nil, ErrTaskUnknown
	}

	return cmd, nil
}

// List returns a list of available service names.
// Returns:
//   - []string: A slice of strings representing the service names.
func (s *Service) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.cmds))
	for k := range s.cmds {
		keys = append(keys, k)
	}

	return keys
}

// Close shuts down the service and cleans up resources.
// Returns:
//   - An error if the service cannot be closed properly.
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
	s.webhook.Send("service: closed")
	return nil
}

// ReloadConfig reloads the configuration settings for the service.
// Returns:
//   - changed: A boolean indicating whether the configuration was changed.
//   - err: An error if the configuration cannot be reloaded.
func (s *Service) ReloadConfig() (changed bool, err error) {
	s.mu.Lock()
	s.webhook.Send("reloading config")

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
			<-time.After(time.Millisecond * 500)
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
