package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nickqiaoo/tadk/session"
)

// databaseService is an database implementation of sessionService.Service.
type databaseService struct {
	db *gorm.DB
}

var _ session.Service = (*databaseService)(nil)
var _ session.StoreKindProvider = (*databaseService)(nil)

func (s *databaseService) StoreKind() session.StoreKind {
	return session.StoreKindDatabase
}

// NewSessionService creates a new [session.Service] implementation that uses a
// relational database (e.g., PostgreSQL, Spanner, SQLite) via the GORM library.
//
// It requires a [gorm.Dialector] to specify the database connection and
// accepts optional [gorm.Option] values for further GORM configuration.
//
// It returns the new [session.Service] or an error if the database connection
// [gorm.Open] fails.
func NewSessionService(dialector gorm.Dialector, opts ...gorm.Option) (session.Service, error) {
	db, err := gorm.Open(dialector, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating database session service: %w", err)
	}
	return &databaseService{db: db}, nil
}

// AutoMigrate runs the GORM auto-migration tool to ensure the database schema
// matches the internal storage models (e.g., storageSession, storageEntry).
//
// NOTE: This function relies on a type assertion to the concrete *databaseService
// implementation. It will return an error if the provided session.Service is
// a different implementation.
func AutoMigrate(service session.Service) error {
	dbservice, ok := service.(*databaseService)
	if !ok {
		return fmt.Errorf("invalid session service type")
	}
	err := dbservice.db.AutoMigrate(&storageSession{}, &storageEntry{}, &storageCheckpoint{}, &storageAgentState{}, &storageAppState{}, &storageUserState{})
	if err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}
	return nil
}

// Create generates a session and inserts it to the db, implements session.Service
func (s *databaseService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	if req.AppName == "" || req.UserID == "" {
		return nil, fmt.Errorf("app_name and user_id are required")
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString()
	}

	val := &localSession{
		appName:   req.AppName,
		userID:    req.UserID,
		sessionID: sessionID,
		updatedAt: time.Now(),
	}
	createdSession, err := createStorageSession(val)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(createdSession).Error; err != nil {
			return fmt.Errorf("error creating session on database: %w", err)
		}

		val.updatedAt = createdSession.UpdateTime
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &session.CreateResponse{
		Session: val,
	}, nil
}

// Get retrieves a single session from the database using its composite primary key.
func (s *databaseService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	// Ensure all parts of the composite key are provided.
	appName, userID, sessionID := req.AppName, req.UserID, req.SessionID
	if appName == "" || userID == "" || sessionID == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", appName, userID, sessionID)
	}

	var foundSession storageSession
	err := s.db.WithContext(ctx).
		Where(&storageSession{
			AppName: appName,
			UserID:  userID,
			ID:      sessionID,
		}).
		First(&foundSession).Error
	if err != nil {
		// For any error including ErrRecordNotFound, return it as a system error.
		return nil, fmt.Errorf("database error while fetching session: %w", err)
	}

	// Fetch entries
	entryQuery := s.db.WithContext(ctx).
		Model(&storageEntry{}).
		Where("app_name = ?", appName).
		Where("user_id = ?", userID).
		Where("session_id = ?", sessionID)

	// Apply conditional filters from the request
	if !req.After.IsZero() {
		entryQuery = entryQuery.Where("timestamp >= ?", req.After)
	}

	// Order by timestamp DESC to get the most recent entries when limiting
	entryQuery = entryQuery.Order("timestamp DESC")

	if req.NumRecentEvents > 0 {
		entryQuery = entryQuery.Limit(req.NumRecentEvents)
	}

	var storageEntries []storageEntry
	if err := entryQuery.Find(&storageEntries).Error; err != nil {
		// This is a system failure, not a "not found"
		return nil, fmt.Errorf("database error while fetching entries: %w", err)
	}

	responseSession, err := createSessionFromStorageSession(&foundSession)
	if err != nil {
		return nil, fmt.Errorf("failed to map storage object: %w", err)
	}

	// We fetched in DESC order to get the most recent ones (due to LIMIT).
	// Now we reverse them to be in chronological ASC order for the response.
	// Convert storage rows to response entries.
	responseEntries := make([]session.DurableEntry, 0, len(storageEntries))
	for i := len(storageEntries) - 1; i >= 0; i-- {
		entry, err := createEntryFromStorageEntry(&storageEntries[i])
		if err != nil {
			return nil, fmt.Errorf("failed to map storage entry: %w", err)
		}
		if entry != nil {
			responseEntries = append(responseEntries, entry)
		}
	}
	responseSession.entries = responseEntries

	return &session.GetResponse{
		Session: responseSession,
	}, nil
}

