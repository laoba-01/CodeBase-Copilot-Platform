package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/codebase-copilot/core/internal/domain"
)

// ParseGoRepo walks a Go repo directory and extracts index nodes + edges.
func ParseGoRepo(repoPath, repoID string) ([]*domain.IndexNode, []*domain.CallEdge, []*domain.DepEdge, error) {
	fset := token.NewFileSet()

	// Parse all .go files (excluding vendor, testdata)
	pkgs, err := parser.ParseDir(fset, repoPath, func(fi os.FileInfo) bool {
		name := fi.Name()
		return !strings.HasSuffix(name, "_test.go") && strings.HasSuffix(name, ".go")
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, nil, err
	}

	var nodes []*domain.IndexNode
	var calls []*domain.CallEdge
	var deps []*domain.DepEdge

	// Map to track: node name → node ID (for call edge linking)
	funcMap := make(map[string]string) // "pkg.FuncName" → nodeID

	for pkgName, pkg := range pkgs {
		for filename, file := range pkg.Files {
			// Create file node
			fileID := uuid.New().String()
			fileNode := &domain.IndexNode{
				ID:       fileID,
				RepoID:   repoID,
				Type:     domain.NodeTypeFile,
				Name:     filepath.Base(filename),
				FilePath: relPath(repoPath, filename),
				Language: "go",
				Package:  pkgName,
			}
			nodes = append(nodes, fileNode)

			// Extract imports
			for _, imp := range file.Imports {
				depTarget := strings.Trim(imp.Path.Value, "\"")
				deps = append(deps, &domain.DepEdge{
					ID:       uuid.New().String(),
					RepoID:   repoID,
					SourceID: fileID,
					TargetID: depTarget, // external dep or internal file
					DepType:  "import",
				})
			}

			// Extract functions and methods
			ast.Inspect(file, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.FuncDecl:
					funcID := uuid.New().String()
					funcName := decl.Name.Name
					if decl.Recv != nil {
						// Method
						recvType := extractRecvType(decl.Recv)
						funcName = recvType + "." + funcName
					}
					fullName := pkgName + "." + funcName

					startPos := fset.Position(decl.Pos())
					endPos := fset.Position(decl.End())

					funcNode := &domain.IndexNode{
						ID:        funcID,
						RepoID:    repoID,
						Type:      funcType(decl),
						Name:      funcName,
						Signature: buildSignature(decl),
						Code:      extractSource(filename, startPos.Line, endPos.Line),
						FilePath:  relPath(repoPath, filename),
						StartLine: startPos.Line,
						EndLine:   endPos.Line,
						Language:  "go",
						Package:   pkgName,
					}
					nodes = append(nodes, funcNode)
					funcMap[fullName] = funcID

					// Extract calls within function body
					ast.Inspect(decl.Body, func(n ast.Node) bool {
						if call, ok := n.(*ast.CallExpr); ok {
							calleeName := extractCallee(call)
							calls = append(calls, &domain.CallEdge{
								ID:       uuid.New().String(),
								RepoID:   repoID,
								CallerID: funcID,
								CalleeID: calleeName, // will be resolved later
								FilePath: relPath(repoPath, filename),
								Line:     fset.Position(call.Pos()).Line,
							})
						}
						return true
					})
				}
				return true
			})
		}
	}

	// Resolve call edges: try to match callee names to known functions
	for _, call := range calls {
		if resolvedID, ok := funcMap[call.CalleeID]; ok {
			call.CalleeID = resolvedID
		}
		// If not found, calleeID stays as the function name (external or unresolved)
	}

	return nodes, calls, deps, nil
}

func relPath(base, filename string) string {
	rel, _ := filepath.Rel(base, filename)
	return filepath.ToSlash(rel)
}

func funcType(decl *ast.FuncDecl) domain.IndexNodeType {
	if decl.Recv != nil {
		return domain.NodeTypeMethod
	}
	return domain.NodeTypeFunction
}

func extractRecvType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func buildSignature(decl *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")
	if decl.Recv != nil {
		sb.WriteString("(" + extractRecvType(decl.Recv) + ") ")
	}
	sb.WriteString(decl.Name.Name)
	sb.WriteString("(")
	if decl.Type.Params != nil {
		for i, p := range decl.Type.Params.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			for j, n := range p.Names {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(n.Name)
			}
			sb.WriteString(" ")
			sb.WriteString(typeString(p.Type))
		}
	}
	sb.WriteString(")")
	if decl.Type.Results != nil {
		sb.WriteString(" ")
		if len(decl.Type.Results.List) > 1 {
			sb.WriteString("(")
		}
		for i, r := range decl.Type.Results.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(typeString(r.Type))
		}
		if len(decl.Type.Results.List) > 1 {
			sb.WriteString(")")
		}
	}
	return sb.String()
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	default:
		return ""
	}
}

func extractCallee(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			return ident.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	}
	return ""
}

func extractSource(filename string, startLine, endLine int) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return ""
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}
