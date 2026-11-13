package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// stdioInitCompatTransport is deliberately limited to the filesystem MCP
// process, which has no per-client conversational state. The existing Node
// filesystem server accepts repeated initialize handshakes on one stdio pipe;
// the Go MCP SDK rejects them. Some tunnel clients depend on that behavior.
//
// The first initialization is handled by the official SDK. Only subsequent
// initialize calls with *identical canonical parameters* receive a replay of
// the initial response with their own request ID. All other methods and
// notifications pass to the SDK without modification.
type stdioInitCompatTransport struct {
	base  mcp.Transport
	trace func(string)
}

func (t *stdioInitCompatTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	conn, err := t.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &stdioInitCompatConn{Connection: conn, trace: t.trace}, nil
}

type stdioInitCompatConn struct {
	mcp.Connection
	trace func(string)

	mu                    sync.Mutex
	firstInitID           jsonrpc.ID
	firstInitParams       []byte
	firstInitResult       json.RawMessage
	firstInitHandled      bool
	firstInitialized      bool
	duplicateInitNotifies int
}

func (c *stdioInitCompatConn) record(event string) {
	if c.trace != nil {
		c.trace(event)
	}
}

// canonicalInitializeParams compares only protocol-level handshake metadata.
// It is never logged or persisted. A mismatch is not silently treated as a
// new logical session, since this transport cannot isolate that new client.
func canonicalInitializeParams(raw json.RawMessage) ([]byte, error) {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, fmt.Errorf("initialize parameters must be a JSON object")
	}
	return json.Marshal(value)
}

func (c *stdioInitCompatConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	for {
		message, err := c.Connection.Read(ctx)
		if err != nil {
			return nil, err
		}
		req, ok := message.(*jsonrpc.Request)
		if !ok {
			return message, nil
		}
		switch req.Method {
		case "initialize":
			if !req.ID.IsValid() {
				// Preserve the SDK's strict rejection of invalid notifications.
				c.record("initialize_invalid_notification")
				return message, nil
			}
			canonical, err := canonicalInitializeParams(req.Params)
			if err != nil {
				c.record("initialize_invalid_params")
				return message, nil
			}
			c.mu.Lock()
			if !c.firstInitID.IsValid() {
				c.firstInitID = req.ID
				c.firstInitParams = canonical
				c.mu.Unlock()
				c.record("initialize_first_pass")
				return message, nil
			}
			equal := bytes.Equal(c.firstInitParams, canonical)
			result := append(json.RawMessage(nil), c.firstInitResult...)
			c.mu.Unlock()

			if !equal {
				c.record("initialize_changed_params_rejected")
				err = c.Connection.Write(ctx, &jsonrpc.Response{
					ID: req.ID, Error: &jsonrpc.Error{
						Code:    jsonrpc.CodeInvalidParams,
						Message: "different initialization parameters on an existing stdio connection",
					},
				})
				if err != nil {
					return nil, err
				}
				continue
			}
			if len(result) == 0 {
				c.record("initialize_pending_rejected")
				err = c.Connection.Write(ctx, &jsonrpc.Response{
					ID: req.ID, Error: &jsonrpc.Error{
						Code:    jsonrpc.CodeInvalidRequest,
						Message: "previous initialize has not completed",
					},
				})
				if err != nil {
					return nil, err
				}
				continue
			}
			// Replay the SDK's actual response. Never synthesize advertised
			// capabilities, server details, or a protocol version.
			if err := c.Connection.Write(ctx, &jsonrpc.Response{ID: req.ID, Result: result}); err != nil {
				return nil, err
			}
			c.mu.Lock()
			c.duplicateInitNotifies++
			c.mu.Unlock()
			c.record("initialize_identical_replayed")
			continue

		case "notifications/initialized":
			c.mu.Lock()
			if !c.firstInitialized {
				c.firstInitialized = true
				c.mu.Unlock()
				c.record("initialized_first_pass")
				return message, nil
			}
			if c.duplicateInitNotifies > 0 {
				c.duplicateInitNotifies--
				c.mu.Unlock()
				c.record("initialized_repeated_suppressed")
				continue
			}
			c.mu.Unlock()
			c.record("initialized_unsolicited_pass")
			return message, nil
		default:
			if req.Method == "tools/list" {
				c.record("tools_list_pass")
			} else if req.Method == "tools/call" {
				c.record("tools_call_pass")
			}
			return message, nil
		}
	}
}

func (c *stdioInitCompatConn) Write(ctx context.Context, message jsonrpc.Message) error {
	if resp, ok := message.(*jsonrpc.Response); ok {
		c.mu.Lock()
		// Cache only the initial handshake response once. A JSON-RPC client
		// may reuse the same numeric ID for an unrelated future tools/call.
		if c.firstInitID.IsValid() && !c.firstInitHandled && resp.ID == c.firstInitID {
			if resp.Error == nil && len(resp.Result) > 0 {
				c.firstInitResult = append(json.RawMessage(nil), resp.Result...)
				c.firstInitHandled = true
			} else {
				// If the SDK rejected initial negotiation, let a later
				// valid initialize attempt begin a fresh handshake.
				c.firstInitID = jsonrpc.ID{}
				c.firstInitParams = nil
				c.firstInitResult = nil
				c.firstInitHandled = false
				c.firstInitialized = false
			}
		}
		c.mu.Unlock()
	}
	return c.Connection.Write(ctx, message)
}

func (s *Service) Serve(ctx context.Context) error {
	var trace func(string)
	if os.Getenv("DIRHAVEN_MCP_TRACE_INIT") == "1" {
		var sequence atomic.Uint64
		trace = func(event string) {
			// Event strings are constants from this file. No payloads, IDs,
			// client parameters, paths, credentials or tool results are logged.
			fmt.Fprintf(os.Stderr, "filesystem_mcp_method_seq=%d event=%s\n", sequence.Add(1), event)
		}
	}
	return s.MCP().Run(ctx, &stdioInitCompatTransport{
		base:  &mcp.StdioTransport{},
		trace: trace,
	})
}
