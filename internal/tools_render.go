/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

// RenderWebTool implements a tool for rendering web pages with JavaScript execution.
// It uses chromedp to drive a headless Chrome instance. The tool renders the page with a
// viewport of 1920x1080 and a timeout of 30 seconds, returning the final HTML source.
// Future extensions can add screenshot capture or other outputs without breaking the API.

type RenderWebTool struct {
	input *bufio.Reader
}

// RenderWebReq defines the input for the render web tool.
// For now, it only requires a URL, but it is designed for future extensibility.
// If needed, additional fields such as viewport or timeout can be added later.
//
// jsonschema description: The URL of the web page to render.
type RenderWebReq struct {
	Url string `json:"url" jsonschema:"description=The URL of the web page to render"`
}

// RenderWebResp defines the response from the render web tool. It returns
// the fully rendered HTML after allowing JavaScript to execute.
//
// jsonschema description: The rendered HTML content.
type RenderWebResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the render_web call"`
	Html  string `json:"html" jsonschema:"description=The rendered HTML after JavaScript execution"`
}

// GetOp returns the operation name for this tool.
func (t RenderWebTool) GetOp() types.ToolCallOp {
	return types.RenderUrl
}

// RequiresUserApproval indicates whether the tool action requires explicit user approval.
func (t RenderWebTool) RequiresUserApproval() bool {
	return true
}

// NewRenderWebTool initializes a new instance of the RenderWebTool.
func NewRenderWebTool(inputIn *bufio.Reader) types.GptCliTool {
	t := &RenderWebTool{
		input: inputIn,
	}
	return t.Define()
}

// Define registers the tool with gptcli using utilities in the utils package.
func (t RenderWebTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "Retrieve the content of a url and locally render it with JavaScript execution", t.Invoke)
	if err != nil {
		panic(err)
	}
	return ret
}

// Invoke executes the render web tool. It uses chromedp to launch a headless chrome instance,
// navigates to the provided URL, waits for the page to render, and then extracts the HTML source.
// A default viewport of 1920x1080 and a timeout of 30 seconds are used. Future modifications can add
// extra functionality (e.g., screenshot capture) without breaking existing behavior.
func (t RenderWebTool) Invoke(ctx context.Context, req *RenderWebReq) (*RenderWebResp, error) {
	resp := &RenderWebResp{}

	// Require user approval before proceeding
	err := getUserApproval(t.input, t, req)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	// Create a context with a timeout of 30 seconds
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Create chromedp context
	chromeCtx, cancelChrome := chromedp.NewContext(ctx)
	defer cancelChrome()

	var pageText string

	err = chromedp.Run(chromeCtx,
		chromedp.EmulateViewport(1920, 1080),
		chromedp.Navigate(req.Url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		// Capture the visible text content of the page.
		chromedp.Evaluate(`document.body.innerText`, &pageText))
	if err != nil {
		resp.Error = fmt.Sprintf("failed to render page: %v", err)
		return resp, nil
	}

	resp.Html = pageText
	return resp, nil
}
