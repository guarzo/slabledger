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

// PurchaseAssessmentRequest provides context for evaluating a potential purchase.
type PurchaseAssessmentRequest struct {
	CampaignID   string `json:"campaignId"`
	CampaignName string `json:"campaignName"`
	CardName     string `json:"cardName"`
	SetName      string `json:"setName,omitempty"`
	Grade        string `json:"grade"`
	BuyCostCents int    `json:"buyCostCents"`
	CLValueCents int    `json:"clValueCents,omitempty"`
	CertNumber   string `json:"certNumber,omitempty"`
}
