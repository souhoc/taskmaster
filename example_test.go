package taskmaster_test

import (
	"fmt"
	"os"

	"github.com/souhoc/taskmaster"
)

func Example() {
	var cfg taskmaster.Config
	if err := cfg.Init(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	service := taskmaster.New(&cfg, taskmaster.WithOutFile(os.Stdout))
	defer service.Close()
}
