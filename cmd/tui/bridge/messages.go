package bridge

import (
	"github.com/nickqiaoo/tadk/event"
)

type EventMsg struct {
	Event event.Event
}

type ErrorMsg struct {
	Err error
}
