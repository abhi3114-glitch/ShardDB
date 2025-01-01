package sql

import "fmt"

type NodeType int

const (
	NodeScan NodeType = iota
	NodeFilter
	NodeProject
	NodeInsert
)

type PlanNode interface {
	Type() NodeType
	String() string
	Children() []PlanNode
}

type ScanNode struct {
	Table string
}

func (n *ScanNode) Type() NodeType       { return NodeScan }
func (n *ScanNode) String() string       { return fmt.Sprintf("Scan(%s)", n.Table) }
func (n *ScanNode) Children() []PlanNode { return nil }

type PointGetNode struct {
	Table string
	Key   []byte
}

func (n *PointGetNode) Type() NodeType { return NodeScan } // Close enough to Scan for enum? Or Add Value? Reuse Scan for now or New Enum?
// Let's reuse Scan type enum but different Struct.
func (n *PointGetNode) String() string       { return fmt.Sprintf("PointGet(%s, %s)", n.Table, n.Key) }
func (n *PointGetNode) Children() []PlanNode { return nil }

type InsertNode struct {
	Table   string
	Columns []string
	Values  [][]byte // Literal values for now (assumes raw bytes or strings)
}

func (n *InsertNode) Type() NodeType       { return NodeInsert }
func (n *InsertNode) String() string       { return fmt.Sprintf("Insert(%s)", n.Table) }
func (n *InsertNode) Children() []PlanNode { return nil }

type ProjectNode struct {
	Input   PlanNode
	Columns []string
}

func (n *ProjectNode) Type() NodeType       { return NodeProject }
func (n *ProjectNode) String() string       { return fmt.Sprintf("Project(%v)", n.Columns) }
func (n *ProjectNode) Children() []PlanNode { return []PlanNode{n.Input} }

type FilterNode struct {
	Input PlanNode
	Expr  string // Placeholder for expression
}

func (n *FilterNode) Type() NodeType       { return NodeFilter }
func (n *FilterNode) String() string       { return fmt.Sprintf("Filter(%s)", n.Expr) }
func (n *FilterNode) Children() []PlanNode { return []PlanNode{n.Input} }

