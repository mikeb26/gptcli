package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	//"github.com/mikeb26/gptcli/internal"

	"github.com/golang/mock/gomock"
	"github.com/mikeb26/gptcli/internal"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestSplitBlocks(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		blocks []string
	}{
		{
			name:   "empty string",
			text:   "",
			blocks: []string{},
		},
		{
			name:   "no code blocks",
			text:   "This is a test.",
			blocks: []string{"This is a test."},
		},
		{
			name:   "single code block",
			text:   "```\ncode block\n```",
			blocks: []string{"", "```\ncode block\n"},
		},
		{
			name:   "text with code blocks",
			text:   "Some text ```\ncode block\n``` follow-up",
			blocks: []string{"Some text ", "```\ncode block\n```", " follow-up"},
		},
		{
			name:   "multiple code blocks",
			text:   "```\nfirst\n``` interlude ```\nsecond\n``` end",
			blocks: []string{"", "```\nfirst\n```", " interlude ", "```\nsecond\n```", " end"},
		},
		{
			name:   "multiline code block",
			text:   "```\nline1\nline2\nline3\n```",
			blocks: []string{"", "```\nline1\nline2\nline3\n"},
		},
		{
			name:   "code block at start and end",
			text:   "```\nstart\n``` text in between ```\nend\n```",
			blocks: []string{"", "```\nstart\n```", " text in between ", "```\nend\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitBlocks(tt.text)
			assert.Equal(t, tt.blocks, result)
		})
	}
}

func TestInteractiveThreadWork(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOpenAIClient := internal.NewMockOpenAIClient(ctrl)

	expectedDialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "test prompt"},
	}

	expectedResponse := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: "test response",
				},
			},
		},
	}

	mockOpenAIClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)

	tmpFile, err := os.CreateTemp("", "gptcli.testInteractiveThread.*")
	assert.Nil(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	now := time.Now()
	gptCliCtx := NewGptCliContext()
	gptCliCtx.client = mockOpenAIClient
	gptCliCtx.curThreadGroup.dir = filepath.Dir(tmpFile.Name())
	thread := &GptCliThread{
		Name:            "test",
		CreateTime:      now,
		AccessTime:      now,
		ModTime:         now,
		Dialogue:        expectedDialogue,
		SummaryDialogue: []openai.ChatCompletionMessage{},
		fileName:        filepath.Base(tmpFile.Name()),
	}
	gptCliCtx.curThreadGroup.curThreadNum = gptCliCtx.curThreadGroup.addThread(thread)

	time.Sleep(100 * time.Millisecond)

	err = interactiveThreadWork(context.Background(), gptCliCtx, "test prompt")

	assert.Nil(t, err)
	assert.Equal(t, 3, len(thread.Dialogue))
	assert.Equal(t, "test prompt", thread.Dialogue[1].Content)
	assert.Equal(t, "test response", thread.Dialogue[2].Content)
	assert.Less(t, now, thread.AccessTime)
	assert.Less(t, now, thread.ModTime)

	threadFileText, err := os.ReadFile(tmpFile.Name())
	assert.Nil(t, err)
	var threadFromFile GptCliThread
	err = json.Unmarshal(threadFileText, &threadFromFile)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(threadFromFile.Dialogue))
	assert.Equal(t, "test prompt", threadFromFile.Dialogue[1].Content)
	assert.Equal(t, "test response", threadFromFile.Dialogue[2].Content)
	assert.Less(t, now, threadFromFile.AccessTime)
	assert.Less(t, now, threadFromFile.ModTime)
}

func TestGetCmdOrPrompt(t *testing.T) {
	pr, pw := io.Pipe()

	reader := bufio.NewReader(pr)

	tests := []struct {
		name       string
		input      string
		wantPrompt string
		curThread  int
		wantErr    bool
	}{
		{
			name:       "simple command",
			input:      "help\n",
			wantPrompt: "help",
			curThread:  0,
			wantErr:    false,
		},
		{
			name:       "threaded command",
			input:      "some single line prompt to openai\n",
			wantPrompt: "some single line prompt to openai",
			curThread:  1,
			wantErr:    false,
		},
		{
			name:       "multi-line code block input",
			input:      "some multi line prompt to openai```\ncodeblock\n```\n",
			wantPrompt: "some multi line prompt to openai```\ncodeblock\n```\n",
			curThread:  1,
			wantErr:    false,
		},
		{
			name:       "error on input",
			input:      "",
			wantPrompt: "",
			curThread:  0,
			wantErr:    true,
		},
	}

	var err error
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go func() {
				if tt.wantErr {
					pw.Close()
				} else {
					_, err = pw.Write([]byte(tt.input))
					assert.NoError(t, err)
				}
			}()

			gptCliCtx := NewGptCliContext()
			gptCliCtx.input = reader
			gptCliCtx.curThreadGroup.curThreadNum = tt.curThread
			gptCliCtx.curThreadGroup.threads =
				[]*GptCliThread{{Name: "Thread1"}}

			gotPrompt, err := getCmdOrPrompt(gptCliCtx)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPrompt, gotPrompt)
			}

			pr, pw = io.Pipe()
			reader.Reset(pr)
		})
	}
}

