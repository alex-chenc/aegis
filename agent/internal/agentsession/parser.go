package agentsession

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type fileParseResult struct {
	Sessions      map[string]*SessionDelta
	Cursor        FileCursor
	BytesRead     int64
	Unsupported   bool
	LastSourceAt  time.Time
	SourceVersion string
}

func parseJSONLFile(path string, source Source, cursor FileCursor, cfg ScanConfig, redactor *Redactor) (fileParseResult, error) {
	result := fileParseResult{Sessions: make(map[string]*SessionDelta), Cursor: cursor}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	if cursor.SourceIdentity == "" {
		cursor.SourceIdentity = fileIdentity(path, info)
	}
	if info.Size() < cursor.ByteOffset || (cursor.LastFileSize > 0 && info.Size() < cursor.LastFileSize) {
		cursor.ByteOffset = 0
		cursor.LastSourceSequence = 0
		cursor.LastCompleteLineDigest = ""
		cursor.PartialLine = nil
	}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	if _, err := file.Seek(cursor.ByteOffset, io.SeekStart); err != nil {
		return result, err
	}

	reader := bufio.NewReaderSize(file, 256<<10)
	sequence := cursor.LastSourceSequence
	currentSessionID := ""
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > cfg.MaxItemBytes*4 {
			result.Unsupported = true
			if readErr == io.EOF {
				cursor.PartialLine = append([]byte(nil), line...)
				break
			}
			cursor.ByteOffset += int64(len(line))
			result.BytesRead += int64(len(line))
			continue
		}
		if readErr == io.EOF && len(line) == 0 {
			break
		}
		if result.BytesRead+int64(len(line)) > cfg.MaxNewBytes {
			cursor.PartialLine = append([]byte(nil), line...)
			break
		}
		if readErr == io.EOF && len(line) > 0 && line[len(line)-1] != '\n' {
			cursor.PartialLine = append([]byte(nil), line...)
			break
		}
		result.BytesRead += int64(len(line))
		cursor.ByteOffset += int64(len(line))
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			if readErr == io.EOF {
				break
			}
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(trimmed), &record); err != nil {
			result.Unsupported = true
			if readErr == io.EOF {
				break
			}
			continue
		}
		recordDigest := digestBytes([]byte(trimmed))
		items, sessionID, project, model, timestamp, version, known := normalizeRecordWithSession(source, record, recordDigest, redactor, currentSessionID)
		if !known {
			result.Unsupported = true
		}
		if sessionID == "" {
			if readErr == io.EOF {
				break
			}
			continue
		}
		if source == SourceCodex {
			currentSessionID = sessionID
		}
		if timestamp.IsZero() {
			timestamp = info.ModTime()
		}
		if timestamp.After(result.LastSourceAt) {
			result.LastSourceAt = timestamp
		}
		if version != "" {
			result.SourceVersion = version
		}
		delta := result.Sessions[sessionID]
		if delta == nil {
			delta = &SessionDelta{Source: source, SessionID: sessionID, ProjectPath: project, Model: model, LastSourceAt: timestamp, SourceVersion: version}
			result.Sessions[sessionID] = delta
		}
		if project != "" {
			delta.ProjectPath = project
		}
		if model != "" {
			delta.Model = model
		}
		if timestamp.After(delta.LastSourceAt) {
			delta.LastSourceAt = timestamp
		}
		for _, item := range items {
			sequence++
			item.SourceSequence = sequence
			delta.Items = append(delta.Items, item)
		}
		if readErr == io.EOF {
			break
		}
	}
	cursor.LastFileSize = info.Size()
	cursor.LastFileMTime = info.ModTime()
	cursor.LastSourceSequence = sequence
	if cursor.ByteOffset > 0 {
		cursor.LastCompleteLineDigest = digestBytes([]byte(strconv.FormatInt(cursor.ByteOffset, 10)))
	}
	result.Cursor = cursor
	for _, delta := range result.Sessions {
		delta.Unsupported = result.Unsupported
	}
	return result, nil
}

