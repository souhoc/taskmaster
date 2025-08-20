package util

import (
	"fmt"
	"os"
)

type Logger struct {
	wh   Webhook
	file *os.File
}

func NewLogger(whUrl string, file *os.File) *Logger {
	var wh Webhook
	hostname, err := os.Hostname()
	if err != nil {
		wh.Username = "Taskmaster"
	} else {
		wh.Username = hostname
	}
	wh.Url = whUrl

	return &Logger{
		wh:   wh,
		file: file,
	}
}

func (l Logger) Write(b []byte) (n int, err error) {
	if l.wh.Url != "" {
		if err = l.wh.Send(string(b)); err != nil {
			return
		}
	}

	n, err = fmt.Fprint(l.file, string(b))
	return
}
