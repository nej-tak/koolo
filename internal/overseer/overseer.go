package overseer

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hectorgimenez/koolo/internal/event"
)

var (
	instance *Overseer
	once     sync.Once
)

type Overseer struct {
	Sm  KSupervisorManager
	Ws  OverseerWebSocketServer
	Api *OverseerApi
}

type OverseerWebSocketServer interface {
	GetOverseerChannel() chan []byte
}

type OverseerTerminal struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (t *OverseerTerminal) wsSend(msg []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn.WriteMessage(1, msg)
}

func (t *OverseerTerminal) Respond(msg []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn.WriteMessage(1, msg)
}

type OverseerBroadcastEvent struct {
	EventType  string      `json:"type"`
	Supervisor string      `json:"supervisor"`
	Timestamp  int64       `json:"timestamp"`
	Data       interface{} `json:"data"`
}

// Setup initializes and returns the singleton Overseer instance
func Setup(ws OverseerWebSocketServer, sm KSupervisorManager) (*Overseer, error) {
	once.Do(func() {
		api := NewOverseerApi("http://192.168.1.209:8090") // todo config flag
		instance = &Overseer{
			Sm:  sm,
			Ws:  ws,
			Api: api,
		}
	})
	return instance, nil
}

// GetInstance returns the singleton Overseer instance
func GetInstance() *Overseer {
	return instance
}

// createBroadcastMessage constructs the JSON message for the given event
func createBroadcastMessage(eventType string, evt event.Event) ([]byte, error) {
	overseerEvent := OverseerBroadcastEvent{
		EventType:  eventType,
		Supervisor: getSupervisor(evt),
		Timestamp:  time.Now().Unix(),
		Data:       extractEventData(evt),
	}

	return json.Marshal(overseerEvent)
}

// extractEventData extracts the event-specific data from an event, excluding the BaseEvent fields
func extractEventData(evt event.Event) map[string]interface{} {
	data := make(map[string]interface{})
	val := reflect.ValueOf(evt)
	typ := reflect.TypeOf(evt)

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		if field.Name == "BaseEvent" {
			continue
		}
		camelCaseName := toCamelCase(field.Name)
		data[camelCaseName] = val.Field(i).Interface()
	}
	return data
}

// getSupervisor returns the supervisor value from the event
func getSupervisor(evt event.Event) string {
	if baseEvent, ok := evt.(interface{ Supervisor() string }); ok {
		return baseEvent.Supervisor()
	}
	return ""
}

func toCamelCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// my dumb ass couldnt figure out how to reference
// the struct in koolo/manager without circular import
type KSupervisorManager interface{}
