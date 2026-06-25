package agenttools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// RegisterMCP registers every MCP-exposed tool from the catalog onto the server.
// It replaces the per-tool mcp.AddTool calls that used to live in the mcp
// package, so the MCP surface is now derived from the same catalog as the chat
// surface.
func RegisterMCP(server *mcp.Server, d Deps) {
	for _, t := range All() {
		if t.Exposes(TransportMCP) {
			t.registerMCP(server, d)
		}
	}
}
