package cost_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/aws"
	"github.com/mooncitizen/togen/internal/workspace"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// A price refresh that moves a number shows up here as a diff to review.
func TestAwsBasicTableMatchesTheGolden(t *testing.T) {
	project, errs, err := workspace.Validate(filepath.Join("..", "..", "examples", "aws-basic"))
	if err != nil || len(errs) > 0 {
		t.Fatalf("examples/aws-basic: %v %v", err, errs)
	}
	snapshot, err := cost.Bundled(ir.ProviderAWS)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := cost.Estimate(project, aws.Cost(), snapshot, snapshot.Taken())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(doc.Table(), "\n") + "\n"

	golden := filepath.Join("testdata", "aws-basic.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("table (-want +got):\n%s\nrun go test ./internal/cost -run Golden -update to accept it", diff)
	}
}
