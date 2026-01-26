package logger

import (
	"fmt"

	"k8s.io/klog/v2"
)

type Logger interface {
	Infof(format string, a ...any)
}

type klogLogger struct{}

func (l *klogLogger) Infof(format string, a ...any) {
	klog.InfoDepth(2, fmt.Sprintf(format, a...))
}

func NewKlog() Logger {
	return &klogLogger{}
}
