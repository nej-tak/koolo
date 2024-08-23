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

type IngameTerminalMsgEvent struct {
	event.BaseEvent
	Target   string
	Command  string
	Terminal *OverseerTerminal
}

func IngameTerminalMsg(target, command string, terminal *OverseerTerminal) IngameTerminalMsgEvent {
	return IngameTerminalMsgEvent{
		BaseEvent: event.Text("Overseer", "IngameTerminalMsg"),
		Target:    target,
		Command:   command,
		Terminal:  terminal,
	}
}
