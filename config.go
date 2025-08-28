package taskmaster

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var paths []string = []string{"config.yaml"}

type Config struct {
	Webhook    string           `yaml:"webhook"`
	DropToUser string           `yaml:"dropToUser"`
	Tasks      map[string]*Task `yaml:"tasks"`
}

func (c *Config) Init(configPath string) error {
	if configPath != "" {
		_, err := os.Stat(configPath)
		if err != nil {
			return err
		}
		paths = append(paths, configPath)
	} else {
		paths = append(paths, "config.yaml")
		dir, err := os.UserConfigDir()
		if err == nil {
			dir = filepath.Join(dir, "taskmaster")
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("can't create user cache dir: %v", err)
		}
		paths = append(paths, filepath.Join(dir, "config.yaml"))
	}

	return c.Load()
}

func (c *Config) Load() error {
	var configPath string
	for _, path := range paths {
		_, err := os.Stat(path)
		if err == nil {
			configPath = path
			break
		}
	}
	if configPath == "" {
		return errors.New("config: missing config file")
	}

	f, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("config: failed to open file: %w", err)
	}
	defer f.Close()

	if err := yaml.NewDecoder(f).Decode(c); err != nil {
		return fmt.Errorf("config: failed to decode file: %w", err)
	}

	// Verify the user
	if c.DropToUser != "" {
		if _, err := user.Lookup(c.DropToUser); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	}

	// Verify each tasks and init task.done
	for name, task := range c.Tasks {
		if strings.Contains(name, "_") {
			return fmt.Errorf("config: unauthorized character found in task name: %s", name)
		}
		if task.NumProcs > maxNumProcs {
			return fmt.Errorf("config: too many process: task %s has %d", name, task.NumProcs)
		}
		if task.NumProcs == 0 {
			task.NumProcs = 1
		}

		if task.StopTime <= time.Duration(0) {
			task.StopTime = defaultStopTime
		}

		if task.StartTime <= time.Duration(0) {
			task.StartTime = defaultStartTime
		}

		if task.StartRetries == 0 {
			task.StartRetries = defaultStartRetries
		}
	}

	return nil
}

// Compare returns true if c is the same as d
func (c Config) Compare(d Config) bool {
	if len(c.Tasks) != len(d.Tasks) {
		return false
	}

	for name, taskC := range c.Tasks {
		taskD, exists := d.Tasks[name]
		if !exists {
			return false
		}
		if !taskC.Compare(*taskD) {
			return false
		}
	}
	return true
}
