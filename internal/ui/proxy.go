/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"fmt"
	"sync"

	"github.com/mikeb26/gptcli/internal/types"
)

// ProxyUI is a types.GptCliUI implementation that forwards UI dialogue
// requests (option selection, bool selection, and free-form input) over a
// channel so they can be executed by a single, UI-owning goroutine.
//
// This is primarily intended for ncurses usage where all ncurses calls must be
// confined to one goroutine, while other goroutines (e.g. background LLM
// workers) still need to request user interaction.
//
// Usage:
//  1) Create a proxy: p := ui.NewProxyUI(0)
//  2) Pass p where a types.GptCliUI is required (e.g. tool approval stack).
//  3) In the UI goroutine, read from p.Requests and execute them against the
//     real UI (e.g. *ui.NcursesUI) using ServeProxyRequest.
//
// Note: ProxyUI only forwards requests; policies such as "only render modals for
// the currently selected thread" should be implemented by the goroutine reading
// from Requests (e.g., by queueing requests until the thread is selected).
//
type ProxyUI struct {
	// Requests is the channel on which UI requests are delivered.
	Requests chan ProxyRequest

	mu     sync.RWMutex
	closed bool
}

func NewProxyUI(buffer int) *ProxyUI {
	if buffer < 0 {
		buffer = 0
	}
	return &ProxyUI{Requests: make(chan ProxyRequest, buffer)}
}

// Close marks the proxy closed. After Close, any new UI request returns an
// error.
//
// Close does NOT close the Requests channel because senders might still attempt
// to write to it; the UI goroutine should stop servicing requests via its own
// lifecycle management.
func (p *ProxyUI) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
}

func (p *ProxyUI) isClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

func (p *ProxyUI) SelectOption(userPrompt string, choices []types.GptCliUIOption) (types.GptCliUIOption, error) {
	if p.isClosed() {
		return types.GptCliUIOption{}, fmt.Errorf("ui proxy closed")
	}

	replyCh := make(chan ProxyResponse, 1)
	p.Requests <- ProxyRequest{
		Kind:       ProxyRequestSelectOption,
		UserPrompt: userPrompt,
		Choices:    choices,
		ReplyCh:    replyCh,
	}

	resp := <-replyCh
	return resp.Option, resp.Err
}

func (p *ProxyUI) SelectBool(userPrompt string, trueOption, falseOption types.GptCliUIOption, defaultOpt *bool) (bool, error) {
	if p.isClosed() {
		return false, fmt.Errorf("ui proxy closed")
	}

	replyCh := make(chan ProxyResponse, 1)
	p.Requests <- ProxyRequest{
		Kind:        ProxyRequestSelectBool,
		UserPrompt:  userPrompt,
		TrueOption:  trueOption,
		FalseOption: falseOption,
		DefaultOpt:  defaultOpt,
		ReplyCh:     replyCh,
	}

	resp := <-replyCh
	return resp.Bool, resp.Err
}

func (p *ProxyUI) Get(userPrompt string) (string, error) {
	if p.isClosed() {
		return "", fmt.Errorf("ui proxy closed")
	}

	replyCh := make(chan ProxyResponse, 1)
	p.Requests <- ProxyRequest{
		Kind:       ProxyRequestGet,
		UserPrompt: userPrompt,
		ReplyCh:    replyCh,
	}

	resp := <-replyCh
	return resp.Text, resp.Err
}

type ProxyRequestKind int

const (
	ProxyRequestSelectOption ProxyRequestKind = iota
	ProxyRequestSelectBool
	ProxyRequestGet
)

// ProxyRequest is a marshaled UI interaction to be executed by the UI goroutine.
//
// ThreadID is optional metadata for consumers that want to route/queue requests
// by thread.
//
type ProxyRequest struct {
	Kind       ProxyRequestKind
	ThreadID   string
	UserPrompt string

	Choices []types.GptCliUIOption

	TrueOption  types.GptCliUIOption
	FalseOption types.GptCliUIOption
	DefaultOpt  *bool

	ReplyCh chan ProxyResponse
}

type ProxyResponse struct {
	Option types.GptCliUIOption
	Bool   bool
	Text   string
	Err    error
}

// ServeProxyRequest executes req against the provided real UI and sends the
// result back on req.ReplyCh.
//
// This helper is designed to be called from the goroutine that owns UI calls
// (e.g. the main ncurses goroutine).
func ServeProxyRequest(realUI types.GptCliUI, req ProxyRequest) {
	if req.ReplyCh == nil {
		return
	}

	var resp ProxyResponse
	switch req.Kind {
	case ProxyRequestSelectOption:
		resp.Option, resp.Err = realUI.SelectOption(req.UserPrompt, req.Choices)
	case ProxyRequestSelectBool:
		resp.Bool, resp.Err = realUI.SelectBool(req.UserPrompt, req.TrueOption, req.FalseOption, req.DefaultOpt)
	case ProxyRequestGet:
		resp.Text, resp.Err = realUI.Get(req.UserPrompt)
	default:
		resp.Err = fmt.Errorf("unknown proxy request kind: %v", req.Kind)
	}

	// Non-blocking if caller provided a buffered channel (recommended).
	req.ReplyCh <- resp
}
