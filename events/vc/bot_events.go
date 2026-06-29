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
	Event vcBotEventBody `json:"event"`
}

type vcBotEventBody struct {
	CallID               string                     `json:"call_id"`
	MeetingNo            string                     `json:"meeting_no"`
	Meeting              vcBotMeeting               `json:"meeting"`
	MeetingActivityItems []vcBotMeetingActivityItem `json:"meeting_activity_items"`
}

type vcBotMeeting struct {
	MeetingNo string `json:"meeting_no"`
}

type vcBotMeetingActivityItem struct {
	ActivityEventType string                  `json:"activity_event_type"`
	Meeting           vcBotMeeting            `json:"meeting"`
	ChatReceivedItems []vcBotChatReceivedItem `json:"chat_received_items"`
}

type vcBotChatReceivedItem struct {
	Content     string      `json:"content"`
	MessageType json.Number `json:"message_type"`
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
	out := &VCBotEventOutput{
		Type:              eventType,
		EventID:           envelope.Header.EventID,
		Timestamp:         envelope.Header.CreateTime,
		CallID:            envelope.Event.CallID,
		MeetingNo:         botMeetingNo(eventType, envelope.Event),
		ActivityEventType: botActivityEventType(envelope.Event),
		RawEvent:          append(json.RawMessage(nil), raw.Payload...),
	}
	if includeEmojiTypes {
		out.ChatEmojiTypes = botEmojiTypes(envelope.Event.MeetingActivityItems)
	}
	return json.Marshal(out)
}

func botMeetingNo(eventType string, event vcBotEventBody) string {
	switch eventType {
	case eventTypeBotMeetingInvited:
		return strings.TrimSpace(event.Meeting.MeetingNo)
	case eventTypeBotMeetingEvent:
		for _, item := range event.MeetingActivityItems {
			if meetingNo := strings.TrimSpace(item.Meeting.MeetingNo); meetingNo != "" {
				return meetingNo
			}
		}
	case eventTypeBotMeetingEnded:
		return strings.TrimSpace(event.MeetingNo)
	}
	return ""
}

func botActivityEventType(event vcBotEventBody) string {
	for _, item := range event.MeetingActivityItems {
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
