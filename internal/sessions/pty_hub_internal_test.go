package sessions

import (
	"sync"
	"testing"
	"time"
)

func TestByteRingWriteAndRead(t *testing.T) {
	r := newByteRing(8)
	r.write([]byte("hello"))
	got := string(r.bytes())
	if got != "hello" {
		t.Fatalf("bytes = %q, want hello", got)
	}

	// Overflow: keep last 8 bytes.
	r.write([]byte("world!!!"))
	got = string(r.bytes())
	if got != "world!!!" {
		t.Fatalf("bytes after overflow = %q, want world!!!", got)
	}
}

func TestByteRingEmpty(t *testing.T) {
	r := newByteRing(16)
	if r.bytes() != nil {
		t.Fatalf("expected nil for empty ring, got %q", r.bytes())
	}
}

func TestPTYHubBroadcastUnsubscribeConcurrent(t *testing.T) {
	h := &ptyHub{
		scrollback:  newByteRing(1024),
		subscribers: make(map[int]*ptySubscriber),
	}
	done := make(chan struct{})

	var streams []*PTYStream
	for i := 0; i < 16; i++ {
		streams = append(streams, h.subscribe(done))
	}

	var wg sync.WaitGroup
	for _, stream := range streams {
		wg.Add(1)
		go func(s *PTYStream) {
			defer wg.Done()
			for range s.Output() {
			}
		}(stream)
		go func(s *PTYStream) {
			time.Sleep(time.Millisecond)
			_ = s.Close()
		}(stream)
	}

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		h.broadcast([]byte("chunk"))
	}

	wg.Wait()
}
