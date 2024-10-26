// Package goLogger procides a logger that writes logs to the provided writer
// and sends logs to the provided log targets.
// The implemented log traget sends logs to New Relic.
package goLogger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

// A LogTarget is an interface that can be implemented to send logs to a specific target.
// For example, a LogTarget can be implemented to send logs to New Relic.
type LogTarget interface {
	SendLog(ctx context.Context, r slog.Record) error
}

// New creates a new logger that writes logs to the provided writer and sends logs to the provided log targets.
// If the writer is nil, logs will be written to os.Stdout.
func New(writer io.Writer, logTargets ...LogTarget) *slog.Logger {
	if writer == nil {
		writer = os.Stdout
	}
	jsonHandler := slog.NewJSONHandler(writer, &slog.HandlerOptions{AddSource: true})
	errorChannel := make(chan error)
	handler := &customHandler{handler: jsonHandler, logTargets: logTargets, errorChannel: errorChannel}
	logger := slog.New(handler)
	tLogCounter := newTargetLogCounter()
	go func() {
		for err := range errorChannel {
			if tLogCounter.get() > 5 {
				continue
			}
			if err != nil {
				logger.Error("Error in custom handler", "error", err)
				tLogCounter.increment()
			}
		}
	}()
	return logger
}

// A customHandler is a slog.Handler that sends logs to the provided log targets.
type customHandler struct {
	handler      slog.Handler
	logTargets   []LogTarget
	errorChannel chan error
}

// Enabled returns true if the provided log level is enabled.
func (h *customHandler) Enabled(context context.Context, level slog.Level) bool {
	return h.handler.Enabled(context, level)
}

// Handle sends the provided log record to the log targets.
func (h *customHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.logTargets != nil {
		for _, target := range h.logTargets {
			go func() {
				err := target.SendLog(ctx, r)
				if err != nil {
					h.errorChannel <- err
				}
			}()
		}
	}
	return h.handler.Handle(ctx, r)
}

// WithAttrs returns a new customHandler with the provided attributes.
func (h *customHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &customHandler{
		handler:      h.handler.WithAttrs(attrs),
		logTargets:   h.logTargets,
		errorChannel: h.errorChannel,
	}
}

// WithGroup returns a new customHandler with the provided group name.
func (h *customHandler) WithGroup(name string) slog.Handler {
	return &customHandler{
		handler:      h.handler.WithGroup(name),
		logTargets:   h.logTargets,
		errorChannel: h.errorChannel,
	}
}

// Handler returns the slog.Handler.
func (h *customHandler) Handler() slog.Handler {
	return h.handler
}

// A newRelicLogger is a LogTarget that sends logs to New Relic.
type newRelicLogger struct {
	newRelicEndpoint   string
	newRelicLicenseKey string
	client             *http.Client
}

// NewNewRelicLogger creates a new newRelicLogger.
func NewNewRelicLogger(newRelicEndpoint, newRelicLicenseKey string) LogTarget {
	return &newRelicLogger{
		newRelicEndpoint:   newRelicEndpoint,
		newRelicLicenseKey: newRelicLicenseKey,
		client:             &http.Client{},
	}
}

// SendLog sends the provided log record to New Relic.
func (l *newRelicLogger) SendLog(ctx context.Context, r slog.Record) error {
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()
	fields := make(map[string]interface{}, r.NumAttrs())
	fields["time"] = r.Time
	fields["message"] = r.Message
	fields["level"] = r.Level.String()
	fields["timestamp"] = r.Time.Unix()
	fields["logtype"] = "application"
	fields["source"] = map[string]interface{}{
		"function": f.Function,
		"line":     f.Line,
	}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})
	req, err := http.NewRequest(http.MethodPost, l.newRelicEndpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", l.newRelicLicenseKey)
	jsonFields, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(jsonFields))
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

// A targetLogCounter is a counter that keeps track of the number of logs created by errors in the log targets.
// If for example the log target is not reachable, the counter will increment.
// If the threshold is reached the logger will stop sending logs to the log targets.
// This prevents infinite loops of logs being created by errors in the log targets.
type targetLogCounter struct {
	counter int
	mu      sync.RWMutex
}

// newTargetLogCounter creates a new targetLogCounter.
func newTargetLogCounter() *targetLogCounter {
	tlc := &targetLogCounter{0, sync.RWMutex{}}
	go func() {
		for {
			if tlc.get() > 0 {
				tlc.decrement()
			}
			time.Sleep(1 * time.Second)
		}
	}()
	return tlc
}

// increment increments the counter.
func (c *targetLogCounter) increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
}

// decrement decrements the counter.
func (c *targetLogCounter) decrement() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter--
}

// get returns the counter.
func (c *targetLogCounter) get() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.counter
}
