package models

import (
	"time"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

// Entry represents a single durable entry in a session.
type Entry struct {
	ID           string           `json:"id"`
	Time         int64            `json:"time"`
	InvocationID string           `json:"invocationId"`
	Branch       string           `json:"branch"`
	Author       string           `json:"author"`
	Message      *message.Message `json:"message,omitempty"`
	ErrorCode    string           `json:"errorCode"`
	ErrorMessage string           `json:"errorMessage"`
}

// FromSessionEntry maps durable session entry to the REST Entry data struct.
func FromSessionEntry(entry session.DurableEntry) Entry {
	if entry == nil {
		return Entry{}
	}
	base := entry.Base()
	if base == nil {
		return Entry{}
	}
	var eventTime int64
	if base.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339Nano, base.Timestamp); err == nil {
			eventTime = ts.Unix()
		}
	}
	result := Entry{
		ID:   base.ID,
		Time: eventTime,
	}
	if msgEntry, ok := entry.(*session.MessageEntry); ok {
		result.Message = msgEntry.Message
		result.Author = msgEntry.Author
		result.InvocationID = msgEntry.InvocationID
		result.Branch = msgEntry.Branch
		result.ErrorCode = msgEntry.ErrorCode
		result.ErrorMessage = msgEntry.ErrorMessage
	}
	return result
}

// ToSessionEntry maps REST Entry data struct to the durable session entry model.
func ToSessionEntry(event Entry, parentID string) session.DurableEntry {
	entry := session.NewMessageLogEntry(parentID, event.Message, event.Author, event.InvocationID, event.Branch)
	if entry == nil {
		entry = &session.MessageEntry{
			EntryBase: session.NewEntryBase(session.EntryTypeMessage, parentID),
			Author:    event.Author,
		}
	}
	if event.ID != "" {
		entry.ID = event.ID
	}
	if event.Time != 0 {
		entry.Timestamp = time.Unix(event.Time, 0).Format(time.RFC3339Nano)
	}
	entry.ErrorCode = event.ErrorCode
	entry.ErrorMessage = event.ErrorMessage
	return entry
}
