package d2ir

import (
	"errors"
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
)

type Node interface {
	node()
	ast() d2ast.Node
	Copy(newp Parent) Node
	fmt.Stringer
}

var _ Node = &Scalar{}
var _ Node = &Field{}
var _ Node = &Edge{}
var _ Node = &Array{}
var _ Node = &Map{}

type Parent interface {
	Node
	Parent() Parent
}

var _ Parent = &Field{}
var _ Parent = &Edge{}
var _ Parent = &Array{}
var _ Parent = &Map{}

type Value interface {
	Node
	value()
}

var _ Value = &Scalar{}
var _ Value = &Array{}
var _ Value = &Map{}

type Composite interface {
	Node
	Value
	composite()
}

var _ Composite = &Array{}
var _ Composite = &Map{}

func (n *Scalar) node() {}
func (n *Field) node()  {}
func (n *Edge) node()   {}
func (n *Array) node()  {}
func (n *Map) node()    {}

func (n *Scalar) Parent() Parent { return n.parent }
func (n *Field) Parent() Parent  { return n.parent }
func (n *Edge) Parent() Parent   { return n.parent }
func (n *Array) Parent() Parent  { return n.parent }
func (n *Map) Parent() Parent    { return n.parent }

func (n *Scalar) value() {}
func (n *Array) value()  {}
func (n *Map) value()    {}

func (n *Array) composite() {}
func (n *Map) composite()   {}

func (n *Scalar) String() string { return d2format.Format(n.ast()) }
func (n *Field) String() string  { return d2format.Format(n.ast()) }
func (n *Edge) String() string   { return d2format.Format(n.ast()) }
func (n *Array) String() string  { return d2format.Format(n.ast()) }
func (n *Map) String() string    { return d2format.Format(n.ast()) }

type Scalar struct {
	parent Parent
	Value  d2ast.Scalar `json:"value"`
}

func (s *Scalar) Copy(newp Parent) Node {
	tmp := *s
	s = &tmp

	s.parent = newp
	return s
}

func (s *Scalar) Equal(s2 *Scalar) bool {
	if _, ok := s.Value.(d2ast.String); ok {
		if _, ok = s2.Value.(d2ast.String); ok {
			return s.Value.ScalarString() == s2.Value.ScalarString()
		}
	}
	return s.Value.Type() == s2.Value.Type() && s.Value.ScalarString() == s2.Value.ScalarString()

}

type Map struct {
	parent Parent
	Fields []*Field `json:"fields"`
	Edges  []*Edge  `json:"edges"`
}

func (m *Map) Copy(newp Parent) Node {
	tmp := *m
	m = &tmp

	m.parent = newp
	m.Fields = append([]*Field(nil), m.Fields...)
	for i := range m.Fields {
		m.Fields[i] = m.Fields[i].Copy(m).(*Field)
	}
	m.Edges = append([]*Edge(nil), m.Edges...)
	for i := range m.Edges {
		m.Edges[i] = m.Edges[i].Copy(m).(*Edge)
	}
	return m
}

// Root reports whether the Map is the root of the D2 tree.
// The root map has no parent.
func (m *Map) Root() bool {
	return m.parent == nil
}

type Field struct {
	parent *Map

	Name string `json:"name"`

	Primary   *Scalar   `json:"primary,omitempty"`
	Composite Composite `json:"composite,omitempty"`

	References []KeyReference `json:"references,omitempty"`
}

func (f *Field) Copy(newp Parent) Node {
	tmp := *f
	f = &tmp

	f.parent = newp.(*Map)
	f.References = append([]KeyReference(nil), f.References...)
	if f.Primary != nil {
		f.Primary = f.Primary.Copy(f).(*Scalar)
	}
	if f.Composite != nil {
		f.Composite = f.Composite.Copy(f).(Composite)
	}
	return f
}

type EdgeID struct {
	SrcPath  []string `json:"src_path"`
	SrcArrow bool     `json:"src_arrow"`

	DstPath  []string `json:"dst_path"`
	DstArrow bool     `json:"dst_arrow"`

	// If nil, then any EdgeID with equal src/dst/arrows matches.
	Index *int `json:"index"`
}

func NewEdgeIDs(k *d2ast.Key) (eida []*EdgeID) {
	for _, ke := range k.Edges {
		eida = append(eida, &EdgeID{
			SrcPath:  d2format.KeyPath(ke.Src),
			SrcArrow: ke.SrcArrow == "<",
			DstPath:  d2format.KeyPath(ke.Dst),
			DstArrow: ke.DstArrow == ">",
		})
	}
	if k.EdgeIndex != nil && k.EdgeIndex.Int != nil {
		eida[0].Index = k.EdgeIndex.Int
	}
	return eida
}

func (eid *EdgeID) Copy() *EdgeID {
	tmp := *eid
	eid = &tmp

	eid.SrcPath = append([]string(nil), eid.SrcPath...)
	eid.DstPath = append([]string(nil), eid.DstPath...)
	return eid
}

