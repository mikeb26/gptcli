/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

type FilePatchTool struct {
	approvalUI ToolApprovalUI
}

type FilePatchReq struct {
	Input string `json:"input" jsonschema:"description=The patch content you wish to be applied."`
}

type FilePatchResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the apply_patch call"`
}

func (g FilePatchTool) GetOp() types.ToolCallOp {
	return types.FilePatch
}

func (t FilePatchTool) RequiresUserApproval() bool {
	return true
}

// BuildApprovalRequest implements ToolWithCustomApproval for
// FilePatchTool. It analyzes the patch content to determine all affected
// files, computes their common root directory, and builds a
// directory-scoped approval request. This allows policies granted for a
// directory (e.g. via the file tools) to automatically apply to
// apply_patch as well.
func (t FilePatchTool) BuildApprovalRequest(arg any) ToolApprovalRequest {
	req, ok := arg.(*FilePatchReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	paths := collectPatchPaths(req.Input)
	if len(paths) == 0 {
		// If we cannot infer any paths from the patch text, fall back to the
		// default behavior.
		return DefaultApprovalRequest(t, arg)
	}

	// Normalize all paths to absolute, cleaned form so that approvals
	// are keyed consistently regardless of the working directory or how
	// the patch paths are expressed.
	absPaths := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			} else {
				p = filepath.Clean(p)
			}
		} else {
			p = filepath.Clean(p)
		}
		absPaths = append(absPaths, p)
	}

	rootDir := commonRootDir(absPaths)

	var dirPolicyID string
	if rootDir != string(filepath.Separator) && rootDir != "." && rootDir != "" {
		dirPolicyID = am.ApprovalPolicyID(
			am.ApprovalSubsysTools,
			am.ApprovalGroupFileIO,
			am.ApprovalTargetDir,
			rootDir,
		)
	}

	promptBuilder := &strings.Builder{}
	fmt.Fprintf(promptBuilder,
		"gptcli would like to %v affecting files under %q.\n",
		t.GetOp(), rootDir)
	fmt.Fprintf(promptBuilder, "This patch touches %d file(s):\n", len(paths))

	sort.Strings(absPaths)
	const maxList = 10
	for i, p := range absPaths {
		if i >= maxList {
			fmt.Fprintf(promptBuilder, "  ... and %d more\n", len(absPaths)-maxList)
			break
		}
		fmt.Fprintf(promptBuilder, "  - %s\n", p)
	}
	promptBuilder.WriteString("Allow?")

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		},
	}

	if dirPolicyID != "" {
		choices = append(choices, am.ApprovalChoice{
			Key:      "dw",
			Label:    "Yes, and allow all future reads/writes within this directory (recursively)",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: dirPolicyID,
			Actions: []am.ApprovalAction{
				am.ApprovalActionWrite,
				am.ApprovalActionRead,
			},
		})
	}

	choices = append(choices, am.ApprovalChoice{
		Key:   "n",
		Label: "No",
		Scope: am.ApprovalScopeDeny,
	})

	return ToolApprovalRequest{
		Tool:            t,
		Arg:             arg,
		Prompt:          promptBuilder.String(),
		RequiredActions: []am.ApprovalAction{am.ApprovalActionWrite},
		Choices:         choices,
	}
}
func NewFilePatchTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &FilePatchTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

//go:embed apply_patch.txt
var ApplyPatchDesc string

