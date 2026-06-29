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
	MeetingNo         string          `json:"meeting_no,omitempty"         desc:"Meeting number when present in the bot event payload"`
	ActivityEventType string          `json:"activity_event_type,omitempty" desc:"Meeting activity event subtype when present"`
	ChatEmojiTypes    []string        `json:"chat_emoji_types,omitempty"   desc:"Feishu post emotion emoji_type values extracted from vc.bot.meeting_activity_v1 payloads"`
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

func processVCBotEvent(raw *event.RawEvent, includeEmojiTypes bool) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event map[string]any `json:"event"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	out := &VCBotEventOutput{
		Type:              envelope.Header.EventType,
		EventID:           envelope.Header.EventID,
		Timestamp:         envelope.Header.CreateTime,
		CallID:            jsonString(envelope.Event["call_id"]),
		MeetingNo:         botMeetingNo(envelope.Event),
		ActivityEventType: botActivityEventType(envelope.Event),
		RawEvent:          append(json.RawMessage(nil), raw.Payload...),
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}
	if includeEmojiTypes {
		out.ChatEmojiTypes = botEmojiTypes(envelope.Event)
	}
	return json.Marshal(out)
}

func botMeetingNo(event map[string]any) string {
	for _, key := range []string{"meeting_no", "meeting_number"} {
		if s := jsonString(event[key]); s != "" {
			return s
		}
	}
	for _, key := range []string{"meeting", "meeting_info"} {
		meeting := jsonMap(event[key])
		if s := jsonString(meeting["meeting_no"]); s != "" {
			return s
		}
	}
	for _, item := range jsonMapSlice(event["meeting_activity_items"]) {
		if s := botMeetingNo(item); s != "" {
			return s
		}
	}
	return ""
}

func botActivityEventType(event map[string]any) string {
	if s := jsonString(event["activity_event_type"]); s != "" {
		return s
	}
	for _, item := range jsonMapSlice(event["meeting_activity_items"]) {
		if s := jsonString(item["activity_event_type"]); s != "" {
			return s
		}
	}
	return ""
}

func botEmojiTypes(event map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	collectEmojiTypesFromChatItems(event["chat_received_items"], seen, &out)
	collectEmojiTypesFromChatItems(event["chat_messages"], seen, &out)
	for _, item := range jsonMapSlice(event["meeting_activity_items"]) {
		collectEmojiTypesFromChatItems(item["chat_received_items"], seen, &out)
		collectEmojiTypesFromChatItems(item["chat_messages"], seen, &out)
	}
	return out
}

func collectEmojiTypesFromChatItems(value any, seen map[string]bool, out *[]string) {
	for _, item := range jsonMapSlice(value) {
		if !isBotMeetingReactionItem(item) {
			continue
		}
		addEmojiType(jsonString(item["content"]), seen, out)
		for _, key := range []string{"emoji_type", "chat_emoji_type", "reaction_type"} {
			addEmojiTypeFromValue(item[key], seen, out)
		}
		for _, s := range jsonStringSlice(item["chat_emoji_types"]) {
			addEmojiType(s, seen, out)
		}
	}
}

func addEmojiTypeFromValue(value any, seen map[string]bool, out *[]string) {
	if s := jsonString(value); s != "" {
		addEmojiType(s, seen, out)
		return
	}
	m := jsonMap(value)
	for _, key := range []string{"emoji_type", "chat_emoji_type", "reaction_type"} {
		addEmojiType(jsonString(m[key]), seen, out)
	}
}

func isBotMeetingReactionItem(v map[string]any) bool {
	switch raw := v["message_type"].(type) {
	case json.Number:
		n, err := raw.Int64()
		return err == nil && n == 3
	case float64:
		return raw == 3
	case string:
		return strings.TrimSpace(raw) == "3"
	default:
		return false
	}
}

func addEmojiType(value string, seen map[string]bool, out *[]string) {
	value = strings.TrimSpace(value)
	if value == "" || seen[value] {
		return
	}
	seen[value] = true
	*out = append(*out, value)
}

func jsonString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

func jsonStringSlice(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := jsonString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

func jsonMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func jsonMapSlice(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m := jsonMap(item); m != nil {
			out = append(out, m)
		}
	}
	return out
}
