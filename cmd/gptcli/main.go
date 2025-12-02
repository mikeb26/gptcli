/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"

	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
)

const (
	CommandName           = "gptcli"
	KeyFileFmt            = ".%v.key"
	PrefsFile             = "prefs.json"
	ThreadsDir            = "threads"
	ArchiveDir            = "archive_threads"
	CodeBlockDelim        = "```"
	CodeBlockDelimNewline = "```\n"
	ThreadParseErrFmt     = "Could not parse %v. Please enter a valid thread number.\n"
	ThreadNoExistErrFmt   = "Thread %v does not exist. To list threads try 'ls'.\n"
	RowFmt                = "| %8v | %18v | %18v | %18v | %-18v\n"
	RowSpacer             = "----------------------------------------------------------------------------------------------\n"
)

var subCommandTab = map[string]func(ctx context.Context,
	gptCliCtx *GptCliContext, args []string) error{

	"help":      helpMain,
	"version":   versionMain,
	"upgrade":   upgradeMain,
	"config":    configMain,
	"ls":        lsThreadsMain,
	"menu":      menuMain,
	"thread":    threadSwitchMain,
	"new":       newThreadMain,
	"summary":   summaryToggleMain,
	"archive":   archiveThreadMain,
	"unarchive": unarchiveThreadMain,
	"exit":      exitMain,
	"quit":      exitMain,
	"search":    searchMain,
	"cat":       catMain,
	"reasoning": reasoningMain,
}

type Prefs struct {
	SummarizePrior bool   `json:"summarize_prior"`
	Vendor         string `json:"vendor"`
}

type GptCliContext struct {
	client             types.GptCliAIClient
	input              *bufio.Reader
	ui                 types.GptCliUI
	needConfig         bool
	curSummaryToggle   bool
	prefs              Prefs
	threadGroups       []*GptCliThreadGroup
	archiveThreadGroup *GptCliThreadGroup
	mainThreadGroup    *GptCliThreadGroup
	curThreadGroup     *GptCliThreadGroup
}

func NewGptCliContext(ctx context.Context) *GptCliContext {

	inputLocal := bufio.NewReader(os.Stdin)

	gptCliCtx := &GptCliContext{
		client:           nil,
		input:            inputLocal,
		ui:               ui.NewStdioUI().WithBufReader(inputLocal),
		needConfig:       true,
		curSummaryToggle: false,
		prefs: Prefs{
			SummarizePrior: false,
			Vendor:         internal.DefaultVendor,
		},
		archiveThreadGroup: nil,
		mainThreadGroup:    nil,
		curThreadGroup:     nil,
		threadGroups:       make([]*GptCliThreadGroup, 0),
	}

	threadsDirLocal, err := getThreadsDir()
	if err != nil {
		threadsDirLocal = "/tmp"
	}
	archiveDirLocal, err := getArchiveDir()
	if err != nil {
		archiveDirLocal = "/tmp"
	}

	gptCliCtx.threadGroups = append(gptCliCtx.threadGroups,
		NewGptCliThreadGroup("", threadsDirLocal))
	gptCliCtx.threadGroups = append(gptCliCtx.threadGroups,
		NewGptCliThreadGroup("a", archiveDirLocal))

	gptCliCtx.mainThreadGroup = gptCliCtx.threadGroups[0]
	gptCliCtx.archiveThreadGroup = gptCliCtx.threadGroups[1]
	gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
	err = gptCliCtx.loadPrefs()
	if err == nil {
		gptCliCtx.needConfig = false
	}

	return gptCliCtx
}

func (gptCliCtx *GptCliContext) load(ctx context.Context) error {

	gptCliCtx.needConfig = true
	err := gptCliCtx.loadPrefs()
	if err != nil {
		return err
	}
	keyText, err := loadKey(gptCliCtx.prefs.Vendor)
	if err != nil {
		return err
	}

	gptCliCtx.client = internal.NewEINOClient(ctx, gptCliCtx.prefs.Vendor,
		gptCliCtx.ui, keyText, internal.DefaultModels[gptCliCtx.prefs.Vendor], 0)

	for _, thrGrp := range gptCliCtx.threadGroups {
		err := thrGrp.loadThreads()
		if err != nil {
			return err
		}
	}
	gptCliCtx.needConfig = false

	return nil
}

//go:embed help.txt
var helpText string

func helpMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	fmt.Print(helpText)

	return nil
}

func exitMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.curThreadGroup.curThreadNum == 0 {
		return io.EOF
	}

	gptCliCtx.curThreadGroup.curThreadNum = 0
	gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup

	return nil
}

