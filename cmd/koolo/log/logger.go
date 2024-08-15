package log

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/hectorgimenez/koolo/internal/event"
)

var logFileHandler *os.File

type eventHandler struct {
	slog.Handler
	supervisor string
}

func (h *eventHandler) Handle(ctx context.Context, r slog.Record) error {
	event.Send(event.OnLog(event.Text(h.supervisor, "log"), r.Message, int(r.Level)))

	// Call the embedded handler's Handle method
	err := h.Handler.Handle(ctx, r)
	if err != nil {
		return err
	}

	return nil
}

func NewLogger(debug bool, logDir, supervisor string) (*slog.Logger, error) {
	return createLogger(debug, logDir, supervisor, false)
}

func NewEventLogger(debug bool, logDir, supervisor string) (*slog.Logger, error) {
	return createLogger(debug, logDir, supervisor, true)
}

func createLogger(debug bool, logDir, supervisor string, withEvents bool) (*slog.Logger, error) {
	if logDir == "" {
		logDir = "logs"
	}

	if _, err := os.Stat(logDir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(logDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("error creating log directory: %w", err)
		}
	}

	fileName := "koolo-log-" + time.Now().Format("2006-01-02-15-04-05") + ".txt"
	if supervisor != "" {
		fileName = fmt.Sprintf("koolo-log-%s-%s.txt", supervisor, time.Now().Format("2006-01-02-15-04-05"))
	}

	lfh, err := os.Create(logDir + "/" + fileName)
	if err != nil {
		return nil, err
	}
	logFileHandler = lfh

	level := slog.LevelDebug
	if !debug {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key != slog.TimeKey {
				return a
			}

			t := a.Value.Time()
			a.Value = slog.StringValue(t.Format(time.TimeOnly))

			return a
		},
	}

	baseHandler := slog.NewTextHandler(io.MultiWriter(logFileHandler, os.Stdout), opts)

	if withEvents {
		eventHandler := &eventHandler{
			Handler:    baseHandler,
			supervisor: supervisor,
		}
		return slog.New(eventHandler), nil
	}

	return slog.New(baseHandler), nil
}

func FlushLog() error {
	if logFileHandler != nil {
		logFileHandler.Sync()
		return logFileHandler.Close()
	}
	return nil
}
