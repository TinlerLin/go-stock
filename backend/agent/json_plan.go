package agent

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
)

// jsonPlan is a custom Plan implementation that uses encoding/json instead of sonic.
// sonic (used by Eino's defaultPlan) is stricter than encoding/json and may reject
// valid JSON containing CJK characters returned by LLMs.
type jsonPlan struct {
	Steps []string `json:"steps"`
}

func (p *jsonPlan) FirstStep() string {
	if len(p.Steps) == 0 {
		return ""
	}
	return p.Steps[0]
}

func (p *jsonPlan) MarshalJSON() ([]byte, error) {
	type planTyp jsonPlan
	return json.Marshal((*planTyp)(p))
}

func (p *jsonPlan) UnmarshalJSON(bytes []byte) error {
	type planTyp jsonPlan
	return json.Unmarshal(bytes, (*planTyp)(p))
}

// newJSONPlan creates a new jsonPlan instance for use with planexecute.
func newJSONPlan(ctx context.Context) planexecute.Plan {
	return &jsonPlan{}
}
