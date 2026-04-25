package runner

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nickqiaoo/tadk/event"
)

type RawAgentEventSSEFormat int

const (
	// RawAgentEventSSEDataOnly writes events as "data: {json}\n\n".
	RawAgentEventSSEDataOnly RawAgentEventSSEFormat = iota
	// RawAgentEventSSENamedEvent writes events as "event: <type>\ndata: {json}\n\n".
	RawAgentEventSSENamedEvent
)

type RawAgentEventSSEOptions struct {
	Format RawAgentEventSSEFormat
}

// WriteRawAgentEventSSE writes one raw AgentEvent as an SSE message.
//
// This helper is for exposing runner.Run's native AgentEvent stream over HTTP.
// UI-message protocols such as Vercel AI SDK live under ui/stream.
func WriteRawAgentEventSSE(w io.Writer, ev event.Event, opts RawAgentEventSSEOptions) error {
	if ev == nil {
		return nil
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}
	if opts.Format == RawAgentEventSSENamedEvent {
		_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.EventType(), payload)
	} else {
		_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	}
	if err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}
	return nil
}
