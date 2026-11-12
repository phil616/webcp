package global

import (
	"fmt"
	"log/slog"
)

type logger struct{}

var LOG logger

func (logger) Warnf(format string, args ...any) { slog.Warn(fmt.Sprintf(format, args...)) }
