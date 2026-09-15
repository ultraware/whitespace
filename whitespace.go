package whitespace

import (
	"flag"
	"go/ast"
	"go/token"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Settings contains settings for edge-cases.
type Settings struct {
	MultiIf   bool
	MultiFunc bool

	IgnoreLeading  bool
	IgnoreTrailing bool
}

// NewAnalyzer creates a new whitespace analyzer.
func NewAnalyzer(settings *Settings) *analysis.Analyzer {
	if settings == nil {
		settings = &Settings{}
	}

	return &analysis.Analyzer{
		Name:  "whitespace",
		Doc:   "Whitespace is a linter that checks for unnecessary newlines at the start and end of functions, if, for, etc.",
		Flags: flags(settings),
		Run: func(p *analysis.Pass) (any, error) {
			Run(p, settings)
			return nil, nil
		},
		RunDespiteErrors: true,
	}
}

func flags(settings *Settings) flag.FlagSet {
	flags := flag.NewFlagSet("", flag.ExitOnError)
	flags.BoolVar(&settings.MultiIf, "multi-if", settings.MultiIf, "Check that multi line if-statements have a leading newline")
	flags.BoolVar(&settings.MultiFunc, "multi-func", settings.MultiFunc, "Check that multi line functions have a leading newline")
	flags.BoolVar(&settings.IgnoreLeading, "ignore-leading", settings.IgnoreLeading, "Do not check leading newlines")
	flags.BoolVar(&settings.IgnoreTrailing, "ignore-trailing", settings.IgnoreTrailing, "Do not check trailing newlines")

	return *flags
}

func Run(pass *analysis.Pass, settings *Settings) {
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if !strings.HasSuffix(filename, ".go") {
			continue
		}

		fileMessages := runFile(file, pass.Fset, *settings)

		for _, message := range fileMessages {
			pass.Report(message)
		}
	}
}

func runFile(file *ast.File, fset *token.FileSet, settings Settings) []analysis.Diagnostic {
	var messages []analysis.Diagnostic

	for _, f := range file.Decls {
		decl, ok := f.(*ast.FuncDecl)
		if !ok || decl.Body == nil { // decl.Body can be nil for e.g. cgo
			continue
		}

		vis := visitor{file.Comments, fset, nil, make(map[*ast.BlockStmt]bool), settings}
		ast.Walk(&vis, decl)

		messages = append(messages, vis.messages...)
	}

	return messages
}

type visitor struct {
	comments    []*ast.CommentGroup
	fset        *token.FileSet
	messages    []analysis.Diagnostic
	wantNewline map[*ast.BlockStmt]bool
	settings    Settings
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return v
	}

	if stmt, ok := node.(*ast.IfStmt); ok && v.settings.MultiIf {
		checkMultiLine(v, stmt.Body, stmt.Cond)
	}

	if stmt, ok := node.(*ast.FuncLit); ok && v.settings.MultiFunc {
		checkMultiLine(v, stmt.Body, stmt.Type)
	}

	if stmt, ok := node.(*ast.FuncDecl); ok && v.settings.MultiFunc {
		checkMultiLine(v, stmt.Body, stmt.Type)
	}

	if stmt, ok := node.(*ast.BlockStmt); ok {
		wantNewline := v.wantNewline[stmt]

		comments := v.comments
		if wantNewline {
			comments = nil // Comments also count as a newline if we want a newline
		}

		opening, first, last := firstAndLast(comments, v.fset, stmt)

		if !v.settings.IgnoreLeading {
			startMsg := checkStart(v.fset, opening, first)
			if wantNewline && startMsg == nil && len(stmt.List) >= 1 {
				v.messages = append(v.messages, analysis.Diagnostic{
					Pos:     opening,
					Message: "multi-line statement should be followed by a newline",
					SuggestedFixes: []analysis.SuggestedFix{{
						TextEdits: []analysis.TextEdit{{
							Pos:     stmt.List[0].Pos(),
							End:     stmt.List[0].Pos(),
							NewText: []byte("\n"),
						}},
					}},
				})
			} else if !wantNewline && startMsg != nil {
				v.messages = append(v.messages, *startMsg)
			}
		}

		if !v.settings.IgnoreTrailing {
			if msg := checkEnd(v.fset, stmt.Rbrace, last); msg != nil {
				v.messages = append(v.messages, *msg)
			}
		}
	}

	return v
}