func TestThreadSwitchMain(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOpenAIClient := internal.NewMockOpenAIClient(ctrl)

	tmpFile, err := os.CreateTemp("", "gptcli.testThreadSwitch.*")
	assert.Nil(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	now := time.Now()
	gptCliCtx := NewGptCliContext()
	gptCliCtx.client = mockOpenAIClient
	gptCliCtx.curThreadGroup.dir = filepath.Dir(tmpFile.Name())
	thread := &GptCliThread{
		Name:            "test",
		CreateTime:      now,
		AccessTime:      now,
		ModTime:         now,
		Dialogue:        nil,
		SummaryDialogue: []openai.ChatCompletionMessage{},
		fileName:        filepath.Base(tmpFile.Name()),
	}
	gptCliCtx.curThreadGroup.curThreadNum = gptCliCtx.curThreadGroup.addThread(thread)

	tests := []struct {
		name      string
		args      []string
		wantErr   bool
		errMsg    string
		newThread int
	}{
		{
			name:      "successful thread switch",
			args:      []string{"thread", "1"},
			wantErr:   false,
			errMsg:    "",
			newThread: 1,
		},
		{
			name:      "non-existent thread switch",
			args:      []string{"thread", "2"},
			wantErr:   true,
			errMsg:    "Thread 2 does not exist. To list threads try 'ls'.\n",
			newThread: 1, // No change expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := threadSwitchMain(context.Background(), gptCliCtx, tt.args)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}

			// Verify the current thread number has been set correctly
			assert.Equal(t, tt.newThread, gptCliCtx.curThreadGroup.curThreadNum)
		})
	}
}

func TestSummarizeDialogue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := internal.NewMockOpenAIClient(ctrl)
	gptCliCtx := GptCliContext{
		client: mockClient,
	}

	initialDialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "Hello!"},
		{Role: openai.ChatMessageRoleAssistant, Content: "Hi! How can I assist you today?"},
	}

	expectedSummaryContent := "User greeted and asked for assistance."
	expectedSummaryMessage := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: expectedSummaryContent,
	}

	expectedModel := openai.GPT4oMini
	expectedRequest := openai.ChatCompletionRequest{
		Model: expectedModel,
		Messages: append(initialDialogue, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: SummarizeMsg,
		}),
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Eq(expectedRequest)).
		Return(openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{
				Message: expectedSummaryMessage,
			}},
		}, nil).Times(1)

	summaryDialogue, err := summarizeDialogue(context.Background(), &gptCliCtx, initialDialogue)

	assert.NoError(t, err)
	assert.Len(t, summaryDialogue, 2)
	assert.Equal(t, expectedSummaryContent, summaryDialogue[1].Content)
}

func TestArchiveThreadMain(t *testing.T) {
	threadsDirLocal, err := os.MkdirTemp("", "gptcli_test_*")
	assert.Nil(t, err)
	defer os.RemoveAll(threadsDirLocal)

	archiveDirLocal, err := os.MkdirTemp("", "gptcli_atest_*")
	assert.Nil(t, err)
	defer os.RemoveAll(archiveDirLocal)

	threadNum := 1
	threadName := "TestThread"
	threadFilePath := threadsDirLocal + "/" + strconv.Itoa(threadNum) + ".json"
	archiveFilePath := archiveDirLocal + "/" + strconv.Itoa(threadNum) + ".json"
	err = os.WriteFile(threadFilePath, []byte("{}"), 0644)
	assert.Nil(t, err)

	now := time.Now()
	gptCliCtx := NewGptCliContext()
	gptCliCtx.curThreadGroup.dir = threadsDirLocal
	gptCliCtx.archiveThreadGroup.dir = archiveDirLocal
	thread := &GptCliThread{
		Name:            threadName,
		CreateTime:      now,
		AccessTime:      now,
		ModTime:         now,
		Dialogue:        []openai.ChatCompletionMessage{},
		SummaryDialogue: []openai.ChatCompletionMessage{},
		fileName:        strconv.Itoa(threadNum) + ".json",
	}
	gptCliCtx.curThreadGroup.curThreadNum = gptCliCtx.curThreadGroup.addThread(thread)

	args := []string{"archive", strconv.Itoa(threadNum)}
	err = archiveThreadMain(context.Background(), gptCliCtx, args)

	assert.Nil(t, err)
	assert.Equal(t, 0, gptCliCtx.curThreadGroup.totThreads)
	_, err = os.Stat(threadFilePath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(archiveFilePath)
	assert.Nil(t, err)
}
