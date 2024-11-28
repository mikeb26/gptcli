/* Copyright © 2023-2024 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/mikeb26/gptcli/internal"
	"github.com/sashabaranov/go-openai"
)

const (
	CommandName           = "gptcli"
	KeyFile               = ".openai.key"
	SessionFile           = ".openai.session"
	PrefsFile             = "prefs.json"
	ThreadsDir            = "threads"
	ArchiveDir            = "archive_threads"
	CodeBlockDelim        = "```"
	CodeBlockDelimNewline = "```\n"
	ThreadParseErrFmt     = "Could not parse %v. Please enter a valid thread number.\n"
	ThreadNoExistErrFmt   = "Thread %v does not exist. To list threads try 'ls'.\n"
)

const SystemMsg = `You are gptcli, a CLI based utility that otherwise acts
exactly like ChatGPT. All subsequent user messages you receive are input from a
CLI interface and your responses will be displayed on a CLI interface. Your
source code is available at https://github.com/mikeb26/gptcli.`

const SummarizeMsg = `Please summarize the entire prior conversation
history. The resulting summary should be optimized for consumption by a more
recent version of GPT than yourself. The purpose of the summary is to reduce the
costs of using GPT by reducing token counts.`

var subCommandTab = map[string]func(ctx context.Context,
	gptCliCtx *GptCliContext, args []string) error{

	"help":      helpMain,
	"version":   versionMain,
	"upgrade":   upgradeMain,
	"config":    configMain,
	"ls":        lsThreadsMain,
	"thread":    threadSwitchMain,
	"new":       newThreadMain,
	"summary":   summaryToggleMain,
	"archive":   archiveThreadMain,
	"unarchive": unarchiveThreadMain,
	"exit":      exitMain,
	"quit":      exitMain,
	"billing":   billingMain,
}

type GptCliThread struct {
	Name            string                         `json:"name"`
	CreateTime      time.Time                      `json:"ctime"`
	AccessTime      time.Time                      `json:"atime"`
	ModTime         time.Time                      `json:"mtime"`
	Dialogue        []openai.ChatCompletionMessage `json:"dialogue"`
	SummaryDialogue []openai.ChatCompletionMessage `json:"summary_dialogue,omitempty"`

	fileName string
}

type GptCliThreadGroup struct {
	prefix       string
	threads      []*GptCliThread
	totThreads   int
	dir          string
	curThreadNum int
}

type Prefs struct {
	SummarizePrior bool `json:"summarize_prior"`
}

type GptCliContext struct {
	client             internal.OpenAIClient
	haveSess           bool
	sessClient         internal.OpenAIClient
	input              *bufio.Reader
	needConfig         bool
	curSummaryToggle   bool
	prefs              Prefs
	threadGroups       []*GptCliThreadGroup
	archiveThreadGroup *GptCliThreadGroup
	mainThreadGroup    *GptCliThreadGroup
	curThreadGroup     *GptCliThreadGroup
}

func NewGptCliContext() *GptCliContext {
	var clientLocal *openai.Client
	needConfigLocal := false
	keyText, err := loadKey()
	if err != nil {
		needConfigLocal = true
	} else {
		clientLocal = openai.NewClient(keyText)
	}
	var sessClientLocal *openai.Client
	haveSessLocal := false
	sessText, err := loadSess()
	if err == nil {
		sessClientLocal = openai.NewClient(sessText)
		haveSessLocal = true
	}

	gptCliCtx := &GptCliContext{
		client:           clientLocal,
		haveSess:         haveSessLocal,
		sessClient:       sessClientLocal,
		input:            bufio.NewReader(os.Stdin),
		needConfig:       needConfigLocal,
		curSummaryToggle: false,
		prefs: Prefs{
			SummarizePrior: false,
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

	return gptCliCtx
}

func NewGptCliThreadGroup(prefixIn string, dirIn string) *GptCliThreadGroup {

	thrGrp := &GptCliThreadGroup{
		prefix:       prefixIn,
		threads:      make([]*GptCliThread, 0),
		totThreads:   0,
		dir:          dirIn,
		curThreadNum: 0,
	}

	return thrGrp
}

func (gptCliCtx *GptCliContext) load() error {
	err := gptCliCtx.loadPrefs()
	if err != nil {
		return err
	}
	if gptCliCtx.needConfig {
		return nil
	}
	for _, thrGrp := range gptCliCtx.threadGroups {
		err := thrGrp.loadThreads()
		if err != nil {
			return err
		}
	}
	return nil
}

func (thrGrp *GptCliThreadGroup) loadThreads() error {
	thrGrp.curThreadNum = 0
	thrGrp.totThreads = 0
	thrGrp.threads = make([]*GptCliThread, 0)

	dEntries, err := os.ReadDir(thrGrp.dir)
	if err != nil {
		return fmt.Errorf("Failed to read dir %v: %w", thrGrp.dir, err)
	}

	for _, dEnt := range dEntries {
		fullpath := filepath.Join(thrGrp.dir, dEnt.Name())
		threadFileText, err := os.ReadFile(fullpath)
		if err != nil {
			return fmt.Errorf("Failed to read %v: %w", fullpath, err)
		}

		var thread GptCliThread
		err = json.Unmarshal(threadFileText, &thread)
		if err != nil {
			return fmt.Errorf("Failed to parse %v: %w", fullpath, err)
		}
		thread.fileName = genUniqFileName(thread.Name, thread.CreateTime)
		if thread.fileName != dEnt.Name() {
			oldPath := filepath.Join(thrGrp.dir, dEnt.Name())
			newPath := filepath.Join(thrGrp.dir, thread.fileName)
			fmt.Fprintf(os.Stderr, "Renaming thread %v to %v\n",
				oldPath, newPath)
			_ = os.Remove(oldPath)
			_ = thread.save(thrGrp.dir)
		}

		_ = thrGrp.addThread(&thread)
	}

	return nil
}

func (gptCliCtx *GptCliContext) loadPrefs() error {
	if gptCliCtx.needConfig {
		return nil
	}

	filePath, err := getPrefsPath()
	if err != nil {
		return fmt.Errorf("Failed to get prefs path: %w", err)
	}
	prefsFileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("Failed to read prefs: %w", err)
	}
	err = json.Unmarshal(prefsFileContent, &gptCliCtx.prefs)
	if err != nil {
		return err
	}
	gptCliCtx.curSummaryToggle = gptCliCtx.prefs.SummarizePrior

	return nil
}

func (gptCliCtx *GptCliContext) savePrefs() error {
	prefsFileContent, err := json.Marshal(gptCliCtx.prefs)
	if err != nil {
		return fmt.Errorf("Failed to marshal prefs: %w", err)
	}

	filePath, err := getPrefsPath()
	if err != nil {
		return fmt.Errorf("Failed to get prefs path: %w", err)
	}
	err = os.WriteFile(filePath, prefsFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save prefs: %w", err)
	}

	return nil
}

func (thread *GptCliThread) save(dir string) error {
	threadFileContent, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v: %w", thread.Name, err)
	}

	filePath := filepath.Join(dir, thread.fileName)
	err = os.WriteFile(filePath, threadFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v(%v): %w", thread.Name,
			filePath, err)
	}

	return nil
}

func (thread *GptCliThread) remove(dir string) error {
	filePath := filepath.Join(dir, thread.fileName)
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("Failed to delete thread %v(%v): %w", thread.Name,
			filePath, err)
	}

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

func centsToDollarString(cents float64) string {
	ret := fmt.Sprintf("$%.2f", cents*0.01)
	if ret == "$0.00" {
		ret = "<$0.01"
	}

	return ret
}

func billingMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.needConfig {
		return fmt.Errorf("You must run 'config' before querying billing usage history.\n")
	}

	if !gptCliCtx.haveSess {
		return fmt.Errorf("A session key must first be configured to use the billing feature. try 'config'")
	}
	endDate := time.Now().Add(24 * time.Hour)
	startDate := endDate.Add(-(30 * 24 * time.Hour))
	resp, err := gptCliCtx.sessClient.GetBillingUsage(ctx, startDate, endDate)
	if err != nil {
		return err
	}

	fmt.Printf("Usage from %v - %v:\n", startDate.Format(time.DateOnly),
		endDate.Format(time.DateOnly))

	var printedDate bool
	for _, d := range resp.DailyCosts {
		printedDate = false
		for _, li := range d.LineItems {
			if li.Cost == 0 {
				continue
			}

			if !printedDate {
				fmt.Printf("%v:\n", d.Time.Format(time.DateOnly))
				printedDate = true
			}
			fmt.Printf("\t%v: %v\n", li.Name, centsToDollarString(li.Cost))
		}
	}

	fmt.Printf("\nTotal: %v\n", centsToDollarString(resp.TotalUsage))

	return nil
}

//go:embed version.txt
var versionText string

const DevVersionText = "v0.devbuild"

func versionMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	fmt.Printf("gptcli-%v\n", versionText)

	return nil
}

func upgradeMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	if versionText == DevVersionText {
		fmt.Fprintf(os.Stderr, "Skipping gptcli upgrade on development version\n")
		return nil
	}
	latestVer, err := getLatestVersion()
	if err != nil {
		return err
	}
	if latestVer == versionText {
		fmt.Printf("gptcli %v is already the latest version\n",
			versionText)
		return nil
	}

	fmt.Printf("A new version of gptcli is available (%v). Upgrade? (Y/N) [Y]: ",
		latestVer)
	shouldUpgrade, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}

	shouldUpgrade = strings.ToUpper(strings.TrimSpace(shouldUpgrade))
	if len(shouldUpgrade) == 0 {
		shouldUpgrade = "Y"
	}
	if shouldUpgrade[0] != 'Y' {
		return nil
	}

	fmt.Printf("Upgrading gptcli from %v to %v...\n", versionText,
		latestVer)

	err = upgradeViaGithub(latestVer)
	if err != nil {
		return err
	}

	return io.EOF
}

func getLatestVersion() (string, error) {
	const LatestReleaseUrl = "https://api.github.com/repos/mikeb26/gptcli/releases/latest"

	client := http.Client{
		Timeout: time.Second * 30,
	}

	resp, err := client.Get(LatestReleaseUrl)
	if err != nil {
		return "", err
	}

	releaseJsonDoc, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var releaseDoc map[string]any
	err = json.Unmarshal(releaseJsonDoc, &releaseDoc)
	if err != nil {
		return "", err
	}

	latestRelease, ok := releaseDoc["tag_name"].(string)
	if !ok {
		return "", fmt.Errorf("Could not parse %v", LatestReleaseUrl)
	}

	return latestRelease, nil
}

func upgradeViaGithub(latestVer string) error {
	const LatestDownloadFmt = "https://github.com/mikeb26/gptcli/releases/download/%v/gptcli"

	client := http.Client{
		Timeout: time.Second * 30,
	}

	resp, err := client.Get(fmt.Sprintf(LatestDownloadFmt, latestVer))
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)

	}

	tmpFile, err := os.CreateTemp("", "gptcli-*")
	if err != nil {
		return fmt.Errorf("Failed to create temp file: %w", err)
	}
	binaryContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	_, err = tmpFile.Write(binaryContent)
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	err = tmpFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	err = tmpFile.Close()
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	myBinaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Could not determine path to gptcli: %w", err)
	}
	myBinaryPath, err = filepath.EvalSymlinks(myBinaryPath)
	if err != nil {
		return fmt.Errorf("Could not determine path to gptcli: %w", err)
	}

	myBinaryPathBak := myBinaryPath + ".bak"
	err = os.Rename(myBinaryPath, myBinaryPathBak)
	if err != nil {
		return fmt.Errorf("Could not replace existing %v; do you need to be root?: %w",
			myBinaryPath, err)
	}
	err = os.Rename(tmpFile.Name(), myBinaryPath)
	if errors.Is(err, syscall.EXDEV) {
		// invalid cross device link occurs when rename() is attempted aross
		// different filesystems; copy instead
		err = os.WriteFile(myBinaryPath, binaryContent, 0755)
		_ = os.Remove(tmpFile.Name())
	}
	if err != nil {
		err := fmt.Errorf("Could not replace existing %v; do you need to be root?: %w",
			myBinaryPath, err)
		_ = os.Rename(myBinaryPathBak, myBinaryPath)
		return err
	}
	_ = os.Remove(myBinaryPathBak)

	fmt.Printf("Upgrade %v to %v complete\n", myBinaryPath, latestVer)

	return nil
}

func checkAndPrintUpgradeWarning() bool {
	if versionText == DevVersionText {
		return false
	}
	latestVer, err := getLatestVersion()
	if err != nil {
		return false
	}
	if latestVer == versionText {
		return false
	}

	fmt.Fprintf(os.Stderr, "*WARN*: A new version of gptcli is available (%v). Please upgrade via 'upgrade'.\n\n",
		latestVer)

	return true
}

func checkAndUpgradeConfig() {
	// versions v0.3.5 and earlier don't have the archive dir
	archiveDir, err := getArchiveDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "*WARN*: Unable to add archive directory: %v\n",
			err)
		return
	}
	err = os.MkdirAll(archiveDir, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "*WARN*: Unable to add archive directory %v: %v",
			archiveDir, err)
		return
	}
}

func lsThreadsMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.mainThreadGroup.totThreads == 0 {
		fmt.Printf("You haven't created any threads yet. To create a thread use the 'new' command.\n")
		return nil
	}

	showAll := false

	f := flag.NewFlagSet("ls", flag.ContinueOnError)
	f.BoolVar(&showAll, "all", false, "Also show archive threads")
	err := f.Parse(args[1:])
	if err != nil {
		return err
	}

	fmt.Printf("%v", gptCliCtx.mainThreadGroup.String(true, !showAll))
	if showAll {
		fmt.Printf("%v", gptCliCtx.archiveThreadGroup.String(false, true))
	}

	return nil
}

func (thrGrp *GptCliThreadGroup) String(header bool, footer bool) string {
	var sb strings.Builder

	const rowFmt = "| %8v | %18v | %18v | %18v | %-18v\n"
	const rowSpacer = "----------------------------------------------------------------------------------------------\n"

	if header {
		sb.WriteString(rowSpacer)
		sb.WriteString(fmt.Sprintf(rowFmt, "Thread#", "Last Accessed", "Last Modified",
			"Created", "Name"))
		sb.WriteString(rowSpacer)
	}

	for idx, t := range thrGrp.threads {
		cTime := t.CreateTime.Format("01/02/2006 03:04pm")
		aTime := t.AccessTime.Format("01/02/2006 03:04pm")
		mTime := t.ModTime.Format("01/02/2006 03:04pm")
		today := time.Now().UTC().Truncate(24 * time.Hour).Format("01/02/2006")
		yesterday := time.Now().UTC().Add(-24 * time.Hour).Truncate(24 * time.Hour).Format("01/02/2006")
		cTime = strings.ReplaceAll(cTime, today, "Today")
		aTime = strings.ReplaceAll(aTime, today, "Today")
		mTime = strings.ReplaceAll(mTime, today, "Today")
		cTime = strings.ReplaceAll(cTime, yesterday, "Yesterday")
		aTime = strings.ReplaceAll(aTime, yesterday, "Yesterday")
		mTime = strings.ReplaceAll(mTime, yesterday, "Yesterday")

		threadNum := fmt.Sprintf("%v%v", thrGrp.prefix, idx+1)
		sb.WriteString(fmt.Sprintf(rowFmt, threadNum, aTime, mTime, cTime,
			t.Name))
	}

	if footer {
		sb.WriteString(rowSpacer)
	}

	return sb.String()
}

func parseThreadNum(gptCliCtx *GptCliContext,
	userInput string) (*GptCliThreadGroup, int, error) {

	prefix := strings.TrimRight(userInput, "0123456789")
	suffix := userInput[len(prefix):]
	threadNum, err := strconv.ParseUint(suffix, 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf(ThreadParseErrFmt, userInput)
	}

	for _, thrGrp := range gptCliCtx.threadGroups {
		if prefix == thrGrp.prefix {
			return thrGrp, int(threadNum), nil
		}
	}

	return nil, 0, fmt.Errorf(ThreadParseErrFmt, userInput)
}

func threadSwitchMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if len(args) != 2 {
		return fmt.Errorf("Syntax is 'thread <thread#>' e.g. 'thread 1'\n")
	}
	thrGrp, threadNum, err := parseThreadNum(gptCliCtx, args[1])
	if err != nil {
		return err
	}
	if gptCliCtx.curThreadGroup != thrGrp {
		gptCliCtx.curThreadGroup = thrGrp
	}
	return thrGrp.threadSwitch(int(threadNum))
}

func (thrGrp *GptCliThreadGroup) threadSwitch(threadNum int) error {
	if threadNum > thrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", thrGrp.prefix, threadNum)
		return fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thrGrp.curThreadNum = threadNum
	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	thread.AccessTime = time.Now()
	err := thread.save(thrGrp.dir)
	if err != nil {
		return err
	}

	_ = printStringViaPager(thread.String())

	return nil
}

func (thread *GptCliThread) String() string {
	var sb strings.Builder

	for _, msg := range thread.Dialogue {
		if msg.Role == openai.ChatMessageRoleSystem {
			continue
		}

		if msg.Role == openai.ChatMessageRoleAssistant {
			blocks := splitBlocks(msg.Content)
			for idx, b := range blocks {
				if idx%2 == 0 {
					sb.WriteString(color.CyanString("%v\n", b))
				} else {
					sb.WriteString(color.GreenString("%v\n", b))
				}
			}
			continue
		}

		// should be msg.Role == openai.ChatMessageRoleUser
		sb.WriteString(fmt.Sprintf("gptcli/%v> %v\n",
			thread.Name, msg.Content))
	}

	return sb.String()
}

func printStringViaPager(content string) error {
	cmd := exec.Command("less", "-r", "-F")
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

func genUniqFileName(name string, cTime time.Time) string {
	return fmt.Sprintf("%v_%v.json",
		strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(name))), 16),
		cTime.Unix())
}

func newThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.needConfig {
		return fmt.Errorf("You must run 'config' before creating a thread.\n")
	}

	fmt.Printf("Enter new thread's name: ")
	name, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	cTime := time.Now()
	fileName := genUniqFileName(name, cTime)

	dialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: SystemMsg},
	}

	curThread := &GptCliThread{
		Name:            name,
		CreateTime:      cTime,
		AccessTime:      cTime,
		ModTime:         cTime,
		Dialogue:        dialogue,
		SummaryDialogue: make([]openai.ChatCompletionMessage, 0),
		fileName:        fileName,
	}
	gptCliCtx.mainThreadGroup.curThreadNum =
		gptCliCtx.mainThreadGroup.addThread(curThread)

	return nil
}

func (thrGrp *GptCliThreadGroup) addThread(curThread *GptCliThread) int {
	thrGrp.totThreads++
	thrGrp.threads = append(thrGrp.threads, curThread)

	return thrGrp.totThreads
}

func archiveThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if len(args) != 2 {
		return fmt.Errorf("Syntax is 'archive <thread#>' e.g. 'archive 1'\n")
	}
	thrGrp, threadNum, err := parseThreadNum(gptCliCtx, args[1])
	if err != nil {
		return err
	}

	if thrGrp == gptCliCtx.archiveThreadGroup {
		return fmt.Errorf("gptcli: Thread already archived")
	} else if thrGrp != gptCliCtx.mainThreadGroup {
		panic("BUG: archiveThreadMain() only supports 2 thread groups currently")
	}

	err = thrGrp.moveThread(int(threadNum), gptCliCtx.archiveThreadGroup)
	if err != nil {
		return fmt.Errorf("gptcli: Failed to archive thread: %w", err)
	}

	fmt.Printf("gptcli: Archived thread %v. Remaining threads renumbered.\n",
		threadNum)

	lsArgs := []string{"ls"}
	return lsThreadsMain(ctx, gptCliCtx, lsArgs)
}

func unarchiveThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if len(args) != 2 {
		return fmt.Errorf("Syntax is 'unarchive a<thread#>' e.g. 'unarchive 1'\n")
	}
	thrGrp, threadNum, err := parseThreadNum(gptCliCtx, args[1])
	if err != nil {
		return err
	}

	if thrGrp == gptCliCtx.mainThreadGroup {
		return fmt.Errorf("gptcli: Thread already unarchived")
	} else if thrGrp != gptCliCtx.archiveThreadGroup {
		panic("BUG: unarchiveThreadMain() only supports 2 thread groups currently")
	}

	err = thrGrp.moveThread(int(threadNum), gptCliCtx.mainThreadGroup)
	if err != nil {
		return fmt.Errorf("gptcli: Failed to unarchive thread: %w", err)
	}

	fmt.Printf("gptcli: Unarchived thread %v. Remaining threads renumbered.\n",
		threadNum)

	lsArgs := []string{"ls"}
	return lsThreadsMain(ctx, gptCliCtx, lsArgs)
}

func (srcThrGrp *GptCliThreadGroup) moveThread(threadNum int,
	dstThrGrp *GptCliThreadGroup) error {

	if threadNum > srcThrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", srcThrGrp.prefix, threadNum)
		return fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thread := srcThrGrp.threads[threadNum-1]

	err := thread.save(dstThrGrp.dir)
	if err != nil {
		return err
	}
	err = thread.remove(srcThrGrp.dir)
	if err != nil {
		_ = thread.remove(dstThrGrp.dir)
		return err
	}
	srcThrGrp.curThreadNum = 0

	dstThrGrp.addThread(thread)

	return srcThrGrp.loadThreads()
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

func configMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(configDir, 0700)
	if err != nil {
		return fmt.Errorf("Could not create config directory %v: %w",
			configDir, err)
	}
	keyPath := path.Join(configDir, KeyFile)
	_, err = os.Stat(keyPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Could not open OpenAI API key file %v: %w", keyPath, err)
	}
	fmt.Printf("Enter your OpenAI API key: ")
	key, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	err = os.WriteFile(keyPath, []byte(key), 0600)
	if err != nil {
		return fmt.Errorf("Could not write OpenAI API key file %v: %w", keyPath, err)
	}
	sessPath := path.Join(configDir, SessionFile)
	_, err = os.Stat(sessPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Could not open OpenAI Session file %v: %w", keyPath, err)
	}
	fmt.Printf("Enter your OpenAI Session key (optional): ")
	sess, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	sess = strings.TrimSpace(sess)
	if len(sess) != 0 {
		err = os.WriteFile(sessPath, []byte(sess), 0600)
		if err != nil {
			return fmt.Errorf("Could not write OpenAI Session file %v: %w", keyPath, err)
		}
		gptCliCtx.haveSess = true
	} else {
		gptCliCtx.haveSess = false
	}
	threadsPath := path.Join(configDir, ThreadsDir)
	err = os.MkdirAll(threadsPath, 0700)
	if err != nil {
		return fmt.Errorf("Could not create threads directory %v: %w",
			threadsPath, err)
	}
	archivePath := path.Join(configDir, ArchiveDir)
	err = os.MkdirAll(archivePath, 0700)
	if err != nil {
		return fmt.Errorf("Could not create archive directory %v: %w",
			archivePath, err)
	}

	gptCliCtx.client = openai.NewClient(key)
	if gptCliCtx.haveSess {
		gptCliCtx.sessClient = openai.NewClient(sess)
	}
	gptCliCtx.needConfig = false

	fmt.Printf("Summarize dialogue when continuing threads? (reduces costs for less precise replies from OpenAI) [N]: ")
	shouldSummarize, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}

	shouldSummarize = strings.ToUpper(strings.TrimSpace(shouldSummarize))
	if len(shouldSummarize) == 0 {
		shouldSummarize = "N"
	}
	gptCliCtx.prefs.SummarizePrior = (shouldSummarize[0] == 'Y')
	gptCliCtx.curSummaryToggle = gptCliCtx.prefs.SummarizePrior

	return gptCliCtx.savePrefs()
}

func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("Could not find user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", CommandName), nil
}

func getKeyPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, KeyFile), nil
}

func getPrefsPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, PrefsFile), nil
}

func getSessPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SessionFile), nil
}

func getThreadsDir() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, ThreadsDir), nil
}

func getArchiveDir() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, ArchiveDir), nil
}

func loadKey() (string, error) {
	keyPath, err := getKeyPath()
	if err != nil {
		return "", fmt.Errorf("Could not load OpenAI API key: %w", err)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("Could not load OpenAI API key: "+
				"run `%v config` to configure", CommandName)
		}
		return "", fmt.Errorf("Could not load OpenAI API key: %w", err)
	}
	return string(data), nil
}

func loadSess() (string, error) {
	sessPath, err := getSessPath()
	if err != nil {
		return "", fmt.Errorf("Could not load OpenAI Session: %w", err)
	}
	data, err := os.ReadFile(sessPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("Could not load OpenAI Session: "+
				"run `%v config` to configure", CommandName)
		}
		return "", fmt.Errorf("Could not load OpenAI Session: %w", err)
	}
	return string(data), nil
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
	dialogue []openai.ChatCompletionMessage) ([]openai.ChatCompletionMessage,
	error) {

	summaryDialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: SystemMsg},
	}

	msg := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: SummarizeMsg,
	}
	dialogue = append(dialogue, msg)

	fmt.Printf("gptcli: summarizing...\n")
	resp, err := gptCliCtx.client.CreateChatCompletion(ctx,
		openai.ChatCompletionRequest{
			Model:    openai.GPT4oMini,
			Messages: dialogue,
		},
	)
	if err != nil {
		return summaryDialogue, err
	}
	if len(resp.Choices) != 1 {
		return summaryDialogue, fmt.Errorf("gptcli: BUG: Expected 1 response, got %v",
			len(resp.Choices))
	}

	msg = openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: resp.Choices[0].Message.Content,
	}
	summaryDialogue = append(summaryDialogue, msg)

	return summaryDialogue, nil
}

func interactiveThreadWork(ctx context.Context,
	gptCliCtx *GptCliContext, prompt string) error {

	msg := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	}

	thrGrp := gptCliCtx.curThreadGroup
	if thrGrp == gptCliCtx.archiveThreadGroup {
		return fmt.Errorf("Cannot edit archived thread; use unarchive first")
	}
	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	dialogue := thread.Dialogue
	summaryDialogue := dialogue

	dialogue = append(dialogue, msg)
	dialogue2Send := dialogue

	var err error
	if gptCliCtx.curSummaryToggle && len(dialogue) > 2 {
		if len(thread.SummaryDialogue) > 0 {
			summaryDialogue = thread.SummaryDialogue
		}
		summaryDialogue, err = summarizeDialogue(ctx, gptCliCtx, summaryDialogue)
		if err != nil {
			return err
		}
		summaryDialogue = append(summaryDialogue, msg)
		dialogue2Send = summaryDialogue
	}

	fmt.Printf("gptcli: processing...\n")

	resp, err := gptCliCtx.client.CreateChatCompletion(ctx,
		openai.ChatCompletionRequest{
			Model:    openai.GPT4o,
			Messages: dialogue2Send,
		},
	)
	if err != nil {
		return err
	}

	if len(resp.Choices) != 1 {
		return fmt.Errorf("gptcli: BUG: Expected 1 response, got %v",
			len(resp.Choices))
	}
	blocks := splitBlocks(resp.Choices[0].Message.Content)
	for idx, b := range blocks {
		if idx%2 == 0 {
			color.Cyan("%v", b)
		} else {
			color.Green("%v", b)
		}
	}

	msg = openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: resp.Choices[0].Message.Content,
	}
	thread.Dialogue = append(dialogue, msg)
	thread.ModTime = time.Now()
	thread.AccessTime = time.Now()
	if gptCliCtx.curSummaryToggle {
		thread.SummaryDialogue = append(summaryDialogue, msg)
	}

	err = thread.save(thrGrp.dir)
	if err != nil {
		return err
	}

	return nil
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
	gptCliCtx := NewGptCliContext()

	if !gptCliCtx.needConfig {
		checkAndUpgradeConfig()
	}

	err := gptCliCtx.load()
	if err != nil {
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
