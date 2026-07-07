package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// Level is a log severity threshold.
type Level int

const (
	LevelError Level = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

// ParseLevel parses a log level name.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return LevelError, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "info":
		return LevelInfo, nil
	case "debug":
		return LevelDebug, nil
	default:
		return LevelInfo, fmt.Errorf("logging: unknown level %q (use error, warn, info, debug)", s)
	}
}

func (l Level) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelWarn:
		return "WARN"
	case LevelInfo:
		return "INFO"
	case LevelDebug:
		return "DEBUG"
	default:
		return "INFO"
	}
}

// Field is one key=value log field.
type Field struct {
	Key   string
	Value string
}

// F builds a log field.
func F(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Options configures a Logger.
type Options struct {
	Level   Level
	NoColor bool
	Color   *bool // nil = auto from TTY and NO_COLOR
	Writer  io.Writer
}

// Logger writes structured human-readable logs to stderr by default.
type Logger struct {
	mu        sync.Mutex
	w         io.Writer
	level     Level
	color     bool
	component string
}

var (
	defaultLogger   = New(Options{Level: LevelInfo})
	defaultLoggerMu sync.RWMutex
)

// Default returns the process-wide logger.
func Default() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// SetDefault replaces the process-wide logger.
func SetDefault(l *Logger) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	if l == nil {
		defaultLogger = New(Options{Level: LevelInfo})
		return
	}
	defaultLogger = l
}

// New creates a logger.
func New(opts Options) *Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	color := resolveColor(w, opts.NoColor, opts.Color)
	return &Logger{
		w:     w,
		level: opts.Level,
		color: color,
	}
}

func resolveColor(w io.Writer, noColor bool, force *bool) bool {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	if force != nil {
		return *force
	}
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// WithComponent returns a child logger with a fixed component label.
func (l *Logger) WithComponent(name string) *Logger {
	if l == nil {
		return Default().WithComponent(name)
	}
	return &Logger{
		w:         l.w,
		level:     l.level,
		color:     l.color,
		component: name,
	}
}

// Enabled reports whether messages at level would be emitted.
func (l *Logger) Enabled(level Level) bool {
	if l == nil {
		return level <= LevelInfo
	}
	return level <= l.level
}

func (l *Logger) log(level Level, msg string, fields []Field) {
	if l == nil || !l.Enabled(level) {
		return
	}
	line := l.format(level, msg, fields)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w, line)
}

func (l *Logger) format(level Level, msg string, fields []Field) string {
	ts := time.Now().Format("15:04:05")
	levelLabel := level.String()
	component := l.component

	if l.color {
		levelLabel = colorize(levelColor(level), levelLabel)
		if component != "" {
			component = colorize("\033[1;35m", component)
		}
		msg = colorize("\033[1m", msg)
	}

	var b strings.Builder
	b.WriteString(ts)
	b.WriteByte(' ')
	b.WriteString(levelLabel)
	if component != "" {
		b.WriteByte(' ')
		b.WriteString(component)
	}
	b.WriteByte(' ')
	b.WriteString(msg)
	for _, f := range fields {
		b.WriteByte(' ')
		if l.color {
			b.WriteString(colorize("\033[36m", f.Key))
			b.WriteByte('=')
			b.WriteString(colorize("\033[0m", f.Value))
		} else {
			b.WriteString(f.Key)
			b.WriteByte('=')
			b.WriteString(f.Value)
		}
	}
	return b.String()
}

func levelColor(level Level) string {
	switch level {
	case LevelError:
		return "\033[31m"
	case LevelWarn:
		return "\033[33m"
	case LevelInfo:
		return "\033[32m"
	case LevelDebug:
		return "\033[2m"
	default:
		return "\033[0m"
	}
}

func colorize(code, s string) string {
	return code + s + "\033[0m"
}

// Error logs at error level.
func (l *Logger) Error(msg string, fields ...Field) { l.log(LevelError, msg, fields) }

// Warn logs at warn level.
func (l *Logger) Warn(msg string, fields ...Field) { l.log(LevelWarn, msg, fields) }

// Info logs at info level.
func (l *Logger) Info(msg string, fields ...Field) { l.log(LevelInfo, msg, fields) }

// Debug logs at debug level.
func (l *Logger) Debug(msg string, fields ...Field) { l.log(LevelDebug, msg, fields) }

// Component returns Default() scoped to a component name.
func Component(name string) *Logger {
	return Default().WithComponent(name)
}
