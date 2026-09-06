package ir

type ID struct {
	Type string
	Name string
}

func (id ID) String() string { return id.Type + "." + id.Name }

type Attr struct {
	Key   string
	Value Value
}

type Attrs []Attr

func (a Attrs) Get(key string) (Value, bool) {
	for _, attr := range a {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return nil, false
}

func (a *Attrs) Set(key string, v Value) {
	for i := range *a {
		if (*a)[i].Key == key {
			(*a)[i].Value = v
			return
		}
	}
	*a = append(*a, Attr{Key: key, Value: v})
}

type Value interface{ isValue() }

type (
	String string
	Number float64
	Bool   bool
	List   []Value
	Map    Attrs
	Block  []Attrs
)

type Step struct {
	Field   string
	Index   int
	IsIndex bool
}

func Field(name string) Step { return Step{Field: name} }
func Index(i int) Step       { return Step{Index: i, IsIndex: true} }

type RefKind int

const (
	RefResource RefKind = iota
	RefData
)

type Ref struct {
	Kind   RefKind
	Target ID
	Path   []Step
}

type Var struct{ Name string }

type JSON struct{ Value Value }

type Concat struct{ Parts []Value }

type Call struct {
	Fn   string
	Args []Value
}

func (String) isValue() {}
func (Number) isValue() {}
func (Bool) isValue()   {}
func (List) isValue()   {}
func (Map) isValue()    {}
func (Block) isValue()  {}
func (Ref) isValue()    {}
func (Var) isValue()    {}
func (JSON) isValue()   {}
func (Concat) isValue() {}
func (Call) isValue()   {}

func Str(s string) String                { return String(s) }
func Num(n float64) Number               { return Number(n) }
func A(key string, v Value) Attr         { return Attr{Key: key, Value: v} }
func L(values ...Value) List             { return List(values) }
func M(attrs ...Attr) Map                { return Map(attrs) }
func B(entries ...Attrs) Block           { return Block(entries) }
func R(id ID, path ...Step) Ref          { return Ref{Kind: RefResource, Target: id, Path: path} }
func D(id ID, path ...Step) Ref          { return Ref{Kind: RefData, Target: id, Path: path} }
func V(name string) Var                  { return Var{Name: name} }
func J(v Value) JSON                     { return JSON{Value: v} }
func C(parts ...Value) Concat            { return Concat{Parts: parts} }
func Fn(name string, args ...Value) Call { return Call{Fn: name, Args: args} }

// jsonencode is expressed as JSON rather than a Call, and is listed so an emitter can name it.
var Calls = []string{"filebase64sha256", "jsonencode"}

type Provider struct {
	Name    string
	Source  string
	Version string
	Config  Attrs
}

type DataSource struct {
	Type        string
	Name        string
	Args        Attrs
	SourceLabel string
}

type Resource struct {
	Type        string
	Name        string
	Args        Attrs
	DependsOn   []ID
	SourceNode  string
	SourceLabel string
}

type Variable struct {
	Name        string
	Description string
	Type        string
	Default     Value
	Sensitive   bool
}

type Output struct {
	Name        string
	Description string
	Value       Value
	Sensitive   bool
}

type Graph struct {
	TerraformVersion string
	Providers        []Provider
	Data             []DataSource
	Resources        []Resource
	Variables        []Variable
	Outputs          []Output
}
