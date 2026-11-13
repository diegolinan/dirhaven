package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

type scriptedMCPConnection struct {
	incoming []jsonrpc.Message
	written  []jsonrpc.Message
	mu       sync.Mutex
}

func (s *scriptedMCPConnection) Read(context.Context) (jsonrpc.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.incoming) == 0 {
		panic("fixture exhausted")
	}
	msg := s.incoming[0]
	s.incoming = s.incoming[1:]
	return msg, nil
}
func (s *scriptedMCPConnection) Write(_ context.Context, msg jsonrpc.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.written = append(s.written, msg)
	return nil
}
func (s *scriptedMCPConnection) Close() error      { return nil }
func (s *scriptedMCPConnection) SessionID() string { return "" }

func testRequest(id int64, method string, params string) *jsonrpc.Request {
	jid, err := jsonrpc.MakeID(float64(id))
	if err != nil {
		panic(err)
	}
	return &jsonrpc.Request{ID: jid, Method: method, Params: json.RawMessage(params)}
}
func testNotification(method string) *jsonrpc.Request {
	return &jsonrpc.Request{Method: method, Params: json.RawMessage(`{}`)}
}
func testInitParams() string {
	return `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}`
}
func TestFilesystemRepeatedInitializeReplaysOnlyIdenticalFirstResponse(t *testing.T) {
	ctx := context.Background()
	first := testRequest(1, "initialize", testInitParams())
	second := testRequest(3, "initialize", `{ "clientInfo":{"version":"1","name":"test"},"capabilities":{},"protocolVersion":"2025-06-18" }`)
	underlying := &scriptedMCPConnection{incoming: []jsonrpc.Message{
		first, testNotification("notifications/initialized"),
		testRequest(2, "tools/list", `{}`),
		second, testNotification("notifications/initialized"),
		testRequest(4, "tools/list", `{}`),
	}}
	var events []string
	c := &stdioInitCompatConn{Connection: underlying, trace: func(event string) { events = append(events, event) }}
	next, err := c.Read(ctx)
	if err != nil || next != first {
		t.Fatalf("first initialize not delegated: %v", err)
	}
	original := json.RawMessage(`{"protocolVersion":"2025-06-18","serverInfo":{"name":"go"}}`)
	if err = c.Write(ctx, &jsonrpc.Response{ID: first.ID, Result: original}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"notifications/initialized", "tools/list", "tools/list"} {
		next, err = c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if req, ok := next.(*jsonrpc.Request); !ok || req.Method != expected {
			t.Fatalf("got %T want %s", next, expected)
		}
	}
	if len(underlying.written) != 2 {
		t.Fatalf("expected init and replay responses, got %d", len(underlying.written))
	}
	replayed, ok := underlying.written[1].(*jsonrpc.Response)
	if !ok || replayed.ID != second.ID || replayed.Error != nil || !bytes.Equal(replayed.Result, original) {
		t.Fatalf("replayed response had wrong ID, payload or status: %#v", underlying.written[1])
	}
	expectedEvents := []string{"initialize_first_pass", "initialized_first_pass", "tools_list_pass", "initialize_identical_replayed", "initialized_repeated_suppressed", "tools_list_pass"}
	if len(events) != len(expectedEvents) {
		t.Fatalf("events=%v", events)
	}
	for i, v := range expectedEvents {
		if events[i] != v {
			t.Fatalf("event %d got=%q want=%q", i, events[i], v)
		}
	}
}
func TestFilesystemRepeatedInitializeWithDifferentParamsFailsClosed(t *testing.T) {
	first := testRequest(1, "initialize", testInitParams())
	different := testRequest(3, "initialize", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"other-client","version":"1"}}`)
	after := testRequest(4, "tools/list", `{}`)
	underlying := &scriptedMCPConnection{incoming: []jsonrpc.Message{first, different, after}}
	c := &stdioInitCompatConn{Connection: underlying}
	ctx := context.Background()
	if _, err := c.Read(ctx); err != nil {
		t.Fatal(err)
	}
	c.Write(ctx, &jsonrpc.Response{ID: first.ID, Result: json.RawMessage(`{"some":"original"}`)})
	next, err := c.Read(ctx)
	if err != nil || next != after {
		t.Fatalf("mismatched init was not rejected and discarded: %v", err)
	}
	if len(underlying.written) != 2 {
		t.Fatalf("wrong write count %d", len(underlying.written))
	}
	rejected, ok := underlying.written[1].(*jsonrpc.Response)
	if !ok || rejected.Error == nil || rejected.ID != different.ID {
		t.Fatalf("different handshake accepted: %#v", underlying.written[1])
	}
	wire, ok := rejected.Error.(*jsonrpc.Error)
	if !ok || wire.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("unexpected error: %v", rejected.Error)
	}
	if len(rejected.Result) != 0 {
		t.Fatalf("unexpected data disclosed in rejected reinit")
	}
}
func TestFilesystemInitRejectionAllowsNewFirstHandshake(t *testing.T) {
	first := testRequest(1, "initialize", testInitParams())
	second := testRequest(2, "initialize", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"retry","version":"1"}}`)
	underlying := &scriptedMCPConnection{incoming: []jsonrpc.Message{first, second}}
	c := &stdioInitCompatConn{Connection: underlying}
	ctx := context.Background()
	if got, err := c.Read(ctx); err != nil || got != first {
		t.Fatalf("first handshake failed: %v", err)
	}
	_ = c.Write(ctx, &jsonrpc.Response{ID: first.ID, Error: &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "bad initialization"}})
	got, err := c.Read(ctx)
	if err != nil || got != second {
		t.Fatalf("retry after initial failure not passed to SDK: %v", err)
	}
	if len(underlying.written) != 1 {
		t.Fatalf("unexpected replay after failure")
	}
}
func TestFilesystemInitResultNotOverwrittenOnReusedRPCID(t *testing.T) {
	first := testRequest(1, "initialize", testInitParams())
	second := testRequest(3, "initialize", testInitParams())
	after := testRequest(4, "tools/list", `{}`)
	underlying := &scriptedMCPConnection{incoming: []jsonrpc.Message{first, second, after}}
	c := &stdioInitCompatConn{Connection: underlying}
	ctx := context.Background()
	if _, err := c.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, &jsonrpc.Response{ID: first.ID, Result: json.RawMessage(`{"serverInfo":{"name":"original"}}`)}); err != nil {
		t.Fatal(err)
	}
	_ = c.Write(ctx, &jsonrpc.Response{ID: first.ID, Result: json.RawMessage(`{"response":"unrelated tool result"}`)})
	next, err := c.Read(ctx)
	if err != nil || next != after {
		t.Fatalf("tools/list not delivered: %v", err)
	}
	replay := underlying.written[len(underlying.written)-1].(*jsonrpc.Response)
	if !bytes.Contains(replay.Result, []byte("original")) || bytes.Contains(replay.Result, []byte("unrelated")) {
		t.Fatalf("replayed cached response was overwritten by reused JSONRPC ID")
	}
}
func TestFilesystemUnsolicitedDuplicateInitializedNotSilenced(t *testing.T) {
	first := testRequest(1, "initialize", testInitParams())
	initial := testNotification("notifications/initialized")
	unsolicited := testNotification("notifications/initialized")
	c := &stdioInitCompatConn{Connection: &scriptedMCPConnection{incoming: []jsonrpc.Message{first, initial, unsolicited}}}
	ctx := context.Background()
	if _, err := c.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if x, err := c.Read(ctx); err != nil || x != initial {
		t.Fatal("initial notification not passed")
	}
	if x, err := c.Read(ctx); err != nil || x != unsolicited {
		t.Fatal("unsolicited duplicate must remain SDK-owned")
	}
}
