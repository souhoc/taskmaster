package taskmaster

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Tasks map[string]TaskConfig `yaml:"tasks"`
}

func (c *Config) Init(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)

	configPath := flags.String("config", "", "Config yaml file path, if precised, overwrite flags")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *configPath == "" {
		return fmt.Errorf("Missing config")
	}

	f, err := os.Open(*configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := yaml.NewDecoder(f).Decode(c); err != nil {
		return err
	}
	return nil
}

type TaskConfig struct {
	Cmd          string            `yaml:"cmd"`
	Args         []string          `yaml:"args"`
	NumProcs     int               `yaml:"numprocs"`
	Umask        os.FileMode       `yaml:"umask"`
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
