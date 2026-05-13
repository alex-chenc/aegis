package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Type    string      `json:"type"`    // thinking | tool_call | tool_result | content | done | error
	Content string      `json:"content"` // 内容
	Tool    string      `json:"tool,omitempty"`
	CallID  string      `json:"call_id,omitempty"`
	Args    interface{} `json:"args,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	TimeMs  int64       `json:"time_ms,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SSEWriter writes SSE events to an HTTP response. It is safe for concurrent use.
type SSEWriter struct {
	mu      sync.Mutex
	writer  http.ResponseWriter
	flushed bool
}

// NewSSEWriter creates a new SSE writer
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &SSEWriter{writer: w}
}

// Write writes an SSE event
func (w *SSEWriter) Write(event SSEEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE event: %w", err)
	}

	fmt.Fprintf(w.writer, "data: %s\n\n", string(data))
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
		w.flushed = true
	}
	return nil
}

// WriteThinking writes a thinking event
func (w *SSEWriter) WriteThinking(content string) error {
	return w.Write(SSEEvent{Type: "thinking", Content: content})
}

// WriteToolCall writes a tool call event
func (w *SSEWriter) WriteToolCall(tool, callID string, args interface{}) error {
	return w.Write(SSEEvent{
		Type:   "tool_call",
		Tool:   tool,
		CallID: callID,
		Args:   args,
	})
}

// WriteToolResult writes a tool result event
func (w *SSEWriter) WriteToolResult(callID string, result interface{}, timeMs int64) error {
	return w.Write(SSEEvent{
		Type:   "tool_result",
		CallID: callID,
		Result: result,
		TimeMs: timeMs,
	})
}

// WriteToolError writes a tool error event
func (w *SSEWriter) WriteToolError(callID, errMsg string) error {
	return w.Write(SSEEvent{
		Type:   "tool_error",
		CallID: callID,
		Error:  errMsg,
	})
}

// WriteContent writes a content event
func (w *SSEWriter) WriteContent(content string) error {
	return w.Write(SSEEvent{Type: "content", Content: content})
}

// WriteDone writes a done event
func (w *SSEWriter) WriteDone() error {
	return w.Write(SSEEvent{Type: "done"})
}

// WriteError writes an error event
func (w *SSEWriter) WriteError(errMsg string) error {
	return w.Write(SSEEvent{Type: "error", Content: errMsg})
}

// Close closes the SSE stream
func (w *SSEWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.flushed {
		// Write already acquires mu, so call the underlying write directly.
		data, _ := json.Marshal(SSEEvent{Type: "done"})
		fmt.Fprintf(w.writer, "data: %s\n\n", string(data))
		if f, ok := w.writer.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// Flush flushes any buffered data
func (w *SSEWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
}

// WriteComment writes an SSE comment (ignored by EventSource, keeps connection alive)
func (w *SSEWriter) WriteComment(comment string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := fmt.Fprintf(w.writer, ": %s\n\n", comment)
	if err != nil {
		return fmt.Errorf("failed to write SSE comment: %w", err)
	}
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// ChatStreamResponse represents a streaming chat response
type ChatStreamResponse struct {
	Content string
	Done    bool
}

// ChatStream handles streaming responses from LLM
type ChatStream struct {
	resp            *http.Response
	reader          *bufio.Reader
	done            bool
	contentBuffer   string
	reasoningBuffer string
}

// NewChatStream creates a new chat stream
func NewChatStream(resp *http.Response) *ChatStream {
	return &ChatStream{
		resp:   resp,
		reader: bufio.NewReaderSize(resp.Body, 4096),
	}
}

// Recv receives the next chunk from the stream
func (s *ChatStream) Recv() (*ChatStreamResponse, error) {
	if s.done {
		return nil, io.EOF
	}

	// Read line by line (SSE format: "data: {...}")
	line, err := s.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		s.done = true
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	line = strings.TrimSpace(line)

	// Check for [DONE] marker or empty line
	if line == "[DONE]" || line == "" {
		if err == io.EOF {
			s.done = true
		}
		return &ChatStreamResponse{Done: s.done}, nil
	}

	// Remove "data: " prefix if present
	line = strings.TrimPrefix(line, "data: ")

	var openAIChunk struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningDetails []struct {
					Text string `json:"text"`
				} `json:"reasoning_details"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := json.Unmarshal([]byte(line), &openAIChunk); err == nil && len(openAIChunk.Choices) > 0 {
		content := s.deltaText(openAIChunk.Choices[0].Delta.Content, &s.contentBuffer)
		for _, detail := range openAIChunk.Choices[0].Delta.ReasoningDetails {
			content += s.deltaText(detail.Text, &s.reasoningBuffer)
		}

		resp := &ChatStreamResponse{
			Content: content,
		}

		if openAIChunk.Choices[0].FinishReason == "stop" || err == io.EOF {
			s.done = true
			resp.Done = true
		}

		return resp, nil
	}

	var anthropicChunk struct {
		Type  string `json:"type"`
		Delta struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	}

	if err := json.Unmarshal([]byte(line), &anthropicChunk); err != nil {
		return s.Recv()
	}

	if anthropicChunk.Type == "message_stop" {
		s.done = true
		return nil, io.EOF
	}

	var content string
	if anthropicChunk.Type == "content_block_delta" {
		switch anthropicChunk.Delta.Type {
		case "text_delta":
			content = anthropicChunk.Delta.Text
		case "thinking_delta":
			content = anthropicChunk.Delta.Thinking
		}
	}

	resp := &ChatStreamResponse{
		Content: content,
	}

	if err == io.EOF {
		s.done = true
		resp.Done = true
	}

	return resp, nil
}

func (s *ChatStream) deltaText(text string, buffer *string) string {
	if text == "" {
		return ""
	}
	if *buffer != "" && strings.HasPrefix(text, *buffer) {
		delta := strings.TrimPrefix(text, *buffer)
		*buffer = text
		return delta
	}
	*buffer += text
	return text
}

// Close closes the stream
func (s *ChatStream) Close() {
	s.resp.Body.Close()
}

// generateCallID generates a unique call ID
func generateCallID() string {
	return fmt.Sprintf("call_%d", time.Now().UnixNano())
}
