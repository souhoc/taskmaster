package term

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"
)

var (
	Exit = errors.New("exit")
)

type Term struct {
	history     []string
	historyIdx  int
	cmds        []Cmd
	input       string
	cursorPos   int
	completions []string

	done chan struct{}
}

func New() *Term {
	t := Term{
		history:    make([]string, 0),
		historyIdx: -1,
		cmds:       make([]Cmd, 0),
		done:       make(chan struct{}),
	}

	t.AddCmd("exit", "End the program.", t.defaultExitHandler)
	t.AddCmd("help", "List commands or get information about a specific one.", t.defaultHelpHandler)

	return &t
}

func (t *Term) SetCompletions(completions ...string) {
	t.completions = completions
}

func (t *Term) Done() <-chan struct{} {
	return t.done
}

func (t *Term) Stop() {
	close(t.done)
}

func (t *Term) clearLine() {
	fmt.Print("\r\033[2K") // Clear entire line
}

func (t *Term) printPrompt() {
	fmt.Printf("taskmaster$ %s", t.input)
}

func (t *Term) refreshLine() {
	t.clearLine()
	t.printPrompt()
}

func (t *Term) addToHistory(cmd string) {
	if cmd != "" && (len(t.history) == 0 || t.history[len(t.history)-1] != cmd) {
		t.history = append(t.history, cmd)
	}
	t.historyIdx = -1
}

func (t *Term) handleArrowKeys(seq []byte) {
	if len(seq) < 3 {
		return
	}

	switch seq[2] {
	case 'A': // Up arrow
		if len(t.history) > 0 {
			if t.historyIdx == -1 {
				t.historyIdx = len(t.history) - 1
			} else if t.historyIdx > 0 {
				t.historyIdx--
			}
			if t.historyIdx >= 0 && t.historyIdx < len(t.history) {
				t.input = t.history[t.historyIdx]
				t.cursorPos = len(t.input)
				t.refreshLine()
			}
		}
	case 'B': // Down arrow
		if len(t.history) > 0 && t.historyIdx != -1 {
			if t.historyIdx < len(t.history)-1 {
				t.historyIdx++
				t.input = t.history[t.historyIdx]
			} else {
				t.historyIdx = -1
				t.input = ""
			}
			t.cursorPos = len(t.input)
			t.refreshLine()
		}
	}
}

func (t *Term) handleInput() (string, error) {
	var buffer [1]byte
	t.input = ""
	t.cursorPos = 0

	for {
		n, err := os.Stdin.Read(buffer[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", io.EOF
		}

		char := buffer[0]

		switch char {
		case ETX:
			fmt.Println()
			t.input = ""
			t.cursorPos = 0
			t.printPrompt()
		case EOT:
			return "", io.EOF
		case HT:
			t.handleCompletion()
		case NL:
			fmt.Println()
			return t.input, nil
		case ESC:
			var seq [3]byte
			seq[0] = char
			n, _ := os.Stdin.Read(seq[1:])
			if n >= 2 {
				t.handleArrowKeys(seq[:])
			}
		case BS, 127:
			if len(t.input) > 0 && t.cursorPos > 0 {
				t.input = t.input[:t.cursorPos-1] + t.input[t.cursorPos:]
				t.cursorPos--
			}
		default:
			if char >= 32 && char <= 126 { // Printable characters
				t.input = t.input[:t.cursorPos] + string(char) + t.input[t.cursorPos:]
				t.cursorPos++
			}
		}
		t.refreshLine()
	}
}

func (t *Term) handleCompletion() {
	if t.input == "" || len(t.completions) == 0 {
		return
	}

	words := strings.Fields(t.input)
	lastWord := words[len(words)-1]

	var matches []string
	for _, cmd := range t.cmds {
		if strings.HasPrefix(cmd.name, lastWord) {
			matches = append(matches, cmd.name)
		}
	}
	for _, completion := range t.completions {
		if strings.HasPrefix(completion, lastWord) {
			matches = append(matches, completion)
		}
	}

	if len(matches) == 1 {
		// Single match - complete it
		completion := matches[0]
		if len(words) == 1 {
			t.input = completion
		} else {
			words[len(words)-1] = completion
			t.input = strings.Join(words, " ")
		}
		t.cursorPos = len(t.input)
	} else if len(matches) > 1 {
		// Multiple matches - show them
		fmt.Println()
		for _, match := range matches {
			fmt.Printf("%s  ", match)
		}
		fmt.Println()
		t.printPrompt()
	}

}

func (t *Term) executeCmd(cmdLine string) error {
	cmdLine = strings.TrimSpace(cmdLine)
	if cmdLine == "" {
		return nil
	}

	args := strings.Fields(cmdLine)
	cmd := args[0]

	idx := slices.IndexFunc(t.cmds, func(c Cmd) bool {
		return c.name == cmd
	})
	if idx == -1 {
		fmt.Printf("unknow command: %s\n", cmd)
		return fmt.Errorf("unknow command: %s", cmd)
	}

	st := time.Now()
	err := t.cmds[idx].handler(args...)
	slog.Info("exec",
		slog.String("cmd", cmd),
		slog.Any("args", args),
		slog.Duration("time", time.Since(st)),
	)

	return err
}

func (t *Term) Run() error {
	fmt.Println("Welcome to taskmaster! Type 'exit' to quit.")

	fd := os.Stdin.Fd()
	oldState, err := MakeRaw(fd)
	if err != nil {
		fmt.Printf("Error setting raw mode: %v\n", err)
		return err
	}
	defer Restore(fd, oldState)

	go func() {
		for {
			t.printPrompt()

			input, err := t.handleInput()
			if err != nil {
				if err == io.EOF {
					t.Stop()
					return
				}
				fmt.Printf("Error reading input: %v\n", err)
				t.Stop()
				return
			}

			if input != "" {
				t.addToHistory(input)
				if err := t.executeCmd(input); err != nil {
					if err == Exit {
						t.Stop()
						return
					}
				}
				t.input = ""
			}
		}
	}()

	<-t.done
	fmt.Println()
	return nil
}

// Termios represents the terminal settings.
type Termios syscall.Termios
