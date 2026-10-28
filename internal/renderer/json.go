package renderer

import (
	"encoding/json"
	"io"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderJSON serializes the CostReport into indented JSON.
func RenderJSON(w io.Writer, r *awsclient.CostReport) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}
