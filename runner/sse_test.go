package runner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
)

func TestWriteRawAgentEventSSENamedEvent(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteRawAgentEventSSE(&buf,
		event.NewMessageEnd(message.NewAssistantMessage("done")).(*event.MessageEnd),
		RawAgentEventSSEOptions{Format: RawAgentEventSSENamedEvent},
	)
	if err != nil {
		t.Fatalf("WriteRawAgentEventSSE() error = %v", err)
	}

	got := buf.String()
	if !strings.HasPrefix(got, "event: message_end\ndata: {") {
		t.Fatalf("output prefix = %q, want named event SSE", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("output suffix = %q, want blank-line terminated SSE event", got)
	}
}

func TestWriteRawAgentEventSSEDataOnly(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteRawAgentEventSSE(&buf,
		event.NewMessageEnd(message.NewAssistantMessage("done")).(*event.MessageEnd),
		RawAgentEventSSEOptions{},
	)
	if err != nil {
		t.Fatalf("WriteRawAgentEventSSE() error = %v", err)
	}

	got := buf.String()
	if !strings.HasPrefix(got, "data: {") {
		t.Fatalf("output prefix = %q, want data-only SSE", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("output suffix = %q, want blank-line terminated SSE event", got)
	}
}
