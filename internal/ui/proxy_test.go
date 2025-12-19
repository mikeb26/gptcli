/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
)

type mockUI struct {
	selectOptionFn func(userPrompt string, choices []types.GptCliUIOption) (types.GptCliUIOption, error)
	selectBoolFn   func(userPrompt string, trueOption, falseOption types.GptCliUIOption, defaultOpt *bool) (bool, error)
	getFn          func(userPrompt string) (string, error)
}

func (m *mockUI) SelectOption(userPrompt string, choices []types.GptCliUIOption) (types.GptCliUIOption, error) {
	if m.selectOptionFn == nil {
		return types.GptCliUIOption{}, errors.New("SelectOption not implemented")
	}
	return m.selectOptionFn(userPrompt, choices)
}

func (m *mockUI) SelectBool(userPrompt string, trueOption, falseOption types.GptCliUIOption, defaultOpt *bool) (bool, error) {
	if m.selectBoolFn == nil {
		return false, errors.New("SelectBool not implemented")
	}
	return m.selectBoolFn(userPrompt, trueOption, falseOption, defaultOpt)
}

func (m *mockUI) Get(userPrompt string) (string, error) {
	if m.getFn == nil {
		return "", errors.New("Get not implemented")
	}
	return m.getFn(userPrompt)
}

func TestNewProxyUI_NegativeBufferBecomesZero(t *testing.T) {
	p := NewProxyUI(-10)
	if p == nil {
		t.Fatalf("expected non-nil proxy")
	}
	if cap(p.Requests) != 0 {
		t.Fatalf("expected Requests channel cap 0, got %d", cap(p.Requests))
	}
}

func TestProxyUI_ClosePreventsNewRequests(t *testing.T) {
	p := NewProxyUI(1)
	p.Close()

	_, err := p.SelectOption("x", []types.GptCliUIOption{{Key: "a", Label: "A"}})
	if err == nil {
		t.Fatalf("expected error from SelectOption after Close")
	}

	_, err = p.SelectBool("x", types.GptCliUIOption{Key: "t", Label: "True"}, types.GptCliUIOption{Key: "f", Label: "False"}, nil)
	if err == nil {
		t.Fatalf("expected error from SelectBool after Close")
	}

	_, err = p.Get("x")
	if err == nil {
		t.Fatalf("expected error from Get after Close")
	}

	select {
	case req := <-p.Requests:
		t.Fatalf("did not expect any request sent after Close, got: %+v", req)
	case <-time.After(30 * time.Millisecond):
		// ok
	}
}

func TestProxyUI_SelectOption_ForwardsRequestAndReturnsResponse(t *testing.T) {
	p := NewProxyUI(1)
	choices := []types.GptCliUIOption{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		opt, err := p.SelectOption("pick one", choices)
		if err != nil {
			t.Errorf("SelectOption returned error: %v", err)
			return
		}
		if opt.Key != "b" {
			t.Errorf("expected returned option key 'b', got %q", opt.Key)
		}
	}()

	select {
	case req := <-p.Requests:
		if req.Kind != ProxyRequestSelectOption {
			t.Fatalf("expected kind ProxyRequestSelectOption, got %v", req.Kind)
		}
		if req.ThreadID != "" {
			t.Fatalf("expected empty ThreadID, got %q", req.ThreadID)
		}
		if req.UserPrompt != "pick one" {
			t.Fatalf("expected UserPrompt 'pick one', got %q", req.UserPrompt)
		}
		if !reflect.DeepEqual(req.Choices, choices) {
			t.Fatalf("expected Choices %+v, got %+v", choices, req.Choices)
		}
		if req.ReplyCh == nil {
			t.Fatalf("expected non-nil ReplyCh")
		}
		req.ReplyCh <- ProxyResponse{Option: types.GptCliUIOption{Key: "b", Label: "B"}}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for proxy request")
	}

	select {
	case <-done:
		// ok
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for SelectOption to return")
	}
}

func TestProxyUI_SelectBool_ForwardsRequestAndReturnsResponse(t *testing.T) {
	p := NewProxyUI(1)
	trueOpt := types.GptCliUIOption{Key: "y", Label: "Yes"}
	falseOpt := types.GptCliUIOption{Key: "n", Label: "No"}
	def := true

	done := make(chan struct{})
	go func() {
		defer close(done)
		val, err := p.SelectBool("proceed?", trueOpt, falseOpt, &def)
		if err != nil {
			t.Errorf("SelectBool returned error: %v", err)
			return
		}
		if !val {
			t.Errorf("expected true, got false")
		}
	}()

	select {
	case req := <-p.Requests:
		if req.Kind != ProxyRequestSelectBool {
			t.Fatalf("expected kind ProxyRequestSelectBool, got %v", req.Kind)
		}
		if req.UserPrompt != "proceed?" {
			t.Fatalf("expected UserPrompt 'proceed?', got %q", req.UserPrompt)
		}
		if req.TrueOption != trueOpt {
			t.Fatalf("expected TrueOption %+v, got %+v", trueOpt, req.TrueOption)
		}
		if req.FalseOption != falseOpt {
			t.Fatalf("expected FalseOption %+v, got %+v", falseOpt, req.FalseOption)
		}
		if req.DefaultOpt == nil || *req.DefaultOpt != true {
			t.Fatalf("expected DefaultOpt pointer to true, got %#v", req.DefaultOpt)
		}
		if req.ReplyCh == nil {
			t.Fatalf("expected non-nil ReplyCh")
		}
		req.ReplyCh <- ProxyResponse{Bool: true}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for proxy request")
	}

	select {
	case <-done:
		// ok
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for SelectBool to return")
	}
}

