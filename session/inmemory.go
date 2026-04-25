package session

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"rsc.io/omap"
	"rsc.io/ordered"
)

// inMemoryService is an in-memory implementation of sessionService.Service.
// Thread-safe.
type inMemoryService struct {
	mu          sync.RWMutex
	sessions    omap.Map[string, *session] // session.ID) -> storedSession
	checkpoints map[string]*Checkpoint
	agentStates map[string]map[string]*AgentState
}

var _ StoreKindProvider = (*inMemoryService)(nil)

func (s *inMemoryService) Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	if req.AppName == "" || req.UserID == "" {
		return nil, fmt.Errorf("app_name and user_id are required, got app_name: %q, user_id: %q", req.AppName, req.UserID)
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString()
	}

	key := id{
		appName:   req.AppName,
		userID:    req.UserID,
		sessionID: sessionID,
	}

	encodedKey := key.Encode()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions.Get(encodedKey); ok {
		return nil, fmt.Errorf("session %s already exists", req.SessionID)
	}

	val := &session{
		id:        key,
		updatedAt: time.Now(),
	}

	s.sessions.Set(encodedKey, val)

	copiedSession := copySessionShell(val)
	copiedSession.entries = append(copiedSession.entries, val.entries...)

	return &CreateResponse{
		Session: copiedSession,
	}, nil
}

func (s *inMemoryService) StoreKind() StoreKind {
	return StoreKindMemory
}

func (s *inMemoryService) Get(ctx context.Context, req *GetRequest) (*GetResponse, error) {
	appName, userID, sessionID := req.AppName, req.UserID, req.SessionID
	if appName == "" || userID == "" || sessionID == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", appName, userID, sessionID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	id := id{
		appName:   appName,
		userID:    userID,
		sessionID: sessionID,
	}

	res, ok := s.sessions.Get(id.Encode())
	if !ok {
		return nil, fmt.Errorf("session %+v not found", req.SessionID)
	}

	// When no filters are applied, return the actual stored session so that
	// subsequent AppendEntry calls operate on the live object.
	if req.NumRecentEvents == 0 && req.After.IsZero() {
		return &GetResponse{Session: res}, nil
	}

	copiedSession := copySessionShell(res)

	filteredEntries := res.entries
	if req.NumRecentEvents > 0 {
		start := max(len(filteredEntries)-req.NumRecentEvents, 0)
		// create a new slice header pointing to the same array
		filteredEntries = filteredEntries[start:]
	}
	// apply timestamp filter, assuming list is sorted
	if !req.After.IsZero() && len(filteredEntries) > 0 {
		firstIndexToKeep := sort.Search(len(filteredEntries), func(i int) bool {
			// Find the first event that is not before the timestamp
			return !entryTimestamp(filteredEntries[i]).Before(req.After)
		})
		filteredEntries = filteredEntries[firstIndexToKeep:]
	}

	copiedSession.entries = make([]DurableEntry, 0, len(filteredEntries))
	copiedSession.entries = append(copiedSession.entries, filteredEntries...)

	return &GetResponse{
		Session: copiedSession,
	}, nil
}

func (s *inMemoryService) List(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	appName, userID := req.AppName, req.UserID
	if appName == "" {
		return nil, fmt.Errorf("app_name is required, got app_name: %q", appName)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	lo := id{appName: appName, userID: userID}.Encode()

	var hi string
	if userID == "" {
		hi = id{appName: appName + "\x00"}.Encode()
	} else {
		hi = id{appName: appName, userID: userID + "\x00"}.Encode()
	}

	sessions := make([]Session, 0)
	for k, storedSession := range s.sessions.Scan(lo, hi) {
		var key id
		if err := key.Decode(k); err != nil {
			return nil, fmt.Errorf("failed to decode key: %w", err)
		}

		if key.appName != appName && key.userID != userID {
			break
		}
		copiedSession := copySessionShell(storedSession)
		sessions = append(sessions, copiedSession)
	}
	return &ListResponse{
		Sessions: sessions,
	}, nil
}

func (s *inMemoryService) Delete(ctx context.Context, req *DeleteRequest) error {
	appName, userID, sessionID := req.AppName, req.UserID, req.SessionID
	if appName == "" || userID == "" || sessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", appName, userID, sessionID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := id{
		appName:   appName,
		userID:    userID,
		sessionID: sessionID,
	}

	s.sessions.Delete(id.Encode())
	delete(s.checkpoints, id.Encode())
	return nil
}

func (s *inMemoryService) GetCheckpoint(ctx context.Context, req *CheckpointRequest) (*Checkpoint, error) {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", req.AppName, req.UserID, req.SessionID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := id{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.Encode()
	checkpoint, ok := s.checkpoints[key]
	if !ok {
		return nil, nil
	}
	return cloneCheckpoint(checkpoint)
}

