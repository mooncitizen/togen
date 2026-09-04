package hcl

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/mooncitizen/togen/internal/ir"
)

func render(t *testing.T, v ir.Value) string {
	t.Helper()
	toks, err := tokens(v)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	f := hclwrite.NewEmptyFile()
	f.Body().SetAttributeRaw("x", toks)
	return strings.TrimSuffix(string(hclwrite.Format(f.Bytes())), "\n")
}

func TestTokens(t *testing.T) {
	cases := []struct {
		name  string
		value ir.Value
		want  string
	}{
		{"string", ir.Str("hello"), `x = "hello"`},
		{"string with quotes", ir.Str(`say "hi"`), `x = "say \"hi\""`},
		{"string with interpolation", ir.Str("${nope}"), `x = "$${nope}"`},
		{"lone dollar", ir.Str("$default"), `x = "$default"`},
		{"whole number", ir.Num(5432), `x = 5432`},
		{"fractional number", ir.Num(1.5), `x = 1.5`},
		{"bool", ir.Bool(true), `x = true`},
		{"empty list", ir.L(), `x = []`},
		{"list", ir.L(ir.Str("a"), ir.Num(2)), `x = ["a", 2]`},
		{
			"ref",
			ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id")),
			`x = aws_vpc.main.id`,
		},
		{
			"ref with index",
			ir.R(ir.ID{Type: "aws_db_instance", Name: "main_db"},
				ir.Field("master_user_secret"), ir.Index(0), ir.Field("secret_arn")),
			`x = aws_db_instance.main_db.master_user_secret[0].secret_arn`,
		},
		{
			"data ref",
			ir.D(ir.ID{Type: "aws_availability_zones", Name: "available"},
				ir.Field("names"), ir.Index(0)),
			`x = data.aws_availability_zones.available.names[0]`,
		},
		{"var", ir.V("handler_package"), `x = var.handler_package`},
		{
			"map",
			ir.M(ir.A("Name", ir.Str("x")), ir.A("a.b", ir.Str("y"))),
			"x = {\n  Name  = \"x\"\n  \"a.b\" = \"y\"\n}",
		},
		{
			"json",
			ir.J(ir.M(
				ir.A("Version", ir.Str("2012-10-17")),
				ir.A("Resource", ir.R(ir.ID{Type: "aws_s3_bucket", Name: "b"}, ir.Field("arn"))),
			)),
			"x = jsonencode({\n  Version  = \"2012-10-17\"\n  Resource = aws_s3_bucket.b.arn\n})",
		},
		{
			"concat with trailing ref",
			ir.C(ir.Str("integrations/"),
				ir.R(ir.ID{Type: "aws_apigatewayv2_integration", Name: "x"}, ir.Field("id"))),
			`x = "integrations/${aws_apigatewayv2_integration.x.id}"`,
		},
		{
			"concat with leading ref",
			ir.C(ir.R(ir.ID{Type: "a", Name: "b"}, ir.Field("arn")), ir.Str("/*/*")),
			`x = "${a.b.arn}/*/*"`,
		},
		{
			"concat escapes literal parts",
			ir.C(ir.Str(`a"${b`), ir.V("x")),
			`x = "a\"$${b${var.x}"`,
		},
		{"call", ir.Fn("filebase64sha256", ir.V("x")), `x = filebase64sha256(var.x)`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := render(t, c.value); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func TestTokensRejects(t *testing.T) {
	for _, c := range []struct {
		name  string
		value ir.Value
	}{
		{"block", ir.B(ir.Attrs{ir.A("a", ir.Str("b"))})},
		{"unknown call", ir.Fn("nope", ir.Str("x"))},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := tokens(c.value); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
}

func TestTraversal(t *testing.T) {
	tr := traversal(ir.D(ir.ID{Type: "aws_caller_identity", Name: "current"}, ir.Field("account_id")))
	got := string(hclwrite.TokensForTraversal(tr).Bytes())
	if want := "data.aws_caller_identity.current.account_id"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestQuoted(t *testing.T) {
	got := string(quoted("a\nb").Bytes())
	if want := `"a\nb"`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
