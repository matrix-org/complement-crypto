package js

import "encoding/json"

type MessageType int

const (
	MessageTypeEvent MessageType = 1
	// TODO: MessageTypeVerification
)

type ControlMessage struct {
	Type MessageType
	Data json.RawMessage
}
