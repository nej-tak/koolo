package overseer

import (
	"errors"
	"reflect"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/hectorgimenez/d2go/pkg/data/difficulty"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/event"
)

type TmpConfigOverrides struct {
	diff difficulty.Difficulty
}

func (o *Overseer) TerminalMessageHandler(evt TerminalMessageReceivedEvent) {
	args := strings.Fields(evt.Msg)

	if len(args) < 1 {
		return
	}

	handlers := map[string]func(TerminalMessageReceivedEvent, []string){
		"copy": wrapHandlerFn(handleCopy),
		"tmp":  wrapHandlerFn(handleTmpCopy),
	}

	supervisorName := args[0] // legacy for now, atm will be $ from the overseer-react-terminal
	cmd := args[1]
	cmdArgs := []string{}

	if len(args) > 2 {
		cmdArgs = args[2:]
	}

	if handler, found := handlers[cmd]; found {
		handler(evt, cmdArgs)
	} else {
		handleUnknown(evt, supervisorName)
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

	event.Send(TerminalMessageReceived(string(message), terminal))
}

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
