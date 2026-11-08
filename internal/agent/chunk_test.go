package agent

import (
	"strings"
	"testing"

	"argus/internal/protocol"
)

func TestSplitChunks(t *testing.T) {
	payload := strings.Repeat("x", chunkSize*2+100)
	chunks := splitChunks("s1", protocol.MsgScreenshotData, payload)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 0 || chunks[0].Total != 3 || chunks[0].Stream != "s1" {
		t.Fatalf("bad first chunk metadata: %+v", chunks[0])
	}
	if chunks[2].Index != 2 {
		t.Fatalf("bad last chunk index: %+v", chunks[2])
	}

	var b strings.Builder
	for _, c := range chunks {
		b.WriteString(c.Data)
	}
	if b.String() != payload {
		t.Fatal("reassembly mismatch")
	}
}

func TestSplitChunksSmall(t *testing.T) {
	payload := "short"
	chunks := splitChunks("s1", protocol.MsgScreenshotData, payload)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Data != payload {
		t.Fatal("single chunk should equal payload")
	}
}
