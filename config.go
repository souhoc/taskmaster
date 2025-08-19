package taskmaster

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/souhoc/taskmaster/util"
	"gopkg.in/yaml.v3"
)

var configPath string

var paths []string = []string{"config.yaml"}

type Config struct {
	Webhook    string           `yaml:"webhook"`
	DropToUser string           `yaml:"dropToUser"`
	Tasks      map[string]*Task `yaml:"tasks"`
}

func (c *Config) Init(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)

	dir, err := os.UserConfigDir()
	if err == nil {
		dir = filepath.Join(dir, "taskmaster")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Printf("can't create user cache dir: %v", err)
	}
	paths = append(paths, filepath.Join(dir, "config.yaml"))

	flags.StringVar(&configPath, "config", "", "Config yaml file path. (On Unix systems, it returns $XDG_CONFIG_HOME as specified by https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html if non-empty, else $HOME/.config. On Darwin, it returns $HOME/Library/Application Support. On Windows, it returns %AppData%. On Plan 9, it returns $home/lib).")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	return c.Load()
}

func (c *Config) Load() error {
	if configPath != "" {
		_, err := os.Stat(configPath)
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
	} else {
		for _, path := range paths {
			_, err := os.Stat(path)
			if err == nil {
				configPath = path
				break
			}
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

	// Verify the Webhook
	if c.Webhook != "" {
		wh := util.Webhook{Url: c.Webhook, Username: "test"}
		if err := wh.Send("test valid webhook"); err != nil {
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
			task.StopTime = defaultTimeout
		}

		if task.StartRetries == 0 {
			task.StartRetries = defaultStartRetries
		}

		task.done = make(chan error, 1)
	}

	return nil
}

// Reload should not be called outside of service unless you know what you are doing.
// It would mess with the mutex.
func (c *Config) Reload() (old *Config, err error) {
	if configPath == "" {
		return nil, fmt.Errorf("config: file missing")
	}

	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	defer f.Close()

	var newCfg Config
	if err := yaml.NewDecoder(f).Decode(&newCfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if !c.Compare(newCfg) {
		old = new(Config)
		*old = *c
		*c = newCfg
		return old, nil
	}

	return nil, nil
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