func normalizeRecord(source Source, record map[string]any, sourceDigest string, redactor *Redactor) ([]Item, string, string, string, time.Time, string, bool) {
	return normalizeRecordWithSession(source, record, sourceDigest, redactor, "")
}

func normalizeRecordWithSession(source Source, record map[string]any, sourceDigest string, redactor *Redactor, fallbackSessionID string) ([]Item, string, string, string, time.Time, string, bool) {
	switch source {
	case SourceClaude:
		return normalizeClaude(record, sourceDigest, redactor)
	case SourceCodex:
		return normalizeCodex(record, sourceDigest, redactor, fallbackSessionID)
	default:
		return nil, "", "", "", time.Time{}, "", false
	}
}

func normalizeClaude(record map[string]any, sourceDigest string, redactor *Redactor) ([]Item, string, string, string, time.Time, string, bool) {
	sessionID := stringValue(record["sessionId"])
	project := stringValue(record["cwd"])
	timestamp := parseTime(record["timestamp"])
	typeName := stringValue(record["type"])
	message, _ := record["message"].(map[string]any)
	model := stringValue(message["model"])
	if sessionID == "" {
		return nil, "", project, model, timestamp, "", false
	}
	if typeName != "user" && typeName != "assistant" {
		return nil, sessionID, project, model, timestamp, "", typeName == "summary" || typeName == "system" || typeName == "progress"
	}
	content := message["content"]
	items := make([]Item, 0, 2)
	messageID := stringValue(record["uuid"])
	parentID := stringValue(record["parentUuid"])
	turnID := parentID
	metadata := map[string]any{"source_type": typeName}
	if parentID != "" {
		metadata["parent_uuid"] = parentID
	}
	if typeName == "user" {
		if text := contentText(content); text != "" {
			items = append(items, newItem(ItemUserMessage, "user", text, messageID, "", turnID, timestamp, sourceDigest, redactor))
		}
		for _, block := range contentBlocks(content) {
			if stringValue(block["type"]) != "tool_result" {
				continue
			}
			toolID := stringValue(block["tool_use_id"])
			result := block["content"]
			items = append(items, newItem(ItemToolResult, "tool", contentText(result), messageID, toolID, turnID, timestamp, sourceDigest, redactor))
		}
	} else {
		if text := assistantText(content); text != "" {
			items = append(items, newItem(ItemAssistantMessage, "assistant", text, messageID, "", turnID, timestamp, sourceDigest, redactor))
		}
		for _, block := range contentBlocks(content) {
			if stringValue(block["type"]) != "tool_use" {
				continue
			}
			toolID := stringValue(block["id"])
			payload, count := redactJSON(redactor, block["input"])
			item := newItem(ItemToolCall, "assistant", payload, messageID, toolID, turnID, timestamp, sourceDigest, nil)
			item.Metadata = metadata
			item.RedactionCount += count
			items = append(items, item)
		}
	}
	return items, sessionID, project, model, timestamp, stringValue(message["version"]), true
}

