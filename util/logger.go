package util

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

type LoggerHandler struct {
	handler slog.Handler
	url     string
	form    url.Values
	mu      *sync.Mutex
	w       io.Writer
}

func (h *LoggerHandler) clone() *LoggerHandler {
	return &LoggerHandler{
		handler: h.handler,
		url:     h.url,
		form:    h.form,
		mu:      h.mu,
		w:       h.w,
	}
}

func NewLogger(whUrl string, w io.Writer) *LoggerHandler {
	form := url.Values{}
	hostname, err := os.Hostname()
	if err != nil {
		form.Set("username", "Taskmaster")
	} else {
		form.Set("username", hostname)
	}

	opts := slog.HandlerOptions{}
	if os.Getenv("DEBUG") == "true" {
		opts.AddSource = true
		opts.Level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(w, &opts)

	return &LoggerHandler{
		handler: handler,
		url:     whUrl,
		form:    form,
		mu:      new(sync.Mutex),
		w:       w,
	}
}

// Enabled checks if the given log level is enabled.
func (h *LoggerHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle processes a log record and sends it to the Discord webhook.
func (h *LoggerHandler) Handle(ctx context.Context, r slog.Record) error {
	var str strings.Builder

	time := r.Time.Format("15:04:05.000")

	fmt.Fprintf(&str, "[%s] %5s: %s ", time, r.Level, r.Message)

	attrs := make([]string, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a.String())
		return true
	})

	str.Write([]byte(strings.Join(attrs, " ")))
	h.mu.Lock()
	fmt.Fprintln(h.w, str.String())
	h.mu.Unlock()

	// First, pass the record to the underlying handler
	// if err := h.handler.Handle(ctx, r); err != nil {
	// 	return err
	// }

	if h.url == "" {
		return nil
	}

	h.form.Set("content", str.String())

	resp, err := http.PostForm(h.url, h.form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// WithAttrs returns a new Handler with the given attributes added.
func (h *LoggerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h.clone()
	h2.handler = h.handler.WithAttrs(attrs)
	return h2
}

// WithGroup returns a new Handler with the given group name added.
func (h *LoggerHandler) WithGroup(name string) slog.Handler {
	h2 := h.clone()
	h2.handler = h.handler.WithGroup(name)
	return h2
}
