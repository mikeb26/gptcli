package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
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
	gptCliCtx := GptCliContext{
		client:       mockOpenAIClient,
		totThreads:   1,
		needConfig:   false,
		curThreadNum: 1,
		threads: []*GptCliThread{
			{
				Dialogue:   expectedDialogue,
				filePath:   tmpFile.Name(),
				ModTime:    now,
				AccessTime: now,
			},
		},
	}

	time.Sleep(100 * time.Millisecond)

	err = interactiveThreadWork(context.Background(), &gptCliCtx, "test prompt")

	assert.Nil(t, err)
	assert.Equal(t, 3, len(gptCliCtx.threads[0].Dialogue))
	assert.Equal(t, "test prompt", gptCliCtx.threads[0].Dialogue[1].Content)
	assert.Equal(t, "test response", gptCliCtx.threads[0].Dialogue[2].Content)
	assert.Less(t, now, gptCliCtx.threads[0].AccessTime)
	assert.Less(t, now, gptCliCtx.threads[0].ModTime)

	threadFileText, err := ioutil.ReadFile(tmpFile.Name())
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go func() {
				if tt.wantErr {
					pw.Close()
				} else {
					pw.Write([]byte(tt.input))
				}
			}()

			gptCliCtx := GptCliContext{
				input:        reader,
				curThreadNum: tt.curThread,
				threads: []*GptCliThread{
					{Name: "Thread1"},
				},
			}

			gotPrompt, err := getCmdOrPrompt(&gptCliCtx)

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
