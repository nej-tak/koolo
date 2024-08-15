package overseer

import (
	"context"
	"reflect"

	"github.com/hectorgimenez/koolo/internal/event"
)

// Handle processes the given event using the singleton Overseer instance
func Handle(ctx context.Context, e event.Event) error {
	var message []byte
	var err error

	switch evt := e.(type) {
	case event.LogEvent:
		message, err = createBroadcastMessage("log", evt)
		if err != nil {
			return err
		}

	case TerminalMessageReceivedEvent:
		os := GetInstance()
		os.TerminalMessageHandler(evt)

	default:
		// for debugging atm, will POST all events to the
		// overseer events api
		// return handlePostEvent(e)
	}

	if message != nil {
		os := GetInstance()
		os.Ws.GetOverseerChannel() <- message
	}

	return nil
}

func handlePostEvent(e event.Event) error {
	eventType := reflect.TypeOf(e).Name()
	value := reflect.ValueOf(e)
	typ := value.Type()

	fieldValues := make(map[string]interface{})
	for i := 0; i < value.NumField(); i++ {
		fieldName := typ.Field(i).Name
		fieldValue := value.Field(i).Interface()
		if fieldName != "BaseEvent" {
			fieldValues[fieldName] = fieldValue
		}
	}

	return GetInstance().Api.PostEvent(eventType, e.Supervisor(), fieldValues)
}
