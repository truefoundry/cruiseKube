package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/truefoundry/cruiseKube/pkg/contextutils"
)

var defaultLogger *slog.Logger

func init() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)
}

// Convert args to slog attributes (key-value pairs)
func argsToAttrs(args ...any) []slog.Attr {
	var attrs []slog.Attr
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key := fmt.Sprint(args[i])
			value := args[i+1]
			attrs = append(attrs, slog.Any(key, value))
		}
	}
	return attrs
}

// Structured logging functions with context
func Info(ctx context.Context, msg string, args ...any) {
	attrs := getContextualAttrs(ctx)
	allAttrs := make([]slog.Attr, 0, len(attrs)+len(args)/2)
	allAttrs = append(allAttrs, attrs...)
	allAttrs = append(allAttrs, argsToAttrs(args...)...)
	defaultLogger.LogAttrs(ctx, slog.LevelInfo, msg, allAttrs...)
}

func Error(ctx context.Context, msg string, args ...any) {
	attrs := getContextualAttrs(ctx)
	allAttrs := make([]slog.Attr, 0, len(attrs)+len(args)/2)
	allAttrs = append(allAttrs, attrs...)
	allAttrs = append(allAttrs, argsToAttrs(args...)...)
	defaultLogger.LogAttrs(ctx, slog.LevelError, msg, allAttrs...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	attrs := getContextualAttrs(ctx)
	allAttrs := make([]slog.Attr, 0, len(attrs)+len(args)/2)
	allAttrs = append(allAttrs, attrs...)
	allAttrs = append(allAttrs, argsToAttrs(args...)...)
	defaultLogger.LogAttrs(ctx, slog.LevelWarn, msg, allAttrs...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	attrs := getContextualAttrs(ctx)
	allAttrs := make([]slog.Attr, 0, len(attrs)+len(args)/2)
	allAttrs = append(allAttrs, attrs...)
	allAttrs = append(allAttrs, argsToAttrs(args...)...)
	defaultLogger.LogAttrs(ctx, slog.LevelDebug, msg, allAttrs...)
}

// Printf-style logging functions
func Infof(ctx context.Context, format string, args ...any) {
	attrs := getContextualAttrs(ctx)
	msg := fmt.Sprintf(format, args...)
	defaultLogger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func Errorf(ctx context.Context, format string, args ...any) {
	attrs := getContextualAttrs(ctx)
	msg := fmt.Sprintf(format, args...)
	defaultLogger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

func Warnf(ctx context.Context, format string, args ...any) {
	attrs := getContextualAttrs(ctx)
	msg := fmt.Sprintf(format, args...)
	defaultLogger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

func Debugf(ctx context.Context, format string, args ...any) {
	attrs := getContextualAttrs(ctx)
	msg := fmt.Sprintf(format, args...)
	defaultLogger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Compatibility functions for existing log package usage
func Printf(ctx context.Context, format string, args ...any) {
	Infof(ctx, format, args...)
}

func Println(ctx context.Context, args ...any) {
	attrs := getContextualAttrs(ctx)
	msg := fmt.Sprint(args...)
	defaultLogger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func Fatal(ctx context.Context, msg string, args ...any) {
	Error(ctx, msg, args...)
	os.Exit(1)
}

func Fatalf(ctx context.Context, format string, args ...any) {
	Errorf(ctx, format, args...)
	os.Exit(1)
}

// Configuration functions
func SetLevel(level slog.Level) {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)
}

func SetTextOutput() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)
}

func getContextualAttrs(ctx context.Context) []slog.Attr {
	var attrs []slog.Attr
	for k, v := range contextutils.GetAttributes(ctx) {
		attrs = append(attrs, slog.String(k, v))
	}
	return attrs
}
