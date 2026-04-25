package database

import (
	"iter"
	"sync"
	"time"

	"github.com/nickqiaoo/tadk/session"
)

// TODO localSession is identical to session.session. Move to sessioninternal
type localSession struct {
	appName   string
	userID    string
	sessionID string

	// guards all mutable fields
	mu        sync.RWMutex
	entries   []session.DurableEntry
	updatedAt time.Time
}

func (s *localSession) ID() string {
	return s.sessionID
}

func (s *localSession) AppName() string {
	return s.appName
}

func (s *localSession) UserID() string {
	return s.userID
}

func (s *localSession) Entries() session.Entries {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return entries(s.entries)
}

func (s *localSession) LastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.updatedAt
}

func (s *localSession) appendEntry(entry session.DurableEntry) error {
	if entry == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, entry)
	return nil
}

type entries []session.DurableEntry

func (e entries) All() iter.Seq[session.DurableEntry] {
	return func(yield func(session.DurableEntry) bool) {
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

func (e entries) At(i int) session.DurableEntry {
	if i < 0 || i >= len(e) {
		return nil
	}
	return e[i]
}

var (
	_ session.Session = (*localSession)(nil)
)
