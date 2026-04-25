package controllers

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/gorilla/mux"
	semconv "go.opentelemetry.io/otel/semconv/v1.36.0"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/models"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/services"
	"github.com/nickqiaoo/tadk/session"
)

// DebugAPIController is the controller for the Debug API.
type DebugAPIController struct {
	sessionService session.Service
	agentloader    agent.Loader
	debugTelemetry *services.DebugTelemetry
}

// NewDebugAPIController creates the controller for the Debug API.
func NewDebugAPIController(sessionService session.Service, agentLoader agent.Loader, spansExporter *services.DebugTelemetry) *DebugAPIController {
	return &DebugAPIController{
		sessionService: sessionService,
		agentloader:    agentLoader,
		debugTelemetry: spansExporter,
	}
}

// EventSpanHandler returns the debug span for the event.
func (c *DebugAPIController) EventSpanHandler(rw http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	eventID := params["event_id"]
	if eventID == "" {
		http.Error(rw, "event_id parameter is required", http.StatusBadRequest)
		return
	}
	spans := c.debugTelemetry.GetSpansByEventID(eventID)
	key := string(semconv.GenAIOperationNameKey)
	// Return only generate content and execute tool spans.
	wantedOperations := []string{"execute_tool", "generate_content"}
	for _, span := range spans {
		opName := span.Attributes[key]
		if slices.Contains(wantedOperations, opName) {
			response := convertEventSpan(span)
			// Return the first span that matches the wanted operations - single event should contain only a single generate content or execute tool span.
			EncodeJSONResponse(response, http.StatusOK, rw)
			return
		}
	}
	http.Error(rw, fmt.Sprintf("event not found: %s", eventID), http.StatusNotFound)
}

// ADK web expects different format than in [SessionSpansHandler].
// The main difference is that span attributes need to be flattened in the response.
func convertEventSpan(span services.DebugSpan) map[string]any {
	flattened := map[string]any{
		"name":           span.Name,
		"start_time":     span.StartTime,
		"end_time":       span.EndTime,
		"trace_id":       span.TraceID,
		"span_id":        span.SpanID,
		"parent_span_id": span.ParentSpanID,
		"logs":           span.Logs,
	}
	for k, v := range span.Attributes {
		flattened[string(k)] = v
	}
	return flattened
}

// SessionSpansHandler returns the debug spans for the session.
func (c *DebugAPIController) SessionSpansHandler(rw http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	sessionID := params["session_id"]
	if sessionID == "" {
		http.Error(rw, "session_id parameter is required", http.StatusBadRequest)
		return
	}
	spans := c.debugTelemetry.GetSpansBySessionID(sessionID)
	EncodeJSONResponse(spans, http.StatusOK, rw)
}

// EventGraphHandler returns the debug information for the session and session events in form of graph.
func (c *DebugAPIController) EventGraphHandler(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sessionID, err := models.SessionIDFromHTTPParameters(vars)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := c.sessionService.Get(req.Context(), &session.GetRequest{
		AppName:   sessionID.AppName,
		UserID:    sessionID.UserID,
		SessionID: sessionID.ID,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	eventID := vars["event_id"]
	if eventID == "" {
		http.Error(rw, "event_id parameter is required", http.StatusBadRequest)
		return
	}

	var entry session.DurableEntry
	for it := range resp.Session.Entries().All() {
		if b := it.Base(); b != nil && b.ID == eventID {
			entry = it
			break
		}
	}

	if entry == nil {
		http.Error(rw, "event not found", http.StatusNotFound)
		return
	}

	msgEntry, _ := entry.(*session.MessageEntry)
	author := ""
	if msgEntry != nil {
		author = msgEntry.Author
	}

	highlightedPairs := [][]string{}
	fc := functionalCalls(entry)
	fr := functionalResponses(entry)

	if len(fc) > 0 {
		for _, f := range fc {
			if f.Name != "" {
				highlightedPairs = append(highlightedPairs, []string{f.Name, author})
			}
		}
	} else if len(fr) > 0 {
		for _, f := range fr {
			if f.CallID != "" {
				highlightedPairs = append(highlightedPairs, []string{f.CallID, author})
			}
		}
	} else {
		highlightedPairs = append(highlightedPairs, []string{author, author})
	}

	agent, err := c.agentloader.LoadAgent(sessionID.AppName)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	graph, err := services.GetAgentGraph(req.Context(), agent, highlightedPairs)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	EncodeJSONResponse(map[string]string{"dotSrc": graph}, http.StatusOK, rw)
}

func functionalCalls(entry session.DurableEntry) []message.ToolCall {
	msgEntry, ok := entry.(*session.MessageEntry)
	if !ok || msgEntry == nil || msgEntry.Message == nil {
		return nil
	}
	return msgEntry.Message.ToolCalls()
}

func functionalResponses(entry session.DurableEntry) []message.ToolResult {
	msgEntry, ok := entry.(*session.MessageEntry)
	if !ok || msgEntry == nil || msgEntry.Message == nil {
		return nil
	}
	return msgEntry.Message.ToolResults()
}
