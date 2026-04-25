package file

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nickqiaoo/tadk/session"
)

// Manager handles JSONL-based session persistence with a tree-structured entry model.
// It is inspired by PI's SessionManager: entries form a linked list via ID/ParentID,
// supporting branching, compaction, and random-access navigation.
//
// File layout:
//
//	{Dir}/{SessionID}.jsonl
//
// First line is a SessionHeader, followed by DurableEntry lines.
type Manager struct {
	mu sync.Mutex

	dir       string
	sessionID string
	cwd       string

	// In-memory state
	header  *session.SessionHeader
	entries []session.DurableEntry
	byID    map[string]session.DurableEntry
	leafID  string

	// File state
	sessionFile string
	flushed     bool
}

// NewManager creates a new session and writes the header to a JSONL file.
func NewManager(dir, cwd string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	sessionID := uuid.NewString()
	fileTimestamp := time.Now().Format("2006-01-02T15-04-05")
	sessionFile := filepath.Join(dir, fmt.Sprintf("%s_%s.jsonl", fileTimestamp, sessionID))

	header := session.NewSessionHeader(sessionID, cwd)

	m := &Manager{
		dir:         dir,
		sessionID:   sessionID,
		cwd:         cwd,
		header:      header,
		entries:     nil,
		byID:        make(map[string]session.DurableEntry),
		sessionFile: sessionFile,
	}

	// Write header
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session header: %w", err)
	}
	if err := os.WriteFile(sessionFile, append(headerJSON, '\n'), 0644); err != nil {
		return nil, fmt.Errorf("failed to write session header: %w", err)
	}
	m.flushed = true

	return m, nil
}

// LoadManager loads an existing session from a JSONL file.
func LoadManager(sessionFile string) (*Manager, error) {
	f, err := os.Open(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer for large lines (e.g., tool results with big output)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	// First line must be the session header
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty session file")
	}

	var header session.SessionHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("failed to parse session header: %w", err)
	}
	if header.Type != session.EntryTypeSession {
		return nil, fmt.Errorf("first line is not a session header (type=%s)", header.Type)
	}

	m := &Manager{
		dir:         filepath.Dir(sessionFile),
		sessionID:   header.ID,
		cwd:         header.Cwd,
		header:      &header,
		byID:        make(map[string]session.DurableEntry),
		sessionFile: sessionFile,
		flushed:     true,
	}

	// Read remaining entries
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		entry, err := session.ParseJSONLLine(line)
		if err != nil {
			// Skip malformed lines gracefully
			continue
		}
		m.entries = append(m.entries, entry)
		if b := entry.Base(); b != nil {
			m.byID[b.ID] = entry
			m.leafID = b.ID
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	return m, nil
}

// SessionID returns the current session's ID.
func (m *Manager) SessionID() string { return m.sessionID }

// SessionFile returns the path to the JSONL session file.
func (m *Manager) SessionFile() string { return m.sessionFile }

// LeafID returns the ID of the current leaf entry (latest entry).
func (m *Manager) LeafID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.leafID
}

// Entries returns a copy of all entries.
func (m *Manager) Entries() []session.DurableEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]session.DurableEntry, len(m.entries))
	copy(out, m.entries)
	return out
}

// AppendMessage appends a MessageEntry as child of the current leaf.
func (m *Manager) AppendMessage(entry *session.MessageEntry) error {
	if entry == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure parentId points to current leaf
	if entry.ParentID == "" {
		entry.ParentID = m.leafID
	}

	return m.appendRaw(entry)
}

// AppendCompaction appends a CompactionEntry.
func (m *Manager) AppendCompaction(entry *session.CompactionEntry) error {
	if entry == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry.ParentID == "" {
		entry.ParentID = m.leafID
	}
	return m.appendRaw(entry)
}

// AppendModelChange appends a ModelChangeEntry.
func (m *Manager) AppendModelChange(provider, modelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := &session.ModelChangeEntry{
		EntryBase: session.NewEntryBase(session.EntryTypeModelChange, m.leafID),
		Provider:  provider,
		ModelID:   modelID,
	}
	return m.appendRaw(entry)
}

// AppendCustom appends a CustomEntry (not sent to LLM).
func (m *Manager) AppendCustom(customType string, data any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := &session.CustomEntry{
		EntryBase:  session.NewEntryBase(session.EntryTypeCustom, m.leafID),
		CustomType: customType,
		Data:       data,
	}
	return m.appendRaw(entry)
}

// BuildContext builds the session context from the current entries.
func (m *Manager) BuildContext() *session.SessionContext {
	m.mu.Lock()
	defer m.mu.Unlock()
	return session.BuildSessionContext(m.entries, m.leafID)
}

// appendRaw appends a durable entry to the in-memory list and persists to JSONL.
func (m *Manager) appendRaw(e session.DurableEntry) error {
	m.entries = append(m.entries, e)
	if b := e.Base(); b != nil {
		m.byID[b.ID] = e
		m.leafID = b.ID
	}
	return m.persist(e)
}

// persist writes a single entry to the JSONL file.
func (m *Manager) persist(e session.DurableEntry) error {
	line, err := session.MarshalJSONLLine(e)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(m.sessionFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}
	return nil
}
