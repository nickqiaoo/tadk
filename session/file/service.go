// Package file provides a file-based session storage service using JSONL format.
// Each session is a directory containing:
//
//	entries.jsonl              append-only durable entry log
//	checkpoint.json            optional session-level checkpoint (resume state)
//	.lock                      flock(2) advisory lock for single-owner semantics
//	agents/{address}/state.json  optional per-agent state
//
// The directory path itself encodes app/user/session identity; no meta or
// manifest file is needed because those are fully derivable.
package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nickqiaoo/tadk/session"
)

type agentStateFile struct {
	Version      int            `json:"version"`
	AgentAddress string         `json:"agentAddress"`
	Data         map[string]any `json:"data,omitempty"`
	UpdatedAt    string         `json:"updatedAt,omitempty"`
}

// fileSession implements session.Session for file-based JSONL storage.
// A single *fileSession is cached per (app,user,sid) inside fileService so
// readers returned by Get and writers reached by AppendEntry share the same
// entries slice. Entries() always returns a snapshot copy under RLock.
type fileSession struct {
	mu        sync.RWMutex
	appName   string
	userID    string
	sessionID string
	entries   []session.DurableEntry
	updatedAt time.Time
	dir       string
}

func (s *fileSession) ID() string      { return s.sessionID }
func (s *fileSession) AppName() string { return s.appName }
func (s *fileSession) UserID() string  { return s.userID }

func (s *fileSession) LastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

// Entries returns a snapshot of the entry log at call time. The returned slice
// is a freshly allocated copy: later appends on this session do not mutate it.
func (s *fileSession) Entries() session.Entries {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := make(fileEntries, len(s.entries))
	copy(snap, s.entries)
	return snap
}

type fileEntries []session.DurableEntry

func (e fileEntries) All() iter.Seq[session.DurableEntry] {
	return func(yield func(session.DurableEntry) bool) {
		for _, entry := range e {
			if !yield(entry) {
				return
			}
		}
	}
}

