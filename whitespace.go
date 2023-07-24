package whitespace

import (
	"flag"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// MessageType describes what should happen to fix the warning.
type MessageType uint8

// List of MessageTypes.
const (
	MessageTypeRemove MessageType = iota + 1
	MessageTypeAdd
)

// RunningMode describes the mode the linter is run in. This can be either
// native or golangci-lint.
type RunningMode uint8

const (
	RunningModeNative RunningMode = iota
	RunningModeGolangCI
)

// Message contains a message and diagnostic information.
type Message struct {
	// Diagnostic is what position the diagnostic should be put at. This isn't
	// always the same as the fix start, f.ex. when we fix trailing newlines we
	// put the diagnostic at the right bracket but we fix between the end of the
	// last statement and the bracket.
	Diagnostic token.Pos

	// FixStart is the span start of the fix.
	FixStart token.Pos

	// FixEnd is the span end of the fix.
	FixEnd token.Pos

	// LineNumber represent the actual line number in the file. This is set when
	// finding the diagnostic to make it easier to suggest fixes in
	// golangci-lint.
	LineNumber int

	// MessageType represents the type of message it is.
	MessageType MessageType

	// Message is the diagnostic to show.
	Message string
}

// Settings contains settings for edge-cases.
type Settings struct {
	Mode      RunningMode
	MultiIf   bool
	MultiFunc bool
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

	return *flags
}

func Run(pass *analysis.Pass, settings *Settings) []Message {
	messages := []Message{}

	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if !strings.HasSuffix(filename, ".go") {
			continue
		}

		fileMessages := runFile(file, pass.Fset, *settings)

		if settings.Mode == RunningModeGolangCI {
			messages = append(messages, fileMessages...)
			continue
		}

		for _, message := range fileMessages {
			pass.Report(analysis.Diagnostic{
				Pos:      message.Diagnostic,
				Category: "whitespace",
				Message:  message.Message,
				SuggestedFixes: []analysis.SuggestedFix{
					{
						TextEdits: []analysis.TextEdit{
							{
								Pos:     message.FixStart,
								End:     message.FixEnd,
								NewText: []byte("\n"),
							},
						},
					},
				},
			})
		}
	}

	return messages
}

func runFile(file *ast.File, fset *token.FileSet, settings Settings) []Message {
	var messages []Message

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
	messages    []Message
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

		first, last := firstAndLast(comments, v.fset, stmt.Pos(), stmt.End(), stmt.List)
		startMsg := checkStart(v.fset, stmt.Lbrace, first)

		if wantNewline && startMsg == nil && len(stmt.List) >= 1 {
			v.messages = append(v.messages, Message{
				Diagnostic:  stmt.Lbrace,
				FixStart:    stmt.List[0].Pos(),
				FixEnd:      stmt.List[0].Pos(),
				LineNumber:  v.fset.PositionFor(stmt.List[0].Pos(), false).Line,
				MessageType: MessageTypeAdd,
				Message:     "multi-line statement should be followed by a newline",
			})
		} else if !wantNewline && startMsg != nil {
			v.messages = append(v.messages, *startMsg)
		}

		if msg := checkEnd(v.fset, stmt.Rbrace, last); msg != nil {
			v.messages = append(v.messages, *msg)
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
	return fset.PositionFor(pos, false).Line
}

func firstAndLast(comments []*ast.CommentGroup, fset *token.FileSet, start, end token.Pos, stmts []ast.Stmt) (ast.Node, ast.Node) {
	if len(stmts) == 0 {
		return nil, nil
	}

	first, last := ast.Node(stmts[0]), ast.Node(stmts[len(stmts)-1])

	for _, c := range comments {
		if posLine(fset, c.Pos()) == posLine(fset, start) || posLine(fset, c.End()) == posLine(fset, end) {
			continue
		}

		if c.Pos() < start || c.End() > end {
			continue
		}
		if c.Pos() < first.Pos() {
			first = c
		}
		if c.End() > last.End() {
			last = c
		}
	}

	return first, last
}

func checkStart(fset *token.FileSet, start token.Pos, first ast.Node) *Message {
	if first == nil {
		return nil
	}

	if posLine(fset, start)+1 < posLine(fset, first.Pos()) {
		// We need to know what column we're at to mark or fix start at the
		// first node position minus its columns mins the size of a newline.
		firstColumn := fset.PositionFor(first.Pos(), false).Column

		return &Message{
			Diagnostic: start,
			// We remove 3 for the first statement position to account for the
			// newline but not having to consider potential comments between the
			// left bracket and the first statement.
			FixStart:    first.Pos() - token.Pos(firstColumn) - 1,
			FixEnd:      first.Pos(),
			LineNumber:  fset.PositionFor(first.Pos(), false).Line - 1,
			MessageType: MessageTypeRemove,
			Message:     "unnecessary leading newline",
		}
	}

	return nil
}

func checkEnd(fset *token.FileSet, end token.Pos, last ast.Node) *Message {
	if last == nil {
		return nil
	}

	if posLine(fset, end)-1 > posLine(fset, last.End()) {
		return &Message{
			Diagnostic:  end,
			FixStart:    last.End(),
			FixEnd:      end,
			LineNumber:  fset.PositionFor(last.End(), false).Line + 1,
			MessageType: MessageTypeRemove,
			Message:     "unnecessary trailing newline",
		}
	}

	return nil
}
