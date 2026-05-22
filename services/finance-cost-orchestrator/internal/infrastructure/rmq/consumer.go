// Package rmq consumer placeholder. The orchestrator will consume worker
// "chunk-done" replies in S8c.5; implementation lands then.
package rmq

// Consumer is a placeholder type. Methods will be added in S8c.5.
type Consumer struct {
	conn *Connection
}

// NewConsumer constructs a Consumer bound to the given connection.
func NewConsumer(conn *Connection) *Consumer {
	return &Consumer{conn: conn}
}
