package dto

import (
	"encoding/json"
	"time"
)

type ListActivityRequest struct {
	WorkspaceID string
	Limit       int
	Cursor      string
	From        string
	To          string
	ActorID     string
	Action      string
}

type ActivityLogResponse struct {
	ID         string          `json:"id"`
	ActorID    string          `json:"actor_id"`
	ActorName  string          `json:"actor_name"`
	ActorRole  string          `json:"actor_role"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	TargetName string          `json:"target_name"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

type ListActivityResponse struct {
	Items      []ActivityLogResponse `json:"items"`
	NextCursor string                `json:"next_cursor"`
}
