// Package session defines the canonical session types used throughout the framework.
package session

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

// EntryType identifies the kind of session entry persisted in JSONL.
type EntryType string

const (
	EntryTypeSession       EntryType = "session"        // file header
	EntryTypeMessage       EntryType = "message"        // conversation message
	EntryTypeCompaction    EntryType = "compaction"     // context compression record
	EntryTypeModelChange   EntryType = "model_change"   // model switch record
	EntryTypeBranchSummary EntryType = "branch_summary" // branch summary
	EntryTypeCustom        EntryType = "custom"         // extension data (not sent to LLM)
	EntryTypeCustomMessage EntryType = "custom_message" // extension message (sent to LLM as user message)
	EntryTypeLabel         EntryType = "label"          // bookmark on an entry
	EntryTypeSessionInfo   EntryType = "session_info"   // session metadata
)

// SessionVersion is the current JSONL session format version.
const SessionVersion = 1

// AuthorUser is the canonical author string used for end-user messages in
// persisted session entries. Downstream code (e.g. content-history selection
// in llmagent) compares entry.Author against this literal to distinguish
// user input from agent replies.
const AuthorUser = "user"

// ---------------------------------------------------------------------------
// DurableEntry is the interface for all session entries that can be persisted.
// ---------------------------------------------------------------------------

// DurableEntry is the interface for all session entries that can be persisted.
type DurableEntry interface {
	// Base returns the common fields for this entry.
	Base() *EntryBase
	// EntryType returns the kind of session entry.
	EntryType() EntryType
}

// ---------------------------------------------------------------------------
// Session Header (first line of JSONL file)
// ---------------------------------------------------------------------------

// SessionHeader is the first entry in a JSONL session file.
type SessionHeader struct {
	Type          EntryType `json:"type"` // always "session"
	Version       int       `json:"version"`
	ID            string    `json:"id"`
	Timestamp     string    `json:"timestamp"`
	Cwd           string    `json:"cwd"`
	ParentSession string    `json:"parentSession,omitempty"`
}

// NewSessionHeader creates a new session header.
func NewSessionHeader(id, cwd string) *SessionHeader {
	return &SessionHeader{
		Type:      EntryTypeSession,
		Version:   SessionVersion,
		ID:        id,
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Cwd:       cwd,
	}
}

// ---------------------------------------------------------------------------
// Entry Base (common fields for all non-header entries)
// ---------------------------------------------------------------------------

// EntryBase contains the common fields for all session entries.
// Entries form a tree via ID/ParentID, enabling branching and compaction.
type EntryBase struct {
	Type      EntryType `json:"type"`
	ID        string    `json:"id"`
	ParentID  string    `json:"parentId,omitempty"`
	Timestamp string    `json:"timestamp"`
}

// NewEntryBase creates an EntryBase with a generated ID and current timestamp.
func NewEntryBase(entryType EntryType, parentID string) EntryBase {
	return EntryBase{
		Type:      entryType,
		ID:        uuid.NewString()[:8],
		ParentID:  parentID,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	}
}

// ---------------------------------------------------------------------------
// Message Entry
// ---------------------------------------------------------------------------

// MessageEntry stores a conversation message (user, assistant, or tool).
type MessageEntry struct {
	EntryBase
	Message *message.Message `json:"message"`

	// Metadata for auditing and replay.
	Author       string           `json:"author,omitempty"`
	InvocationID string           `json:"invocationId,omitempty"`
	Branch       string           `json:"branch,omitempty"`
	Model        string           `json:"model,omitempty"`
	Provider     string           `json:"provider,omitempty"`
	Usage        *model.Usage     `json:"usage,omitempty"`
	StopReason   model.StopReason `json:"stopReason,omitempty"`
	ErrorCode    string           `json:"errorCode,omitempty"`
	ErrorMessage string           `json:"errorMessage,omitempty"`
}

// Base returns the common fields for this entry.
func (e *MessageEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *MessageEntry) EntryType() EntryType { return EntryTypeMessage }

// NewMessageLogEntry creates a durable message entry directly.
func NewMessageLogEntry(parentID string, msg *message.Message, author, invocationID, branch string) *MessageEntry {
	if msg == nil {
		return nil
	}
	return &MessageEntry{
		EntryBase:    NewEntryBase(EntryTypeMessage, parentID),
		Message:      msg,
		Author:       author,
		InvocationID: invocationID,
		Branch:       branch,
	}
}

