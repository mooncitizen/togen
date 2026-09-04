package ir

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func testGraph(resources ...Resource) *Graph {
	return &Graph{
		TerraformVersion: ">= 1.5",
		Provider: Provider{
			Name:    "aws",
			Source:  "hashicorp/aws",
			Version: "~> 6.0",
			Config:  Attrs{A("region", Str("eu-west-2"))},
		},
		Resources: resources,
	}
}

func TestConstructorsBuildValues(t *testing.T) {
	vpc := ID{Type: "aws_vpc", Name: "main"}
	cases := []struct {
		name string
		got  Value
		want Value
	}{
		{"str", Str("x"), String("x")},
		{"num", Num(5), Number(5)},
		{"list", L(Str("a"), Num(1)), List{String("a"), Number(1)}},
		{"map", M(A("a", Num(1))), Map{{Key: "a", Value: Number(1)}}},
		{"block", B(Attrs{A("a", Num(1))}, Attrs{A("a", Num(2))}), Block{{{Key: "a", Value: Number(1)}}, {{Key: "a", Value: Number(2)}}}},
		{"resource ref", R(vpc, Field("id")), Ref{Kind: RefResource, Target: vpc, Path: []Step{{Field: "id"}}}},
		{"data ref", D(vpc, Index(0)), Ref{Kind: RefData, Target: vpc, Path: []Step{{Index: 0, IsIndex: true}}}},
		{"var", V("region"), Var{Name: "region"}},
		{"json", J(Str("a")), JSON{Value: String("a")}},
		{"concat", C(Str("a"), V("b")), Concat{Parts: []Value{String("a"), Var{Name: "b"}}}},
		{"call", Fn("filebase64sha256", Str("f.zip")), Call{Fn: "filebase64sha256", Args: []Value{String("f.zip")}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if d := cmp.Diff(c.want, c.got); d != "" {
				t.Fatal(d)
			}
		})
	}
}

func TestIDString(t *testing.T) {
	if got := (ID{Type: "aws_vpc", Name: "main"}).String(); got != "aws_vpc.main" {
		t.Fatalf("String() = %q", got)
	}
}

func TestAttrsGetAndSet(t *testing.T) {
	a := Attrs{A("one", Num(1))}
	if v, ok := a.Get("one"); !ok || v != Number(1) {
		t.Fatalf("Get(one) = %v, %v", v, ok)
	}
	if _, ok := a.Get("two"); ok {
		t.Fatal("Get(two) should not be found")
	}
	a.Set("two", Num(2))
	a.Set("one", Num(3))
	want := Attrs{A("one", Num(3)), A("two", Num(2))}
	if d := cmp.Diff(want, a); d != "" {
		t.Fatal(d)
	}
}

func TestValidateGraphAcceptsRefsToResourcesInTheGraph(t *testing.T) {
	g := testGraph(
		Resource{Type: "aws_vpc", Name: "main", Args: Attrs{A("cidr_block", Str("10.0.0.0/16"))}},
		Resource{Type: "aws_subnet", Name: "a", Args: Attrs{A("vpc_id", R(ID{Type: "aws_vpc", Name: "main"}, Field("id")))}},
	)
	if errs := ValidateGraph(g); len(errs) > 0 {
		t.Fatalf("errors = %v", messages(errs))
	}
}

func TestValidateGraphFindsDanglingRefsAnywhereInTheValueTree(t *testing.T) {
	gone := ID{Type: "aws_db_instance", Name: "gone"}
	g := testGraph(Resource{
		Type: "aws_iam_role_policy",
		Name: "p",
		Args: Attrs{A("policy", J(M(A("Statement", L(M(A("Resource", L(R(gone, Field("arn"))))))))))},
	})
	want := Errors{{
		Path:    "aws_iam_role_policy.p.policy.Statement[0].Resource[0]",
		Message: "reference to unknown resource 'aws_db_instance.gone'",
	}}
	if d := cmp.Diff(want, ValidateGraph(g)); d != "" {
		t.Fatal(d)
	}
}

func TestValidateGraphFindsDuplicateIDsAndDanglingDependsOn(t *testing.T) {
	g := testGraph(
		Resource{Type: "aws_vpc", Name: "main"},
		Resource{Type: "aws_vpc", Name: "main"},
		Resource{Type: "aws_subnet", Name: "a", DependsOn: []ID{{Type: "aws_nat_gateway", Name: "x"}}},
	)
	want := []string{
		"duplicate resource 'aws_vpc.main'",
		"depends_on refers to unknown resource 'aws_nat_gateway.x'",
	}
	if d := cmp.Diff(want, messages(ValidateGraph(g))); d != "" {
		t.Fatal(d)
	}
}

func TestValidateGraphChecksDataSources(t *testing.T) {
	g := testGraph(Resource{
		Type: "aws_subnet",
		Name: "a",
		Args: Attrs{A("availability_zone", D(ID{Type: "aws_availability_zones", Name: "gone"}, Field("names"), Index(0)))},
	})
	g.Data = []DataSource{
		{Type: "aws_availability_zones", Name: "available"},
		{Type: "aws_availability_zones", Name: "available"},
	}
	want := []string{
		"duplicate data source 'aws_availability_zones.available'",
		"reference to unknown data source 'aws_availability_zones.gone'",
	}
	if d := cmp.Diff(want, messages(ValidateGraph(g))); d != "" {
		t.Fatal(d)
	}
}

func TestValidateGraphChecksVariablesAndOutputs(t *testing.T) {
	gone := ID{Type: "aws_vpc", Name: "gone"}
	g := testGraph()
	g.Variables = []Variable{{Name: "cidr", Type: "string", Default: R(gone, Field("cidr_block"))}}
	g.Outputs = []Output{{Name: "vpc_id", Value: R(gone, Field("id"))}}
	want := Errors{
		{Path: "variable.cidr", Message: "reference to unknown resource 'aws_vpc.gone'"},
		{Path: "output.vpc_id", Message: "reference to unknown resource 'aws_vpc.gone'"},
	}
	if d := cmp.Diff(want, ValidateGraph(g)); d != "" {
		t.Fatal(d)
	}
}

func TestValidateGraphChecksTheProviderConfig(t *testing.T) {
	g := testGraph()
	g.Provider.Config = Attrs{A("region", R(ID{Type: "aws_vpc", Name: "gone"}, Field("id")))}
	want := Errors{{
		Path:    "provider.aws.region",
		Message: "reference to unknown resource 'aws_vpc.gone'",
	}}
	if d := cmp.Diff(want, ValidateGraph(g)); d != "" {
		t.Fatal(d)
	}
}

func TestValidateGraphRejectsUnknownCalls(t *testing.T) {
	g := testGraph(Resource{
		Type: "aws_lambda_function",
		Name: "f",
		Args: Attrs{
			A("source_code_hash", Fn("filebase64sha256", Str("f.zip"))),
			A("role", Fn("nope")),
		},
	})
	want := Errors{{
		Path:    "aws_lambda_function.f.role",
		Message: "unknown function 'nope'",
	}}
	if d := cmp.Diff(want, ValidateGraph(g)); d != "" {
		t.Fatal(d)
	}
}