func checkMultiLine(v *visitor, body *ast.BlockStmt, stmtStart ast.Node) {
	start, end := posLine(v.fset, stmtStart.Pos()), posLine(v.fset, stmtStart.End())

	if end > start { // Check only multi line conditions
		v.wantNewline[body] = true
	}
}

func posLine(fset *token.FileSet, pos token.Pos) int {
	return fset.Position(pos).Line
}

func firstAndLast(comments []*ast.CommentGroup, fset *token.FileSet, stmt *ast.BlockStmt) (token.Pos, ast.Node, ast.Node) {
	openingPos := stmt.Lbrace + 1

	if len(stmt.List) == 0 {
		return openingPos, nil, nil
	}

	first, last := ast.Node(stmt.List[0]), ast.Node(stmt.List[len(stmt.List)-1])

	// comments is sorted by position (guaranteed by go/ast), so a comment
	// that can possibly matter for this block is bounded to a small,
	// contiguous slice around it. Scanning every comment in the file for
	// every block made this quadratic in large files with many comments.
	lo := sort.Search(len(comments), func(i int) bool { return comments[i].End() >= stmt.Pos() })

	openingLine := posLine(fset, openingPos)
	stmtPosLine := posLine(fset, stmt.Pos())
	stmtEndLine := posLine(fset, stmt.End())

	for _, c := range comments[lo:] {
		if c.Pos() > stmt.End() {
			break
		}

		// If the comment is on the same line as the opening pos (initially the
		// left bracket) but it starts after the pos the comment must be after
		// the bracket and where that comment ends should be considered where
		// the fix should start.
		if posLine(fset, c.Pos()) == openingLine && c.Pos() > openingPos {
			if posLine(fset, c.End()) != openingLine {
				// This is a multiline comment that spans from the `LBrace` line
				// to a line further down. This should always be seen as ok!
				first = c
			} else {
				openingPos = c.End()
				openingLine = posLine(fset, openingPos)
			}
		}

		if posLine(fset, c.Pos()) == stmtPosLine || posLine(fset, c.End()) == stmtEndLine {
			continue
		}

		if c.Pos() < stmt.Pos() || c.End() > stmt.End() {
			continue
		}

		if c.Pos() < first.Pos() {
			first = c
		}

		if c.End() > last.End() {
			last = c
		}
	}

	return openingPos, first, last
}

func checkStart(fset *token.FileSet, start token.Pos, first ast.Node) *analysis.Diagnostic {
	if first == nil {
		return nil
	}

	if posLine(fset, start)+1 < posLine(fset, first.Pos()) {
		return &analysis.Diagnostic{
			Pos:     start,
			Message: "unnecessary leading newline",
			SuggestedFixes: []analysis.SuggestedFix{{
				TextEdits: []analysis.TextEdit{{
					Pos:     start,
					End:     first.Pos(),
					NewText: []byte("\n"),
				}},
			}},
		}
	}

	return nil
}

func checkEnd(fset *token.FileSet, end token.Pos, last ast.Node) *analysis.Diagnostic {
	if last == nil {
		return nil
	}

	if posLine(fset, end)-1 > posLine(fset, last.End()) {
		return &analysis.Diagnostic{
			Pos:     end,
			Message: "unnecessary trailing newline",
			SuggestedFixes: []analysis.SuggestedFix{{
				TextEdits: []analysis.TextEdit{{
					Pos:     last.End(),
					End:     end,
					NewText: []byte("\n"),
				}},
			}},
		}
	}

	return nil
}
