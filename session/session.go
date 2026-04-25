package session

import (
	"iter"
	"time"
)

// Session represents a series of interactions between a user and agents.
//
// When a user starts interacting with your agent, session holds everything
// related to that one specific chat thread.
type Session interface {
	// ID returns the unique identifier of the session.
	ID() string
	// AppName returns name of the app.
	AppName() string
	// UserID returns the id of the user.
	UserID() string
	// Entries returns the durable session log entries.
	Entries() Entries
	// LastUpdateTime returns the time of the last update.
	LastUpdateTime() time.Time
}

// Entries define a standard interface for a durable [DurableEntry] list.
type Entries interface {
	All() iter.Seq[DurableEntry]
	Len() int
	At(i int) DurableEntry
}
