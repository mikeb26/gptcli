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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

type RetrieveUrlTool struct {
	approver am.Approver
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
	// RespBodyFilename, when set, causes the retrieved (or rendered) result to be
	// written to a local file instead of returned directly in the response body.
	// This can be useful to avoid overloading the LLM context window.
	RespBodyFilename string `json:"resp_body_filename,omitempty" jsonschema:"description=Filename to write the result to instead of returning it directly (optional)"`
}

type RetrieveUrlResp struct {
	Error         string      `json:"error" jsonschema:"description=The error status of the retrieve_url call"`
	Status        string      `json:"status" jsonschema:"description=The HTTP status of the response to the request"`
	StatusCode    int         `json:"statuscode" jsonschema:"description=The integer HTTP status code of the response to the request"`
	Header        http.Header `json:"header" jsonschema:"description=The header returned by the response to the request"`
	Body          string      `json:"body,omitempty" jsonschema:"description=The body returned by the response to the request; empty if body_filename present and non-empty"`
	ContentLength int64       `json:"contentlen" jsonschema:"description=The length of the content returned by the response to the request"`
	// Mode indicates whether Body (or the written file content) contains the raw
	// HTTP response body, or the JavaScript-rendered page text.
	Mode string `json:"mode" jsonschema:"description=Indicates whether the result is raw HTTP body ('raw') or JavaScript-rendered page text ('rendered')"`
	// BodyLen is the length in bytes of the result that was returned (in
	// Body) or written to OutputFilename.
	BodyLen int `json:"body_len" jsonschema:"description=The byte length of the	returned or written result; may differ from contentlen when mode==rendered or returned content length header is incorrect"`
	// BodyFilename echoes the requested output filename when present
	BodyFilename string `json:"body_filename,omitempty"	jsonschema:"description=The output filename that the result body was written to (if any)"`
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
func (t RetrieveUrlTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*RetrieveUrlReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	// Writing to local files is intentionally treated as a separate, explicit
	// user-approved operation. We do not allow this to be auto-approved via cached
	// web policies.
	if strings.TrimSpace(req.RespBodyFilename) != "" {
		return buildWebApprovalRequestWithFileOutput(t, arg, req.Url, req.Method, req.RespBodyFilename)
	}

	return buildWebApprovalRequest(t, arg, req.Url, req.Method)
}

func buildWebApprovalRequestWithFileOutput(t types.Tool, arg any, rawURL, method, outputFilename string) am.ApprovalRequest {
	// We intentionally do not include RequiredActions or any policy-backed
	// choices here, so that callers are always explicitly prompted when writing
	// to the local filesystem.

	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		m = "GET"
	}

	promptBuilder := &strings.Builder{}
	promptBuilder.WriteString(fmt.Sprintf("gptcli would like to %v(%v): %v\n", t.GetOp(), m, rawURL))
	promptBuilder.WriteString(fmt.Sprintf("It will also write the result to: %v\n", outputFilename))
	promptBuilder.WriteString("Allow?")

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		},
		{
			Key:   "n",
			Label: "No",
			Scope: am.ApprovalScopeDeny,
		},
	}

	return am.ApprovalRequest{
		Prompt:  promptBuilder.String(),
		Choices: choices,
	}
}
func NewRetrieveUrlTool(approver am.Approver) types.LlmTool {
	t := &RetrieveUrlTool{
		approver: approver,
	}

	return t.Define()
}

func (t RetrieveUrlTool) Define() types.LlmTool {
	ret, err := utils.InferTool(string(t.GetOp()), "Retrieve the raw content of a url without any additional processing",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t RetrieveUrlTool) Invoke(ctx context.Context,
	req *RetrieveUrlReq) (*RetrieveUrlResp, error) {

	ret := &RetrieveUrlResp{Mode: "raw"}

	err := GetUserApproval(ctx, t.approver, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	requestMethod := normalizeHTTPRequestMethod(req.Method)

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
	ret.BodyLen = len(ret.Body)

	if requestMethod == "GET" && shouldAutoRenderRetrievedBody(httpResp.Header, ret.Body) {
		renderedText, rerr := renderVisibleText(ctx, req.Url)
		if rerr != nil {
			// Best-effort: if rendering fails, fall back to raw body.
			ret.Error = fmt.Sprintf("auto-render failed: %v", rerr)
		} else if strings.TrimSpace(renderedText) != "" {
			ret.Mode = "rendered"
			ret.Body = renderedText
			ret.BodyLen = len(ret.Body)
		}
	}

	if strings.TrimSpace(req.RespBodyFilename) != "" {
		werr := writeTextFile(req.RespBodyFilename, ret.Body)
		if werr != nil {
			ret.Error = werr.Error()
			return ret, nil
		}
		ret.BodyFilename = req.RespBodyFilename
		// Avoid sending potentially large content back in the tool result.
		ret.Body = ""
	}

	return ret, nil
}

func normalizeHTTPRequestMethod(method string) string {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		return "GET"
	}
	return m
}

func shouldAutoRenderRetrievedBody(header http.Header, body string) bool {
	if body == "" {
		return false
	}

	ct := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	// If the server is literally returning JavaScript, it is almost never useful
	// to dump it into the LLM context window.
	if strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript") {
		return true
	}

	// Heuristic: large HTML pages with lots of scripts are likely SPA shells.
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") || strings.Contains(body, "<html") {
		if len(body) > 200_000 && strings.Count(strings.ToLower(body), "<script") >= 5 {
			return true
		}
	}

	// Heuristic: very large payloads that look minified/JS-heavy.
	if len(body) > 500_000 {
		return true
	}

	return false
}

func renderVisibleText(ctx context.Context, pageURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	chromeCtx, cancelChrome := chromedp.NewContext(ctx)
	defer cancelChrome()

	var pageText string
	err := chromedp.Run(chromeCtx,
		chromedp.EmulateViewport(1920, 1080),
		chromedp.Navigate(pageURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Evaluate(`document.body.innerText`, &pageText),
	)
	if err != nil {
		return "", fmt.Errorf("failed to render page: %w", err)
	}

	return pageText, nil
}

func writeTextFile(filename, content string) error {
	path := filepath.Clean(filename)
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
