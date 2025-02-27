/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type RetrieveUrlTool struct{}

func (t RetrieveUrlTool) GetOp() ToolCallOp {
	return RetrieveUrl
}

func (t RetrieveUrlTool) RequiresUserApproval() bool {
	return false
}

type RetrieveUrlResponse struct {
	Status        string
	StatusCode    int
	Proto         string
	ProtoMajor    int
	ProtoMinor    int
	Header        http.Header
	Body          string
	ContentLength int64
}

func (RetrieveUrlTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"url": {
				Type:        jsonschema.String,
				Description: "The url to retrieve",
			},
			"headers": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.String,
				},
				Description: "Optional request headers as an array of strings formatted as 'Key: Value'",
			},
			"method": {
				Type:        jsonschema.String,
				Description: "Optional HTTP request method (e.g., GET, POST, etc.); defaults to GET",
			},
			"body": {
				Type:        jsonschema.String,
				Description: "Optional HTTP request body",
			},
		},
		Required: []string{"url"},
	}
	f := openai.FunctionDefinition{
		Name:        string(RetrieveUrl),
		Description: "retrieve the content of a url",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (RetrieveUrlTool) Invoke(args map[string]any) (string, error) {
	url, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'url' arg")
	}

	requestMethod := "GET"
	m, ok := args["method"].(string)
	if ok && m != "" {
		requestMethod = strings.ToUpper(m)
	}

	var bodyReader io.Reader
	bodyVal, ok := args["body"].(string)
	if ok && bodyVal != "" {
		bodyReader = strings.NewReader(bodyVal)
	}

	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	req, err := http.NewRequest(requestMethod, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to create request for '%v' with method '%v': %w", url, requestMethod, err)
	}

	headersArg, ok := args["headers"]
	if ok {
		headersArray, ok := headersArg.([]interface{})
		if !ok {
			return "", fmt.Errorf("gptcli: 'headers' should be an array of strings")
		}
		for _, headerVal := range headersArray {
			headerStr, ok := headerVal.(string)
			if !ok {
				return "", fmt.Errorf("gptcli: each header should be a string")
			}
			parts := strings.SplitN(headerStr, ":", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("gptcli: invalid header format '%v', expected 'Key: Value'", headerStr)
			}
			headerKey := strings.TrimSpace(parts[0])
			headerValue := strings.TrimSpace(parts[1])
			req.Header.Add(headerKey, headerValue)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%v: failed to fetch '%v': %w", RetrieveUrl, url, err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%v: failed to read '%v': %w", RetrieveUrl, url, err)
	}

	ret := RetrieveUrlResponse{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        resp.Header,
		Body:          string(content),
		ContentLength: resp.ContentLength,
	}

	encodedRet, err := json.Marshal(ret)
	return string(encodedRet), err
}
