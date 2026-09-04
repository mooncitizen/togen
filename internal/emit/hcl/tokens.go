package hcl

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"togen/internal/ir"
)

var ident = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func tokens(v ir.Value) (hclwrite.Tokens, error) {
	switch v := v.(type) {
	case ir.String:
		return hclwrite.TokensForValue(cty.StringVal(string(v))), nil
	case ir.Number:
		return hclwrite.TokensForValue(number(float64(v))), nil
	case ir.Bool:
		return hclwrite.TokensForValue(cty.BoolVal(bool(v))), nil
	case ir.List:
		elems := make([]hclwrite.Tokens, 0, len(v))
		for _, item := range v {
			t, err := tokens(item)
			if err != nil {
				return nil, err
			}
			elems = append(elems, t)
		}
		return hclwrite.TokensForTuple(elems), nil
	case ir.Map:
		attrs := make([]hclwrite.ObjectAttrTokens, 0, len(v))
		for _, a := range v {
			t, err := tokens(a.Value)
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{Name: key(a.Key), Value: t})
		}
		return hclwrite.TokensForObject(attrs), nil
	case ir.Ref:
		return hclwrite.TokensForTraversal(traversal(v)), nil
	case ir.Var:
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "var"},
			hcl.TraverseAttr{Name: v.Name},
		}), nil
	case ir.JSON:
		inner, err := tokens(v.Value)
		if err != nil {
			return nil, err
		}
		return hclwrite.TokensForFunctionCall("jsonencode", inner), nil
	case ir.Concat:
		return concat(v)
	case ir.Call:
		if !slices.Contains(ir.Calls, v.Fn) {
			return nil, fmt.Errorf("unknown function %q", v.Fn)
		}
		args := make([]hclwrite.Tokens, 0, len(v.Args))
		for _, a := range v.Args {
			t, err := tokens(a)
			if err != nil {
				return nil, err
			}
			args = append(args, t)
		}
		return hclwrite.TokensForFunctionCall(v.Fn, args...), nil
	case ir.Block:
		return nil, errors.New("a block cannot be used as a value")
	}
	return nil, fmt.Errorf("unsupported value %T", v)
}

func number(n float64) cty.Value {
	if n == float64(int64(n)) {
		return cty.NumberIntVal(int64(n))
	}
	return cty.NumberFloatVal(n)
}

func traversal(r ir.Ref) hcl.Traversal {
	var tr hcl.Traversal
	if r.Kind == ir.RefData {
		tr = append(tr, hcl.TraverseRoot{Name: "data"}, hcl.TraverseAttr{Name: r.Target.Type})
	} else {
		tr = append(tr, hcl.TraverseRoot{Name: r.Target.Type})
	}
	tr = append(tr, hcl.TraverseAttr{Name: r.Target.Name})
	for _, s := range r.Path {
		if s.IsIndex {
			tr = append(tr, hcl.TraverseIndex{Key: cty.NumberIntVal(int64(s.Index))})
		} else {
			tr = append(tr, hcl.TraverseAttr{Name: s.Field})
		}
	}
	return tr
}

func concat(c ir.Concat) (hclwrite.Tokens, error) {
	out := hclwrite.Tokens{{Type: hclsyntax.TokenOQuote, Bytes: []byte(`"`)}}
	for _, part := range c.Parts {
		if lit, ok := literal(part); ok {
			out = append(out, &hclwrite.Token{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(lit)})
			continue
		}
		expr, err := tokens(part)
		if err != nil {
			return nil, err
		}
		out = append(out, &hclwrite.Token{Type: hclsyntax.TokenTemplateInterp, Bytes: []byte("${")})
		out = append(out, expr...)
		out = append(out, &hclwrite.Token{Type: hclsyntax.TokenTemplateSeqEnd, Bytes: []byte("}")})
	}
	return append(out, &hclwrite.Token{Type: hclsyntax.TokenCQuote, Bytes: []byte(`"`)}), nil
}

func literal(v ir.Value) (string, bool) {
	switch v := v.(type) {
	case ir.String:
		return escape(string(v)), true
	case ir.Number:
		return string(hclwrite.TokensForValue(number(float64(v))).Bytes()), true
	case ir.Bool:
		if v {
			return "true", true
		}
		return "false", true
	}
	return "", false
}

func key(k string) hclwrite.Tokens {
	if ident.MatchString(k) {
		return hclwrite.TokensForIdentifier(k)
	}
	return quoted(k)
}

func quoted(s string) hclwrite.Tokens {
	out := hclwrite.Tokens{{Type: hclsyntax.TokenOQuote, Bytes: []byte(`"`)}}
	if s != "" {
		out = append(out, &hclwrite.Token{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(escape(s))})
	}
	return append(out, &hclwrite.Token{Type: hclsyntax.TokenCQuote, Bytes: []byte(`"`)})
}

var escaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
	`${`, `$${`,
	`%{`, `%%{`,
)

func escape(s string) string { return escaper.Replace(s) }
