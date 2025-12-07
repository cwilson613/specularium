package service

// EventType defines the type of event
type EventType string

const (
	EventHostCreated      EventType = "host_created"
	EventHostUpdated      EventType = "host_updated"
	EventHostDeleted      EventType = "host_deleted"
	EventConnectionCreated EventType = "connection_created"
	EventConnectionUpdated EventType = "connection_updated"
	EventConnectionDeleted EventType = "connection_deleted"
	EventPositionsUpdated  EventType = "positions_updated"
	EventInfraReloaded     EventType = "infrastructure_reloaded"
)

// Event represents an event that occurred in the system
type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// EventBus allows publishing and subscribing to events
type EventBus struct {
	subscribers []chan<- Event
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make([]chan<- Event, 0),
	}
}

// Subscribe adds a subscriber to receive events
func (eb *EventBus) Subscribe(ch chan<- Event) {
	eb.subscribers = append(eb.subscribers, ch)
}

// Publish sends an event to all subscribers
func (eb *EventBus) Publish(event Event) {
	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber is slow, skip
		}
	}
}
