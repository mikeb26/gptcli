/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal"
)

type RetrieveUrlTool struct {
	input *bufio.Reader
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

func (t RetrieveUrlTool) GetOp() ToolCallOp {
	return RetrieveUrl
}

func (t RetrieveUrlTool) RequiresUserApproval() bool {
	return false
}

func NewRetrieveUrlTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &RetrieveUrlTool{
		input: inputIn,
	}

	return t.Define()
}

func (t RetrieveUrlTool) Define() internal.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "retrieve the content of a url",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t RetrieveUrlTool) Invoke(ctx context.Context,
	req *RetrieveUrlReq) (*RetrieveUrlResp, error) {

	ret := &RetrieveUrlResp{}

	err := getUserApproval(t.input, t, req)
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