// List retrieves sessions from the database using its appName and optional UserID
func (s *databaseService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	appName, userID := req.AppName, req.UserID
	if appName == "" {
		return nil, fmt.Errorf("app_name is required, got app_name: %q", req.AppName)
	}

	var foundSessions []storageSession
	listQuery := s.db.WithContext(ctx).
		Where(&storageSession{
			AppName: appName,
		})

	if userID != "" {
		listQuery = listQuery.Where(&storageSession{
			UserID: userID,
		})
	}

	err := listQuery.Find(&foundSessions).Error
	if err != nil {
		// Specifically check if the error is "record not found".
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// This is not a system failure. The record simply doesn't exist.
			return &session.ListResponse{
				Sessions: make([]session.Session, 0),
			}, nil
		}
		// For any other error (e.g., connection lost), return it as a system error.
		return nil, fmt.Errorf("database error while fetching session: %w", err)
	}

	// Create response sessions, transform the storageSessions into
	responseSessions := make([]session.Session, 0, len(foundSessions))
	for _, storage := range foundSessions {
		s := storage
		sess, err := createSessionFromStorageSession(&s)
		if err != nil {
			// If we encounter a single mapping error, we fail the whole request.
			return nil, fmt.Errorf("failed to map storage object for session %s: %w", s.ID, err)
		}

		responseSessions = append(responseSessions, sess)
	}

	return &session.ListResponse{
		Sessions: responseSessions,
	}, nil
}

// Delete, deletes a session given a specific id returning error on failure, implements session.Service
func (s *databaseService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	appName, userID, sessionID := req.AppName, req.UserID, req.SessionID
	if appName == "" || userID == "" || sessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q", appName, userID, sessionID)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target := &storageSession{}

		result := tx.Where(&storageSession{
			AppName: req.AppName,
			UserID:  req.UserID,
			ID:      req.SessionID,
		}).Delete(target)

		if result.Error != nil {
			return fmt.Errorf("database error during session deletion: %w", result.Error)
		}

		return nil // Returning nil commits the transaction
	})
}

func (s *databaseService) GetCheckpoint(ctx context.Context, req *session.CheckpointRequest) (*session.Checkpoint, error) {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id are required")
	}

	var stored storageCheckpoint
	err := s.db.WithContext(ctx).
		Where(&storageCheckpoint{AppName: req.AppName, UserID: req.UserID, SessionID: req.SessionID}).
		First(&stored).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("database error while fetching checkpoint: %w", err)
	}

	var checkpoint session.Checkpoint
	if err := json.Unmarshal(stored.Data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}
	checkpoint.Revision = stored.Revision
	return &checkpoint, nil
}

func (s *databaseService) SaveCheckpoint(ctx context.Context, req *session.SaveCheckpointRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required")
	}
	if req.Checkpoint == nil {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stored storageCheckpoint
		err := tx.Where(&storageCheckpoint{AppName: req.AppName, UserID: req.UserID, SessionID: req.SessionID}).
			First(&stored).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("database error while fetching checkpoint: %w", err)
		}

		if req.ExpectedRevision != 0 && stored.Revision != req.ExpectedRevision {
			return fmt.Errorf("checkpoint revision mismatch: got %d, want %d", stored.Revision, req.ExpectedRevision)
		}

		checkpoint := *req.Checkpoint
		if checkpoint.Version == 0 {
			checkpoint.Version = 1
		}
		checkpoint.Revision = stored.Revision + 1
		checkpoint.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		data, err := json.Marshal(&checkpoint)
		if err != nil {
			return fmt.Errorf("failed to marshal checkpoint: %w", err)
		}

		stored.AppName = req.AppName
		stored.UserID = req.UserID
		stored.SessionID = req.SessionID
		stored.Revision = checkpoint.Revision
		stored.Data = data
		stored.UpdateTime = time.Now()
		if err := tx.Save(&stored).Error; err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}
		return nil
	})
}

