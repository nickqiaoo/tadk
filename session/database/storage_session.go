package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

// storageSession corresponds to the 'sessions' table.
type storageSession struct {
	AppName    string `gorm:"primaryKey;"`
	UserID     string `gorm:"primaryKey;"`
	ID         string `gorm:"primaryKey;"`
	State      map[string]any
	CreateTime time.Time `gorm:"precision:6"`
	UpdateTime time.Time `gorm:"precision:6"`

	// Has-Many relationship: A session has many entries.
	Entries []storageEntry `gorm:"foreignKey:AppName,UserID,SessionID;references:AppName,UserID,ID;constraint:OnDelete:CASCADE"`
}

// TableName explicitly sets the table name for the storageSession struct.
func (storageSession) TableName() string {
	return "sessions"
}

// Helper to map from internal struct to GORM struct
func createStorageSession(s *localSession) (*storageSession, error) {
	return &storageSession{
		UserID:     s.userID,
		AppName:    s.appName,
		ID:         s.sessionID,
		State:      nil,
		CreateTime: time.Now(),
		UpdateTime: time.Now(),
	}, nil
}

// Helper to map from GORM struct to internal struct
func createSessionFromStorageSession(storage *storageSession) (*localSession, error) {
	return &localSession{
		appName:   storage.AppName,
		userID:    storage.UserID,
		sessionID: storage.ID,
		updatedAt: storage.UpdateTime,
	}, nil
}

// storageEntry corresponds to the 'entries' table.
// It stores any DurableEntry in a type + payload schema.
type storageEntry struct {
	ID        string `gorm:"primaryKey;"`
	AppName   string `gorm:"primaryKey;"`
	UserID    string `gorm:"primaryKey;"`
	SessionID string `gorm:"primaryKey;"`

	Type      string
	Timestamp time.Time `gorm:"precision:6"`
	ParentID  string
	Payload   dynamicJSON

	// Belongs-To relationship: An entry belongs to a session.
	Session storageSession `gorm:"foreignKey:AppName,UserID,SessionID;references:AppName,UserID,ID"`
}

// TableName explicitly sets the table name for the storageEntry struct.
func (storageEntry) TableName() string {
	return "entries"
}

// storageCheckpoint stores durable runtime control state for a session.
type storageCheckpoint struct {
	AppName    string `gorm:"primaryKey;"`
	UserID     string `gorm:"primaryKey;"`
	SessionID  string `gorm:"primaryKey;"`
	Revision   int64
	Data       dynamicJSON
	UpdateTime time.Time `gorm:"precision:6"`

	Session storageSession `gorm:"foreignKey:AppName,UserID,SessionID;references:AppName,UserID,ID;constraint:OnDelete:CASCADE"`
}

func (storageCheckpoint) TableName() string {
	return "checkpoints"
}

// storageAgentState stores durable agent-local state for a session.
type storageAgentState struct {
	AppName      string `gorm:"primaryKey;"`
	UserID       string `gorm:"primaryKey;"`
	SessionID    string `gorm:"primaryKey;"`
	AgentAddress string `gorm:"primaryKey;"`
	Data         dynamicJSON
	UpdateTime   time.Time `gorm:"precision:6"`

	Session storageSession `gorm:"foreignKey:AppName,UserID,SessionID;references:AppName,UserID,ID;constraint:OnDelete:CASCADE"`
}

func (storageAgentState) TableName() string {
	return "agent_states"
}

// derefOrZero safely dereferences a pointer, returning the zero value
// of its type if the pointer is nil.
func derefOrZero[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// createStorageEntry translates a DurableEntry into a GORM-compatible storageEntry.
func createStorageEntry(sess session.Session, entry session.DurableEntry) (*storageEntry, error) {
	if sess == nil {
		return nil, fmt.Errorf("session is nil")
	}
	return createStorageEntryByIDs(sess.AppName(), sess.UserID(), sess.ID(), entry)
}

func createStorageEntryByIDs(appName, userID, sessionID string, entry session.DurableEntry) (*storageEntry, error) {
	if entry == nil {
		return nil, fmt.Errorf("entry is nil")
	}

	b := entry.Base()
	if b == nil {
		return nil, fmt.Errorf("entry has no base")
	}

	timestamp, _ := time.Parse(time.RFC3339Nano, b.Timestamp)
	payload, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entry payload: %w", err)
	}

	return &storageEntry{
		ID:        b.ID,
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		Type:      string(entry.EntryType()),
		Timestamp: timestamp,
		ParentID:  b.ParentID,
		Payload:   payload,
	}, nil
}

// createEntryFromStorageEntry translates a GORM storageEntry back into an
// application-level DurableEntry.
func createEntryFromStorageEntry(se *storageEntry) (session.DurableEntry, error) {
	if se == nil || len(se.Payload) == 0 {
		return nil, nil
	}

	entry, err := session.UnmarshalDurableEntry(se.Payload)
	if err == nil {
		return entry, nil
	}

	// For unknown types, try to unmarshal as a MessageEntry for backward compatibility
	// with old Content-based rows.
	var msg *message.Message
	if err := json.Unmarshal(se.Payload, &msg); err == nil && msg != nil {
		return &session.MessageEntry{
			EntryBase: session.EntryBase{
				Type:      session.EntryTypeMessage,
				ID:        se.ID,
				ParentID:  se.ParentID,
				Timestamp: se.Timestamp.Format(time.RFC3339Nano),
			},
			Message: msg,
		}, nil
	}
	return nil, fmt.Errorf("unsupported entry type %q", se.Type)
}

// AppState corresponds to the 'app_states' table.
type storageAppState struct {
	AppName    string `gorm:"primaryKey;"`
	State      stateMap
	UpdateTime time.Time `gorm:"precision:6"`
}

// TableName explicitly sets the table name for the AppState struct.
func (storageAppState) TableName() string {
	return "app_states"
}

// UserState corresponds to the 'user_states' table.
type storageUserState struct {
	AppName    string `gorm:"primaryKey;"`
	UserID     string `gorm:"primaryKey;"`
	State      stateMap
	UpdateTime time.Time `gorm:"precision:6"`
}

// TableName explicitly sets the table name for the UserState struct.
func (storageUserState) TableName() string {
	return "user_states"
}
