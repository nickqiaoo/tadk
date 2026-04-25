package bridge

import (
	"iter"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nickqiaoo/tadk/event"
)

func Start(program *tea.Program, events iter.Seq2[event.Event, error]) {
	if program == nil {
		return
	}
	go func() {
		for ev, err := range events {
			if err != nil {
				program.Send(ErrorMsg{Err: err})
			}
			if ev != nil {
				program.Send(EventMsg{Event: ev})
			}
		}
	}()
}

func StartCmd(program *tea.Program, events iter.Seq2[event.Event, error]) tea.Cmd {
	return func() tea.Msg {
		Start(program, events)
		return nil
	}
}
