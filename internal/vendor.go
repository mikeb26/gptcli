/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

const DefaultVendor = "openai"

type VendorInfo struct {
	Name            string
	FullName        string
	ApiKeyUrl       string
	SupportedModels []string
	DefaultModel    string
}

var vendorInfos = map[string]VendorInfo{
	"google": {
		Name:            "google",
		FullName:        "Google",
		ApiKeyUrl:       "https://aistudio.google.com/app/api-keys",
		SupportedModels: []string{"gemini-3-pro-preview", "gemini-3-flash-preview"},
		DefaultModel:    "gemini-3-pro-preview",
	},
	"anthropic": {
		Name:      "anthropic",
		FullName:  "Anthropic",
		ApiKeyUrl: "https://platform.claude.com/settings/keys",
		SupportedModels: []string{"claude-sonnet-4-5-20250929",
			"claude-opus-4-5-20251101", "claude-haiku-4-5-20251001"},
		DefaultModel: "claude-sonnet-4-5-20250929",
	},
	"openai": {
		Name:            "openai",
		FullName:        "OpenAI",
		ApiKeyUrl:       "https://platform.openai.com/api-keys",
		SupportedModels: []string{"gpt-5.2", "gpt-5-mini", "gpt-5.2-pro"},
		DefaultModel:    "gpt-5.2",
	},
}

func GetVendors() []string {
	ret := make([]string, 0)

	for k, _ := range vendorInfos {
		ret = append(ret, k)
	}

	return ret
}

func GetVendorInfo(name string) VendorInfo {
	v, ok := vendorInfos[name]
	if !ok {
		return VendorInfo{}
	}

	return v
}