func (e fileEntries) Len() int { return len(e) }
func (e fileEntries) At(i int) session.DurableEntry {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

func (s *fileSession) entriesPath() string {
	return filepath.Join(s.dir, "entries.jsonl")
}

// appendEntryJSONL appends a durable entry to the JSONL file.
func (s *fileSession) appendEntryJSONL(entry session.DurableEntry) error {
	if entry == nil {
		return nil
	}

	f, err := os.OpenFile(s.entriesPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open entries file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(entry); err != nil {
		return fmt.Errorf("failed to write session entry: %w", err)
	}
	return nil
}

// loadEntriesJSONL reads all entries from the JSONL file.
func (s *fileSession) loadEntriesJSONL() ([]session.DurableEntry, error) {
	f, err := os.Open(s.entriesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []session.DurableEntry{}, nil
		}
		return nil, fmt.Errorf("failed to open entries file: %w", err)
	}
	defer f.Close()

	var entries []session.DurableEntry
	parentID := ""
	scanner := bufio.NewScanner(f)
	// Increase buffer size for large entries.
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var probe struct {
			Type session.EntryType `json:"type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil, fmt.Errorf("failed to parse entry line: %w", err)
		}
		if probe.Type == "" {
			return nil, fmt.Errorf("unsupported session entry without type")
		}
		entry, err := session.ParseJSONLLine(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse session entry: %w", err)
		}
		if b := entry.Base(); b != nil {
			if b.ParentID == "" {
				b.ParentID = parentID
			}
			parentID = b.ID
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// ServiceConfig configures the file-based session service.
type ServiceConfig struct {
	// Dir is the directory where session files are stored. If empty, defaults
	// to "./sessions".
	Dir string
}

// NewService creates a new file-based session service.
//
// This storage is intended for local single-owner agent runs. Concurrent
// access from a second service instance (same or different process) is
// rejected at lock acquisition time via flock(2). Server and database
// production modes should use a durable database store instead.
func NewService(cfg ServiceConfig) (session.Service, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = "sessions"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	return &fileService{
		dir:      dir,
		sessions: make(map[string]*fileSession),
		locks:    make(map[string]*os.File),
	}, nil
}

type fileService struct {
	dir string

	mu       sync.Mutex
	sessions map[string]*fileSession // sessionDir -> cached session
	locks    map[string]*os.File     // sessionDir -> held .lock fd
}

var _ session.Service = (*fileService)(nil)
var _ session.StoreKindProvider = (*fileService)(nil)

func (s *fileService) StoreKind() session.StoreKind {
	return session.StoreKindFile
}

func (s *fileService) userDir(appName, userID string) string {
	return filepath.Join(s.dir, sanitizeFilename(appName), sanitizeFilename(userID))
}

func (s *fileService) sessionDir(appName, userID, sessionID string) string {
	return filepath.Join(s.userDir(appName, userID), sanitizeFilename(sessionID))
}

func agentDir(sessionDir, agentAddress string) string {
	if agentAddress == "" {
		agentAddress = "root"
	}
	return filepath.Join(sessionDir, "agents", sanitizeFilename(agentAddress))
}

// lockSessionLocked acquires an exclusive advisory lock on sessionDir/.lock.
// It must be called with s.mu held. The fd is kept open in s.locks until
// Close or Delete releases it; flock releases automatically on fd close or
// process exit.
func (s *fileService) lockSessionLocked(sessionDir string) error {
	if _, ok := s.locks[sessionDir]; ok {
		return nil
	}
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}
	lockPath := filepath.Join(sessionDir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open session lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return fmt.Errorf("session %s is locked by another owner: %w", filepath.Base(sessionDir), err)
	}
	s.locks[sessionDir] = f
	return nil
}

// releaseLockLocked releases the flock held on sessionDir, if any.
func (s *fileService) releaseLockLocked(sessionDir string) {
	f, ok := s.locks[sessionDir]
	if !ok {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
	delete(s.locks, sessionDir)
}

// Close releases all locks held by this service. Subsequent calls are no-ops.
func (s *fileService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for dir := range s.locks {
		s.releaseLockLocked(dir)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	return name
}

func (s *fileService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	sessionDir := s.sessionDir(req.AppName, req.UserID, sessionID)
	entriesPath := filepath.Join(sessionDir, "entries.jsonl")

	if _, err := os.Stat(entriesPath); err == nil {
		return nil, fmt.Errorf("session %s already exists", sessionID)
	}
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}
	if err := s.lockSessionLocked(sessionDir); err != nil {
		return nil, err
	}
	if err := os.WriteFile(entriesPath, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create entries file: %w", err)
	}

	fs := &fileSession{
		appName:   req.AppName,
		userID:    req.UserID,
		sessionID: sessionID,
		entries:   []session.DurableEntry{},
		updatedAt: time.Now(),
		dir:       sessionDir,
	}
	s.sessions[sessionDir] = fs

	return &session.CreateResponse{Session: fs}, nil
}

// loadSession returns the cached *fileSession for the given identifiers,
// loading it from disk on first access. Must be called with s.mu held.
func (s *fileService) loadSessionLocked(appName, userID, sessionID string) (*fileSession, error) {
	sessionDir := s.sessionDir(appName, userID, sessionID)
	if fs, ok := s.sessions[sessionDir]; ok {
		return fs, nil
	}
	if _, err := os.Stat(sessionDir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session %s not found", sessionID)
		}
		return nil, fmt.Errorf("failed to stat session directory: %w", err)
	}
	if err := s.lockSessionLocked(sessionDir); err != nil {
		return nil, err
	}
	loader := &fileSession{dir: sessionDir, sessionID: sessionID}
	entries, err := loader.loadEntriesJSONL()
	if err != nil {
		return nil, err
	}

	updatedAt := time.Time{}
	if len(entries) > 0 {
		updatedAt = entryTimestamp(entries[len(entries)-1])
	}
	if updatedAt.IsZero() {
		if info, err := os.Stat(sessionDir); err == nil {
			updatedAt = info.ModTime()
		}
	}

	fs := &fileSession{
		appName:   appName,
		userID:    userID,
		sessionID: sessionID,
		entries:   entries,
		updatedAt: updatedAt,
		dir:       sessionDir,
	}
	s.sessions[sessionDir] = fs
	return fs, nil
}

// requireSessionLocked returns the cached session or an error if the session
// directory does not exist. Unlike loadSessionLocked it does not create an
// empty cache entry; it is used by checkpoint/state endpoints that should not
// materialize a session as a side effect.
func (s *fileService) requireSessionDirLocked(appName, userID, sessionID string) (string, error) {
	sessionDir := s.sessionDir(appName, userID, sessionID)
	if _, ok := s.sessions[sessionDir]; ok {
		if err := s.lockSessionLocked(sessionDir); err != nil {
			return "", err
		}
		return sessionDir, nil
	}
	if _, err := os.Stat(sessionDir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("session %s not found", sessionID)
		}
		return "", fmt.Errorf("failed to stat session directory: %w", err)
	}
	if err := s.lockSessionLocked(sessionDir); err != nil {
		return "", err
	}
	return sessionDir, nil
}

func (s *fileService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fs, err := s.loadSessionLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}

	// Without filters, return the cached live *fileSession directly. Entries()
	// returns a fresh snapshot on each call, so later AppendEntry writes are
	// visible to callers who keep the returned Session.
	if req.NumRecentEvents == 0 && req.After.IsZero() {
		return &session.GetResponse{Session: fs}, nil
	}

	fs.mu.RLock()
	entries := append([]session.DurableEntry(nil), fs.entries...)
	updatedAt := fs.updatedAt
	fs.mu.RUnlock()

	if req.NumRecentEvents > 0 && len(entries) > req.NumRecentEvents {
		entries = entries[len(entries)-req.NumRecentEvents:]
	}
	if !req.After.IsZero() {
		filtered := entries[:0]
		for _, entry := range entries {
			if !entryTimestamp(entry).Before(req.After) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}

	snapshot := &fileSession{
		appName:   fs.appName,
		userID:    fs.userID,
		sessionID: fs.sessionID,
		entries:   entries,
		updatedAt: updatedAt,
		dir:       fs.dir,
	}
	return &session.GetResponse{Session: snapshot}, nil
}

func (s *fileService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	appDir := s.userDir(req.AppName, "")

	sessions := make([]session.Session, 0)
	userDirs, err := os.ReadDir(appDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &session.ListResponse{Sessions: sessions}, nil
		}
		return nil, fmt.Errorf("failed to read app directory: %w", err)
	}

	for _, userDir := range userDirs {
		if !userDir.IsDir() {
			continue
		}
		if req.UserID != "" && userDir.Name() != sanitizeFilename(req.UserID) {
			continue
		}

		userPath := filepath.Join(appDir, userDir.Name())
		sessionDirs, err := os.ReadDir(userPath)
		if err != nil {
			continue
		}

		for _, f := range sessionDirs {
			if !f.IsDir() {
				continue
			}
			sessionDir := filepath.Join(userPath, f.Name())
			if _, err := os.Stat(filepath.Join(sessionDir, "entries.jsonl")); err != nil {
				continue
			}
			if fs, ok := s.sessions[sessionDir]; ok {
				sessions = append(sessions, fs)
				continue
			}
			updatedAt := time.Time{}
			if info, err := os.Stat(sessionDir); err == nil {
				updatedAt = info.ModTime()
			}
			sessions = append(sessions, &fileSession{
				appName:   req.AppName,
				userID:    userDir.Name(),
				sessionID: f.Name(),
				entries:   nil,
				updatedAt: updatedAt,
				dir:       sessionDir,
			})
		}
	}

	return &session.ListResponse{Sessions: sessions}, nil
}

func (s *fileService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir := s.sessionDir(req.AppName, req.UserID, req.SessionID)
	s.releaseLockLocked(sessionDir)
	delete(s.sessions, sessionDir)
	if err := os.RemoveAll(sessionDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove session directory: %w", err)
	}
	return nil
}

func (s *fileService) GetCheckpoint(ctx context.Context, req *session.CheckpointRequest) (*session.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir, err := s.requireSessionDirLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(sessionDir, "checkpoint.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}
	var checkpoint session.Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %w", err)
	}
	return &checkpoint, nil
}

func (s *fileService) SaveCheckpoint(ctx context.Context, req *session.SaveCheckpointRequest) error {
	if req.Checkpoint == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir, err := s.requireSessionDirLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return err
	}

	path := filepath.Join(sessionDir, "checkpoint.json")
	var current session.Checkpoint
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &current); err != nil {
			return fmt.Errorf("failed to parse checkpoint: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read checkpoint: %w", err)
	}
	if req.ExpectedRevision != 0 && current.Revision != req.ExpectedRevision {
		return fmt.Errorf("checkpoint revision mismatch: got %d, want %d", current.Revision, req.ExpectedRevision)
	}

	checkpoint, err := cloneCheckpoint(req.Checkpoint)
	if err != nil {
		return err
	}
	if checkpoint.Version == 0 {
		checkpoint.Version = 1
	}
	checkpoint.Revision = current.Revision + 1
	checkpoint.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	for _, pending := range checkpoint.PendingInterrupts {
		if pending == nil {
			continue
		}
		if pending.AgentAddress == "" {
			pending.AgentAddress = "root"
		}
	}
	return writeJSONAtomic(path, checkpoint)
}

func (s *fileService) ClearCheckpoint(ctx context.Context, req *session.CheckpointRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir, err := s.requireSessionDirLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(sessionDir, "checkpoint.json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear checkpoint: %w", err)
	}
	return nil
}

func (s *fileService) GetAgentState(ctx context.Context, req *session.AgentStateRequest) (*session.AgentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir, err := s.requireSessionDirLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(agentDir(sessionDir, req.AgentAddress), "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read agent state: %w", err)
	}
	var state session.AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse agent state: %w", err)
	}
	return &state, nil
}

func (s *fileService) SaveAgentState(ctx context.Context, req *session.SaveAgentStateRequest) error {
	if req == nil || req.State == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir, err := s.requireSessionDirLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return err
	}

	state := *req.State
	if state.Version == 0 {
		state.Version = 1
	}
	if state.AgentAddress == "" {
		state.AgentAddress = req.AgentAddress
	}
	state.UpdatedAt = time.Now().Format(time.RFC3339Nano)

	dir := agentDir(sessionDir, state.AgentAddress)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create agent state directory: %w", err)
	}
	return writeJSONAtomic(filepath.Join(dir, "state.json"), &agentStateFile{
		Version:      state.Version,
		AgentAddress: state.AgentAddress,
		Data:         state.Data,
		UpdatedAt:    state.UpdatedAt,
	})
}

func (s *fileService) ClearAgentState(ctx context.Context, req *session.AgentStateRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir, err := s.requireSessionDirLocked(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return err
	}
	if req.AgentAddress == "" {
		return nil
	}
	path := filepath.Join(agentDir(sessionDir, req.AgentAddress), "state.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear agent state: %w", err)
	}
	return nil
}

func cloneCheckpoint(checkpoint *session.Checkpoint) (*session.Checkpoint, error) {
	if checkpoint == nil {
		return nil, nil
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal checkpoint: %w", err)
	}
	var clone session.Checkpoint
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}
	return &clone, nil
}

func (s *fileService) AppendEntry(ctx context.Context, req *session.AppendEntryRequest) error {
	if req == nil || req.Entry == nil {
		return nil
	}

	s.mu.Lock()
	fs, err := s.loadSessionLocked(req.AppName, req.UserID, req.SessionID)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to load session for append: %w", err)
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.appendEntryJSONL(req.Entry); err != nil {
		return err
	}
	fs.entries = append(fs.entries, req.Entry)
	if ts := entryTimestamp(req.Entry); !ts.IsZero() {
		fs.updatedAt = ts
	} else {
		fs.updatedAt = time.Now()
	}
	return nil
}

func entryTimestamp(entry session.DurableEntry) time.Time {
	if entry == nil || entry.Base() == nil {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, entry.Base().Timestamp)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}