func (t FilePatchTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), ApplyPatchDesc, t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t FilePatchTool) Invoke(ctx context.Context, req *FilePatchReq) (*FilePatchResp, error) {
	ret := &FilePatchResp{}

	err := GetUserApproval(ctx, t.approvalUI, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	err = processPatch(req.Input)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	return ret, nil
}

// the rest of this file is a lightly edited o4-mini translation of:
// https://cookbook.openai.com/examples/gpt4-1_prompting_guide#reference-implementation-apply_patchpy

type PatchActionType string

const (
	PatchActionAdd    PatchActionType = "add"
	PatchActionDelete                 = "delete"
	PatchActionUpdate                 = "update"
)

const (
	PatchSentinelToken      = "***"
	PatchSentinelBeginPatch = PatchSentinelToken + " Begin Patch"
	PatchSentinelEOP        = PatchSentinelToken + " End Patch"
	PatchSentinelUpdateFile = PatchSentinelToken + " Update File"
	PatchSentinelMove       = PatchSentinelToken + " Move to"
	PatchSentinelDelete     = PatchSentinelToken + " Delete File"
	PatchSentinelAdd        = PatchSentinelToken + " Add File"
	PatchSentinelEOF        = PatchSentinelToken + " End of File"

	// sentinels with colon/whitespace
	PatchSentinelUpdateFile2 = PatchSentinelUpdateFile + ": "
	PatchSentinelMove2       = PatchSentinelMove + ": "
	PatchSentinelDelete2     = PatchSentinelDelete + ": "
	PatchSentinelAdd2        = PatchSentinelAdd + ": "

	// sentinels with colon but no whitespace
	PatchSentinelUpdateFile3 = PatchSentinelUpdateFile + ":"
	PatchSentinelDelete3     = PatchSentinelDelete + ":"
	PatchSentinelAdd3        = PatchSentinelAdd + ":"
)

type FileChange struct {
	Type       PatchActionType
	OldContent string
	NewContent string
	MovePath   string
}

type Commit struct {
	Changes map[string]FileChange
}

type Chunk struct {
	OrigIndex int
	DelLines  []string
	InsLines  []string
}

type PatchAction struct {
	Type     PatchActionType
	NewFile  string
	Chunks   []Chunk
	MovePath string
}

type Patch struct {
	Actions map[string]PatchAction
}

type Parser struct {
	currentFiles map[string]string
	lines        []string
	index        int
	patch        Patch
	fuzz         int
}

func NewParser(currentFiles map[string]string, lines []string) *Parser {
	return &Parser{
		currentFiles: currentFiles,
		lines:        lines,
		index:        0,
		patch:        Patch{Actions: make(map[string]PatchAction)},
		fuzz:         0,
	}
}

func (p *Parser) curLine() (string, error) {
	if p.index >= len(p.lines) {
		return "", errors.New("unexpected end of input while parsing patch")
	}
	return p.lines[p.index], nil
}

func norm(line string) string {
	return strings.TrimRight(line, "\r")
}

func (p *Parser) isDone(prefixes ...string) bool {
	if p.index >= len(p.lines) {
		return true
	}
	if len(prefixes) > 0 {
		nl := norm(p.lines[p.index])
		for _, pre := range prefixes {
			if strings.HasPrefix(nl, pre) {
				return true
			}
		}
	}
	return false
}

func (p *Parser) startsWith(prefix string) bool {
	if p.index >= len(p.lines) {
		return false
	}
	return strings.HasPrefix(norm(p.lines[p.index]), prefix)
}

func (p *Parser) readStr(prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("readStr requires non-empty prefix")
	}
	if p.index >= len(p.lines) {
		return "", nil
	}
	line := p.lines[p.index]
	nl := norm(line)
	if strings.HasPrefix(nl, prefix) {
		p.index++
		return line[len(prefix):], nil
	}
	return "", nil
}

func (p *Parser) readLine() (string, error) {
	if p.index >= len(p.lines) {
		return "", errors.New("unexpected end of input while reading line")
	}
	line := p.lines[p.index]
	p.index++
	return line, nil
}

func (p *Parser) Parse() error {
	for !p.isDone(PatchSentinelEOP) {
		// PatchActionUpdate
		if path, _ := p.readStr(PatchSentinelUpdateFile2); path != "" {
			if _, ok := p.patch.Actions[path]; ok {
				return fmt.Errorf("duplicate update for file: %s", path)
			}
			moveTo, _ := p.readStr(PatchSentinelMove2)
			if _, ok := p.currentFiles[path]; !ok {
				return fmt.Errorf("update file error – missing file: %s", path)
			}
			text := p.currentFiles[path]
			action, err := p.parseUpdateFile(text)
			if err != nil {
				return err
			}
			if moveTo != "" {
				action.MovePath = moveTo
			}
			p.patch.Actions[path] = action
			continue
		}
		// PatchActionDelete
		if path, _ := p.readStr(PatchSentinelDelete2); path != "" {
			if _, ok := p.patch.Actions[path]; ok {
				return fmt.Errorf("duplicate delete for file: %s", path)
			}
			if _, ok := p.currentFiles[path]; !ok {
				return fmt.Errorf("delete file error – missing file: %s", path)
			}
			p.patch.Actions[path] = PatchAction{Type: PatchActionDelete}
			continue
		}
		// PatchActionAdd
		if path, _ := p.readStr(PatchSentinelAdd2); path != "" {
			if _, ok := p.patch.Actions[path]; ok {
				return fmt.Errorf("duplicate add for file: %s", path)
			}
			if _, ok := p.currentFiles[path]; ok {
				return fmt.Errorf("add file error – file already exists: %s", path)
			}
			action, err := p.parseAddFile()
			if err != nil {
				return err
			}
			p.patch.Actions[path] = action
			continue
		}
		// Unknown
		line, _ := p.curLine()
		return fmt.Errorf("unknown line while parsing: %s", line)
	}
	if !p.startsWith(PatchSentinelEOP) {
		return errors.New(fmt.Sprintf("missing %v sentinel", PatchSentinelEOP))
	}
	p.index++ // consume it
	return nil
}

