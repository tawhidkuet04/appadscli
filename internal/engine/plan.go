package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tawhidjoarder/adastra/internal/api"
	"github.com/tawhidjoarder/adastra/internal/store"
)

// ChangePlan is the PR-style proposal file written by `watch` (propose mode)
// and applied by `plan apply --confirm`.
type ChangePlan struct {
	CreatedAt time.Time    `json:"createdAt"`
	Source    string       `json:"source"` // watch | bids | harvest | manual
	Account   string       `json:"account"`
	Changes   []PlanChange `json:"changes"`
}

// PlanChange is one proposed mutation, expressed as the literal API call.
type PlanChange struct {
	Description string          `json:"description"`
	Method      string          `json:"method"` // POST | PUT
	Path        string          `json:"path"`
	Body        json.RawMessage `json:"body"`
	EntityType  string          `json:"entityType"`
	EntityID    string          `json:"entityId"`
}

// WritePlan saves a plan file.
func WritePlan(p *ChangePlan, path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// ReadPlan loads a plan file.
func ReadPlan(path string) (*ChangePlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p ChangePlan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s is not a valid plan file: %w", path, err)
	}
	return &p, nil
}

// ApplyPlan executes every change in order, stopping at the first failure.
func ApplyPlan(ctx context.Context, c *api.Client, st *store.Store, p *ChangePlan) ([]map[string]any, error) {
	var results []map[string]any
	for i, ch := range p.Changes {
		var body any
		if len(ch.Body) > 0 {
			if err := json.Unmarshal(ch.Body, &body); err != nil {
				return results, fmt.Errorf("change %d has invalid body: %w", i+1, err)
			}
		}
		var out json.RawMessage
		var err error
		switch ch.Method {
		case "POST":
			err = c.Post(ctx, ch.Path, body, &out)
		case "PUT":
			err = c.Put(ctx, ch.Path, body, &out)
		default:
			err = fmt.Errorf("unsupported method %q", ch.Method)
		}
		res := map[string]any{"description": ch.Description, "ok": err == nil}
		if err != nil {
			res["error"] = err.Error()
			results = append(results, res)
			return results, fmt.Errorf("change %d (%s) failed: %w", i+1, ch.Description, err)
		}
		_ = st.LogMutation(ch.EntityType, ch.EntityID, "plan-apply", ch.Description)
		results = append(results, res)
	}
	return results, nil
}
