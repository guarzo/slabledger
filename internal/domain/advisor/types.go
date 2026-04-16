package advisor

import "github.com/guarzo/slabledger/internal/domain/ai"

// Re-export shared AI types so existing imports continue to work.
type Role = ai.Role

const (
	RoleUser      = ai.RoleUser
	RoleAssistant = ai.RoleAssistant
	RoleTool      = ai.RoleTool
)

type Message = ai.Message
type ToolCall = ai.ToolCall
type EventType = ai.EventType

const (
	EventDelta      = ai.EventDelta
	EventToolStart  = ai.EventToolStart
	EventToolResult = ai.EventToolResult
	EventDone       = ai.EventDone
	EventError      = ai.EventError
	EventScore      = ai.EventScore
)

type StreamEvent = ai.StreamEvent