func (s *databaseService) ClearCheckpoint(ctx context.Context, req *session.CheckpointRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" {
		return fmt.Errorf("app_name, user_id, session_id are required")
	}
	err := s.db.WithContext(ctx).
		Where(&storageCheckpoint{AppName: req.AppName, UserID: req.UserID, SessionID: req.SessionID}).
		Delete(&storageCheckpoint{}).Error
	if err != nil {
		return fmt.Errorf("database error while clearing checkpoint: %w", err)
	}
	return nil
}

func (s *databaseService) GetAgentState(ctx context.Context, req *session.AgentStateRequest) (*session.AgentState, error) {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.AgentAddress == "" {
		return nil, fmt.Errorf("app_name, user_id, session_id, agent_address are required")
	}

	var stored storageAgentState
	err := s.db.WithContext(ctx).
		Where(&storageAgentState{
			AppName:      req.AppName,
			UserID:       req.UserID,
			SessionID:    req.SessionID,
			AgentAddress: req.AgentAddress,
		}).
		First(&stored).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("database error while fetching agent state: %w", err)
	}

	state := &session.AgentState{
		Version:      1,
		AgentAddress: stored.AgentAddress,
		UpdatedAt:    stored.UpdateTime.Format(time.RFC3339Nano),
	}
	if len(stored.Data) > 0 {
		if err := json.Unmarshal(stored.Data, &state.Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent state: %w", err)
		}
	}
	return state, nil
}

func (s *databaseService) SaveAgentState(ctx context.Context, req *session.SaveAgentStateRequest) error {
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

	var raw dynamicJSON
	if req.State.Data != nil {
		data, err := json.Marshal(req.State.Data)
		if err != nil {
			return fmt.Errorf("failed to marshal agent state: %w", err)
		}
		raw = data
	}
	stored := &storageAgentState{
		AppName:      req.AppName,
		UserID:       req.UserID,
		SessionID:    req.SessionID,
		AgentAddress: agentAddress,
		Data:         raw,
		UpdateTime:   time.Now(),
	}
	if err := s.db.WithContext(ctx).Save(stored).Error; err != nil {
		return fmt.Errorf("failed to save agent state: %w", err)
	}
	return nil
}

func (s *databaseService) ClearAgentState(ctx context.Context, req *session.AgentStateRequest) error {
	if req.AppName == "" || req.UserID == "" || req.SessionID == "" || req.AgentAddress == "" {
		return fmt.Errorf("app_name, user_id, session_id, agent_address are required")
	}
	if err := s.db.WithContext(ctx).
		Where(&storageAgentState{
			AppName:      req.AppName,
			UserID:       req.UserID,
			SessionID:    req.SessionID,
			AgentAddress: req.AgentAddress,
		}).
		Delete(&storageAgentState{}).Error; err != nil {
		return fmt.Errorf("database error while clearing agent state: %w", err)
	}
	return nil
}

func (s *databaseService) AppendEntry(ctx context.Context, req *session.AppendEntryRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.Entry == nil {
		return fmt.Errorf("entry is nil")
	}

	b := req.Entry.Base()
	if b == nil {
		return fmt.Errorf("entry has no base")
	}

	entryTimestamp, _ := time.Parse(time.RFC3339Nano, b.Timestamp)
	b.Timestamp = entryTimestamp.Truncate(time.Microsecond).Format(time.RFC3339Nano)

	return s.applyEntry(ctx, req.AppName, req.UserID, req.SessionID, req.Entry, entryTimestamp)
}

func (s *databaseService) applyEntry(ctx context.Context, appName, userID, sessionID string, entry session.DurableEntry, entryTimestamp time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var storageSess storageSession
		err := tx.Where(&storageSession{AppName: appName, UserID: userID, ID: sessionID}).
			First(&storageSess).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("session not found, cannot apply event")
			}
			return fmt.Errorf("failed to get session: %w", err)
		}

		storageEv, err := createStorageEntryByIDs(appName, userID, sessionID, entry)
		if err != nil {
			return fmt.Errorf("failed to map entry to storage model: %w", err)
		}
		if err := tx.Create(storageEv).Error; err != nil {
			return fmt.Errorf("failed to save entry: %w", err)
		}

		result := tx.Model(&storageSession{}).
			Where("app_name = ? AND user_id = ? AND id = ? AND update_time <= ?",
				appName, userID, sessionID, entryTimestamp).
			Update("update_time", entryTimestamp)
		if result.Error != nil {
			return fmt.Errorf("failed to update session state: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("stale session error: session was updated concurrently")
		}
		return nil
	})
}
