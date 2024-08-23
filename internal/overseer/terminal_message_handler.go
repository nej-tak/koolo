package overseer

import (
	"reflect"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/hectorgimenez/koolo/internal/event"
)

type TmpConfigOverrides struct {
	//diff difficulty.Difficulty
}

func (o *Overseer) TerminalMessageHandler(evt TerminalMessageReceivedEvent) {
	args := strings.Fields(evt.Msg)

	handlers := map[string]func(TerminalMessageReceivedEvent, []string){
		//"copy": wrapHandlerFn(handleCopy),
		//"tmp":  wrapHandlerFn(handleTmpCopy),
	}

	cmd := args[0]
	cmdArgs := []string{}

	if len(args) > 1 {
		cmdArgs = args[1:]
	}

	if handler, found := handlers[cmd]; found {
		handler(evt, cmdArgs)
	} else {
		handleUnknown(evt, "")
	}
}

// wrapHandlerFn injects the evt
func wrapHandlerFn(fn interface{}) func(TerminalMessageReceivedEvent, []string) {
	return func(evt TerminalMessageReceivedEvent, cmdArgs []string) {
		callHandlerFn(fn, evt, cmdArgs)
	}
}

func callHandlerFn(fn interface{}, evt TerminalMessageReceivedEvent, args []string) {
	fnType := reflect.TypeOf(fn)
	// The number of arguments expected by the function, excluding the evt argument
	expectedArgs := fnType.NumIn() - 1

	// Inject empty args where missing to match the number expected by the function
	if len(args) < expectedArgs {
		args = append(args, make([]string, expectedArgs-len(args))...)
	} else if len(args) > expectedArgs {
		args = args[:expectedArgs]
	}

	// Convert []string to []reflect.Value
	in := make([]reflect.Value, len(args)+1) // Add +1 for the evt parameter
	in[0] = reflect.ValueOf(evt)             // The first argument is the event
	for i, arg := range args {
		in[i+1] = reflect.ValueOf(arg) // Adjust index for args
	}
	// Call the function dynamically
	reflect.ValueOf(fn).Call(in)
}

/*
func handleTmpCopy(evt TerminalMessageReceivedEvent, source, diff string) {
	err := makeTmpCopy(source, &TmpConfigOverrides{
		diff: difficulty.Difficulty(diff),
	})
	if err != nil {
		respond(evt, "Error: "+err.Error())
		return
	}
	respond(evt, "Created tmp config: "+source+"_tmp")
}

func handleCopy(evt TerminalMessageReceivedEvent, source, target string) {
	err := config.CreateFromSource(source, target)
	if err != nil {
		respond(evt, "Error: "+err.Error())
		return
	}
	respond(evt, "Created new config: "+target)
}
*/

func respond(evt TerminalMessageReceivedEvent, response string) {
	evt.Terminal.wsSend([]byte(response))
}

func handleUnknown(evt TerminalMessageReceivedEvent, terminalSupervisor string) {
	evt.Terminal.wsSend([]byte("no handler found for " + evt.Msg + "@" + terminalSupervisor))
}

func WebsocketTextHandler(conn *websocket.Conn, message []byte) {
	terminal := &OverseerTerminal{
		conn: conn,
	}

	if isInGameMsg(string(message)) {
		target, command := parseInGameMsg(string(message))
		if target != "" {
			event.Send(IngameTerminalMsg(target, command, terminal))
			return
		}
	}

	event.Send(TerminalMessageReceived(string(message), terminal))
}

func isInGameMsg(msg string) bool {
	return strings.HasPrefix(msg, "@")
}

func parseInGameMsg(msg string) (string, string) {
	parts := strings.SplitN(msg[1:], " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

/*
func makeTmpCopy(sourceName string, overrides *TmpConfigOverrides) error {
	if sourceName == "" {
		return errors.New("missing source supervisor name")
	}

	err := config.CreateFromSource(sourceName, "")
	if err != nil {
		return err
	}

	tmp := sourceName + "_tmp"
	cfg := config.Characters[tmp]
	cfg.Overseer.Tmp = true
	cfg.KillD2OnStop = false

	if overrides != nil {
		if overrides.diff != "" {
			cfg.Game.Difficulty = overrides.diff
		}
	}

	return config.SaveSupervisorConfig(tmp, cfg)
}
*/
