package indexer

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/codebase-copilot/core/internal/domain"
)

// languageExt maps file extensions to language identifiers.
var languageExt = map[string]string{
	".c":    "c",
	".h":    "c",
	".py":   "python",
	".java": "java",
	".js":   "javascript",
	".ts":   "typescript",
	".jsx":  "javascript",
	".tsx":  "typescript",
	".rs":   "rust",
	".cpp":  "cpp",
	".hpp":  "cpp",
	".cc":   "cpp",
	".rb":   "ruby",
	".php":  "php",
	".swift": "swift",
	".kt":   "kotlin",
	".scala": "scala",
	".go":   "go",
	".cs":   "csharp",
	".lua":  "lua",
}

// ctagsEntry represents a parsed line from ctags default output.
// Format: tagname\tfilepath\taddress;"\tfield:value\t...
type ctagsEntry struct {
	Name      string
	Path      string
	Line      int
	Kind      string
	Language  string
	Scope     string
	Signature string
	End       int
}

// parseCtagsLine parses a single ctags default-format line.
func parseCtagsLine(line string) *ctagsEntry {
	// Format: name\tfile\taddress;"\tfield:value\t...
	parts := strings.Split(line, "\t")
	if len(parts) < 3 {
		return nil
	}

	entry := &ctagsEntry{
		Name: parts[0],
		Path: parts[1],
	}

	// Parse address field: line_num;"  or  /pattern/;"
	addr := parts[2]
	if idx := strings.Index(addr, `;"`); idx >= 0 {
		addr = addr[:idx]
	}
	// Try numeric line number
	if n, err := fmt.Sscanf(addr, "%d", &entry.Line); n == 1 && err == nil {
		// line already set
	} else {
		entry.Line = 1 // fallback
	}

	// Parse extension fields: key:value
	for i := 2; i < len(parts); i++ {
		field := parts[i]
		if idx := strings.Index(field, `;"`); idx >= 0 && i == 2 {
			// already handled address
			field = field[idx+2:]
			if field == "" {
				continue
			}
		}
		kv := strings.SplitN(field, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val := kv[1]
		switch key {
		case "kind":
			entry.Kind = val
		case "language":
			entry.Language = val
		case "line":
			fmt.Sscanf(val, "%d", &entry.Line)
		case "end":
			fmt.Sscanf(val, "%d", &entry.End)
		case "signature":
			entry.Signature = val
		case "scope":
			entry.Scope = val
		}
	}

	return entry
}

// detectLanguage scans a repo directory and returns the dominant language
// by counting file extensions.
func detectLanguage(repoPath string) string {
	counts := make(map[string]int)
	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip hidden dirs, vendor, testdata
		name := info.Name()
		dir := filepath.ToSlash(filepath.Dir(path))
		for _, skip := range []string{"/.git/", "/vendor/", "/testdata/", "/node_modules/", "/__pycache__/"} {
			if strings.HasPrefix(name, ".") || strings.Contains(dir, skip) {
				return nil
			}
		}
		ext := filepath.Ext(name)
		if lang, ok := languageExt[ext]; ok {
			counts[lang]++
		}
		return nil
	})

	var bestLang string
	var bestCount int
	for lang, count := range counts {
		if count > bestCount {
			bestCount = count
			bestLang = lang
		}
	}
	if bestLang == "" {
		return "unknown"
	}
	return bestLang
}

// sourceExtensions returns file extensions to scan for a given language.
func sourceExtensions(lang string) string {
	switch lang {
	case "c":
		return `\.(c|h)$`
	case "cpp":
		return `\.(cpp|hpp|cc|cxx|hxx)$`
	case "python":
		return `\.(py)$`
	case "java":
		return `\.(java)$`
	case "javascript", "typescript":
		return `\.(js|ts|jsx|tsx)$`
	case "rust":
		return `\.(rs)$`
	case "ruby":
		return `\.(rb)$`
	case "php":
		return `\.(php)$`
	case "swift":
		return `\.(swift)$`
	case "kotlin":
		return `\.(kt)$`
	case "scala":
		return `\.(scala)$`
	case "csharp":
		return `\.(cs)$`
	case "lua":
		return `\.(lua)$`
	default:
		return `\.(c|h|cpp|hpp|py|java|js|ts|rs|rb|php|swift|kt|scala|cs|lua)$`
	}
}

