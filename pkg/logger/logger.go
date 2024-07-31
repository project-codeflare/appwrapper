/*
Copyright 2024 IBM Corporation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logger

import (
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/errors"
)

// logSink implements a filtered log sink
type logSink struct {
	sink logr.LogSink
}

func (l logSink) Init(info logr.RuntimeInfo) {
	l.sink.Init(info)
}

func (l logSink) Enabled(level int) bool {
	return l.sink.Enabled(level)
}
func (l logSink) Info(level int, msg string, keysAndValues ...any) {
	l.sink.Info(level, msg, keysAndValues...)
}

func (l logSink) Error(err error, msg string, keysAndValues ...any) {
	// replace StatusReasonConflict errors with debug messages
	if errors.IsConflict(err) {
		l.sink.Info(1, msg, append(keysAndValues, "error", err.Error())...)
	} else {
		l.sink.Error(err, msg, keysAndValues...)
	}
}

func (l logSink) WithValues(keysAndValues ...any) logr.LogSink {
	return logSink{l.sink.WithValues(keysAndValues...)}
}

func (l logSink) WithName(name string) logr.LogSink {
	return logSink{l.sink.WithName(name)}
}

// FilteredLogger returns a copy of the logger with a filtered sink
func FilteredLogger(logger logr.Logger) logr.Logger {
	return logger.WithSink(logSink{logger.GetSink()})
}
