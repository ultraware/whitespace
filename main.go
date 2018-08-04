package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

type codeError struct {
	File    string
	Line    int
	Message string
}

func newCodeError(file string, line int, msg string) error {
	return codeError{file, line, msg}
}

func (e codeError) Error() string {
	return fmt.Sprint(e.File, `:`, e.Line, `:`, e.Message)
}

var fset = token.NewFileSet()
var msgs []error

func init() {
	flag.Parse()
}

func main() {
	pkgs, err := parser.ParseDir(fset, flag.Arg(0), nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, f := range file.Decls {
				decl, ok := f.(*ast.FuncDecl)
				if !ok {
					continue
				}
				ast.Walk(&visitor{}, decl)
			}
		}
	}

	for _, v := range msgs {
		fmt.Println(v)
	}
}

type visitor struct{}

func (v visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return v
	}

	if stmt, ok := node.(*ast.BlockStmt); ok {
		first, last := firstAndLast(stmt.List)

		checkStart(stmt.Lbrace, first)
		checkEnd(stmt.Rbrace, last)
	}

	return v
}

func posLine(pos token.Pos) int {
	return fset.Position(pos).Line
}

func posFile(pos token.Pos) string {
	return fset.Position(pos).Filename
}

func firstAndLast(stmts []ast.Stmt) (ast.Node, ast.Node) {
	if len(stmts) == 0 {
		return nil, nil
	}

	return stmts[0], stmts[len(stmts)-1]
}

func checkStart(start token.Pos, first ast.Node) {
	if first == nil {
		return
	}

	if posLine(start)+1 < posLine(first.Pos()) {
		msgs = append(msgs, newCodeError(posFile(start), posLine(start)+1, `unnecessary newline`))
	}
}

func checkEnd(end token.Pos, last ast.Node) {
	if last == nil {
		return
	}

	if posLine(end)-1 > posLine(last.End()) {
		msgs = append(msgs, newCodeError(posFile(end), posLine(end)-1, `unnecessary newline`))
	}
}
