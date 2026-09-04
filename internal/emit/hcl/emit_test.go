package hcl

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"togen/internal/ir"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func graph() *ir.Graph {
	return &ir.Graph{
		TerraformVersion: ">= 1.5",
		Provider: ir.Provider{
			Name:    "aws",
			Source:  "hashicorp/aws",
			Version: "~> 6.0",
			Config:  ir.Attrs{ir.A("region", ir.Str("eu-west-2"))},
		},
		Data: []ir.DataSource{
			{
				Type:        "aws_availability_zones",
				Name:        "available",
				SourceLabel: "network",
				Args:        ir.Attrs{ir.A("state", ir.Str("available"))},
			},
		},
		Resources: []ir.Resource{
			{
				Type:        "aws_vpc",
				Name:        "main",
				SourceLabel: "network",
				Args: ir.Attrs{
					ir.A("cidr_block", ir.Str("10.0.0.0/16")),
					ir.A("tags", ir.M(ir.A("Name", ir.Str("x")))),
					ir.A("enable_dns_support", ir.Bool(true)),
				},
			},
			{
				Type:        "aws_subnet",
				Name:        "a",
				SourceLabel: "network",
				Args: ir.Attrs{
					ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
					ir.A("cidr_block", ir.Str("10.0.0.0/24")),
					ir.A("availability_zone", ir.D(
						ir.ID{Type: "aws_availability_zones", Name: "available"},
						ir.Field("names"), ir.Index(0))),
				},
			},
			{
				Type:        "aws_lambda_function",
				Name:        "handler",
				SourceNode:  "n2",
				SourceLabel: "handler",
				Args: ir.Attrs{
					ir.A("function_name", ir.Str("shop-dev-handler")),
					ir.A("filename", ir.V("handler_package")),
					ir.A("source_code_hash", ir.Fn("filebase64sha256", ir.V("handler_package"))),
					ir.A("environment", ir.B(ir.Attrs{
						ir.A("variables", ir.M(ir.A("LOG_LEVEL", ir.Str("info")))),
					})),
					ir.A("tracing_config", ir.B(ir.Attrs{
						ir.A("mode", ir.Str("Active")),
					})),
				},
				DependsOn: []ir.ID{{Type: "aws_subnet", Name: "a"}},
			},
		},
		Variables: []ir.Variable{
			{
				Name:        "handler_package",
				Description: "Path to the zip",
				Type:        "string",
				Default:     ir.Str("functions/handler.zip"),
			},
		},
		Outputs: []ir.Output{
			{
				Name:        "api_url",
				Description: "Public URL",
				Value: ir.R(ir.ID{Type: "aws_apigatewayv2_api", Name: "api"},
					ir.Field("api_endpoint")),
			},
			{
				Name:        "db",
				Description: "Connection details",
				Value: ir.M(
					ir.A("host", ir.R(ir.ID{Type: "aws_db_instance", Name: "main_db"}, ir.Field("address"))),
					ir.A("port", ir.Num(5432)),
				),
			},
		},
	}
}

func TestEmit(t *testing.T) {
	files, err := Emit(graph())
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}

	want := []string{"providers.tf", "main.tf", "variables.tf", "outputs.tf"}
	if len(files) != len(want) {
		t.Fatalf("got %d files, want %d", len(files), len(want))
	}
	for _, name := range want {
		got, ok := files[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		path := filepath.Join("testdata", name+".golden")
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			continue
		}
		golden, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if diff := cmp.Diff(string(golden), string(got)); diff != "" {
			t.Errorf("%s (-golden +got):\n%s", name, diff)
		}
	}
}

func TestEmitOmitsEmptyFiles(t *testing.T) {
	g := graph()
	g.Variables = nil
	g.Outputs = nil
	files, err := Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	for _, name := range []string{"variables.tf", "outputs.tf"} {
		if _, ok := files[name]; ok {
			t.Errorf("%s should be omitted", name)
		}
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
}

func TestEmitRejectsBlockAtValuePosition(t *testing.T) {
	g := graph()
	g.Outputs = []ir.Output{{Name: "bad", Value: ir.B(ir.Attrs{ir.A("a", ir.Str("b"))})}}
	if _, err := Emit(g); err == nil {
		t.Fatal("want an error, got nil")
	}
}

func TestTerraformFmt(t *testing.T) {
	bin, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform is not on PATH")
	}
	files, err := Emit(graph())
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	out, err := exec.Command(bin, "fmt", "-check", "-diff", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("terraform fmt reported changes: %v\n%s", err, out)
	}
}