func (p *Parser) parseAddFile() (PatchAction, error) {
	var lines []string
	for !p.isDone(PatchSentinelEOP, PatchSentinelUpdateFile3,
		PatchSentinelDelete3, PatchSentinelAdd3) {
		s, err := p.readLine()
		if err != nil {
			return PatchAction{}, err
		}
		if !strings.HasPrefix(s, "+") {
			return PatchAction{}, fmt.Errorf("invalid add file line (missing '+'): %s", s)
		}
		lines = append(lines, s[1:])
	}
	content := strings.Join(lines, "\n")
	return PatchAction{Type: PatchActionAdd, NewFile: content}, nil
}

func (p *Parser) parseUpdateFile(text string) (PatchAction, error) {
	action := PatchAction{Type: PatchActionUpdate}
	origLines := strings.Split(text, "\n")
	idx := 0

	for !p.isDone(PatchSentinelEOP, PatchSentinelUpdateFile3,
		PatchSentinelDelete3, PatchSentinelAdd3, PatchSentinelEOF) {
		defStr, _ := p.readStr("@@ ")
		if defStr == "" && norm(p.lines[p.index]) == "@@" {
			// some diffs use a bare "@@"
			p.index++
		}
		// now find next section
		nextCtx, chunks, endIdx, eof, err := peekNextSection(p.lines, p.index)
		if err != nil {
			return action, err
		}
		newIdx, fuzz := findContext(origLines, nextCtx, idx, eof)
		if newIdx < 0 {
			return action, fmt.Errorf("invalid context at %d: %v", idx, nextCtx)
		}
		p.fuzz += fuzz
		for _, ch := range chunks {
			ch.OrigIndex += newIdx
			action.Chunks = append(action.Chunks, ch)
		}
		idx = newIdx + len(nextCtx)
		p.index = endIdx
	}
	return action, nil
}

func findContextCore(lines, ctx []string, start int) (int, int) {
	if len(ctx) == 0 {
		return start, 0
	}
	// exact
	for i := start; i+len(ctx) <= len(lines); i++ {
		ok := true
		for j := 0; j < len(ctx); j++ {
			if lines[i+j] != ctx[j] {
				ok = false
				break
			}
		}
		if ok {
			return i, 0
		}
	}
	// strip \r
	for i := start; i+len(ctx) <= len(lines); i++ {
		ok := true
		for j := 0; j < len(ctx); j++ {
			if strings.TrimRight(lines[i+j], "\r") != strings.TrimRight(ctx[j], "\r") {
				ok = false
				break
			}
		}
		if ok {
			return i, 1
		}
	}
	// strip whitespace
	for i := start; i+len(ctx) <= len(lines); i++ {
		ok := true
		for j := 0; j < len(ctx); j++ {
			if strings.TrimSpace(lines[i+j]) != strings.TrimSpace(ctx[j]) {
				ok = false
				break
			}
		}
		if ok {
			return i, 100
		}
	}
	return -1, 0
}

func findContext(lines, ctx []string, start int, eof bool) (int, int) {
	if eof {
		idx, fuzz := findContextCore(lines, ctx, len(lines)-len(ctx))
		if idx >= 0 {
			return idx, fuzz
		}
		idx, fuzz2 := findContextCore(lines, ctx, start)
		return idx, fuzz2 + 10000
	}
	return findContextCore(lines, ctx, start)
}

