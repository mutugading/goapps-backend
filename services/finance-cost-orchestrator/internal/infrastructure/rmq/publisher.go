// Package rmq publisher placeholder. Real publish logic lands in S8c.4 when the
// orchestrator starts emitting chunk messages to worker queues.
package rmq

// Publisher is a placeholder type. Methods will be added in S8c.4.
type Publisher struct {
	conn *Connection
}

// NewPublisher constructs a Publisher bound to the given connection.
func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}