func (eid *EdgeID) Match(eid2 *EdgeID) bool {
	if eid.Index != nil && eid2.Index != nil {
		if *eid.Index != *eid2.Index {
			return false
		}
	}

	if len(eid.SrcPath) != len(eid2.SrcPath) {
		return false
	}
	if eid.SrcArrow != eid2.SrcArrow {
		return false
	}
	for i, s := range eid.SrcPath {
		if !strings.EqualFold(s, eid2.SrcPath[i]) {
			return false
		}
	}

	if len(eid.DstPath) != len(eid2.DstPath) {
		return false
	}
	if eid.DstArrow != eid2.DstArrow {
		return false
	}
	for i, s := range eid.DstPath {
		if !strings.EqualFold(s, eid2.DstPath[i]) {
			return false
		}
	}

	return true
}

func (eid *EdgeID) trimCommon() (common []string, _ *EdgeID) {
	eid = eid.Copy()
	for len(eid.SrcPath) > 1 && len(eid.DstPath) > 1 {
		if !strings.EqualFold(eid.SrcPath[0], eid.DstPath[0]) {
			return common, eid
		}
		common = append(common, eid.SrcPath[0])
		eid.SrcPath = eid.SrcPath[1:]
		eid.DstPath = eid.DstPath[1:]
	}
	return common, eid
}

type Edge struct {
	parent *Map

	ID *EdgeID `json:"edge_id"`

	Primary *Scalar `json:"primary,omitempty"`
	Map     *Map    `json:"map,omitempty"`

	References []EdgeReference `json:"references,omitempty"`
}

func (e *Edge) Copy(newp Parent) Node {
	tmp := *e
	e = &tmp

	e.parent = newp.(*Map)
	e.References = append([]EdgeReference(nil), e.References...)
	if e.Primary != nil {
		e.Primary = e.Primary.Copy(e).(*Scalar)
	}
	if e.Map != nil {
		e.Map = e.Map.Copy(e).(*Map)
	}
	return e
}

type Array struct {
	parent Parent
	Values []Value `json:"values"`
}

func (a *Array) Copy(newp Parent) Node {
	tmp := *a
	a = &tmp

	a.parent = newp
	a.Values = append([]Value(nil), a.Values...)
	for i := range a.Values {
		a.Values[i] = a.Values[i].Copy(a).(Value)
	}
	return a
}

type KeyReference struct {
	String  *d2ast.StringBox `json:"string"`
	KeyPath *d2ast.KeyPath   `json:"key_path"`

	Context *RefContext `json:"-"`
}

func (kr KeyReference) KeyPathIndex() int {
	for i, sb := range kr.KeyPath.Path {
		if sb == kr.String {
			return i
		}
	}
	panic("d2ir.KeyReference.KeyPathIndex: String not in KeyPath?")
}

func (kr KeyReference) EdgeDest() bool {
	return kr.KeyPath == kr.Context.Edge.Dst
}

func (kr KeyReference) InEdge() bool {
	return kr.KeyPath != kr.Context.Key.Key
}

type EdgeReference struct {
	Context *RefContext `json:"-"`
}

type RefContext struct {
	Key   *d2ast.Key
	Edge  *d2ast.Edge
	Scope *d2ast.Map

	// UnresolvedScopeMap is prior to interpreting _
	ScopeMap *Map
	UnresolvedScopeMap *Map
}

func (rc RefContext) EdgeIndex() int {
	for i, e := range rc.Key.Edges {
		if e == rc.Edge {
			return i
		}
	}
	panic("d2ir.RefContext.EdgeIndex: Edge not in Key.Edges?")
}

func (m *Map) FieldCountRecursive() int {
	if m == nil {
		return 0
	}
	acc := len(m.Fields)
	for _, f := range m.Fields {
		if f_m, ok := f.Composite.(*Map); ok {
			acc += f_m.FieldCountRecursive()
		}
	}
	for _, e := range m.Edges {
		if e.Map != nil {
			acc += e.Map.FieldCountRecursive()
		}
	}
	return acc
}

func (m *Map) EdgeCountRecursive() int {
	if m == nil {
		return 0
	}
	acc := len(m.Edges)
	for _, e := range m.Edges {
		if e.Map != nil {
			acc += e.Map.EdgeCountRecursive()
		}
	}
	return acc
}

func (m *Map) Get(ida []string) *Field {
	if len(ida) == 0 {
		return nil
	}

	s := ida[0]
	rest := ida[1:]

	for _, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			return f
		}
		if f_m, ok := f.Composite.(*Map); ok {
			return f_m.Get(rest)
		}
	}
	return nil
}

func (m *Map) Ensure(ida []string) (*Field, error) {
	if len(ida) == 0 {
		return nil, errors.New("empty ida")
	}

	s := ida[0]
	rest := ida[1:]

	for _, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			return f, nil
		}
		switch fc := f.Composite.(type) {
		case *Map:
			return fc.Ensure(rest)
		case *Array:
			return nil, errors.New("cannot index into array")
		}
		f.Composite = &Map{
			parent: f,
		}
		return f.Composite.(*Map).Ensure(rest)
	}

	f := &Field{
		parent: m,
		Name:   s,
	}
	m.Fields = append(m.Fields, f)
	if len(rest) == 0 {
		return f, nil
	}
	f.Composite = &Map{
		parent: f,
	}
	return f.Composite.(*Map).Ensure(rest)
}

