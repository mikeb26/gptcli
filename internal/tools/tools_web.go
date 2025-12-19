/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

type RetrieveUrlTool struct {
	approvalUI ToolApprovalUI
}

type RetrieveUrlRequestHeader struct {
	Key   string `json:"key" jsonschema:"description=The HTTP header key"`
	Value string `json:"value" jsonschema:"description=The HTTP header value"`
}

type RetrieveUrlReq struct {
	Url     string                     `json:"url" jsonschema:"description=The URL to send the request to"`
	Headers []RetrieveUrlRequestHeader `json:"headers" jsonschema:"description=The HTTP headers to include with the request (optional)"`
	Method  string                     `json:"method" jsonschema:"description=HTTP request method (e.g., GET, POST, etc.); defaults to GET if not set (optional)"`
	Body    string                     `json:"body" jsonschema:"description=HTTP request body (optional)"`
}

type RetrieveUrlResp struct {
	Error         string      `json:"error" jsonschema:"description=The error status of the retrieve_url call"`
	Status        string      `json:"status" jsonschema:"description=The HTTP status of the response to the request"`
	StatusCode    int         `json:"statuscode" jsonschema:"description=The integer HTTP status code of the response to the request"`
	Header        http.Header `json:"header" jsonschema:"description=The header returned by the response to the request"`
	Body          string      `json:"body" jsonschema:"description=The body returned by the response to the request"`
	ContentLength int64       `json:"contentlen" jsonschema:"description=The length of the content returned by the response to the request"`
}

func (t RetrieveUrlTool) GetOp() types.ToolCallOp {
	return types.RetrieveUrl
}

func (t RetrieveUrlTool) RequiresUserApproval() bool {
	return true
}

// BuildApprovalRequest implements ToolWithCustomApproval for
// RetrieveUrlTool so that read/write permissions can be cached on a
// per-URL (file-equivalent) and per-domain (directory-equivalent)
// basis. HTTP GET, HEAD, and OPTIONS are treated as reads, while all
// other methods are treated as writes (which also imply read).
func (t RetrieveUrlTool) BuildApprovalRequest(arg any) ToolApprovalRequest {
	req, ok := arg.(*RetrieveUrlReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	return buildWebApprovalRequest(t, arg, req.Url, req.Method)
}
func NewRetrieveUrlTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &RetrieveUrlTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t RetrieveUrlTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "Retrieve the raw content of a url without any additional processing",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t RetrieveUrlTool) Invoke(ctx context.Context,
	req *RetrieveUrlReq) (*RetrieveUrlResp, error) {

	ret := &RetrieveUrlResp{}

	err := GetUserApproval(ctx, t.approvalUI, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	requestMethod := "GET"
	if req.Method != "" {
		requestMethod = strings.ToUpper(req.Method)
	}

	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	httpReq, err := http.NewRequest(requestMethod, req.Url, bodyReader)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	for _, reqHdr := range req.Headers {
		httpReq.Header.Add(strings.TrimSpace(reqHdr.Key),
			strings.TrimSpace(reqHdr.Value))
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		err = fmt.Errorf("Failed to retrieve %v: %w", req.Url, err)
		ret.Error = err.Error()
		return ret, nil
	}

	defer httpResp.Body.Close()
	content, err := io.ReadAll(httpResp.Body)
	if err != nil {
		err = fmt.Errorf("Failed to read response body: %w", err)
		ret.Error = err.Error()
		return ret, nil
	}

	ret.Status = httpResp.Status
	ret.StatusCode = httpResp.StatusCode
	ret.Header = httpResp.Header
	ret.Body = string(content)
	ret.ContentLength = httpResp.ContentLength

	return ret, nil
}