func printStringViaPager(content string) error {
	cmd := exec.Command("less", "-r", "-X")
	cmd.Stdout = os.Stdout
	inPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	err = cmd.Start()
	if err != nil {
		inPipe.Close()
		return fmt.Errorf("failed to start less command: %w", err)
	}
	_, err = inPipe.Write([]byte(content))
	if err != nil {
		inPipe.Close()
		return fmt.Errorf("failed to write to stdin pipe: %w", err)
	}
	inPipe.Close()

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("less command failed: %w", err)
	}

	return nil
}

func printToScreen(str2print string) {
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		height = 25
		err = nil
	}
	if strings.Count(str2print, "\n") >= height {
		err = printStringViaPager(str2print)
		if err != nil {
			return
		} // else
	} // else

	fmt.Printf("%v", str2print)
}

func genUniqFileName(name string, cTime time.Time) string {
	return fmt.Sprintf("%v_%v.json",
		strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(name))), 16),
		cTime.Unix())
}

func summaryToggleMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	usageErr := fmt.Errorf("Syntax is 'summary [<on|off>]' e.g. 'summary on'\n")

	if len(args) == 1 {
		gptCliCtx.curSummaryToggle = !gptCliCtx.curSummaryToggle
	} else if len(args) != 2 {
		return usageErr
	} else {
		if strings.ToLower(args[1]) == "on" {
			gptCliCtx.curSummaryToggle = true
		} else if strings.ToLower(args[1]) == "off" {
			gptCliCtx.curSummaryToggle = false
		} else {
			return usageErr
		}
	}

	if gptCliCtx.curSummaryToggle {
		fmt.Printf("summaries enabled; summaries of the thread history are sent for followups in order to reduce costs.\n")
	} else {
		fmt.Printf("summaries disabled; the full thread history is sent for	followups in order to get more precise responses.\n")
	}

	return nil
}

func threadContainsSearchStr(t *GptCliThread, searchStr string) bool {
	for _, msg := range t.Dialogue {
		if msg.Role == types.GptCliMessageRoleSystem {
			continue
		}

		if strings.Contains(msg.Content, searchStr) {
			return true
		}
	}

	return false
}

func searchMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	usageErr := fmt.Errorf("Syntax is 'search <search_string>[,<search_string>...] e.g. 'search foo'\n")

	if len(args) < 2 {
		return usageErr
	}
	searchStrs := args[1:]

	var sb strings.Builder

	sb.WriteString(threadGroupHeaderString(true))

	for _, thrGrp := range gptCliCtx.threadGroups {
		for tidx, t := range thrGrp.threads {
			count := 0
			for _, searchStr := range searchStrs {
				if threadContainsSearchStr(t, searchStr) {
					count++
				}
			}
			if count == len(searchStrs) {
				threadNum := fmt.Sprintf("%v%v", thrGrp.prefix, tidx+1)
				sb.WriteString(t.HeaderString(threadNum))
			}
		}
	}

	sb.WriteString(threadGroupFooterString())

	fmt.Printf("%v", sb.String())

	return nil
}

func reasoningMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	usageErr := fmt.Errorf("Syntax is 'reasoning <low|medium|high>'\n")

	if len(args) != 2 {
		return usageErr
	}
	reasoningLvl := laclopenai.ReasoningEffortLevel(strings.ToLower(args[1]))
	switch reasoningLvl {
	case laclopenai.ReasoningEffortLevelHigh:
		fallthrough
	case laclopenai.ReasoningEffortLevelMedium:
		fallthrough
	case laclopenai.ReasoningEffortLevelLow:
		break
	default:
		return fmt.Errorf("Unknown reasoning effort: %v", args[1])
	}

	gptCliCtx.client.SetReasoning(reasoningLvl)
	return nil
}

func getMultiLineInputRemainder(gptCliCtx *GptCliContext) (string, error) {
	var ret string
	var tmp string
	var err error

	for !strings.HasSuffix(tmp, CodeBlockDelim) &&
		!strings.HasSuffix(tmp, CodeBlockDelimNewline) {

		tmp, err = gptCliCtx.input.ReadString('\n')
		if err != nil {
			return "", err
		}

		ret = fmt.Sprintf("%v%v", ret, tmp)
	}

	return ret, nil
}

func getCmdOrPrompt(gptCliCtx *GptCliContext) (string, error) {
	var cmdOrPrompt string
	var err error
	thrGrp := gptCliCtx.curThreadGroup
	for len(cmdOrPrompt) == 0 {
		if thrGrp.curThreadNum == 0 {
			fmt.Printf("gptcli> ")
		} else {
			fmt.Printf("gptcli/%v> ",
				thrGrp.threads[thrGrp.curThreadNum-1].Name)
		}
		cmdOrPrompt, err = gptCliCtx.input.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "exit", nil
			}
			return "", err
		}
		cmdOrPrompt = strings.TrimSpace(cmdOrPrompt)
		if strings.HasSuffix(cmdOrPrompt, CodeBlockDelim) {
			text2append, err := getMultiLineInputRemainder(gptCliCtx)
			if err != nil {
				return "", err
			}
			cmdOrPrompt = fmt.Sprintf("%v\n%v", cmdOrPrompt, text2append)
		}
	}

	return cmdOrPrompt, nil
}