func normalizeCodex(record map[string]any, sourceDigest string, redactor *Redactor, fallbackSessionID string) ([]Item, string, string, string, time.Time, string, bool) {
	payload, _ := record["payload"].(map[string]any)
	typeName := stringValue(record["type"])
	timestamp := parseTime(payload["timestamp"])
	if timestamp.IsZero() {
		timestamp = parseTime(record["timestamp"])
	}
	sessionID := ""
	project := ""
	model := ""
	if typeName == "session_meta" {
		sessionID = stringValue(payload["id"])
		project = stringValue(payload["cwd"])
		return nil, sessionID, project, "", timestamp, stringValue(payload["version"]), sessionID != ""
	}
	if typeName == "turn_context" {
		return nil, stringValue(payload["session_id"]), stringValue(payload["cwd"]), stringValue(payload["model"]), timestamp, "", true
	}
	if typeName != "response_item" {
		return nil, stringValue(payload["session_id"]), "", "", timestamp, "", typeName == "event_msg" || typeName == "turn_aborted"
	}
	sessionID = stringValue(payload["session_id"])
	if sessionID == "" {
		sessionID = fallbackSessionID
	}
	if sessionID == "" {
		// Codex records normally carry the session id only in session_meta. The
		// scanner associates later records through the file cursor; do not guess it.
		return nil, "", "", "", timestamp, "", false
	}
	itemType := stringValue(payload["type"])
	messageID := stringValue(payload["id"])
	turnID := stringValue(payload["turn_id"])
	switch itemType {
	case "message":
		role := stringValue(payload["role"])
		text := ""
		for _, block := range contentBlocks(payload["content"]) {
			kind := stringValue(block["type"])
			if kind == "input_text" || kind == "output_text" {
				text += stringValue(block["text"])
			}
		}
		if text == "" {
			text = stringValue(payload["text"])
		}
		roleName := role
		itemKind := ItemAssistantMessage
		if role == "user" {
			itemKind = ItemUserMessage
		}
		return []Item{newItem(itemKind, roleName, text, messageID, "", turnID, timestamp, sourceDigest, redactor)}, sessionID, project, model, timestamp, "", true
	case "function_call":
		payloadText := stringValue(payload["arguments"])
		if payloadText == "" {
			payloadText = "{}"
		}
		redacted, count := redactor.Text(payloadText)
		item := newItem(ItemToolCall, "assistant", redacted, messageID, stringValue(payload["call_id"]), turnID, timestamp, sourceDigest, nil)
		item.RedactionCount = count
		return []Item{item}, sessionID, project, model, timestamp, "", true
	case "function_call_output":
		item := newItem(ItemToolResult, "tool", stringValue(payload["output"]), messageID, stringValue(payload["call_id"]), turnID, timestamp, sourceDigest, redactor)
		return []Item{item}, sessionID, project, model, timestamp, "", true
	case "reasoning":
		return nil, sessionID, project, model, timestamp, "", true
	default:
		return nil, sessionID, project, model, timestamp, "", false
	}
}

func newItem(itemType ItemType, role, content, messageID, partID, turnID string, occurredAt time.Time, sourceDigest string, redactor *Redactor) Item {
	redacted := content
	count := 0
	if redactor != nil {
		redacted, count = redactor.Text(content)
	}
	return Item{
		SourceMessageID: messageID,
		SourcePartID:    partID,
		SourceRevision:  sourceDigest,
		TurnID:          turnID,
		ItemType:        itemType,
		Role:            role,
		OccurredAt:      occurredAt,
		Content:         redacted,
		SourceDigest:    sourceDigest,
		ContentDigest:   digestBytes([]byte(redacted)),
		RedactionState:  redactionState(count, redacted),
		Visibility:      "visible",
		RedactionCount:  count,
	}
}

func redactionState(count int, content string) string {
	if count > 0 {
		return "redacted"
	}
	if content == "" {
		return "metadata_only"
	}
	return "none"
}

func contentBlocks(value any) []map[string]any {
	blocks, _ := value.([]any)
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		if typed, ok := block.(map[string]any); ok {
			result = append(result, typed)
		}
	}
	return result
}

func contentText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	parts := make([]string, 0)
	for _, block := range contentBlocks(value) {
		if text := stringValue(block["text"]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func assistantText(value any) string {
	parts := make([]string, 0)
	for _, block := range contentBlocks(value) {
		if stringValue(block["type"]) == "text" {
			parts = append(parts, stringValue(block["text"]))
		}
	}
	return strings.Join(parts, "")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func parseTime(value any) time.Time {
	text := stringValue(value)
	if text == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return ts
	}
	if number, err := strconv.ParseInt(text, 10, 64); err == nil {
		if number > 1e12 {
			return time.UnixMilli(number).UTC()
		}
		return time.Unix(number, 0).UTC()
	}
	return time.Time{}
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func fileIdentity(path string, info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%s:%d:%d", filepath.Clean(path), stat.Dev, stat.Ino)
	}
	return filepath.Clean(path)
}
