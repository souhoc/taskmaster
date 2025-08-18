package term

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	nameWidth int = 20
)

type CmdHandler func(args ...string) error

func (t *Term) AddCmd(name, description string, handler CmdHandler) {
	t.cmds = append(t.cmds, Cmd{
		name:        name,
		description: description,
		handler:     handler,
	})
}

type Cmd struct {
	name, description string
	handler           CmdHandler
}

func (c Cmd) String() string {
	fill := nameWidth - len(c.name)
	return fmt.Sprintf("%s %s %s",
		c.name,
		strings.Repeat(" ", fill),
		c.description,
	)
}

func (t *Term) defaultHelpHandler(args ...string) error {
	switch len(args) {
	case 1:
		for _, cmd := range t.cmds {
			fmt.Println(cmd)
		}
	case 2:
		idx := slices.IndexFunc(t.cmds, func(cmd Cmd) bool {
			return args[0] == cmd.name
		})
		if idx == -1 {
			return fmt.Errorf("help: command %s not found", args[1])
		}
		fmt.Println(t.cmds[idx])
	default:
		return errors.New("help: too many arguments")
	}

	return nil
}

func (t *Term) defaultExitHandler(...string) error {
	fmt.Print("Пока пока !")
	return Exit
}
