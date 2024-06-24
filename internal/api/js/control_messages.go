package js

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The unique console log line prefix which denotes a control message to the test rig.
// The text after this string is a JSON object which contains information on the type
// of update (e.g a new event, a verification request)
//
// For example:
//
//	CC:{"t":1,"d":{"RoomID":"!foo:bar","Event":{...}}}
const CONSOLE_LOG_CONTROL_STRING = "CC:"

type MessageType int

const (
	MessageTypeEvent MessageType = 1
	MessageTypeSync  MessageType = 2
	// TODO: MessageTypeVerification
)

type ControlMessage struct {
	Type MessageType     `json:"t"`
	Data json.RawMessage `json:"d"`
}

func (c *ControlMessage) AsControlMessageEvent() *ControlMessageEvent {
	if c == nil {
		return nil
	}
	if c.Type != MessageTypeEvent {
		return nil
	}
	var cme ControlMessageEvent
	if err := json.Unmarshal(c.Data, &cme); err != nil {
		return nil
	}
	return &cme
}

func (c *ControlMessage) AsControlMessageSync() *ControlMessageSync {
	if c == nil {
		return nil
	}
	if c.Type != MessageTypeSync {
		return nil
	}
	return &ControlMessageSync{} // no data
}

type ControlMessageSync struct{}

func EmitControlMessageSyncJS() string {
	return fmt.Sprintf(
		`console.log("%s"+JSON.stringify({
			"t":%d,
			"d":{}
		}));`, CONSOLE_LOG_CONTROL_STRING, MessageTypeSync,
	)
}

type ControlMessageEvent struct {
	RoomID string
	Event  JSEvent
}

func EmitControlMessageEventJS(roomIDJsCode, eventJSONJsCode string) string {
	return fmt.Sprintf(
		`console.log("%s"+JSON.stringify({
			"t":%d,
			"d":{
			  RoomID: %s,
			  Event: %s,
			}
		}));`, CONSOLE_LOG_CONTROL_STRING, MessageTypeEvent, roomIDJsCode, eventJSONJsCode,
	)
}

func unpackControlMessage(s string) *ControlMessage {
	if !strings.HasPrefix(s, CONSOLE_LOG_CONTROL_STRING) {
		return nil
	}
	val := strings.TrimPrefix(s, CONSOLE_LOG_CONTROL_STRING)
	var ctrlMsg ControlMessage
	if err := json.Unmarshal([]byte(val), &ctrlMsg); err != nil {
		return nil
	}
	return &ctrlMsg
}
