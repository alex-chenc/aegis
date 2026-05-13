package llm

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestWriteCommentOutputsSSECommentFormat(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewSSEWriter(recorder)

	err := writer.WriteComment("keepalive")
	if err != nil {
		t.Fatalf("WriteComment returned error: %v", err)
	}

	body := recorder.Body.String()
	// SSE comments start with ": " and end with "\n\n"
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Fatalf("expected SSE comment ': keepalive\\n\\n', got: %q", body)
	}
	// Must NOT contain "data:" — comments are not data events
	if strings.Contains(body, "data:") {
		t.Fatalf("WriteComment must not emit data events, got: %q", body)
	}
}

func TestWriteCommentDoesNotAffectSubsequentDataEvents(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewSSEWriter(recorder)

	_ = writer.WriteComment("keepalive")
	_ = writer.WriteDone()

	body := recorder.Body.String()
	// Both comment and data event should be present
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Fatalf("expected keepalive comment, got: %q", body)
	}
	if !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("expected done event after comment, got: %q", body)
	}
}

func TestWriteCommentWithEmptyString(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewSSEWriter(recorder)

	err := writer.WriteComment("")
	if err != nil {
		t.Fatalf("WriteComment with empty string returned error: %v", err)
	}

	body := recorder.Body.String()
	// Empty comment is still valid SSE
	if !strings.Contains(body, ": \n\n") {
		t.Fatalf("expected empty SSE comment, got: %q", body)
	}
}

func TestConcurrentWriteAndComment(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewSSEWriter(recorder)

	var wg sync.WaitGroup
	// Simulate keepalive goroutine writing comments while main goroutine writes events
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = writer.WriteComment("keepalive")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = writer.WriteThinking("processing...")
		}
	}()
	wg.Wait()

	body := recorder.Body.String()
	// Both comments and data events should be present without corruption
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Fatalf("expected keepalive comments in output")
	}
	if !strings.Contains(body, `"type":"thinking"`) {
		t.Fatalf("expected thinking events in output")
	}
}
