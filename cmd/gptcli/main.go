/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	_ "embed"
	"fmt"
	"hash/crc32"
	"os"
	"strconv"
	"strings"
	"time"

	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	gc "github.com/gbin/goncurses"

	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/llmclient"
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
	RowFmt                = "│ %8v │ %18v │ %18v │ %18v │ %-18v\n"
	RowSpacer             = "──────────────────────────────────────────────────────────────────────────────────────────────\n"
)

type Prefs struct {
	SummarizePrior bool   `json:"summarize_prior"`
	Vendor         string `json:"vendor"`
}

type GptCliContext struct {
	client types.GptCliAIClient
	//	input              *bufio.Reader
	ui                 types.GptCliUI
	scr                *gc.Window
	needConfig         bool
	curSummaryToggle   bool
	prefs              Prefs
	threadGroups       []*GptCliThreadGroup
	archiveThreadGroup *GptCliThreadGroup
	mainThreadGroup    *GptCliThreadGroup
	curThreadGroup     *GptCliThreadGroup
}

func NewGptCliContext(ctx context.Context) *GptCliContext {

	//	inputLocal := bufio.NewReader(os.Stdin)

	var uiLocal types.GptCliUI
	var err error
	var scrLocal *gc.Window
	scrLocal, err = gcInit()
	if err != nil {
		panic("fix me")
	}
	uiLocal = ui.NewNcursesUI(scrLocal)

	gptCliCtx := &GptCliContext{
		client: nil,
		//		input:            inputLocal,
		ui:               uiLocal,
		scr:              scrLocal,
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

	gptCliCtx.client = llmclient.NewEINOClient(ctx, gptCliCtx.prefs.Vendor,
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

func genUniqFileName(name string, cTime time.Time) string {
	return fmt.Sprintf("%v_%v.json",
		strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(name))), 16),
		cTime.Unix())
}

func summaryToggleMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	// @todo convert to dialogue
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

	// @todo convert to dialogue. also need search results submenu
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

	// @todo convert to options dialogue
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

func main() {
	ctx := context.Background()
	gptCliCtx := NewGptCliContext(ctx)
	defer gcExit()

	// @todo needConfig?
	err := gptCliCtx.load(ctx)
	if err != nil && !gptCliCtx.needConfig {
		fmt.Fprintf(os.Stderr, "gptcli: Failed to load: %v\n", err)
		os.Exit(1)
	}

	menuMain(ctx, gptCliCtx, make([]string, 2))
}
