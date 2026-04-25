package stream

import (
	"fmt"
	"io"
	"net/http"
)

// WriteHeaders sets the HTTP response headers for the UI Message Stream Protocol (v5+).
// Must be called before writing any Parts.
func WriteHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Vercel-AI-UI-Message-Stream", "v1")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
}

// Writer writes Parts to an io.Writer in SSE format.
// It automatically flushes after each write if the writer supports http.Flusher.
type Writer struct {
	w       io.Writer
	flusher http.Flusher
}

// NewWriter creates a Writer that writes to w.
// If w implements http.Flusher, each write is followed by a flush.
func NewWriter(w io.Writer) *Writer {
	sw := &Writer{w: w}
	if f, ok := w.(http.Flusher); ok {
		sw.flusher = f
	}
	return sw
}

// Write formats and writes a single Part to the underlying writer.
func (sw *Writer) Write(p Part) error {
	formatted, err := p.Format()
	if err != nil {
		return fmt.Errorf("failed to format stream part: %w", err)
	}
	if _, err := fmt.Fprint(sw.w, formatted); err != nil {
		return fmt.Errorf("failed to write stream part: %w", err)
	}
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
	return nil
}

// WriteAll writes multiple Parts sequentially.
func (sw *Writer) WriteAll(parts []Part) error {
	for _, p := range parts {
		if err := sw.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// WriteError is a convenience method that writes an Error Part.
func (sw *Writer) WriteError(msg string) error {
	return sw.Write(Error{ErrorText: msg})
}

// WriteDone writes the stream terminator "data: [DONE]\n\n".
func (sw *Writer) WriteDone() error {
	if _, err := fmt.Fprint(sw.w, Done); err != nil {
		return fmt.Errorf("failed to write done: %w", err)
	}
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
	return nil
}