func (s *inMemoryService) GetAgentState(ctx context.Context, req *AgentStateRequest) (*AgentState, error) {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.AgentAddress == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id, agent_address are required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := id{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.Encode()
	byAgent, ok := s.agentStates[key]
	if !ok {
		return nil, nil
	}
	state, ok := byAgent[req.AgentAddress]
	if !ok {
		return nil, nil
	}
	return cloneAgentState(state)
}

func (s *inMemoryService) SaveAgentState(ctx context.Context, req *SaveAgentStateRequest) error {
	if req == nil || req.State == nil {
		return nil
	}
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required")
	}
	agentAddress := req.AgentAddress
	if agentAddress == "" {
		agentAddress = req.State.AgentAddress
	}
	if agentAddress == "" {
		return fmt.Errorf("agent_address is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := id{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.Encode()
	byAgent, ok := s.agentStates[key]
	if !ok {
		byAgent = make(map[string]*AgentState)
		s.agentStates[key] = byAgent
	}
	state, err := cloneAgentState(req.State)
	if err != nil {
		return err
	}
	state.AgentAddress = agentAddress
	if state.Version == 0 {
		state.Version = 1
	}
	state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	byAgent[agentAddress] = state
	return nil
}

func (s *inMemoryService) ClearAgentState(ctx context.Context, req *AgentStateRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.AgentAddress == "" {
		return fmt.Errorf("app_name, user_id, session_id, agent_address are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := id{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.Encode()
	if byAgent, ok := s.agentStates[key]; ok {
		delete(byAgent, req.AgentAddress)
		if len(byAgent) == 0 {
			delete(s.agentStates, key)
		}
	}
	return nil
}

func (s *inMemoryService) SaveCheckpoint(ctx context.Context, req *SaveCheckpointRequest) error {
	if req == nil || req.Checkpoint == nil {
		return fmt.Errorf("checkpoint is required")
	}
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", req.AppName, req.UserID, req.SessionID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := id{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.Encode()
	if _, ok := s.sessions.Get(key); !ok {
		return fmt.Errorf("session %s not found", req.SessionID)
	}
	stored := s.checkpoints[key]
	if req.ExpectedRevision != 0 {
		var storedRevision int64
		if stored != nil {
			storedRevision = stored.Revision
		}
		if storedRevision != req.ExpectedRevision {
			return fmt.Errorf("checkpoint revision mismatch: got %d, want %d", storedRevision, req.ExpectedRevision)
		}
	}

	checkpoint, err := cloneCheckpoint(req.Checkpoint)
	if err != nil {
		return err
	}
	if checkpoint.Version == 0 {
		checkpoint.Version = 1
	}
	if stored != nil {
		checkpoint.Revision = stored.Revision + 1
	} else {
		checkpoint.Revision = 1
	}
	checkpoint.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	s.checkpoints[key] = checkpoint
	return nil
}

func (s *inMemoryService) ClearCheckpoint(ctx context.Context, req *CheckpointRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", req.AppName, req.UserID, req.SessionID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := id{appName: req.AppName, userID: req.UserID, sessionID: req.SessionID}.Encode()
	delete(s.checkpoints, key)
	return nil
}

func (s *inMemoryService) AppendEntry(ctx context.Context, req *AppendEntryRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.Entry == nil {
		return fmt.Errorf("entry is nil")
	}

	key := id{
		appName:   req.AppName,
		userID:    req.UserID,
		sessionID: req.SessionID,
	}.Encode()

	s.mu.Lock()
	defer s.mu.Unlock()

	storedSession, ok := s.sessions.Get(key)
	if !ok {
		return fmt.Errorf("session not found, cannot apply entry")
	}

	storedSession.entries = append(storedSession.entries, req.Entry)
	storedSession.updatedAt = entryTimestamp(req.Entry)
	return nil
}

func (id id) Encode() string {
	return string(ordered.Encode(id.appName, id.userID, id.sessionID))
}

func (id *id) Decode(key string) error {
	return ordered.Decode([]byte(key), &id.appName, &id.userID, &id.sessionID)
}

type id struct {
	appName   string
	userID    string
	sessionID string
}

type session struct {
	id id

	// guards all mutable fields
	mu        sync.RWMutex
	entries   []DurableEntry
	updatedAt time.Time
}

func (s *session) ID() string {
	return s.id.sessionID
}

func (s *session) AppName() string {
	return s.id.appName
}

func (s *session) UserID() string {
	return s.id.userID
}

func (s *session) Entries() Entries {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return entries(s.entries)
}

func (s *session) LastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.updatedAt
}

func (s *session) appendEntry(entry DurableEntry) error {
	s.entries = append(s.entries, entry)
	s.updatedAt = entryTimestamp(entry)
	return nil
}

type entries []DurableEntry

func (e entries) All() iter.Seq[DurableEntry] {
	return func(yield func(DurableEntry) bool) {
		for _, entry := range e {
			if !yield(entry) {
				return
			}
		}
	}
}

func (e entries) Len() int {
	return len(e)
}

func (e entries) At(i int) DurableEntry {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

func entryTimestamp(entry DurableEntry) time.Time {
	if entry == nil {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, entry.Base().Timestamp)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func copySessionShell(sess *session) *session {
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	return &session{
		id: id{
			appName:   sess.id.appName,
			userID:    sess.id.userID,
			sessionID: sess.id.sessionID,
		},
		updatedAt: sess.updatedAt,
	}
}

var _ Service = (*inMemoryService)(nil)

func cloneCheckpoint(checkpoint *Checkpoint) (*Checkpoint, error) {
	if checkpoint == nil {
		return nil, nil
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal checkpoint: %w", err)
	}
	var clone Checkpoint
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}
	return &clone, nil
}

func cloneAgentState(state *AgentState) (*AgentState, error) {
	if state == nil {
		return nil, nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent state: %w", err)
	}
	var clone AgentState
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent state: %w", err)
	}
	return &clone, nil
}
