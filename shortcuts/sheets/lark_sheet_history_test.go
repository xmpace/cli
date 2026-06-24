// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestHistoryShortcuts_DryRun asserts each history shortcut targets the right
// facade-agg tool, routes through the correct read/write invoke endpoint, and
// builds the expected tool input (excel_id always; history_version_id for the
// revert pair).
func TestHistoryShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	const versionID = "histVER123"
	const txnID = "txn-abc-123"

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantPath  string // invoke_read | invoke_write suffix
		wantInput map[string]interface{}
	}{
		{
			name:     "+history-list via --url",
			sc:       HistoryList,
			args:     []string{"--url", testURL},
			toolName: "history_list",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name:     "+history-list via --spreadsheet-token",
			sc:       HistoryList,
			args:     []string{"--spreadsheet-token", testToken},
			toolName: "history_list",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name:     "+history-revert routes to invoke_write with version id",
			sc:       HistoryRevert,
			args:     []string{"--url", testURL, "--history-version-id", versionID},
			toolName: "history_revert",
			wantPath: "invoke_write",
			wantInput: map[string]interface{}{
				"excel_id":           testToken,
				"history_version_id": versionID,
			},
		},
		{
			name:     "+history-revert-status routes to invoke_read with transaction id",
			sc:       HistoryRevertStatus,
			args:     []string{"--url", testURL, "--transaction-id", txnID},
			toolName: "history_revert_status",
			wantPath: "invoke_read",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"transaction_id": txnID,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			callURL := dryRunFirstCallURL(t, tt.sc, tt.args)
			if !containsSuffix(callURL, tt.wantPath) {
				t.Errorf("invoke url = %q, want suffix %q", callURL, tt.wantPath)
			}
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestHistoryRevert_MissingRequiredFlag asserts the required selector is
// enforced in Validate (no request is sent) and the error is tagged with the
// offending flag param: +history-revert needs --history-version-id, while
// +history-revert-status needs --transaction-id (the receipt from +history-revert).
func TestHistoryRevert_MissingRequiredFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sc        common.Shortcut
		wantParam string
	}{
		{HistoryRevert, "--history-version-id"},
		{HistoryRevertStatus, "--transaction-id"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.sc.Command, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, tc.sc, []string{"--url", testURL})
			if err == nil {
				t.Fatalf("%s: expected validation error for missing %s", tc.sc.Command, tc.wantParam)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("%s: error = %T %v, want *errs.ValidationError", tc.sc.Command, err, err)
			}
			if validationErr.Param != tc.wantParam {
				t.Fatalf("%s: param = %q, want %s", tc.sc.Command, validationErr.Param, tc.wantParam)
			}
		})
	}
}

// dryRunFirstCallURL runs the shortcut in --dry-run and returns the first
// api call's url, so tests can assert read vs. write endpoint routing.
func dryRunFirstCallURL(t *testing.T, sc common.Shortcut, args []string) string {
	t.Helper()
	out, err := runShortcut(t, sc, append(args, "--dry-run"))
	if err != nil {
		t.Fatalf("dry-run failed: %v\noutput=%s", err, out)
	}
	dryRun := decodeDryRunRaw(t, out)
	calls, ok := dryRun["api"].([]interface{})
	if !ok || len(calls) == 0 {
		t.Fatalf("dry-run api array empty or wrong shape: %#v", dryRun)
	}
	call, _ := calls[0].(map[string]interface{})
	url, _ := call["url"].(string)
	return url
}

func containsSuffix(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
