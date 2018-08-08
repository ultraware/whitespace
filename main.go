package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func warn(file string, line int, msg string) {
	fmt.Print(file, `:`, line, `:`, msg, "\n")
}

var fset *token.FileSet

func main() {
	if len(os.Args) < 2 {
		fmt.Println(`Usage:`, os.Args[0], ` target1 [target2]...`)
		os.Exit(2)
	}

	for _, arg := range os.Args[1:] {
		fset = token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, arg, nil, parser.ParseComments)
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
		warn(posFile(start), posLine(start)+1, `unnecessary newline`)
	}
}

func checkEnd(end token.Pos, last ast.Node) {
	if last == nil {
		return
	}

	if posLine(end)-1 > posLine(last.End()) {
		warn(posFile(end), posLine(end)-1, `unnecessary newline`)
	}
}