func peekNextSection(lines []string, idx int) (ctx []string, chunks []Chunk, endIdx int, eof bool, err error) {
	var old, delLines, insLines []string
	origIdx := idx
	mode := "keep"

	for idx < len(lines) {
		s := lines[idx]
		if strings.HasPrefix(s, "@@") ||
			strings.HasPrefix(s, PatchSentinelEOP) ||
			strings.HasPrefix(s, PatchSentinelUpdateFile3) ||
			strings.HasPrefix(s, PatchSentinelDelete3) ||
			strings.HasPrefix(s, PatchSentinelAdd3) ||
			strings.HasPrefix(s, PatchSentinelEOF) {
			break
		}
		if s == PatchSentinelToken {
			break
		}
		if strings.HasPrefix(s, PatchSentinelToken) {
			return nil, nil, 0, false, fmt.Errorf("invalid line: %s", s)
		}
		idx++
		last := mode
		if s == "" {
			s = " "
		}
		switch s[0] {
		case '+':
			mode = "add"
		case '-':
			mode = "delete"
		case ' ':
			mode = "keep"
		default:
			return nil, nil, 0, false, fmt.Errorf("invalid line: %s", s)
		}
		lineText := s[1:]
		if mode == "keep" && last != mode && (len(delLines) > 0 || len(insLines) > 0) {
			chunks = append(chunks, Chunk{
				OrigIndex: len(old) - len(delLines),
				DelLines:  append([]string{}, delLines...),
				InsLines:  append([]string{}, insLines...),
			})
			delLines = nil
			insLines = nil
		}
		switch mode {
		case "delete":
			delLines = append(delLines, lineText)
			old = append(old, lineText)
		case "add":
			insLines = append(insLines, lineText)
		case "keep":
			old = append(old, lineText)
		}
	}
	if len(delLines) > 0 || len(insLines) > 0 {
		chunks = append(chunks, Chunk{
			OrigIndex: len(old) - len(delLines),
			DelLines:  delLines,
			InsLines:  insLines,
		})
	}
	if idx < len(lines) && strings.HasPrefix(lines[idx], PatchSentinelEOF) {
		idx++
		return old, chunks, idx, true, nil
	}
	if idx == origIdx {
		return nil, nil, 0, false, errors.New("nothing in this section")
	}
	return old, chunks, idx, false, nil
}

func getUpdatedFile(orig string, action PatchAction, path string) (string, error) {
	if action.Type != PatchActionUpdate {
		return "", errors.New("getUpdatedFile called with non-update action")
	}
	origLines := strings.Split(orig, "\n")
	var dest []string
	oi := 0
	for _, ch := range action.Chunks {
		if ch.OrigIndex > len(origLines) {
			return "", fmt.Errorf("%s: chunk index %d exceeds file length", path, ch.OrigIndex)
		}
		if oi > ch.OrigIndex {
			return "", fmt.Errorf("%s: overlapping chunks at %d > %d", path, oi, ch.OrigIndex)
		}
		dest = append(dest, origLines[oi:ch.OrigIndex]...)
		oi = ch.OrigIndex
		dest = append(dest, ch.InsLines...)
		oi += len(ch.DelLines)
	}
	dest = append(dest, origLines[oi:]...)
	return strings.Join(dest, "\n"), nil
}

func patchToCommit(patch Patch, orig map[string]string) (Commit, error) {
	commit := Commit{Changes: make(map[string]FileChange)}
	for path, action := range patch.Actions {
		switch action.Type {
		case PatchActionDelete:
			old := orig[path]
			commit.Changes[path] = FileChange{Type: PatchActionDelete, OldContent: old}
		case PatchActionAdd:
			if action.NewFile == "" {
				return commit, errors.New("add action without file content")
			}
			commit.Changes[path] = FileChange{Type: PatchActionAdd, NewContent: action.NewFile}
		case PatchActionUpdate:
			newContent, err := getUpdatedFile(orig[path], action, path)
			if err != nil {
				return commit, err
			}
			commit.Changes[path] = FileChange{
				Type:       PatchActionUpdate,
				OldContent: orig[path],
				NewContent: newContent,
				MovePath:   action.MovePath,
			}
		}
	}
	return commit, nil
}

func textToPatch(text string, orig map[string]string) (Patch, int, error) {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 ||
		!strings.HasPrefix(norm(lines[0]), PatchSentinelBeginPatch) ||
		norm(lines[len(lines)-1]) != PatchSentinelEOP {
		return Patch{}, 0, errors.New("invalid patch text – missing sentinels")
	}
	parser := NewParser(orig, lines)
	parser.index = 1
	if err := parser.Parse(); err != nil {
		return Patch{}, 0, err
	}
	return parser.patch, parser.fuzz, nil
}

