package domain

import "time"

// Brand describes the public identity used for a conversation.
type Brand struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	AgentName    string `json:"agent_name,omitempty"`
	SupportEmail string `json:"support_email,omitempty"`
}

// TenantContext identifies the product and brand serving the customer.
type TenantContext struct {
	ID    string `json:"id"`
	Brand Brand  `json:"brand"`
}

// Turn is a compact conversation-history item.
type Turn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest starts one agent run.
type ChatRequest struct {
	Question       string        `json:"question"`
	History        []Turn        `json:"history,omitempty"`
	Tenant         TenantContext `json:"tenant"`
	ConversationID string        `json:"conversation_id,omitempty"`
}

// CustomerContext contains provider-owned customer facts safe for support use.
type CustomerContext struct {
	CustomerID string         `json:"customer_id,omitempty"`
	Anonymous  bool           `json:"anonymous"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// CapabilitySet declares which provider operations are available.
type CapabilitySet struct {
	Provider     string   `json:"provider"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

// KnowledgeItem is one approved knowledge result.
type KnowledgeItem struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score,omitempty"`
	Source  string  `json:"source,omitempty"`
}

// Resource is a provider-neutral customer resource such as a generation, order, or ticket.
type Resource struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Diagnosis is a safe, provider-produced explanation of a resource state.
type Diagnosis struct {
	ResourceID  string   `json:"resource_id"`
	Status      string   `json:"status"`
	Category    string   `json:"category,omitempty"`
	Message     string   `json:"message"`
	Charged     int      `json:"charged,omitempty"`
	Refunded    int      `json:"refunded,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Transaction represents a customer-visible account movement.
type Transaction struct {
	ID         string    `json:"id"`
	ResourceID string    `json:"resource_id,omitempty"`
	Type       string    `json:"type"`
	Amount     int       `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
	Note       string    `json:"note,omitempty"`
}

// CreateCaseInput requests a human-support case.
type CreateCaseInput struct {
	ConversationID string   `json:"conversation_id,omitempty"`
	Subject        string   `json:"subject"`
	Summary        string   `json:"summary"`
	ResourceIDs    []string `json:"resource_ids,omitempty"`
	Priority       string   `json:"priority,omitempty"`
}

// SupportCase is the provider response after a case is created.
type SupportCase struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// StreamEvent is Nivora's stable SSE protocol.
type StreamEvent struct {
	Type           string `json:"type"`
	RequestID      string `json:"request_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Content        string `json:"content,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	Code           string `json:"code,omitempty"`
}
