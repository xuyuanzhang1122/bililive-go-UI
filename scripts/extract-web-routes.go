//go:build ignore

// 从 Go 语法树读取旧管理路由，避免源码格式变化影响生成结果。
package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
)

type route struct {
	Path    string   `json:"path"`
	Handler string   `json:"handler"`
	Methods []string `json:"methods"`
}

func literal(e ast.Expr) string {
	if s, ok := e.(*ast.BasicLit); ok && s.Kind == token.STRING {
		v, _ := strconv.Unquote(s.Value)
		return v
	}
	return ""
}
func main() {
	f, err := parser.ParseFile(token.NewFileSet(), "src/servers/server.go", nil, 0)
	if err != nil {
		panic(err)
	}
	routes := []route{}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		m, ok := c.Fun.(*ast.SelectorExpr)
		if !ok || m.Sel.Name != "Methods" {
			return true
		}
		h, ok := m.X.(*ast.CallExpr)
		if !ok || len(h.Args) != 2 {
			return true
		}
		s, ok := h.Fun.(*ast.SelectorExpr)
		if !ok || s.Sel.Name != "HandleFunc" {
			return true
		}
		receiver, ok := s.X.(*ast.Ident)
		if !ok || receiver.Name != "apiRoute" {
			return true
		}
		handler, ok := h.Args[1].(*ast.Ident)
		if !ok {
			return true
		}
		r := route{Path: literal(h.Args[0]), Handler: handler.Name}
		for _, a := range c.Args {
			if v := literal(a); v != "" {
				r.Methods = append(r.Methods, v)
			}
		}
		if r.Path != "" && len(r.Methods) > 0 {
			routes = append(routes, r)
		}
		return true
	})
	if err := json.NewEncoder(os.Stdout).Encode(routes); err != nil {
		panic(err)
	}
}