// IsFinalResponse reports whether this entry is the final response of an agent turn.
func (e *MessageEntry) IsFinalResponse() bool {
	if e == nil {
		return true
	}
	if e.Message != nil && e.Message.Role == message.RoleToolResult {
		return false
	}
	if e.Message != nil && e.Message.HasToolCalls() {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Compaction Entry
// ---------------------------------------------------------------------------

// CompactionEntry records a context compression event.
type CompactionEntry struct {
	EntryBase
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int    `json:"tokensBefore"`
}

// Base returns the common fields for this entry.
func (e *CompactionEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *CompactionEntry) EntryType() EntryType { return EntryTypeCompaction }

// ---------------------------------------------------------------------------
// Model Change Entry
// ---------------------------------------------------------------------------

// ModelChangeEntry records a model switch.
type ModelChangeEntry struct {
	EntryBase
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// Base returns the common fields for this entry.
func (e *ModelChangeEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *ModelChangeEntry) EntryType() EntryType { return EntryTypeModelChange }

// ---------------------------------------------------------------------------
// Branch Summary Entry
// ---------------------------------------------------------------------------

// BranchSummaryEntry records a summary when returning from a branch.
type BranchSummaryEntry struct {
	EntryBase
	FromID  string `json:"fromId"`
	Summary string `json:"summary"`
}

// Base returns the common fields for this entry.
func (e *BranchSummaryEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *BranchSummaryEntry) EntryType() EntryType { return EntryTypeBranchSummary }

// ---------------------------------------------------------------------------
// Custom Entry (extension data, NOT sent to LLM)
// ---------------------------------------------------------------------------

// CustomEntry stores extension-specific data that is not sent to the LLM.
type CustomEntry struct {
	EntryBase
	CustomType string `json:"customType"`
	Data       any    `json:"data,omitempty"`
}

// Base returns the common fields for this entry.
func (e *CustomEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *CustomEntry) EntryType() EntryType { return EntryTypeCustom }

// ---------------------------------------------------------------------------
// Custom Message Entry (extension message, IS sent to LLM as user message)
// ---------------------------------------------------------------------------

// CustomMessageEntry stores extension-injected messages that participate in LLM context.
type CustomMessageEntry struct {
	EntryBase
	CustomType string `json:"customType"`
	Content    string `json:"content"`
	Display    bool   `json:"display"`
}

// Base returns the common fields for this entry.
func (e *CustomMessageEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *CustomMessageEntry) EntryType() EntryType { return EntryTypeCustomMessage }

// ---------------------------------------------------------------------------
// Label Entry
// ---------------------------------------------------------------------------

// LabelEntry bookmarks a specific entry.
type LabelEntry struct {
	EntryBase
	TargetID string  `json:"targetId"`
	Label    *string `json:"label"` // nil removes the label
}

// Base returns the common fields for this entry.
func (e *LabelEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *LabelEntry) EntryType() EntryType { return EntryTypeLabel }

// ---------------------------------------------------------------------------
// Session Info Entry
// ---------------------------------------------------------------------------

// SessionInfoEntry stores session-level metadata (e.g., display name).
type SessionInfoEntry struct {
	EntryBase
	Name string `json:"name,omitempty"`
}

// Base returns the common fields for this entry.
func (e *SessionInfoEntry) Base() *EntryBase { return &e.EntryBase }

// EntryType returns the kind of session entry.
func (e *SessionInfoEntry) EntryType() EntryType { return EntryTypeSessionInfo }

// ---------------------------------------------------------------------------
// Session Context builder (analogous to PI's buildSessionContext)
// ---------------------------------------------------------------------------

// SessionContext is the result of building context from session entries.
type SessionContext struct {
	Messages []*message.Message
	Model    *ModelChangeEntry
}

// BuildSessionContext walks the entry tree from leaf to root and constructs
// the list of messages for LLM consumption. It handles compaction entries
// by inserting the summary and skipping compacted entries.
func BuildSessionContext(entries []DurableEntry, leafID string) *SessionContext {
	if len(entries) == 0 {
		return &SessionContext{}
	}

	// Build index
	byID := make(map[string]DurableEntry, len(entries))
	for _, e := range entries {
		if b := e.Base(); b != nil {
			byID[b.ID] = e
		}
	}

	// Find leaf
	var leaf DurableEntry
	if leafID != "" {
		leaf = byID[leafID]
	}
	if leaf == nil {
		leaf = entries[len(entries)-1]
	}

	// Walk from leaf to root
	var path []DurableEntry
	for cur := leaf; cur != nil; {
		path = append(path, cur)
		b := cur.Base()
		if b == nil || b.ParentID == "" {
			break
		}
		cur = byID[b.ParentID]
	}
	slices.Reverse(path)

	// Find last compaction in path
	var compaction *CompactionEntry
	compactionIdx := -1
	for i, e := range path {
		if c, ok := e.(*CompactionEntry); ok {
			compaction = c
			compactionIdx = i
		}
	}

	// Build messages
	ctx := &SessionContext{}
	appendMsg := func(e DurableEntry) {
		switch entry := e.(type) {
		case *MessageEntry:
			if entry.Message != nil {
				ctx.Messages = append(ctx.Messages, entry.Message)
			}
		case *CustomMessageEntry:
			// Convert custom message to user message for LLM
			ctx.Messages = append(ctx.Messages, message.NewUserMessage(entry.Content))
		case *BranchSummaryEntry:
			if entry.Summary != "" {
				ctx.Messages = append(ctx.Messages, message.NewUserMessage(
					fmt.Sprintf("Branch summary: %s", entry.Summary)))
			}
		}
	}

	if compaction != nil && compaction.Summary != "" {
		// Insert compaction summary as first message
		ctx.Messages = append(ctx.Messages, message.NewUserMessage(
			fmt.Sprintf("Previous conversation summary: %s", compaction.Summary)))

		// Emit kept messages (from firstKeptEntryID to compaction) then after compaction
		keepFrom := compaction.FirstKeptEntryID
		keeping := false
		for i, e := range path {
			b := e.Base()
			if b != nil && b.ID == keepFrom {
				keeping = true
			}
			if i == compactionIdx {
				keeping = false
				continue
			}
			if i > compactionIdx {
				appendMsg(e)
			} else if keeping {
				appendMsg(e)
			}
		}
	} else {
		for _, e := range path {
			appendMsg(e)
		}
	}

	// Extract latest model change
	for i := len(path) - 1; i >= 0; i-- {
		if e, ok := path[i].(*ModelChangeEntry); ok && e.Provider != "" {
			ctx.Model = &ModelChangeEntry{
				EntryBase: e.EntryBase,
				Provider:  e.Provider,
				ModelID:   e.ModelID,
			}
			break
		}
	}

	return ctx
}

// ---------------------------------------------------------------------------
// JSONL helpers
// ---------------------------------------------------------------------------

// ParseJSONLLine parses a single JSONL line into a DurableEntry.
func ParseJSONLLine(line []byte) (DurableEntry, error) {
	entry, err := UnmarshalDurableEntry(line)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// UnmarshalDurableEntry deserializes a DurableEntry from JSON bytes.
func UnmarshalDurableEntry(data []byte) (DurableEntry, error) {
	var probe struct {
		Type EntryType `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse entry type: %w", err)
	}

	switch probe.Type {
	case EntryTypeMessage:
		var e MessageEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse message entry: %w", err)
		}
		return &e, nil
	case EntryTypeCompaction:
		var e CompactionEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse compaction entry: %w", err)
		}
		return &e, nil
	case EntryTypeModelChange:
		var e ModelChangeEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse model change entry: %w", err)
		}
		return &e, nil
	case EntryTypeBranchSummary:
		var e BranchSummaryEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse branch summary entry: %w", err)
		}
		return &e, nil
	case EntryTypeCustom:
		var e CustomEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse custom entry: %w", err)
		}
		return &e, nil
	case EntryTypeCustomMessage:
		var e CustomMessageEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse custom message entry: %w", err)
		}
		return &e, nil
	case EntryTypeLabel:
		var e LabelEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse label entry: %w", err)
		}
		return &e, nil
	case EntryTypeSessionInfo:
		var e SessionInfoEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse session info entry: %w", err)
		}
		return &e, nil
	default:
		return nil, fmt.Errorf("unsupported entry type %q", probe.Type)
	}
}

// MarshalJSONLLine serializes any entry type to a JSONL line (no trailing newline).
func MarshalJSONLLine(entry DurableEntry) ([]byte, error) {
	return json.Marshal(entry)
}
