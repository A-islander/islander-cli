# 岛民岛 CLI

正式论坛 API：`https://forum-api.islander.top/`；身份 API：`https://user-api.islander.top/`。使用已实现的 CLI，默认 JSON 输出，CLI 默认匿名；TUI 的身份选择不会自动授权 CLI 发布。

启动工作目录有 `./islander`：

```sh
./islander --help
./islander board list
./islander thread list --page 1
./islander thread list --board BOARD_ID --page 1
./islander thread get THREAD_ID --page 1
./islander reply list --thread THREAD_ID --page 1
./islander post get POST_ID
```

`BOARD_ID`、`THREAD_ID`、`POST_ID` 需换成刚读到的真实编号。主楼 `followId=0`，详情当页可能包含主楼，计数也可能包含主楼；按实际 `hasMore` 翻页。`replyArr` 为空不表示正文没有 `No.编号` 引用，必要时从正文提取并用 `post get` 核实。

## 发串、回串与引用

仅在 `participate` 模式且已导入岛民娘专用饼干时使用；先查子命令帮助。

```sh
./islander --cookie mascot reply create --thread THREAD_ID --quote POST_ID --body-file drafts/reply.txt --dry-run
./islander --cookie mascot thread create --board BOARD_ID --body-file drafts/thread.txt --dry-run
```

检查预览身份、目标和全文，再移除 `--dry-run`，对同一参数加 `--confirm HASH`。哈希必须来自该次预览，正文或参数变更后重新预览。`--quote` 用于同站楼层；跨站引用写清来源 URL。`--attach` 添加文件，上传会在真实发布阶段产生副作用。

CLI 支持自己的内容、草稿、附件和 SAGE 等，先看 `--help`。自主聊天不包含删除、SAGE、账号管理授权。

底层鉴权头是 `Authorization: 原始token`，不是 Bearer；优先让 CLI 从存储读取，不把 token 放进命令或提示词。显示 ID `R5RCGeZ` 仅用于记住开发员身份。

来源：本仓库命令帮助、`internal/cli`、`internal/forum` 和现有正式论坛只读试验。端点或帮助变动时以当前代码及真实返回为准。
