// Copyright 2022 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

// Package log provides a thin wrapper over logrus, with a definition
// of a global root logger, its setup functions and convenience wrappers.
//
// The wrappers are introduced to reduce verbosity:
// - logrus.Fields becomes log.Ctx
// - logrus.WithFields becomes log.F(), defined on a Logger type
//
// The usage scenario in a multilayer app is as follows:
// - a new Logger is created in the upper layer with an initial context (request id, api method...)
// - it is passed to lower layer which automatically includes the context, and can further enrich it
// - result - logs across layers are tied together with a common context
//
// Note on concurrency:
// - all Loggers in fact point to the single base log, which serializes logging with its mutexes
// - all context is copied - each layer operates on an independent copy

package log

import (
	"context"
	"path"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

var (
	// log is a global logger instance
	Log = logrus.New()
)

type loggerContextKeyType int

const (
	loggerContextKey loggerContextKeyType = 0
)

// ContextLogger interface for components which support
// logging with context, via setting a logger to an exisiting one,
// thereby inheriting its context.
type ContextLogger interface {
	UseLog(l *Logger)
}

// init initializes the global logger to sane defaults.
func init() {
	Log.Formatter = &logrus.TextFormatter{
		FullTimestamp: true,
	}
	Log.Level = logrus.InfoLevel
	Log.Hooks.Add(ContextHook{})
	Log.ExitFunc = func(int) {}
}

// Setup allows to override the global logger setup.
func Setup(debug bool) {
	if debug == true {
		Log.Level = logrus.DebugLevel
	}
}

// Ctx short for log context, alias for the more verbose logrus.Fields.
type Ctx map[string]interface{}

// Logger is a wrapper for logrus.Entry.
type Logger struct {
	*logrus.Entry
}

// New returns a new Logger with a given context, derived from the global Log.
func New(ctx Ctx) *Logger {
	return NewFromLogger(Log, ctx)
}

// NewEmpty returns a new logger with empty context
func NewEmpty() *Logger {
	return New(Ctx{})
}

// NewFromLogger returns a new Logger derived from a given logrus.Logger,
// instead of the global one.
func NewFromLogger(log *logrus.Logger, ctx Ctx) *Logger {
	return &Logger{log.WithFields(logrus.Fields(ctx))}
}

// NewFromLogger returns a new Logger derived from a given logrus.Logger,
// instead of the global one.
func NewFromEntry(log *logrus.Entry, ctx Ctx) *Logger {
	return &Logger{log.WithFields(logrus.Fields(ctx))}
}

// F returns a new Logger enriched with new context fields.
// It's a less verbose wrapper over logrus.WithFields.
func (l *Logger) F(ctx Ctx) *Logger {
	return &Logger{l.Entry.WithFields(logrus.Fields(ctx))}
}

func (l *Logger) Level() logrus.Level {
	return l.Entry.Logger.Level
}

type ContextHook struct {
}

func (hook ContextHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (hook ContextHook) Fire(entry *logrus.Entry) error {
	//'skip' = 6 is the default call stack skip, which
	//works ootb when Error(), Warn(), etc. are called
	//for Errorf(), Warnf(), etc. - we have to skip 1 lvl up
	for skip := 6; skip < 8; skip++ {
		if pc, file, line, ok := runtime.Caller(skip); ok {
			funcName := runtime.FuncForPC(pc).Name()

			//detect if we're still in logrus (formatting funcs)
			if !strings.Contains(funcName, "github.com/sirupsen/logrus") {
				entry.Data["file"] = path.Base(file)
				entry.Data["func"] = path.Base(funcName)
				entry.Data["line"] = line
				break
			}
		}
	}

	return nil
}

// Grab an instance of Logger that may have been passed in context.Context.
// Returns the logger or creates a new instance if none was found in ctx. Since
// Logger is based on logrus.Entry, if logger instance from context is any of
// logrus.Logger, logrus.Entry, necessary adaption will be applied.
func FromContext(ctx context.Context) *Logger {
	l := ctx.Value(loggerContextKey)
	if l == nil {
		return New(Ctx{})
	}

	switch v := l.(type) {
	case *Logger:
		return v
	case *logrus.Entry:
		return NewFromEntry(v, Ctx{})
	case *logrus.Logger:
		return NewFromLogger(v, Ctx{})
	default:
		return New(Ctx{})
	}
}

// WithContext adds logger to context `ctx` and returns the resulting context.
func WithContext(ctx context.Context, log *Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, log)
}