func (m *Map) Delete(ida []string) bool {
	if len(ida) == 0 {
		return false
	}

	s := ida[0]
	rest := ida[1:]

	for i, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			copy(m.Fields[i:], m.Fields[i+1:])
			return true
		}
		if f_m, ok := f.Composite.(*Map); ok {
			return f_m.Delete(rest)
		}
	}
	return false
}

func (m *Map) GetEdges(eid *EdgeID) []*Edge {
	common, eid := eid.trimCommon()
	if len(common) > 0 {
		f := m.Get(common)
		if f == nil {
			return nil
		}
		if f_m, ok := f.Composite.(*Map); ok {
			return f_m.GetEdges(eid)
		}
		return nil
	}

	var ea []*Edge
	for _, e := range m.Edges {
		if e.ID.Match(eid) {
			ea = append(ea, e)
		}
	}
	return ea
}

func (m *Map) EnsureEdge(eid *EdgeID) (*Edge, error) {
	common, eid := eid.trimCommon()
	if len(common) > 0 {
		f, err := m.Ensure(common)
		if err != nil {
			return nil, err
		}
		switch fc := f.Composite.(type) {
		case *Map:
			return fc.EnsureEdge(eid)
		case *Array:
			return nil, errors.New("cannot index into array")
		}
		f.Composite = &Map{
			parent: f,
		}
		return f.Composite.(*Map).EnsureEdge(eid)
	}

	eid.Index = nil
	ea := m.GetEdges(eid)
	index := len(ea)
	eid.Index = &index
	e := &Edge{
		parent: m,
		ID:     eid,
	}
	m.Edges = append(m.Edges, e)

	return e, nil
}

func (s *Scalar) ast() d2ast.Node {
	return s.Value
}

func (f *Field) ast() d2ast.Node {
	k := &d2ast.Key{
		Key: &d2ast.KeyPath{
			Path: []*d2ast.StringBox{
				d2ast.MakeValueBox(d2ast.RawString(f.Name, true)).StringBox(),
			},
		},
	}

	if f.Primary != nil {
		k.Primary = d2ast.MakeValueBox(f.Primary.ast().(d2ast.Value)).ScalarBox()
	}
	if f.Composite != nil {
		k.Value = d2ast.MakeValueBox(f.Composite.ast().(d2ast.Value))
	}

	return k
}

func (e *Edge) ast() d2ast.Node {
	astEdge := &d2ast.Edge{}

	astEdge.Src = d2ast.MakeKeyPath(e.ID.SrcPath)
	if e.ID.SrcArrow {
		astEdge.SrcArrow = "<"
	}
	astEdge.Dst = d2ast.MakeKeyPath(e.ID.DstPath)
	if e.ID.DstArrow {
		astEdge.DstArrow = ">"
	}

	k := &d2ast.Key{
		Edges: []*d2ast.Edge{astEdge},
	}

	if e.Primary != nil {
		k.Primary = d2ast.MakeValueBox(e.Primary.ast().(d2ast.Value)).ScalarBox()
	}
	if e.Map != nil {
		k.Value = d2ast.MakeValueBox(e.Map.ast().(*d2ast.Map))
	}

	return k
}

func (a *Array) ast() d2ast.Node {
	if a == nil {
		return nil
	}
	astArray := &d2ast.Array{}
	for _, av := range a.Values {
		astArray.Nodes = append(astArray.Nodes, d2ast.MakeArrayNodeBox(av.ast().(d2ast.ArrayNode)))
	}
	return astArray
}

func (m *Map) ast() d2ast.Node {
	if m == nil {
		return nil
	}
	astMap := &d2ast.Map{}
	if m.parent != nil {
		astMap.Range = d2ast.MakeRange(",1:0:0-1:0:0")
	}
	for _, f := range m.Fields {
		astMap.Nodes = append(astMap.Nodes, d2ast.MakeMapNodeBox(f.ast().(d2ast.MapNode)))
	}
	for _, e := range m.Edges {
		astMap.Nodes = append(astMap.Nodes, d2ast.MakeMapNodeBox(e.ast().(d2ast.MapNode)))
	}
	return astMap
}

func (m *Map) appendKeyReferences(i int, kp *d2ast.KeyPath, refctx *RefContext) {
	sb := kp.Path[i]
	f := m.Get([]string{sb.Unbox().ScalarString()})
	if f == nil {
		return
	}

	f.References = append(f.References, KeyReference{
		String: sb,
		KeyPath: kp,
		Context: refctx,
	})
	if i+1 == len(kp.Path) {
		return
	}
	if f_m, ok := f.Composite.(*Map); ok {
		f_m.appendKeyReferences(i+1, kp, refctx)
	}
}

func (m *Map) appendEdgeReferences(e *Edge, refctx *RefContext) {
	e.References = append(e.References, EdgeReference{
		Context: refctx,
	})
	m.appendKeyReferences(0, refctx.Edge.Src, refctx)
	m.appendKeyReferences(0, refctx.Edge.Dst, refctx)
}