// in order to reduce costs, summarize the prior dialogue history with
// the GPT4oMini when resending the thread to OpenAI
func summarizeDialogue(ctx context.Context, gptCliCtx *GptCliContext,
	dialogue []*types.GptCliMessage) ([]*types.GptCliMessage, error) {

	summaryDialogue := []*types.GptCliMessage{
		{Role: types.GptCliMessageRoleSystem,
			Content: prompts.SystemMsg},
	}

	msg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleSystem,
		Content: prompts.SummarizeMsg,
	}
	dialogue = append(dialogue, msg)

	fmt.Printf("gptcli: summarizing...\n")
	msg, err := gptCliCtx.client.CreateChatCompletion(ctx, dialogue)
	if err != nil {
		return summaryDialogue, err
	}

	summaryDialogue = append(summaryDialogue, msg)

	return summaryDialogue, nil
}

func splitBlocks(text string) []string {
	blocks := make([]string, 0)

	inBlock := false
	idx := strings.Index(text, CodeBlockDelim)
	numBlocks := 0
	for ; idx != -1; idx = strings.Index(text, CodeBlockDelim) {
		appendText := text[0:idx]
		if inBlock {
			appendText = CodeBlockDelim + appendText
		} else if numBlocks != 0 {
			blocks[numBlocks-1] = blocks[numBlocks-1] + CodeBlockDelim
		}
		blocks = append(blocks, appendText)
		text = text[idx+len(CodeBlockDelim):]
		inBlock = !inBlock
		numBlocks++
	}
	if len(text) > 0 {
		if inBlock {
			text = text + CodeBlockDelim
		} else if numBlocks != 0 {
			blocks[numBlocks-1] = blocks[numBlocks-1] + CodeBlockDelim
		}
		blocks = append(blocks, text)
	}

	return blocks
}

func (gptCliCtx *GptCliContext) getSubCmd(
	cmdOrPrompt string) func(context.Context, *GptCliContext, []string) error {

	subCmdFunc, ok := subCommandTab[cmdOrPrompt]
	if ok {
		return subCmdFunc
	}
	if gptCliCtx.curThreadGroup.curThreadNum != 0 {
		return nil
	} // else we're not in a current thread; find closest match to allow
	// aliasing. e.g. allow user to type 'a' instead of 'archive' if there's
	// no other subcommand that starts with 'a'.

	var subCmdFound string
	for k, _ := range subCommandTab {
		if strings.HasPrefix(k, cmdOrPrompt) {
			if subCmdFound != "" {
				// ambiguous
				return nil
			}

			subCmdFound = k
		}
	}

	return subCommandTab[subCmdFound]
}

func main() {
	checkAndPrintUpgradeWarning()

	ctx := context.Background()
	gptCliCtx := NewGptCliContext(ctx)

	if !gptCliCtx.needConfig {
		checkAndUpgradeConfig()
	}

	err := gptCliCtx.load(ctx)
	if err != nil && !gptCliCtx.needConfig {
		fmt.Fprintf(os.Stderr, "gptcli: Failed to load: %v\n", err)
		os.Exit(1)
	}

	var fullCmdOrPrompt string
	var cmdOrPrompt string
	for {
		fullCmdOrPrompt, err = getCmdOrPrompt(gptCliCtx)
		if err != nil {
			break
		}
		cmdArgs := strings.Split(fullCmdOrPrompt, " ")
		cmdOrPrompt = cmdArgs[0]
		subCmdFunc := gptCliCtx.getSubCmd(cmdOrPrompt)
		if subCmdFunc == nil {
			if gptCliCtx.curThreadGroup.curThreadNum == 0 {
				fmt.Fprintf(os.Stderr, "gptcli: Unknown command %v. Try	'help'.\n",
					cmdOrPrompt)
				continue
			} // else we're already in a thread
			err = interactiveThreadWork(ctx, gptCliCtx, fullCmdOrPrompt)
		} else {
			err = subCmdFunc(ctx, gptCliCtx, cmdArgs)
		}

		if err != nil {
			break
		}
	}

	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "gptcli: %v. quitting.\n", err)
		os.Exit(1)
	}

	fmt.Printf("gptcli: quitting.\n")
}
