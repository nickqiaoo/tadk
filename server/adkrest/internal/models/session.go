package models

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	"github.com/nickqiaoo/tadk/session"
)

// Session represents an agent's session.
type Session struct {
	ID        string         `json:"id"`
	AppName   string         `json:"appName"`
	UserID    string         `json:"userId"`
	UpdatedAt int64          `json:"lastUpdateTime"`
	Entries   []Entry        `json:"entries"`
}

type CreateSessionRequest struct {
	Entries []Entry        `json:"entries"`
}

type SessionID struct {
	ID      string `mapstructure:"session_id,optional"`
	AppName string `mapstructure:"app_name,required"`
	UserID  string `mapstructure:"user_id,required"`
}

func SessionIDFromHTTPParameters(vars map[string]string) (SessionID, error) {
	var sessionID SessionID
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &sessionID,
	})
	if err != nil {
		return sessionID, err
	}
	err = decoder.Decode(vars)
	if err != nil {
		return sessionID, err
	}
	if sessionID.AppName == "" {
		return sessionID, fmt.Errorf("app_name parameter is required")
	}
	if sessionID.UserID == "" {
		return sessionID, fmt.Errorf("user_id parameter is required")
	}
	return sessionID, nil
}

func FromSession(curSession session.Session) (Session, error) {
	entries := []Entry{}
	for entry := range curSession.Entries().All() {
		entries = append(entries, FromSessionEntry(entry))
	}
	mappedSession := Session{
		ID:        curSession.ID(),
		AppName:   curSession.AppName(),
		UserID:    curSession.UserID(),
		UpdatedAt: curSession.LastUpdateTime().Unix(),
		Entries:   entries,
	}
	return mappedSession, mappedSession.Validate()
}

func (s Session) Validate() error {
	if s.AppName == "" {
		return fmt.Errorf("app_name is empty in received session")
	}
	if s.UserID == "" {
		return fmt.Errorf("user_id is empty in received session")
	}
	if s.ID == "" {
		return fmt.Errorf("session_id is empty in received session")
	}
	if s.UpdatedAt == 0 {
		return fmt.Errorf("updated_at is empty")
	}
	if s.Entries == nil {
		return fmt.Errorf("entries is nil")
	}
	return nil
}
