//go:build darwin

package term

import (
	"syscall"
	"unsafe"
)

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
