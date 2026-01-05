package compare

import (
	"fmt"
	"io"
	"os"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelNone // Disables all logging
)

// Logger is the interface for structured logging
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
}

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
	Value interface{}
}

// F creates a new logging field
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// defaultLogger is a simple structured logger that writes to stderr
type defaultLogger struct {
	level  LogLevel
	output io.Writer
}

// NewLogger creates a new logger with the specified level
func NewLogger(level LogLevel) Logger {
	return &defaultLogger{
		level:  level,
		output: os.Stderr,
	}
}

// NewLoggerWithOutput creates a logger with custom output
func NewLoggerWithOutput(level LogLevel, output io.Writer) Logger {
	return &defaultLogger{
		level:  level,
		output: output,
	}
}

func (l *defaultLogger) log(level LogLevel, levelStr, msg string, fields ...Field) {
	if l.level > level || l.level == LogLevelNone {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	fieldStr := ""
	for _, f := range fields {
		fieldStr += fmt.Sprintf(" %s=%v", f.Key, f.Value)
	}

	fmt.Fprintf(l.output, "%s [%s] %s%s\n", timestamp, levelStr, msg, fieldStr)
}

func (l *defaultLogger) Debug(msg string, fields ...Field) {
	l.log(LogLevelDebug, "DEBUG", msg, fields...)
}

func (l *defaultLogger) Info(msg string, fields ...Field) {
	l.log(LogLevelInfo, "INFO", msg, fields...)
}

func (l *defaultLogger) Warn(msg string, fields ...Field) {
	l.log(LogLevelWarn, "WARN", msg, fields...)
}

func (l *defaultLogger) Error(msg string, fields ...Field) {
	l.log(LogLevelError, "ERROR", msg, fields...)
}

// NopLogger is a logger that discards all output
type NopLogger struct{}

func (NopLogger) Debug(msg string, fields ...Field) {}
func (NopLogger) Info(msg string, fields ...Field)  {}
func (NopLogger) Warn(msg string, fields ...Field)  {}
func (NopLogger) Error(msg string, fields ...Field) {}
