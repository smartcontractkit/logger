// Package logger is used to store details of events in the node.
// Events can be categorized by Debug, Info, Error, Fatal, and Panic.
package logger

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"net/url"
	"os"
	"reflect"
	"runtime"
)

const TraceId = "TraceID"

// Logger holds a field for the logger interface.
type logger struct {
	l *zap.SugaredLogger
}

var l *logger

type Logger interface {
	WithSpan(span trace.Span)

	Info(args ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Infof(format string, values ...interface{})

	Debug(args ...interface{})
	Debugw(msg string, keysAndValues ...interface{})
	Debugf(format string, values ...interface{})

	Warn(args ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
	Warnf(format string, values ...interface{})
	WarnIf(err error)

	Error(args ...interface{})
	Errorw(msg string, keysAndValues ...interface{})
	Errorf(format string, values ...interface{})
	ErrorIf(err error, optionalMsg ...string)
	ErrorIfCalling(f func() error, optionalMsg ...string)

	Panic(args ...interface{})
	Panicf(format string, values ...interface{})
	PanicIf(err error)

	Fatal(args ...interface{})
	Fatalf(format string, values ...interface{})

	Sync() error
}

func init() {
	err := zap.RegisterSink("pretty", prettyConsoleSink(os.Stderr))
	if err != nil {
		fatalLineCounter.Inc()
		log.Fatalf("failed to register pretty printer %+v", err)
	}
	err = registerOSSinks()
	if err != nil {
		fatalLineCounter.Inc()
		log.Fatalf("failed to register os specific sinks %+v", err)
	}

	var level zapcore.Level
	err = level.UnmarshalText([]byte(os.Getenv("LOG_LEVEL")))
	if err != nil {
		fatalLineCounter.Inc()
		log.Fatal(err)
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	zl, err := config.Build(zap.AddCallerSkip(1))
	if err != nil {
		fatalLineCounter.Inc()
		log.Fatal(err)
	}

	l = &logger{
		l: zl.Sugar(),
	}
}

func prettyConsoleSink(s zap.Sink) func(*url.URL) (zap.Sink, error) {
	return func(*url.URL) (zap.Sink, error) {
		return PrettyConsole{s}, nil
	}
}

// Write logs a message at the Info level and returns the length
// of the given bytes.
func (log logger) Write(b []byte) (int, error) {
	log.l.Info(string(b))
	return len(b), nil
}

// CreateProductionLogger returns a log config for the passed directory
// with the given LogLevel and customizes stdout for pretty printing.
func CreateProductionLogger(
	dir string, jsonConsole bool, lvl zapcore.Level, toDisk bool) *zap.Logger {
	config := zap.NewProductionConfig()
	if !jsonConsole {
		config.OutputPaths = []string{"pretty://console"}
	}
	if toDisk {
		destination := logFileURI(dir)
		config.OutputPaths = append(config.OutputPaths, destination)
		config.ErrorOutputPaths = append(config.ErrorOutputPaths, destination)
	}
	config.Level.SetLevel(lvl)

	zl, err := config.Build(zap.AddCallerSkip(1))
	if err != nil {
		fatalLineCounter.Inc()
		log.Fatal(err)
	}
	return zl
}

// Infow logs an info message and any additional given information.
func (log logger) Infow(msg string, keysAndValues ...interface{}) {
	log.l.Infow(msg, keysAndValues...)
	infoLineCounter.Inc()
}

func Infow(msg string, keysAndValues ...interface{}) {
	l.Infow(msg, keysAndValues...)
	infoLineCounter.Inc()
}

// Debugw logs a debug message and any additional given information.
func (log logger) Debugw(msg string, keysAndValues ...interface{}) {
	log.l.Debugw(msg, keysAndValues...)
	debugLineCounter.Inc()
}

// Warnw logs a debug message and any additional given information.
func (log logger) Warnw(msg string, keysAndValues ...interface{}) {
	log.l.Warnw(msg, keysAndValues...)
	warnLineCounter.Inc()
}

// Errorw logs an error message, any additional given information, and includes
// stack trace.
func (log logger) Errorw(msg string, keysAndValues ...interface{}) {
	log.l.Errorw(msg, keysAndValues...)
	errorLineCounter.Inc()
}

// Infof formats and then logs the message.
func (log logger) Infof(format string, values ...interface{}) {
	log.l.Info(fmt.Sprintf(format, values...))
	infoLineCounter.Inc()
}

// Debugf formats and then logs the message.
func (log logger) Debugf(format string, values ...interface{}) {
	log.l.Debug(fmt.Sprintf(format, values...))
	debugLineCounter.Inc()
}

// Warnf formats and then logs the message as Warn.
func (log logger) Warnf(format string, values ...interface{}) {
	log.l.Warn(fmt.Sprintf(format, values...))
	warnLineCounter.Inc()
}

// Panicf formats and then logs the message before panicking.
func (log logger) Panicf(format string, values ...interface{}) {
	log.l.Panic(fmt.Sprintf(format, values...))
	panicLineCounter.Inc()
}

// Info logs an info message.
func (log logger) Info(args ...interface{}) {
	log.l.Info(args...)
	infoLineCounter.Inc()
}

// Debug logs a debug message.
func (log logger) Debug(args ...interface{}) {
	log.l.Debug(args...)
	debugLineCounter.Inc()
}

// Warn logs a message at the warn level.
func (log logger) Warn(args ...interface{}) {
	log.l.Warn(args...)
	warnLineCounter.Inc()
}

// Error logs an error message.
func (log logger) Error(args ...interface{}) {
	log.l.Error(args...)
	errorLineCounter.Inc()
}

// WarnIf logs the error if present.
func (log logger) WarnIf(err error) {
	if err != nil {
		log.l.Warn(err)
		warnLineCounter.Inc()
	}
}

// ErrorIf logs the error if present.
func (log logger) ErrorIf(err error, optionalMsg ...string) {
	if err != nil {
		if len(optionalMsg) > 0 {
			log.l.Error(errors.Wrap(err, optionalMsg[0]))
		} else {
			log.l.Error(err)
		}
		errorLineCounter.Inc()
	}
}

// ErrorIfCalling calls the given function and logs the error of it if there is.
func (log logger) ErrorIfCalling(f func() error, optionalMsg ...string) {
	err := f()
	if err != nil {
		e := errors.Wrap(err, runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name())
		if len(optionalMsg) > 0 {
			log.l.Error(errors.Wrap(e, optionalMsg[0]))
		} else {
			log.l.Error(e)
		}
		errorLineCounter.Inc()
	}
}

// PanicIf logs the error if present.
func (log logger) PanicIf(err error) {
	if err != nil {
		log.l.Panic(err)
	}
}

// Fatal logs a fatal message then exits the application.
func (log logger) Fatal(args ...interface{}) {
	fatalLineCounter.Inc()
	log.l.Fatal(args...)
}

// Errorf logs a message at the error level using Sprintf.
func (log logger) Errorf(format string, values ...interface{}) {
	log.l.Error(fmt.Sprintf(format, values...))
	errorLineCounter.Inc()
}

// Fatalf logs a message at the fatal level using Sprintf.
func (log logger) Fatalf(format string, values ...interface{}) {
	log.l.Fatal(fmt.Sprintf(format, values...))
	fatalLineCounter.Inc()
}

// Panic logs a panic message then panics.
func (log logger) Panic(args ...interface{}) {
	log.l.Panic(args...)
	panicLineCounter.Inc()
}

// WithSpan adds span to the log message
func (log logger) WithSpan(span trace.Span) {
	if span != nil {
		log.l.With(TraceId, span.SpanContext().TraceID().String())
	}
}

// Sync flushes any buffered log entries.
func (log logger) Sync() error {
	return log.l.Sync()
}

var (
	lineCounter = promauto.NewCounterVec(prometheus.CounterOpts{Name: "log_lines_total"}, []string{"level"})

	debugLineCounter  = lineCounter.WithLabelValues(zapcore.DebugLevel.String())
	infoLineCounter   = lineCounter.WithLabelValues(zapcore.InfoLevel.String())
	warnLineCounter   = lineCounter.WithLabelValues(zapcore.WarnLevel.String())
	errorLineCounter  = lineCounter.WithLabelValues(zapcore.ErrorLevel.String())
	dPanicLineCounter = lineCounter.WithLabelValues(zapcore.DPanicLevel.String())
	panicLineCounter  = lineCounter.WithLabelValues(zapcore.PanicLevel.String())
	fatalLineCounter  = lineCounter.WithLabelValues(zapcore.FatalLevel.String())
)
