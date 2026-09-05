package style

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

func lines(p ir.CloudProvider) []string {
	scheme := Schemes[p]
	out := []string{
		"accent " + scheme.Accent,
		"network " + scheme.Network.Kind + " " + scheme.Network.Color,
	}
	if scheme.ResourceGroup != nil {
		out = append(out, "resource group "+scheme.ResourceGroup.Kind+" "+scheme.ResourceGroup.Color)
	}
	for _, t := range ir.NodeTypes {
		k := scheme.Kinds[t]
		out = append(out, fmt.Sprintf("%s %s %s %s %s", t, k.Color, k.Icon, k.Shape, k.Resource))
	}
	return out
}

// Every entry is spelled out so a changed colour or icon shows in the diff.
func TestSchemesArePinned(t *testing.T) {
	want := map[ir.CloudProvider][]string{
		ir.ProviderAWS: {
			"accent #ED7100",
			"network VPC #8C4FFF",
			"service #ED7100 aws/ecs card ECS on Fargate",
			"function #ED7100 aws/lambda card Lambda",
			"database #C925D1 aws/rds cylinder RDS",
			"gateway #8C4FFF aws/api-gateway card API Gateway",
			"queue #E7157B aws/sqs card SQS",
			"bucket #7AA116 aws/s3 card S3",
			"cache #C925D1 aws/elasticache cylinder ElastiCache",
		},
		ir.ProviderGCP: {
			"accent #4285F4",
			"network VPC network #4285F4",
			"service #34A853 gcp/cloud-run card Cloud Run",
			"function #34A853 gcp/cloud-functions card Cloud Functions",
			"database #4285F4 gcp/cloud-sql cylinder Cloud SQL",
			"gateway #4285F4 gcp/api-gateway card API Gateway",
			"queue #F9AB00 gcp/pubsub card Pub/Sub",
			"bucket #EA4335 gcp/cloud-storage card Cloud Storage",
			"cache #4285F4 gcp/memorystore cylinder Memorystore",
		},
		ir.ProviderAzure: {
			"accent #0078D4",
			"network VNet #0078D4",
			"resource group Resource group #0078D4",
			"service #0078D4 azure/container-apps card Container Apps",
			"function #0078D4 azure/functions card Functions",
			"database #7B3FE4 azure/database-postgresql cylinder Database for PostgreSQL",
			"gateway #0078D4 azure/api-management card API Management",
			"queue #FFB900 azure/service-bus card Service Bus",
			"bucket #6BB700 azure/storage-blob card Blob Storage",
			"cache #7B3FE4 azure/cache-redis cylinder Cache for Redis",
		},
	}
	for _, p := range ir.Providers {
		if diff := cmp.Diff(want[p], lines(p)); diff != "" {
			t.Errorf("%s scheme (-want +got):\n%s", p, diff)
		}
	}
}

func TestEveryProviderStylesEveryNodeType(t *testing.T) {
	hex := regexp.MustCompile(`^#[0-9A-F]{6}$`)
	for _, p := range ir.Providers {
		scheme, ok := Schemes[p]
		if !ok {
			t.Errorf("%s has no scheme", p)
			continue
		}
		if !hex.MatchString(scheme.Accent) || !hex.MatchString(scheme.Network.Color) {
			t.Errorf("%s: accent %q or network colour %q is not #RRGGBB", p, scheme.Accent, scheme.Network.Color)
		}
		if len(scheme.Kinds) != len(ir.NodeTypes) {
			t.Errorf("%s styles %d kinds, want %d", p, len(scheme.Kinds), len(ir.NodeTypes))
		}
		for _, nodeType := range ir.NodeTypes {
			k, ok := scheme.Kinds[nodeType]
			if !ok {
				t.Errorf("%s has no style for %s", p, nodeType)
				continue
			}
			if !hex.MatchString(k.Color) {
				t.Errorf("%s %s: colour %q is not #RRGGBB", p, nodeType, k.Color)
			}
			if !strings.HasPrefix(k.Icon, string(p)+"/") {
				t.Errorf("%s %s: icon %q is not in the provider's set", p, nodeType, k.Icon)
			}
			if !slices.Contains(Shapes, k.Shape) {
				t.Errorf("%s %s: shape %q is not one of %v", p, nodeType, k.Shape, Shapes)
			}
			if k.Resource == "" {
				t.Errorf("%s %s has no resource name", p, nodeType)
			}
		}
	}
}

func TestBundledIconsListsEveryIconOnce(t *testing.T) {
	ids := BundledIcons()
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) || len(ids) != len(ir.Providers)*len(ir.NodeTypes) {
		t.Errorf("icons = %v", ids)
	}
	for _, scheme := range Schemes {
		for _, k := range scheme.Kinds {
			if !slices.Contains(ids, k.Icon) {
				t.Errorf("%s is missing", k.Icon)
			}
		}
	}
}

// The catalogue says where each icon comes from, so it lists what the schemes use.
func TestCatalogueListsEveryIconAndColour(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "catalogue", "styles.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for p, scheme := range Schemes {
		for nodeType, k := range scheme.Kinds {
			for _, needle := range []string{k.Icon, k.Color, k.Resource} {
				if !strings.Contains(doc, needle) {
					t.Errorf("docs/catalogue/styles.md does not mention %s for %s %s", needle, p, nodeType)
				}
			}
		}
	}
}
