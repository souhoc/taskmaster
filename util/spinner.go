package util

import (
	"fmt"
	"os"
	"time"
)

type Spinner struct {
	chars []rune
	done  chan struct{}
	f     *os.File
}

// NewSinner return a Spinner.
// If f is nil, it's set to os.Stdout.
func NewSinner(f *os.File) *Spinner {
	if f == nil {
		f = os.Stdout
	}
	return &Spinner{
		chars: []rune{'|', '/', '—', '\\'},
		done:  make(chan struct{}, 1),
		f:     f,
	}
}

func (s *Spinner) Stop() {
	s.done <- struct{}{}
	fmt.Fprintln(s.f, "Done.")
}

func (s *Spinner) Spin(msg string) {
	idx := 0

	for {
		select {
		case <-s.done:
			return
		default:
			fmt.Fprintf(s.f, "\r\033[2K%c %s", s.chars[idx], msg)
			s.f.Sync()
			idx = (idx + 1) % len(s.chars)
			time.Sleep(time.Millisecond * 100)
		}
	}

}