func identifyFilesNeeded(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, PatchSentinelUpdateFile2) {
			out = append(out, line[len(PatchSentinelUpdateFile2):])
		}
		if strings.HasPrefix(line, PatchSentinelDelete2) {
			out = append(out, line[len(PatchSentinelDelete2):])
		}
	}
	return out
}

func identifyFilesAdded(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, PatchSentinelAdd2) {
			out = append(out, line[len(PatchSentinelAdd2):])
		}
	}
	return out
}

// identifyMoveTargets returns the destination paths of any move
// operations described in the patch text.
func identifyMoveTargets(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, PatchSentinelMove2) {
			out = append(out, line[len(PatchSentinelMove2):])
		}
	}
	return out
}

// collectPatchPaths gathers all unique file paths affected by the patch
// text, including updated, deleted, added, and move-target files.
func collectPatchPaths(text string) []string {
	seen := make(map[string]struct{})
	add := func(paths []string) {
		for _, p := range paths {
			if p == "" {
				continue
			}
			seen[p] = struct{}{}
		}
	}

	add(identifyFilesNeeded(text))
	add(identifyFilesAdded(text))
	add(identifyMoveTargets(text))

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// commonRootDir computes the deepest common directory that contains all
// of the provided paths. It is used to scope directory-level approval
// policies for apply_patch.
func commonRootDir(paths []string) string {
	if len(paths) == 0 {
		return "."
	}

	common := filepath.Dir(filepath.Clean(paths[0]))

	isWithin := func(path, root string) bool {
		if root == "" {
			return false
		}
		cleanPath := filepath.Clean(path)
		cleanRoot := filepath.Clean(root)
		if cleanPath == cleanRoot {
			return true
		}
		if !strings.HasSuffix(cleanRoot, string(filepath.Separator)) {
			cleanRoot += string(filepath.Separator)
		}
		return strings.HasPrefix(cleanPath, cleanRoot)
	}

	for _, p := range paths[1:] {
		d := filepath.Dir(filepath.Clean(p))
		for common != string(filepath.Separator) && common != "." && common != "" && !isWithin(d, common) {
			common = filepath.Dir(common)
		}
		if common == "" {
			break
		}
	}
	if common == "" {
		return "."
	}
	return common
}

func loadFiles(paths []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, p := range paths {
		content, err := openFile(p)
		if err != nil {
			return nil, err
		}
		m[p] = content
	}
	return m, nil
}

func applyCommit(commit Commit) error {
	for path, change := range commit.Changes {
		switch change.Type {
		case PatchActionDelete:
			if err := os.Remove(path); err != nil {
				return err
			}
		case PatchActionAdd:
			if change.NewContent == "" {
				return fmt.Errorf("add change for %s has no content", path)
			}
			if err := writeFile(path, change.NewContent); err != nil {
				return err
			}
		case PatchActionUpdate:
			if change.NewContent == "" {
				return fmt.Errorf("update change for %s has no new content", path)
			}
			target := path
			if change.MovePath != "" {
				target = change.MovePath
			}
			if err := writeFile(target, change.NewContent); err != nil {
				return err
			}
			if change.MovePath != "" {
				if err := os.Remove(path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func deleteBeforeBeginPatch(s string) string {
	if idx := strings.Index(s, PatchSentinelBeginPatch); idx != -1 {
		return s[idx:]
	}
	return s
}

func deleteAfterEndPatch(s string) string {
	if idx := strings.Index(s, PatchSentinelEOP); idx != -1 {
		return s[:idx+len(PatchSentinelEOP)]
	}
	return s
}

func processPatch(text string) error {
	text = deleteBeforeBeginPatch(text)
	text = deleteAfterEndPatch(text)

	if !strings.HasPrefix(text, PatchSentinelBeginPatch) {
		return errors.New(fmt.Sprintf("patch text must start with %v",
			PatchSentinelBeginPatch))
	}
	need := identifyFilesNeeded(text)
	orig, err := loadFiles(need)
	if err != nil {
		return err
	}
	patch, _, err := textToPatch(text, orig)
	if err != nil {
		return err
	}
	commit, err := patchToCommit(patch, orig)
	if err != nil {
		return err
	}
	if err := applyCommit(commit); err != nil {
		return err
	}
	return nil
}

func openFile(path string) (string, error) {
	b, err := ioutil.ReadFile(path)
	return string(b), err
}

func writeFile(path, content string) error {
	target := filepath.Clean(path)
	dir := filepath.Dir(target)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return ioutil.WriteFile(target, []byte(content), 0644)
}
