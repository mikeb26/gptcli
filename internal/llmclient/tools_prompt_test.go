/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/golang/mock/gomock"
	"github.com/mikeb26/gptcli/internal/am"
	uipkg "github.com/mikeb26/gptcli/internal/ui"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

type fakeUI struct{}

func (f fakeUI) SelectOption(_ string, choices []types.GptCliUIOption) (types.GptCliUIOption, error) {
	// default to "yes"
	for _, ch := range choices {
		if ch.Key == "y" {
			return ch, nil
		}
	}
	return choices[0], nil
}

func (f fakeUI) SelectBool(_ string, trueOption, _ types.GptCliUIOption, _ *bool) (bool, error) {
	return trueOption.Key == "y", nil
}

func (f fakeUI) Get(_ string) (string, error) {
	return "", nil
}

func TestPromptRunReq_String(t *testing.T) {
	req := &PromptRunReq{Dialogue: []*types.GptCliMessage{{Role: schema.User, Content: "hi"}}}
	out := req.String()
	assert.Contains(t, out, "Role:user")
	assert.Contains(t, out, "Msg:hi")
}

func TestPromptRunTool_Define_OpName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := types.NewMockGptCliAIClient(ctrl)
	approver := am.NewPolicyStoreApprover(
		uipkg.NewUIApprover(fakeUI{}),
		am.NewMemoryApprovalPolicyStore(),
	)

	prt := PromptRunTool{client: mockClient, approver: approver, depth: 0}
	defined := prt.Define()

	info, err := defined.Info(context.Background())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, string(types.PromptRun), info.Name)
}

func TestPromptRunTool_Invoke_CallsClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := types.NewMockGptCliAIClient(ctrl)
	approver := am.NewPolicyStoreApprover(
		uipkg.NewUIApprover(fakeUI{}),
		am.NewMemoryApprovalPolicyStore(),
	)

	ctx := context.Background()

	req := &PromptRunReq{Dialogue: []*types.GptCliMessage{{Role: schema.User, Content: "hi"}}}
	respMsg := &types.GptCliMessage{Role: schema.Assistant, Content: "ok"}

	mockClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(respMsg, nil)

	prt := PromptRunTool{client: mockClient, approver: approver, depth: 1}
	resp, err := prt.Invoke(ctx, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Empty(t, resp.Error)
	assert.Equal(t, respMsg.Content, resp.Message.Content)
}
