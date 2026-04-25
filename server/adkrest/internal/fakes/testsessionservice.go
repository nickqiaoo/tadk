// Package fakes contains a fake implementation of different ADK services used for testing

package fakes

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/nickqiaoo/tadk/session"
)

type TestEntries []session.DurableEntry

func (e TestEntries) All() iter.Seq[session.DurableEntry] {
	return func(yield func(session.DurableEntry) bool) {
		for _, entry := range e {
			if !yield(entry) {
				return
			}
		}
	}
}

func (e TestEntries) Len() int {
	return len(e)
}

func (e TestEntries) At(i int) session.DurableEntry {
	return e[i]
}

type TestSession struct {
	Id             SessionKey
	SessionEntries TestEntries
	UpdatedAt      time.Time
}

func (s TestSession) ID() string {
	return s.Id.SessionID
}

func (s TestSession) AppName() string {
	return s.Id.AppName
}

func (s TestSession) UserID() string {
	return s.Id.UserID
}

func (s TestSession) Entries() session.Entries {
	return s.SessionEntries
}

func (s TestSession) LastUpdateTime() time.Time {
	return s.UpdatedAt
}

type FakeSessionService struct {
	Sessions map[SessionKey]*TestSession
}

type SessionKey struct {
	AppName   string
	UserID    string
	SessionID string
}

func (s *FakeSessionService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	if _, ok := s.Sessions[SessionKey{AppName: req.AppName, UserID: req.UserID, SessionID: req.SessionID}]; ok {
		return nil, fmt.Errorf("session already exists")
	}

	if req.SessionID == "" {
		req.SessionID = "testID"
	}

	testSession := TestSession{
		Id: SessionKey{
			AppName:   req.AppName,
			UserID:    req.UserID,
			SessionID: req.SessionID,
		},
		UpdatedAt: time.Now(),
	}
	s.Sessions[SessionKey{
		AppName:   req.AppName,
		UserID:    req.UserID,
		SessionID: req.SessionID,
	}] = &testSession
	return &session.CreateResponse{
		Session: &testSession,
	}, nil
}

func (s *FakeSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	if sess, ok := s.Sessions[SessionKey{
		AppName:   req.AppName,
		UserID:    req.UserID,
		SessionID: req.SessionID,
	}]; ok {
		return &session.GetResponse{
			Session: sess,
		}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *FakeSessionService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	result := []session.Session{}
	for _, sess := range s.Sessions {
		if sess.Id.AppName != req.AppName || sess.Id.UserID != req.UserID {
			continue
		}
		result = append(result, sess)
	}
	return &session.ListResponse{
		Sessions: result,
	}, nil
}

func (s *FakeSessionService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	id := SessionKey{
		AppName:   req.AppName,
		UserID:    req.UserID,
		SessionID: req.SessionID,
	}
	if _, ok := s.Sessions[id]; !ok {
		return fmt.Errorf("not found")
	}
	delete(s.Sessions, id)
	return nil
}

func (s *FakeSessionService) AppendEntry(ctx context.Context, req *session.AppendEntryRequest) error {
	if req == nil || req.Entry == nil {
		return nil
	}
	key := SessionKey{
		AppName:   req.AppName,
		UserID:    req.UserID,
		SessionID: req.SessionID,
	}
	testSession, ok := s.Sessions[key]
	if !ok {
		return fmt.Errorf("session not found")
	}
	testSession.SessionEntries = append(testSession.SessionEntries, req.Entry)
	if req.Entry.Base() != nil && req.Entry.Base().Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339Nano, req.Entry.Base().Timestamp); err == nil {
			testSession.UpdatedAt = ts
		}
	}
	s.Sessions[key] = testSession
	return nil
}

func (s *FakeSessionService) GetCheckpoint(ctx context.Context, req *session.CheckpointRequest) (*session.Checkpoint, error) {
	return nil, nil
}

func (s *FakeSessionService) SaveCheckpoint(ctx context.Context, req *session.SaveCheckpointRequest) error {
	return nil
}

func (s *FakeSessionService) ClearCheckpoint(ctx context.Context, req *session.CheckpointRequest) error {
	return nil
}

func (s *FakeSessionService) GetAgentState(ctx context.Context, req *session.AgentStateRequest) (*session.AgentState, error) {
	return nil, nil
}

func (s *FakeSessionService) SaveAgentState(ctx context.Context, req *session.SaveAgentStateRequest) error {
	return nil
}

func (s *FakeSessionService) ClearAgentState(ctx context.Context, req *session.AgentStateRequest) error {
	return nil
}

var _ session.Service = (*FakeSessionService)(nil)
