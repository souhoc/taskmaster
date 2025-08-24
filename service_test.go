package taskmaster

import (
	"testing"
	"time"
)

func TestNewService(t *testing.T) {
	cfg := &Config{
		Tasks: map[string]*Task{
			"supertail": {
				Cmd:          "tail",
				Args:         []string{"-f", "/tmp/tail"},
				NumProcs:     1,
				Umask:        0,
				StartRetries: 3,
				StartTime:    time.Second * 3,
				StopTime:     time.Second * 3,
			},
		},
	}
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
	if s.processes == nil || len(s.processes) != len(cfg.Tasks) {
		t.Error("Expected processes to be init")
	}
}

func TestService_Start_v2(t *testing.T) {
	cfg := &Config{
		Tasks: map[string]*Task{
			"supertail": {
				Cmd:          "tail",
				Args:         []string{"-f", "/tmp/tail"},
				NumProcs:     1,
				Umask:        0,
				StartRetries: 3,
				StartTime:    time.Second * 3,
				StopTime:     time.Second * 3,
			},
		},
	}
	s := New(cfg)

	err := s.Start("supertail")
	if err == nil {
		t.Error("Expected Start to have error")
	}

	err = <-s.processes["supertail"].done
	if err == nil {
		t.Error("Expected process having error")
	}
}
