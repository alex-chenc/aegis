package service

import (
	"strings"
	"testing"
)

func TestSplitDocumentForExtractionEmpty(t *testing.T) {
	if got := splitDocumentForExtraction("   "); len(got) != 0 {
		t.Fatalf("expected no chunks for empty content, got %d", len(got))
	}
}

func TestSplitDocumentForExtractionSingleSmallChunk(t *testing.T) {
	doc := "段落一：检查 SSH 配置。\n\n段落二：检查密码复杂度。"
	chunks := splitDocumentForExtraction(doc)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small doc, got %d: %#v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0], "段落一") || !strings.Contains(chunks[0], "段落二") {
		t.Fatalf("chunk should contain both paragraphs, got %q", chunks[0])
	}
}

func TestSplitDocumentForExtractionSplitsLargeDoc(t *testing.T) {
	// Build a doc whose total length exceeds ruleExtractChunkChars so it must
	// be split into multiple chunks.
	para := strings.Repeat("基线检查项 X：执行脚本验证配置是否正确。\n", 40)
	doc := strings.Repeat(para+"\n\n", 20) // many oversized paragraphs

	chunks := splitDocumentForExtraction(doc)
	if len(chunks) < 2 {
		t.Fatalf("expected the large doc to be split into multiple chunks, got %d", len(chunks))
	}

	for i, c := range chunks {
		if len(c) > ruleExtractChunkChars+len(para) {
			t.Fatalf("chunk %d exceeds budget: len=%d (budget=%d)", i, len(c), ruleExtractChunkChars)
		}
	}
}

func TestSplitDocumentForExtractionHardSplitsLongParagraph(t *testing.T) {
	// A single paragraph longer than the budget must be hard-split.
	long := strings.Repeat("x", ruleExtractChunkChars*3)
	chunks := splitDocumentForExtraction(long)
	if len(chunks) < 2 {
		t.Fatalf("expected a very long paragraph to be hard-split, got %d chunks", len(chunks))
	}
	for _, c := range chunks {
		if len(c) > ruleExtractChunkChars {
			t.Fatalf("hard-split chunk exceeded budget: len=%d", len(c))
		}
	}
}
