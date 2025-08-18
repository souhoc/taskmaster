package term

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"syscall"
	"unsafe"
)

var (
	Exit = errors.New("exit")
)

type Term struct {
	history    []string
	historyIdx int
	cmds       []Cmd
	input      string
	cursorPos  int

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
	if t.input == "" {
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
		fmt.Printf("Error: unknow command: %s\n", cmd)
		return fmt.Errorf("unknow command: %s", cmd)
	}

	log.Printf("term: cmd: %s %v\n", cmd, args)

	return t.cmds[idx].handler(args...)
}

func (t *Term) Run() error {
	log.Println("term: running...")
	fmt.Println("Welcome to taskmaster! Type 'exit' to quit.")

	fd := os.Stdin.Fd()
	oldState, err := MakeRaw(fd)
	if err != nil {
		fmt.Printf("Error setting raw mode: %v\n", err)
		return err
	}
	defer Restore(fd, oldState)
	defer log.Println("term: exit")

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
					log.Printf("Error: %v\n", err)
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

// MakeRaw puts the terminal connected to the given file descriptor into raw mode.
func MakeRaw(fd uintptr) (*Termios, error) {
	var oldState Termios
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCGETA,
		uintptr(unsafe.Pointer(&oldState)),
	); err != 0 {
		return nil, err
	}

	newState := oldState

	// Set raw mode flags
	newState.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	newState.Cflag &^= syscall.CS8
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0

	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCSETA,
		uintptr(unsafe.Pointer(&newState)),
	); err != 0 {
		return nil, err
	}

	return &oldState, nil
}

// Restore restores the terminal connected to the given file descriptor to a previous state.
func Restore(fd uintptr, state *Termios) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(state)))
	if err != 0 {
		return err
	}
	return nil
}
