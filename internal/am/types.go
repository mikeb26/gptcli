/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

// ApprovalScope encodes the semantics of a user's approval decision.
type ApprovalScope string

const (
	ApprovalScopeOnce   ApprovalScope = "once"   // just this invocation
	ApprovalScopeTarget ApprovalScope = "target" // tool-defined target (e.g., file, directory, domain)
	ApprovalScopeDeny   ApprovalScope = "deny"   // explicit denial
)

// ApprovalAction represents a capability or operation that may be
// granted by an approval decision (e.g. read vs write).
type ApprovalAction string

const (
	ApprovalActionRead    ApprovalAction = "read"
	ApprovalActionWrite   ApprovalAction = "write"
	ApprovalActionExecute ApprovalAction = "exec"
)

type ApprovalSubsys string

const (
	ApprovalSubsysTools ApprovalSubsys = "tools"
)

type ApprovalGroup string

const (
	ApprovalGroupFileIO ApprovalGroup = "fileio"
	// ApprovalGroupWeb groups approval policies for web / HTTP tools
	// such as url_retrieve. It mirrors the semantics of
	// ApprovalGroupFileIO but operates on URLs and domains instead of
	// filesystem paths.
	ApprovalGroupWeb ApprovalGroup = "web"
	// ApprovalGroupCommand groups approval policies for tools that
	// execute OS-level commands (e.g. cmd_run).
	ApprovalGroupCommand ApprovalGroup = "command"
	// ApprovalGroupEnv groups approval policies for tools that access
	// or mutate environment variables (env_get/env_set).
	ApprovalGroupEnv ApprovalGroup = "env"
)

type ApprovalTarget string

const (
	ApprovalTargetFile   ApprovalTarget = "file"
	ApprovalTargetDir    ApprovalTarget = "directory"
	ApprovalTargetUrl    ApprovalTarget = "url"
	ApprovalTargetDomain ApprovalTarget = "domain"
	// ApprovalTargetCommand represents a specific executable or
	// command name used by cmd_run.
	ApprovalTargetCommand ApprovalTarget = "command"
	// ApprovalTargetCommandInvocation represents a specific
	// command-line invocation (command + arguments) for cmd_run.
	ApprovalTargetCommandInvocation ApprovalTarget = "command_invocation"
	// ApprovalTargetEnvVar represents an individual environment
	// variable name.
	ApprovalTargetEnvVar ApprovalTarget = "env_var"
)

// ApprovalChoice represents a single option the user can select in an
// approval dialogue.
type ApprovalChoice struct {
	// Key is the key the user presses / selects (e.g. "y", "n", "a").
	Key string
	// Label is the human-readable label shown in the UI.
	Label string
	// Scope describes the semantics of this choice.
	Scope ApprovalScope
	// PolicyID is an optional stable identifier used to cache decisions
	// (e.g. "tools:fileio:file:/path/to/file").
	PolicyID string
	// Actions is the set of actions that this choice will permit when
	// persisted as a policy. It is only used for target-scoped choices
	// that result in saving a policy.
	Actions []ApprovalAction
}

// ApprovalPolicyStore is a simple in-memory store that tracks allowed
// actions keyed by a PolicyID.
type ApprovalPolicyStore interface {
	// Check returns the set of allowed actions for the given policy and
	// whether any policy was found.
	Check(policyID string) (actions []ApprovalAction, found bool)
	// Save persists the allowed action set for the given PolicyID.
	// Save uses replace semantics: the stored set becomes exactly the
	// provided actions slice.
	Save(policyID string, actions []ApprovalAction)
}
