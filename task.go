package taskmaster

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

const (
	maxNumProcs = 99

	AutoRestartAlways     autoRestartValue = "always"
	AutoRestartNever      autoRestartValue = "never"
	AutoRestartUnexpecter autoRestartValue = "unexpected"
)

type autoRestartValue string

type Task struct {

	// The command to use to launch the program.
	Cmd string `yaml:"cmd"`

	Args []string `yaml:"args"`

	// The number of processes to start and keep running.
	NumProcs int `yaml:"numprocs"`

	// An umask to set before launching the program
	Umask int `yaml:"umask"`

	// A working directory to set before launching the program
	WorkingDir string `yaml:"workingdir"`

	// Whether to start this program at launch or not.
	AutoStart bool `yaml:"autostart"`

	// Whether the program should be restarted
	// always, never, or on unexpected exits only.
	AutoRestart autoRestartValue `yaml:"autorestart"`

	// Which return codes represent an "expected" exit status.
	ExitCodes []int `yaml:"exitcodes"`

	// How many times a restart should be attempted before aborting
	StartRetries int `yaml:"startretries"`

	// How long the program should be running after it’s started
	// for it to be considered "successfully started"
	StartTime int `yaml:"starttime"`

	// Which signal should be used to stop (i.e. exit gracefully) the program
	StopSignal string `yaml:"stopsignal"`

	// How long to wait after a graceful stop before killing the program
	StopTime int `yaml:"stoptime"`

	Stdout string `yaml:"stdout"`
	Stderr string `yaml:"stderr"`

	// Environment variables to set before launching the program
	Env map[string]string `yaml:"env"`

	done chan error
}

// Compare checks if two Task instances are identical in all fields.
// It returns true if all fields are equal, otherwise it returns false.
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

// Diff compares two Task instances and returns a slice of strings
// indicating the names of fields that differ between the two tasks.
// This method is useful for identifying specific differences between task configurations.
func (t Task) Diff(u Task) []string {
	var differences []string

	if t.Cmd != u.Cmd {
		differences = append(differences, "Cmd")
	}
	if !reflect.DeepEqual(t.Args, u.Args) {
		differences = append(differences, "Args")
	}
	if t.NumProcs != u.NumProcs {
		differences = append(differences, "NumProcs")
	}
	if t.Umask != u.Umask {
		differences = append(differences, "Umask")
	}
	if t.WorkingDir != u.WorkingDir {
		differences = append(differences, "WorkingDir")
	}
	if t.AutoStart != u.AutoStart {
		differences = append(differences, "AutoStart")
	}
	if t.AutoRestart != u.AutoRestart {
		differences = append(differences, "AutoRestart")
	}
	if !reflect.DeepEqual(t.ExitCodes, u.ExitCodes) {
		differences = append(differences, "ExitCodes")
	}
	if t.StartRetries != u.StartRetries {
		differences = append(differences, "StartRetries")
	}
	if t.StartTime != u.StartTime {
		differences = append(differences, "StartTime")
	}
	if t.StopSignal != u.StopSignal {
		differences = append(differences, "StopSignal")
	}
	if t.StopTime != u.StopTime {
		differences = append(differences, "StopTime")
	}
	if t.Stdout != u.Stdout {
		differences = append(differences, "Stdout")
	}
	if t.Stderr != u.Stderr {
		differences = append(differences, "Stderr")
	}
	if !reflect.DeepEqual(t.Env, u.Env) {
		differences = append(differences, "Env")
	}

	return differences
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
