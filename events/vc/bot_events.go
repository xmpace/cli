// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

// VCBotEventOutput is the raw-preserving shape for bot-observed VC events.
type VCBotEventOutput struct {
	Type              string          `json:"type"                         desc:"Event type; one of the supported vc.bot.* keys"`
	EventID           string          `json:"event_id,omitempty"           desc:"Globally unique event ID; safe for deduplication"`
	Timestamp         string          `json:"timestamp,omitempty"          desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	CallID            string          `json:"call_id,omitempty"            desc:"Bot invitation call ID; pass through to vc agent join when present"`
	MeetingNo         string          `json:"meeting_no,omitempty"         desc:"Meeting number from the bot event's declared meeting field"`
	ActivityEventType string          `json:"activity_event_type,omitempty" desc:"First event.meeting_activity_items[].activity_event_type value"`
	ChatEmojiTypes    []string        `json:"chat_emoji_types,omitempty"   desc:"Feishu post emotion emoji_type values from event.meeting_activity_items[].chat_received_items[] where message_type=3; the key is the item's content"`
	RawEvent          json.RawMessage `json:"raw_event,omitempty"          desc:"Original VC bot event payload; authoritative for fields not exposed as stable top-level fields"`
}

func processVCBotMeetingInvited(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw, false)
}

func processVCBotMeetingEvent(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw, true)
}

func processVCBotMeetingEnded(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw, false)
}

type vcBotEventEnvelope struct {
	Header struct {
		EventID    string `json:"event_id"`
		EventType  string `json:"event_type"`
		CreateTime string `json:"create_time"`
	} `json:"header"`
	Event json.RawMessage `json:"event"`
}

type vcBotMeetingActivityEvent struct {
	MeetingActivityItems []json.RawMessage `json:"meeting_activity_items"`
}

type vcBotMeetingActivityItem struct {
	ActivityEventType string `json:"activity_event_type"`
	MeetingNo         string
	ChatReceivedItems []vcBotChatReceivedItem `json:"chat_received_items"`
}

type vcBotChatReceivedItem struct {
	Content     string      `json:"content"`
	MessageType json.Number `json:"message_type"`
}

func decodeBotMeetingActivityItem(data json.RawMessage) (vcBotMeetingActivityItem, bool) {
	var payload struct {
		ActivityEventType string `json:"activity_event_type"`
		Meeting           struct {
			MeetingNo string `json:"meeting_no"`
		} `json:"meeting"`
		ChatReceivedItems []vcBotChatReceivedItem `json:"chat_received_items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return vcBotMeetingActivityItem{}, false
	}
	return vcBotMeetingActivityItem{
		ActivityEventType: payload.ActivityEventType,
		MeetingNo:         strings.TrimSpace(payload.Meeting.MeetingNo),
		ChatReceivedItems: payload.ChatReceivedItems,
	}, true
}

func processVCBotEvent(raw *event.RawEvent, includeEmojiTypes bool) (json.RawMessage, error) {
	var envelope vcBotEventEnvelope
	decoder := json.NewDecoder(bytes.NewReader(raw.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	eventType := envelope.Header.EventType
	if eventType == "" {
		eventType = raw.EventType
	}
	activityItems := botActivityItems(eventType, envelope.Event)
	out := &VCBotEventOutput{
		Type:              eventType,
		EventID:           envelope.Header.EventID,
		Timestamp:         envelope.Header.CreateTime,
		CallID:            botCallID(eventType, envelope.Event),
		MeetingNo:         botMeetingNo(eventType, envelope.Event, activityItems),
		ActivityEventType: botActivityEventType(activityItems),
		RawEvent:          append(json.RawMessage(nil), raw.Payload...),
	}
	if includeEmojiTypes {
		out.ChatEmojiTypes = botEmojiTypes(activityItems)
	}
	return json.Marshal(out)
}

func botCallID(eventType string, event json.RawMessage) string {
	if eventType != eventTypeBotMeetingInvited {
		return ""
	}
	return jsonStringAt(event, "call_id")
}

func botMeetingNo(eventType string, event json.RawMessage, activityItems []vcBotMeetingActivityItem) string {
	switch eventType {
	case eventTypeBotMeetingInvited:
		return jsonStringAt(event, "meeting", "meeting_no")
	case eventTypeBotMeetingEvent:
		for _, item := range activityItems {
			if meetingNo := strings.TrimSpace(item.MeetingNo); meetingNo != "" {
				return meetingNo
			}
		}
	case eventTypeBotMeetingEnded:
		return jsonStringAt(event, "meeting_no")
	}
	return ""
}

func botActivityItems(eventType string, event json.RawMessage) []vcBotMeetingActivityItem {
	if eventType != eventTypeBotMeetingEvent {
		return nil
	}
	var payload vcBotMeetingActivityEvent
	if err := json.Unmarshal(event, &payload); err != nil {
		return nil
	}
	items := make([]vcBotMeetingActivityItem, 0, len(payload.MeetingActivityItems))
	for _, rawItem := range payload.MeetingActivityItems {
		item, ok := decodeBotMeetingActivityItem(rawItem)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

func botActivityEventType(items []vcBotMeetingActivityItem) string {
	for _, item := range items {
		if eventType := strings.TrimSpace(item.ActivityEventType); eventType != "" {
			return eventType
		}
	}
	return ""
}

func botEmojiTypes(items []vcBotMeetingActivityItem) []string {
	seen := map[string]bool{}
	var out []string
	for _, activity := range items {
		if strings.TrimSpace(activity.ActivityEventType) != "chat_received" {
			continue
		}
		for _, item := range activity.ChatReceivedItems {
			if !isBotMeetingReactionItem(item) {
				continue
			}
			addEmojiType(item.Content, seen, &out)
		}
	}
	return out
}

func isBotMeetingReactionItem(v vcBotChatReceivedItem) bool {
	n, err := v.MessageType.Int64()
	return err == nil && n == 3
}

func addEmojiType(value string, seen map[string]bool, out *[]string) {
	value = strings.TrimSpace(value)
	if value == "" || seen[value] {
		return
	}
	seen[value] = true
	*out = append(*out, value)
}

func jsonStringAt(raw json.RawMessage, path ...string) string {
	for _, key := range path {
		if len(raw) == 0 {
			return ""
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return ""
		}
		raw = object[key]
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
