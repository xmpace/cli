// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_history (BE-1: +history-list) ─────────────────────────
//
// Wraps the facade-agg `history_list` tool (read) behind the One-OpenAPI
// invoke_read endpoint. The tool returns a sheet's version history. The
// facade-agg tool already performs the response transform (minor_histories
// trim / id → history_version_id / 4-field projection / RFC3339 create_time),
// so the CLI passes the tool output straight through and does NOT re-implement
// the transform client-side.
//
// History is workbook-level (no sheet selector), mirroring +workbook-info:
// the only locator is --url / --spreadsheet-token (XOR), with --token accepted
// as a parse-time alias for --spreadsheet-token via the shared PostMount hook.
//
// Flags are declared inline here rather than via flagsFor(): the generated
// flag_defs_gen.go / data/flag-defs.json are synced from sheet-skill-spec
// (BE-3) and must not be hand-edited, so this hand-written shortcut owns its
// own flag set. The two locator flags match +workbook-info's shape exactly.

// historyLocatorFlags is the --url / --spreadsheet-token XOR locator pair
// shared by the three history shortcuts. Mirrors +workbook-info's flag-defs
// entry; XOR is enforced in Validate via parseSpreadsheetRef, not by Required.
func historyLocatorFlags() []common.Flag {
	return []common.Flag{
		{Name: "url", Type: "string", Desc: "Spreadsheet locator (a /sheets/ or /wiki/ URL)."},
		{Name: "spreadsheet-token", Type: "string", Desc: "Spreadsheet locator (raw spreadsheet token)."},
	}
}

// HistoryList wraps the history_list tool: list a spreadsheet's history
// versions. Each item carries history_version_id / create_time / action /
// all_block_revision (projected server-side). An empty sheet yields an empty
// list and exit 0.
var HistoryList = common.Shortcut{
	Service:     "sheets",
	Command:     "+history-list",
	Description: "List a spreadsheet's edit history versions (history_version_id, create_time, action, all_block_revision).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       historyLocatorFlags(),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := resolveSpreadsheetToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindRead, "history_list", historyListInput(token))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "history_list", historyListInput(token))
		if err != nil {
			return err
		}
		// Pass the tool output through verbatim — facade-agg already shaped it.
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Capture a history_version_id from the result to feed +history-revert.",
	},
}

func historyListInput(token string) map[string]interface{} {
	return map[string]interface{}{"excel_id": token}
}
