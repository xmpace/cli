// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_history (BE-2: +history-revert / +history-revert-status) ──
//
// Two thin callTool wrappers over the facade-agg history tools:
//   - +history-revert        → history_revert        (write) — async revert
//   - +history-revert-status → history_revert_status (read)  — poll outcome
//
// Both target a single history version via --history-version-id (the id
// surfaced by +history-list). Revert is asynchronous: it returns a receipt /
// transaction id that +history-revert-status then polls, distinguishing
// in-progress / success / failure from the tool output (passed through
// verbatim — no client-side shaping).
//
// ⚠️ Backend state: the facade-agg history_revert / history_revert_status
// tools are registered but their downstream RPC wiring is a DEFERRED
// follow-up; today they return a "not wired yet" guard error from the gateway,
// which surfaces here as a normal tool error. These CLI shortcuts are correct
// thin wrappers and will work end-to-end once the backend follow-up lands —
// this is NOT a CLI blocker. See self_check.md.
//
// Flags are declared inline (historyLocatorFlags + history-version-id) rather
// than via flagsFor(), because flag_defs_gen.go / data/flag-defs.json are
// synced from sheet-skill-spec (BE-3) and must not be hand-edited.

// historyVersionIDFlag is the target-version selector shared by the revert and
// revert-status shortcuts. Requiredness is enforced in Validate (via
// validateHistoryVersionID) rather than through cobra's MarkFlagRequired, so a
// missing value yields a typed, flag-tagged *errs.ValidationError at the
// Validate stage — an actionable error before any request is sent — instead of
// cobra's plain "required flag not set" string.
func historyVersionIDFlag() common.Flag {
	return common.Flag{
		Name: "history-version-id",
		Type: "string",
		Desc: "History version to act on (from +history-list). Required.",
	}
}

func historyRevertFlags() []common.Flag {
	return append(historyLocatorFlags(), historyVersionIDFlag())
}

// validateHistoryVersionID enforces the required, control-char-clean
// --history-version-id. Returns the trimmed value so callers reuse it.
func validateHistoryVersionID(runtime *common.RuntimeContext) (string, error) {
	id := strings.TrimSpace(runtime.Str("history-version-id"))
	if id == "" {
		return "", sheetsValidationForFlag("history-version-id", "--history-version-id is required")
	}
	return id, nil
}

func historyRevertInput(token, versionID string) map[string]interface{} {
	return map[string]interface{}{
		"excel_id":           token,
		"history_version_id": versionID,
	}
}

// HistoryRevert wraps the history_revert tool (write): asynchronously revert a
// spreadsheet to the given history version. --history-version-id is required;
// a missing value fails in Validate before any request is sent. Returns the
// async receipt / transaction id, queryable via +history-revert-status.
var HistoryRevert = common.Shortcut{
	Service:     "sheets",
	Command:     "+history-revert",
	Description: "Revert a spreadsheet to a given history version (asynchronous; poll with +history-revert-status).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       historyRevertFlags(),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, err := validateHistoryVersionID(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		versionID := strings.TrimSpace(runtime.Str("history-version-id"))
		return invokeToolDryRun(token, ToolKindWrite, "history_revert", historyRevertInput(token, versionID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		versionID, err := validateHistoryVersionID(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "history_revert", historyRevertInput(token, versionID))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Revert is asynchronous — pass the returned id to +history-revert-status to track in-progress / success / failure.",
	},
}

// HistoryRevertStatus wraps the history_revert_status tool (read): poll the
// outcome of a prior +history-revert. The tool output distinguishes
// in-progress / success / failure and is passed through verbatim.
var HistoryRevertStatus = common.Shortcut{
	Service:     "sheets",
	Command:     "+history-revert-status",
	Description: "Poll the status of a history revert (in-progress / success / failure).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       historyRevertFlags(),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, err := validateHistoryVersionID(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		versionID := strings.TrimSpace(runtime.Str("history-version-id"))
		return invokeToolDryRun(token, ToolKindRead, "history_revert_status", historyRevertInput(token, versionID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		versionID, err := validateHistoryVersionID(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "history_revert_status", historyRevertInput(token, versionID))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}
