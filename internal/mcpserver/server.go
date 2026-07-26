// Package mcpserver implements the wolt-cli Model Context Protocol server.
// It exposes the same business logic the CLI calls — without going through
// cobra — to MCP clients like Claude Desktop, Claude Code, and Cursor.
package mcpserver

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerName is reported during MCP handshake. Clients display this in their
// tool inventory UI.
const ServerName = "wolt"

// NewServer constructs an MCP server with every wolt-cli tool registered.
// The returned server has not yet been connected to a transport — call
// server.Run(ctx, &mcp.StdioTransport{}) in main.
func NewServer(deps Deps) *mcp.Server {
	version := strings.TrimSpace(deps.Version)
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: version,
		Title:   "Wolt food delivery",
	}, nil)
	srv.AddReceivingMiddleware(newToolResultMiddleware(deps.DuplicateContent))

	tc := newToolCtx(deps)

	registerDiscoveryTools(srv, tc)
	registerVenueTools(srv, tc)
	registerAccountTools(srv, tc)
	registerFavoritesTools(srv, tc)
	registerCartTools(srv, tc)
	registerCheckoutTools(srv, tc)

	return srv
}