func TestProxyUI_Get_ForwardsRequestAndReturnsResponse(t *testing.T) {
	p := NewProxyUI(1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		text, err := p.Get("enter text")
		if err != nil {
			t.Errorf("Get returned error: %v", err)
			return
		}
		if text != "hello" {
			t.Errorf("expected 'hello', got %q", text)
		}
	}()

	select {
	case req := <-p.Requests:
		if req.Kind != ProxyRequestGet {
			t.Fatalf("expected kind ProxyRequestGet, got %v", req.Kind)
		}
		if req.UserPrompt != "enter text" {
			t.Fatalf("expected UserPrompt 'enter text', got %q", req.UserPrompt)
		}
		if req.ReplyCh == nil {
			t.Fatalf("expected non-nil ReplyCh")
		}
		req.ReplyCh <- ProxyResponse{Text: "hello"}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for proxy request")
	}

	select {
	case <-done:
		// ok
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for Get to return")
	}
}

func TestServeProxyRequest_DispatchesToUIAndReplies(t *testing.T) {
	trueOpt := types.GptCliUIOption{Key: "y", Label: "Yes"}
	falseOpt := types.GptCliUIOption{Key: "n", Label: "No"}
	choices := []types.GptCliUIOption{{Key: "a", Label: "A"}}
	def := false

	m := &mockUI{
		selectOptionFn: func(userPrompt string, gotChoices []types.GptCliUIOption) (types.GptCliUIOption, error) {
			if userPrompt != "pick" {
				return types.GptCliUIOption{}, errors.New("wrong prompt")
			}
			if !reflect.DeepEqual(gotChoices, choices) {
				return types.GptCliUIOption{}, errors.New("wrong choices")
			}
			return types.GptCliUIOption{Key: "a", Label: "A"}, nil
		},
		selectBoolFn: func(userPrompt string, gotTrue, gotFalse types.GptCliUIOption, gotDefault *bool) (bool, error) {
			if userPrompt != "bool?" {
				return false, errors.New("wrong prompt")
			}
			if gotTrue != trueOpt || gotFalse != falseOpt {
				return false, errors.New("wrong options")
			}
			if gotDefault == nil || *gotDefault != false {
				return false, errors.New("wrong default")
			}
			return true, nil
		},
		getFn: func(userPrompt string) (string, error) {
			if userPrompt != "get" {
				return "", errors.New("wrong prompt")
			}
			return "ok", nil
		},
	}

	t.Run("SelectOption", func(t *testing.T) {
		replyCh := make(chan ProxyResponse, 1)
		ServeProxyRequest(m, ProxyRequest{Kind: ProxyRequestSelectOption, UserPrompt: "pick", Choices: choices, ReplyCh: replyCh})
		resp := <-replyCh
		if resp.Err != nil {
			t.Fatalf("expected no error, got %v", resp.Err)
		}
		if resp.Option.Key != "a" {
			t.Fatalf("expected option key 'a', got %q", resp.Option.Key)
		}
	})

	t.Run("SelectBool", func(t *testing.T) {
		replyCh := make(chan ProxyResponse, 1)
		ServeProxyRequest(m, ProxyRequest{Kind: ProxyRequestSelectBool, UserPrompt: "bool?", TrueOption: trueOpt, FalseOption: falseOpt, DefaultOpt: &def, ReplyCh: replyCh})
		resp := <-replyCh
		if resp.Err != nil {
			t.Fatalf("expected no error, got %v", resp.Err)
		}
		if !resp.Bool {
			t.Fatalf("expected true, got false")
		}
	})

	t.Run("Get", func(t *testing.T) {
		replyCh := make(chan ProxyResponse, 1)
		ServeProxyRequest(m, ProxyRequest{Kind: ProxyRequestGet, UserPrompt: "get", ReplyCh: replyCh})
		resp := <-replyCh
		if resp.Err != nil {
			t.Fatalf("expected no error, got %v", resp.Err)
		}
		if resp.Text != "ok" {
			t.Fatalf("expected 'ok', got %q", resp.Text)
		}
	})
}

func TestServeProxyRequest_NilReplyChDoesNothing(t *testing.T) {
	m := &mockUI{
		getFn: func(userPrompt string) (string, error) {
			return "", errors.New("should not be called")
		},
	}
	ServeProxyRequest(m, ProxyRequest{Kind: ProxyRequestGet, UserPrompt: "x", ReplyCh: nil})
}

func TestServeProxyRequest_UnknownKindReturnsError(t *testing.T) {
	m := &mockUI{}
	replyCh := make(chan ProxyResponse, 1)
	ServeProxyRequest(m, ProxyRequest{Kind: ProxyRequestKind(999), ReplyCh: replyCh})
	resp := <-replyCh
	if resp.Err == nil {
		t.Fatalf("expected error for unknown kind")
	}
}
