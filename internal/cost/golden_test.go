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

// A price refresh that moves a number shows up here as a diff to review. aws-full has every
// node type, so a matcher that stops picking one sku fails here as well as in the refresh;
// aws-basic carries a usage block, so its table has the usage lines.
func TestExampleTablesMatchTheGoldens(t *testing.T) {
	snapshot, err := cost.Bundled(ir.ProviderAWS)
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range []string{"aws-basic", "aws-full"} {
		t.Run(example, func(t *testing.T) {
			dir := filepath.Join("..", "..", "examples", example)
			project, errs, err := workspace.Validate(dir)
			if err != nil || len(errs) > 0 {
				t.Fatalf("examples/%s: %v %v", example, err, errs)
			}
			config, _, err := workspace.LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			if errs := workspace.CheckConfig(config, project); len(errs) > 0 {
				t.Fatalf("examples/%s/togen.yml: %v", example, errs)
			}
			doc, err := cost.Estimate(project, aws.Cost(), snapshot, config.Usage, snapshot.Taken())
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(doc.Table(), "\n") + "\n"

			golden := filepath.Join("testdata", example+".golden")
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
		})
	}
}
