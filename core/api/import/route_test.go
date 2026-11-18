package importzip

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// An upload that stops sending is dropped after stall, not held open forever; a slow one that keeps sending goes on.
func TestStalledUpload(t *testing.T) {
	stall = 200 * time.Millisecond
	t.Cleanup(func() { stall = time.Minute })
	server := httptest.NewServer(Handler(nil, nil)) // never reaches the base: the body fails first
	defer server.Close()

	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 1,000 bytes announced, a few sent at half the stall each, then nothing.
	if _, err := conn.Write([]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Type: application/zip\r\nContent-Length: 1000\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		time.Sleep(stall / 2)
		if _, err := conn.Write([]byte("PK")); err != nil {
			t.Fatalf("dropped while still sending: %v", err)
		}
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil {
		response.Body.Close()
		if response.StatusCode < 400 {
			t.Errorf("status = %d, want an error", response.StatusCode)
		}
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("the stalled upload is still open after 5 s")
	}
}
