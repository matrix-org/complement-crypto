package js

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matrix-org/complement/ct"
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
	MessageTypeEvent        MessageType = 1
	MessageTypeSync         MessageType = 2
	MessageTypeVerification MessageType = 3
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
		fmt.Println("WARN: unable to unmarshal MessageTypeEvent control message:", err)
		return nil
	}
	return &cme
}

func (c *ControlMessage) AsControlMessageVerification() *ControlMessageVerification {
	if c == nil {
		return nil
	}
	if c.Type != MessageTypeVerification {
		return nil
	}
	var cme ControlMessageVerification
	if err := json.Unmarshal(c.Data, &cme); err != nil {
		fmt.Println("WARN: unable to unmarshal MessageTypeVerification control message:", err)
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

type ControlMessageVerification struct {
	Stage    string
	TxnID    string
	UserID   string
	DeviceID string
	Data     json.RawMessage // data specific to the Stage
}

func EmitControlMessageVerificationJS(stageJSCode, txnIDJSCode, userIDJSCode, deviceIDJSCode, dataJSCode string) string {
	return fmt.Sprint(
		`console.log("`, CONSOLE_LOG_CONTROL_STRING, `"+JSON.stringify({
			"t":`, MessageTypeVerification, `,
			"d":{
			  Stage: `, stageJSCode, `,
			  TxnID: `, txnIDJSCode, `,
			  UserID: `, userIDJSCode, `,
			  DeviceID: `, deviceIDJSCode, `,
			  Data: `, dataJSCode, `,
			}
		}));`,
	)
}

func unpackControlMessage(t ct.TestLike, s string) *ControlMessage {
	if !strings.HasPrefix(s, CONSOLE_LOG_CONTROL_STRING) {
		// depending on the content of the control message, the log line may be double escaped.
		// This has been seen when receiving ControlMessageVerification TransitionSAS messages,
		// likely due to the presence of emoji characters. They end up getting logged like:
		//    "CC:{\"t\":3,\"d\":{\"Stage\":\"TransitionSAS\",...
		if strings.HasPrefix(s, `"`+CONSOLE_LOG_CONTROL_STRING) {
			s = strings.ReplaceAll(s, `\"`, `"`) // map back to unescaped quotes
			s = s[1 : len(s)-1]                  // strip the outer quotes
		} else {
			return nil
		}
	}
	val := strings.TrimPrefix(s, CONSOLE_LOG_CONTROL_STRING)
	var ctrlMsg ControlMessage
	if err := json.Unmarshal([]byte(val), &ctrlMsg); err != nil {
		ct.Errorf(t, "unpackControlMessage: malformed control message: %s for message: %s", err, s)
		return nil
	}
	return &ctrlMsg
}
