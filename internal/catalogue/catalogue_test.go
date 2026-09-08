package catalogue

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

func lines(t *testing.T, provider string) []string {
	t.Helper()
	scheme, ok := Embedded().Scheme(provider)
	if !ok {
		t.Fatalf("%s has no scheme", provider)
	}
	out := []string{
		"accent " + scheme.Accent,
		"network " + scheme.Network.Kind + " " + scheme.Network.Color,
	}
	if scheme.ResourceGroup != nil {
		out = append(out, "resource group "+scheme.ResourceGroup.Kind+" "+scheme.ResourceGroup.Color)
	}
	for _, e := range Embedded().For(provider) {
		k := scheme.Kinds[e.ID]
		out = append(out, fmt.Sprintf("%s %s %s %s %s", e.ID, k.Color, k.Icon, k.Shape, k.Resource))
	}
	return out
}

// Every entry is spelled out so a changed colour or icon shows in the diff.
func TestSchemesArePinned(t *testing.T) {
	want := map[string][]string{
		"aws": {
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
		"gcp": {
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
		"azure": {
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
	for _, p := range Embedded().Providers() {
		if diff := cmp.Diff(want[p], lines(t, p)); diff != "" {
			t.Errorf("%s scheme (-want +got):\n%s", p, diff)
		}
	}
}

func TestEveryEntryIsStyled(t *testing.T) {
	hex := regexp.MustCompile(`^#[0-9A-F]{6}$`)
	for _, p := range Embedded().Providers() {
		scheme, _ := Embedded().Scheme(p)
		if !hex.MatchString(scheme.Accent) || !hex.MatchString(scheme.Network.Color) {
			t.Errorf("%s: accent %q or network colour %q is not #RRGGBB", p, scheme.Accent, scheme.Network.Color)
		}
		entries := Embedded().For(p)
		if len(scheme.Kinds) != len(entries) {
			t.Errorf("%s styles %d kinds, want %d", p, len(scheme.Kinds), len(entries))
		}
		for _, e := range entries {
			if !hex.MatchString(e.Style.Color) {
				t.Errorf("%s %s: colour %q is not #RRGGBB", p, e.ID, e.Style.Color)
			}
			if !slices.Contains(Shapes, e.Style.Shape) {
				t.Errorf("%s %s: shape %q is not one of %v", p, e.ID, e.Style.Shape, Shapes)
			}
		}
	}
}

func TestIconsListsEveryIconOnce(t *testing.T) {
	ids := Embedded().Icons()
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Errorf("icons = %v", ids)
	}
	for _, p := range Embedded().Providers() {
		for _, e := range Embedded().For(p) {
			if !slices.Contains(ids, e.Style.Icon) {
				t.Errorf("%s is missing", e.Style.Icon)
			}
		}
	}
}

func TestBundledIconFilesExist(t *testing.T) {
	for _, id := range Embedded().Icons() {
		path := filepath.Join("..", "..", "ui", "icons", filepath.FromSlash(id)+".svg")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

// The documentation catalogue says where each icon comes from, so it lists what is used.
func TestDocumentedStylesListEveryIconAndColour(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "catalogue", "styles.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, p := range Embedded().Providers() {
		for _, e := range Embedded().For(p) {
			for _, needle := range []string{e.Style.Icon, e.Style.Color, e.Resource} {
				if !strings.Contains(doc, needle) {
					t.Errorf("docs/catalogue/styles.md does not mention %s for %s %s", needle, p, e.ID)
				}
			}
		}
	}
}

func TestRolesCoverEveryEntry(t *testing.T) {
	for _, e := range Embedded().All() {
		if len(e.Roles) == 0 {
			t.Errorf("%s has no roles", e.ID)
		}
	}
	for _, want := range []struct {
		role Role
		ids  []string
	}{
		{RoleEntry, []string{"gateway"}},
		{RoleCompute, []string{"service", "function"}},
		{RoleStore, []string{"database", "bucket", "cache"}},
		{RoleMessaging, []string{"queue"}},
	} {
		if diff := cmp.Diff(want.ids, Embedded().WithRole(want.role)); diff != "" {
			t.Errorf("%s (-want +got):\n%s", want.role, diff)
		}
	}
}

func TestPortableEntriesAreInEveryProvider(t *testing.T) {
	for _, e := range Embedded().All() {
		if !e.Portable() {
			continue
		}
		for _, p := range Embedded().Providers() {
			if _, ok := Embedded().Lookup(p, e.ID); !ok {
				t.Errorf("%s is bare but %s has not got it", e.ID, p)
			}
		}
	}
}

func TestGeneratesEntriesDeclareNoProperties(t *testing.T) {
	for _, p := range Embedded().Providers() {
		for _, e := range Embedded().For(p) {
			if e.Tier == TierGenerates && e.Properties != nil {
				t.Errorf("%s %s generates and declares properties", p, e.ID)
			}
			if e.Tier == TierDraws && e.Properties == nil {
				t.Errorf("%s %s draws and declares no properties", p, e.ID)
			}
		}
	}
}

const drawOnly = `{
  "provider": "aws",
  "accent": "#ED7100",
  "network": { "kind": "VPC", "color": "#8C4FFF" },
  "entries": [
    { "id": "queue", "tier": "generates", "resource": "SQS",
      "style": { "color": "#E7157B", "icon": "aws/sqs", "shape": "card" } },
    { "id": "aws/table", "label": "Table", "group": "Databases", "description": "A key value table.",
      "roles": ["store"], "tier": "draws", "resource": "DynamoDB", "properties": {},
      "style": { "color": "#C925D1", "icon": "aws/rds", "shape": "cylinder" } }
  ]
}`

func fixture(t *testing.T, files map[string]string) *Catalogue {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys["data/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	c, err := Load(fsys, "data")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func minimal(provider, entries string) string {
	return fmt.Sprintf(`{"provider": %q, "accent": "#000000", "network": {"kind": "n", "color": "#000000"}, "entries": [%s]}`, provider, entries)
}

const queueOnly = `{"entries": [{"id": "queue", "label": "Queue", "group": "Messaging", "description": "d", "roles": ["messaging"], "usage": ["messages"]}]}`

func styled(id, tier string) string {
	return fmt.Sprintf(`{"id": %q, "tier": %q, "resource": "R", "style": {"color": "#000000", "icon": "i", "shape": "card"}}`, id, tier)
}

func TestLoadMergesCoreIntoEveryProvider(t *testing.T) {
	c := fixture(t, map[string]string{
		"core.json":  queueOnly,
		"aws.json":   drawOnly,
		"gcp.json":   minimal("gcp", styled("queue", "generates")),
		"azure.json": minimal("azure", styled("queue", "generates")),
	})
	queue, ok := c.Lookup("aws", "queue")
	if !ok || queue.Label != "Queue" || queue.Resource != "SQS" || !queue.HasRole(RoleMessaging) {
		t.Fatalf("aws queue = %+v", queue)
	}
	table, ok := c.Lookup("aws", "aws/table")
	if !ok || table.Tier != TierDraws || table.Portable() {
		t.Fatalf("aws/table = %+v", table)
	}
	if _, ok := c.Lookup("gcp", "aws/table"); ok {
		t.Error("aws/table reached the gcp catalogue")
	}
	if diff := cmp.Diff([]string{"queue", "aws/table"}, c.IDs()); diff != "" {
		t.Errorf("ids (-want +got):\n%s", diff)
	}
}

func TestLoadRejectsBadData(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			"bare id core has not got",
			map[string]string{"core.json": queueOnly, "aws.json": minimal("aws", styled("bucket", "generates"))},
			"which core.json has not got",
		},
		{
			"core id a provider is missing",
			map[string]string{
				"core.json":  queueOnly,
				"aws.json":   minimal("aws", styled("queue", "generates")),
				"gcp.json":   minimal("gcp", ""),
				"azure.json": minimal("azure", styled("queue", "generates")),
			},
			"which gcp.json has not got",
		},
		{
			"duplicate id",
			map[string]string{
				"core.json": queueOnly,
				"aws.json":  minimal("aws", styled("queue", "generates")+","+styled("queue", "generates")),
			},
			"declares 'queue' twice",
		},
		{
			"unknown tier",
			map[string]string{"core.json": queueOnly, "aws.json": minimal("aws", styled("queue", "maybe"))},
			"which is not one of generates or draws",
		},
		{
			"no style",
			map[string]string{"core.json": queueOnly, "aws.json": minimal("aws", `{"id": "queue", "tier": "draws", "resource": "R"}`)},
			"has no style",
		},
		{
			"unknown field",
			map[string]string{"core.json": queueOnly, "aws.json": minimal("aws", `{"id": "queue", "teir": "draws"}`)},
			"unknown field",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for name, body := range c.files {
				fsys["data/"+name] = &fstest.MapFile{Data: []byte(body)}
			}
			_, err := Load(fsys, "data")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %s", err, c.want)
			}
		})
	}
}
