package workspace

import (
	"testing"

	"github.com/mooncitizen/togen/internal/ir"
)

func TestNotGeneratedErrorNamesEveryNode(t *testing.T) {
	err := &NotGeneratedError{Nodes: []ir.NotGenerated{
		{Name: "payments-table", Type: "aws/dynamodb"},
		{Name: "audit-trail", Type: "aws/kinesis"},
	}}
	want := "these nodes draw but generate nothing: payments-table (aws/dynamodb), audit-trail (aws/kinesis)"
	if err.Error() != want {
		t.Errorf("error = %q", err.Error())
	}
}