// ParseUniversal parses any codebase using universal-ctags.
func ParseUniversal(repoPath, repoID string) ([]*domain.IndexNode, []*domain.CallEdge, []*domain.DepEdge, error) {
	lang := detectLanguage(repoPath)
	extPattern := sourceExtensions(lang)

	// Collect source file paths
	var files []string
	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		skipDirs := []string{"/.git/", "/vendor/", "/testdata/", "/node_modules/", "/__pycache__/"}
		for _, skip := range skipDirs {
			if strings.Contains(filepath.Dir(path), skip) {
				return nil
			}
		}
		ext := filepath.Ext(info.Name())
		for _, e := range strings.Split(strings.Trim(extPattern, `\$`), "|") {
			e = strings.Trim(e, `\()`)
			if "."+e == ext {
				files = append(files, path)
				break
			}
		}
		return nil
	})

	if len(files) == 0 {
		return nil, nil, nil, fmt.Errorf("no source files found for language %s", lang)
	}

	// Write file list to temp file for ctags -L
	tmpFile, err := os.CreateTemp("", "ctags-filelist-*.txt")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create temp filelist: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	for _, f := range files {
		fmt.Fprintln(tmpFile, f)
	}
	tmpFile.Close()

	// Run ctags with default output + extended fields
	cmd := exec.Command("ctags",
		"--fields=+lnS",
		"-f", "-",
		"-L", tmpFile.Name(),
	)
	cmd.Dir = repoPath
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ctags pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("start ctags: %w", err)
	}

	var nodes []*domain.IndexNode
	var deps []*domain.DepEdge
	seenFiles := make(map[string]string) // filePath → fileNodeID
	seenFuncs := make(map[string]bool)   // dedup: "filePath:name"

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 || strings.HasPrefix(line, "!") {
			continue
		}

		entry := parseCtagsLine(line)
		if entry == nil {
			continue
		}

		relPath := relPathAbs(repoPath, entry.Path)

		// Ensure file node exists
		if _, exists := seenFiles[relPath]; !exists {
			fileID := uuid.New().String()
			fileNode := &domain.IndexNode{
				ID:       fileID,
				RepoID:   repoID,
				Type:     domain.NodeTypeFile,
				Name:     filepath.Base(entry.Path),
				FilePath: relPath,
				Language: lang,
			}
			nodes = append(nodes, fileNode)
			seenFiles[relPath] = fileID

			// Extract imports/includes for this file (dependency edges)
			fileDeps := extractFileDeps(entry.Path, lang)
			for _, depTarget := range fileDeps {
				deps = append(deps, &domain.DepEdge{
					ID:       uuid.New().String(),
					RepoID:   repoID,
					SourceID: fileID,
					TargetID: depTarget,
					DepType:  "import",
				})
			}
		}

		// Map ctags kind to node type
		nodeType := mapKindToType(entry.Kind, lang)
		if nodeType == "" {
			continue
		}

		// Dedup by file+name
		dedupKey := relPath + ":" + entry.Name
		if seenFuncs[dedupKey] {
			continue
		}
		seenFuncs[dedupKey] = true

		endLine := entry.End
		if endLine == 0 {
			endLine = entry.Line
		}

		// Build full name: scope+name for methods
		fullName := entry.Name
		if entry.Scope != "" {
			fullName = entry.Scope + "." + entry.Name
		}

		node := &domain.IndexNode{
			ID:        uuid.New().String(),
			RepoID:    repoID,
			Type:      nodeType,
			Name:      fullName,
			Signature: entry.Signature,
			Code:      extractSourceLines(entry.Path, entry.Line, endLine),
			FilePath:  relPath,
			StartLine: entry.Line,
			EndLine:   endLine,
			Language:  lang,
			Package:   filepath.Dir(relPath),
		}
		nodes = append(nodes, node)
	}

	if err := cmd.Wait(); err != nil {
		// ctags returns non-zero for some warnings, but results are still valid
		// Only fail hard if we got zero nodes
		if len(nodes) == 0 {
			return nil, nil, nil, fmt.Errorf("ctags: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("scan ctags output: %w", err)
	}

	return nodes, nil, deps, nil
}

// relPathAbs computes a relative path given an absolute repo path.
func relPathAbs(repoPath, absPath string) string {
	rel, err := filepath.Rel(repoPath, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	return filepath.ToSlash(rel)
}

// mapKindToType converts a ctags kind to a domain IndexNodeType.
func mapKindToType(kind, lang string) domain.IndexNodeType {
	switch strings.ToLower(kind) {
	case "f", "function", "func":
		return domain.NodeTypeFunction
	case "m", "method":
		return domain.NodeTypeMethod
	case "c", "class", "struct", "interface", "enum":
		return domain.NodeTypeClass
	default:
		return ""
	}
}

// extractSourceLines reads lines [start, end] from a file.
func extractSourceLines(path string, startLine, endLine int) string {
	data, err := os.ReadFile(path)
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

// extractFileDeps extracts import/include dependencies for a source file.
func extractFileDeps(filePath, lang string) []string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	content := string(data)
	var deps []string
	seen := make(map[string]bool)

	switch lang {
	case "c", "cpp":
		// #include "..." or #include <...>
		re := regexp.MustCompile(`#include\s+[<"]([^>"]+)[>"]`)
		for _, match := range re.FindAllStringSubmatch(content, -1) {
			dep := match[1]
			if !seen[dep] {
				seen[dep] = true
				deps = append(deps, dep)
			}
		}
	case "python":
		// import xxx or from xxx import yyy
		re := regexp.MustCompile(`^(?:from|import)\s+(\S+)`)
		for _, line := range strings.Split(content, "\n") {
			if match := re.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
				dep := match[1]
				if !seen[dep] {
					seen[dep] = true
					deps = append(deps, dep)
				}
			}
		}
	case "javascript", "typescript":
		// import ... from 'xxx' or require('xxx')
		re := regexp.MustCompile(`(?:from\s+["']([^"']+)["']|require\s*\(\s*["']([^"']+)["'])`)
		for _, match := range re.FindAllStringSubmatch(content, -1) {
			dep := match[1]
			if dep == "" {
				dep = match[2]
			}
			if dep != "" && !seen[dep] {
				seen[dep] = true
				deps = append(deps, dep)
			}
		}
	case "java":
		// import xxx.yyy.Zzz;
		re := regexp.MustCompile(`^import\s+([^;]+);`)
		for _, line := range strings.Split(content, "\n") {
			if match := re.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
				dep := match[1]
				if !seen[dep] {
					seen[dep] = true
					deps = append(deps, dep)
				}
			}
		}
	case "rust":
		// use xxx::yyy;
		re := regexp.MustCompile(`^use\s+([^;]+);`)
		for _, line := range strings.Split(content, "\n") {
			if match := re.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
				dep := match[1]
				if !seen[dep] {
					seen[dep] = true
					deps = append(deps, dep)
				}
			}
		}
	}

	return deps
}
