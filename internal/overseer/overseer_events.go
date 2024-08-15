package overseer

import (
	"github.com/hectorgimenez/koolo/internal/event"
)

type TerminalMessageReceivedEvent struct {
	event.BaseEvent
	Msg      string
	Terminal *OverseerTerminal
}

func TerminalMessageReceived(msg string, terminal *OverseerTerminal) TerminalMessageReceivedEvent {
	return TerminalMessageReceivedEvent{
		BaseEvent: event.Text("Overseer", "TerminalMessageReceived"),
		Msg:       msg,
		Terminal:  terminal,
	}
}
