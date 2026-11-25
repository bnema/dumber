package homehtml

import (
	"html/template"
	"text/template/parse"
)

// checkRanges rejects {{range}} over integers that can be known before
// execution: number constants, the len builtin, and variables declared from
// either. JSON data decodes numbers as float64, which range rejects at run
// time, so data paths are safe. Variables are tracked by name across the
// whole template, which is conservative.
//
// Widget templates may not use define, block or template: a template call
// would hide ranges from the depth check, and recursion has no bound.
func checkRanges(t *template.Template) error {
	if len(t.Templates()) > 1 {
		return ErrTemplateSubTemplate
	}
	for _, tt := range t.Templates() {
		if tt.Tree == nil || tt.Tree.Root == nil {
			continue
		}
		c := rangeChecker{intVars: map[string]bool{}}
		c.collect(tt.Tree.Root)
		if callsTemplate(tt.Tree.Root) {
			return ErrTemplateSubTemplate
		}
		if c.bad(tt.Tree.Root) {
			return ErrTemplateIntRange
		}
		if rangeDepth(tt.Tree.Root) > MaxTemplateRangeDepth {
			return ErrTemplateRangeDepth
		}
	}
	return nil
}

type rangeChecker struct {
	intVars map[string]bool
}

// collect records variables declared from integer pipelines.
func (c *rangeChecker) collect(n parse.Node) {
	walkTemplate(n, func(p *parse.PipeNode) {
		if p == nil || len(p.Decl) == 0 {
			return
		}
		if c.intPipe(p) {
			for _, v := range p.Decl {
				c.intVars[v.Ident[0]] = true
			}
		}
	}, nil)
}

func (c *rangeChecker) bad(n parse.Node) bool {
	found := false
	walkTemplate(n, nil, func(r *parse.RangeNode) {
		if c.intPipe(r.Pipe) {
			found = true
		}
	})
	return found
}

// intPipe reports whether the pipeline's result is statically an integer.
func (c *rangeChecker) intPipe(p *parse.PipeNode) bool {
	if p == nil || len(p.Cmds) == 0 {
		return false
	}
	last := p.Cmds[len(p.Cmds)-1]
	if len(last.Args) == 0 {
		return false
	}
	switch a := last.Args[0].(type) {
	case *parse.NumberNode:
		return true
	case *parse.IdentifierNode:
		return a.Ident == "len"
	case *parse.VariableNode:
		return len(a.Ident) == 1 && c.intVars[a.Ident[0]]
	case *parse.PipeNode:
		return c.intPipe(a)
	}
	return false
}

// walkTemplate visits every pipeline and range node under n.
func walkTemplate(n parse.Node, onPipe func(*parse.PipeNode), onRange func(*parse.RangeNode)) {
	switch x := n.(type) {
	case *parse.ListNode:
		if x == nil {
			return
		}
		for _, child := range x.Nodes {
			walkTemplate(child, onPipe, onRange)
		}
	case *parse.ActionNode:
		walkPipe(x.Pipe, onPipe)
	case *parse.IfNode:
		walkBranch(&x.BranchNode, onPipe, onRange)
	case *parse.WithNode:
		walkBranch(&x.BranchNode, onPipe, onRange)
	case *parse.RangeNode:
		if onRange != nil {
			onRange(x)
		}
		walkBranch(&x.BranchNode, onPipe, onRange)
	case *parse.TemplateNode:
		walkPipe(x.Pipe, onPipe)
	}
}

func walkBranch(b *parse.BranchNode, onPipe func(*parse.PipeNode), onRange func(*parse.RangeNode)) {
	walkPipe(b.Pipe, onPipe)
	walkTemplate(b.List, onPipe, onRange)
	if b.ElseList != nil {
		walkTemplate(b.ElseList, onPipe, onRange)
	}
}

func walkPipe(p *parse.PipeNode, onPipe func(*parse.PipeNode)) {
	if p == nil {
		return
	}
	if onPipe != nil {
		onPipe(p)
	}
	for _, cmd := range p.Cmds {
		for _, arg := range cmd.Args {
			if sub, ok := arg.(*parse.PipeNode); ok {
				walkPipe(sub, onPipe)
			}
		}
	}
}

// callsTemplate reports whether n contains a {{template}} call.
func callsTemplate(n parse.Node) bool {
	switch x := n.(type) {
	case *parse.ListNode:
		if x == nil {
			return false
		}
		for _, child := range x.Nodes {
			if callsTemplate(child) {
				return true
			}
		}
	case *parse.IfNode:
		return branchCallsTemplate(&x.BranchNode)
	case *parse.WithNode:
		return branchCallsTemplate(&x.BranchNode)
	case *parse.RangeNode:
		return branchCallsTemplate(&x.BranchNode)
	case *parse.TemplateNode:
		return true
	}
	return false
}

func branchCallsTemplate(b *parse.BranchNode) bool {
	return callsTemplate(b.List) || (b.ElseList != nil && callsTemplate(b.ElseList))
}

// rangeDepth returns the deepest nesting of range nodes under n.
func rangeDepth(n parse.Node) int {
	switch x := n.(type) {
	case *parse.ListNode:
		if x == nil {
			return 0
		}
		deepest := 0
		for _, child := range x.Nodes {
			deepest = max(deepest, rangeDepth(child))
		}
		return deepest
	case *parse.IfNode:
		return branchDepth(&x.BranchNode)
	case *parse.WithNode:
		return branchDepth(&x.BranchNode)
	case *parse.RangeNode:
		return 1 + branchDepth(&x.BranchNode)
	}
	return 0
}

func branchDepth(b *parse.BranchNode) int {
	d := rangeDepth(b.List)
	if b.ElseList != nil {
		d = max(d, rangeDepth(b.ElseList))
	}
	return d
}
