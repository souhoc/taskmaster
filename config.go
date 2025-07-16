package taskmaster

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

var configPath string

type Config struct {
	Tasks map[string]*Task `yaml:"tasks"`
}

func (c *Config) Init(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)

	flags.StringVar(&configPath, "config", "config.yaml", "Config yaml file path")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if configPath == "" {
		return fmt.Errorf("config file missing")
	}

	f, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer f.Close()

	if err := yaml.NewDecoder(f).Decode(c); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	for name, task := range c.Tasks {
		if task.NumProcs > maxNumProcs {
			return fmt.Errorf("config: too many process: task %s has %d", name, task.NumProcs)
		}
		if task.NumProcs == 0 {
			task.NumProcs = 1
		}
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
