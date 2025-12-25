package logging

import (
	"context"
	"io"
)

type contextKey string

const outputWriterKey contextKey = "job_output_writer"

// GetWriter retrieves the job-specific output writer from context.
// It falls back to the global logger output if no writer is found.
func GetWriter(ctx context.Context) io.Writer {
	if writer, ok := ctx.Value(outputWriterKey).(io.Writer); ok && writer != nil {
		return writer
	}
	return GetGlobalOutput()
}

// WithWriter returns a new context with the job-specific output writer attached.
func WithWriter(ctx context.Context, writer io.Writer) context.Context {
	return context.WithValue(ctx, outputWriterKey, writer)
}
