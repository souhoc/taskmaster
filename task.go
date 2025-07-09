package taskmaster

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type Task struct {
	Cmd          string            `yaml:"cmd"`
	Args         []string          `yaml:"args"`
	NumProcs     int               `yaml:"numprocs"`
	Umask        int               `yaml:"umask"`
	WorkingDir   string            `yaml:"workingdir"`
	AutoStart    bool              `yaml:"autostart"`
	AutoRestart  string            `yaml:"autorestart"`
	ExitCodes    []int             `yaml:"exitcodes"`
	StartRetries int               `yaml:"startretries"`
	StartTime    int               `yaml:"starttime"`
	StopSignal   string            `yaml:"stopsignal"`
	StopTime     int               `yaml:"stoptime"`
	Stdout       string            `yaml:"stdout"`
	Stderr       string            `yaml:"stderr"`
	Env          map[string]string `yaml:"env"`
}

func (t Task) Compare(u Task) bool {
	return t.Cmd == u.Cmd &&
		reflect.DeepEqual(t.Args, u.Args) &&
		t.NumProcs == u.NumProcs &&
		t.Umask == u.Umask &&
		t.WorkingDir == u.WorkingDir &&
		t.AutoStart == u.AutoStart &&
		t.AutoRestart == u.AutoRestart &&
		reflect.DeepEqual(t.ExitCodes, u.ExitCodes) &&
		t.StartRetries == u.StartRetries &&
		t.StartTime == u.StartTime &&
		t.StopSignal == u.StopSignal &&
		t.StopTime == u.StopTime &&
		t.Stdout == u.Stdout &&
		t.Stderr == u.Stderr &&
		reflect.DeepEqual(t.Env, u.Env)
}

func (t Task) String() string {
	return fmt.Sprintf(
		"Cmd: %s\n  Args: %s\n  NumProcs: %d\n  Umask: %v\n  WorkingDir: %s\n  AutoStart: %v\n  AutoRestart: %s\n  ExitCodes: %v\n  StartRetries: %d\n  StartTime: %d\n  StopSignal: %s\n  StopTime: %d\n  Stdout: %s\n  Stderr: %s\n  Env: %s",
		t.Cmd,
		strings.Join(t.Args, " "),
		t.NumProcs,
		t.Umask,
		t.WorkingDir,
		t.AutoStart,
		t.AutoRestart,
		t.ExitCodes,
		t.StartRetries,
		t.StartTime,
		t.StopSignal,
		t.StopTime,
		t.Stdout,
		t.Stderr,
		fmt.Sprintf("%v", t.Env),
	)
}

func (t Task) shouldRestart(exitCode int) bool {
	switch t.AutoRestart {
	case "never", "":
		return false
	case "always":
		return true
	case "unexpected":
		return !t.isExpectedExitCode(exitCode)
	default:
		return false
	}
}
func (t Task) isExpectedExitCode(exitCode int) bool {
	expectedCodes := t.ExitCodes
	if len(expectedCodes) == 0 {
		expectedCodes = []int{0}
	}

	return slices.Contains(expectedCodes, exitCode)
}
