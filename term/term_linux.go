//go:build linux

package term

import (
	"syscall"
	"unsafe"
)

// MakeRaw puts the terminal connected to the given file descriptor into raw mode.
func MakeRaw(fd uintptr) (*Termios, error) {
	var oldState Termios
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&oldState)),
	); errno != 0 {
		return nil, errno
	}
	newState := oldState

	// Set raw mode flags
	newState.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	newState.Cflag &^= syscall.CS8
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0

	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TCSETS),
		uintptr(unsafe.Pointer(&newState)),
	); errno != 0 {
		return nil, errno
	}
	return &oldState, nil
}

// Restore restores the terminal connected to the given file descriptor to a previous state.
func Restore(fd uintptr, state *Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(state)))
	if errno != 0 {
		return errno
	}
	return nil
}
