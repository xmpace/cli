# Lark Sheet History

## 概念回顾

每张飞书电子表格保留一串历史版本（`minor_histories`）。每个版本由 `history_version_id` 标识，并附带创建时间（`create_time`）、动作（`action`）与块修订信息（`all_block_revision`）。历史是**工作簿级**的（针对整张电子表格，不针对单个子表）。

回滚（revert）把电子表格的当前内容覆盖回某个历史版本——这是一个**写入 / 不可逆**操作，且为**异步**：发起后立即返回受理标识，真正的回滚在后台进行，需通过状态查询轮询最终结果（进行中 / 成功 / 失败）。

`+history-list` 读取版本列表以挑选目标；`+history-revert` 发起回滚；`+history-revert-status` 轮询回滚结果。

## 使用场景

读取历史版本、发起回滚、查询回滚状态。本 reference 覆盖 3 个 shortcut：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看历史版本列表 | `+history-list` | 返回 `minor_histories`，每条含 `history_version_id` / `create_time` / `action` / `all_block_revision` 四个字段 |
| 回滚到指定历史版本 | `+history-revert` | 传入 `--history-version-id`；异步受理，返回可查询标识 |
| 查询回滚状态 | `+history-revert-status` | 轮询某次回滚的进行中 / 成功 / 失败状态 |

典型工作流：`+history-list` 拿到目标版本的 `history_version_id` → `+history-revert` 发起回滚 → `+history-revert-status` 轮询直到成功或失败。

**注意事项（必须了解）**：
- **回滚是写入 / 不可逆操作**：会用历史版本内容覆盖当前表格，发起前请确认目标 `history_version_id` 正确。
- **回滚是异步的**：`+history-revert` 返回的是受理标识，不代表回滚已完成；必须用 `+history-revert-status` 确认最终结果。
- **`history_version_id` 取自 `+history-list`**：不要臆造或复用别的标识。
- **历史是工作簿级**：定位只需 `--url` / `--spreadsheet-token`（XOR），不需要子表选择器。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+history-list` | read | 工作簿 |
| `+history-revert` | write | 工作簿 |
| `+history-revert-status` | read | 工作簿 |

## Flags

### `+history-list`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+history-revert`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--history-version-id` | string | required | 要回滚到的历史版本（取自 +history-list） |

### `+history-revert-status`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--history-version-id` | string | required | 要查询回滚状态的历史版本（取自 +history-list） |

## Examples

公共定位：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token`（XOR，二选一）。`--history-version-id` 取自 `+history-list` 的输出。

### `+history-list`

```bash
# 列出某张电子表格的历史版本
lark-cli sheets +history-list --url "https://sample.feishu.cn/sheets/SHTxxxxxx"

# 用原始 spreadsheet token 定位
lark-cli sheets +history-list --spreadsheet-token "SHTxxxxxx"
```

### `+history-revert`

```bash
# 回滚到指定历史版本（异步受理）
lark-cli sheets +history-revert --url "https://sample.feishu.cn/sheets/SHTxxxxxx" --history-version-id "<id-from-history-list>"
```

### `+history-revert-status`

```bash
# 查询某次回滚的当前状态（进行中 / 成功 / 失败）
lark-cli sheets +history-revert-status --url "https://sample.feishu.cn/sheets/SHTxxxxxx" --history-version-id "<id-from-history-list>"
```
