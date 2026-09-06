# 岛民岛终端客户端

给岛民的 **Bubble Tea TUI**，给岛民娘 AI 的 **Cobra CLI**。共用论坛 API、饼干和草稿逻辑。TUI 延续原型的深海色分栏界面，窄窗口自动切为单栏。

当前为试用版本，源码仓库：[A-islander/islander-cli](https://github.com/A-islander/islander-cli)。岛民娘的跨岛提示词、接口说明与启动入口见 [islander-poster-girl](islander-poster-girl/README.md)。

## 先试用

当前工作区已编译好 Linux 可执行文件：

```sh
cd /home/hedykan/Develope/islander/islander-cli
./bin/islander
./bin/islander tui
```

直接运行 `islander` 显示 CLI 帮助；`islander tui` 打开交互界面，默认连接正式论坛，以访客身份浏览；选择饼干并确认后才能发布。`islander <命令> --help` 查看该命令的参数，例如 `islander reply create --help`。

**想先练习发帖、回复、引用、附件和删除，可以启动独立的本地试用岛：**

```sh
python3 scripts/demo.py
```

这个入口自动启动本机模拟服务并导入虚构饼干。发帖等操作只影响内存里的测试内容，退出后清除测试数据。需要 Python 3，无第三方 Python 依赖。

`./bin/islander tui --demo` 则保留最初的八条虚构串，仅供离线浏览原型。

## 发串、回复与引用

1. 按 `i` 打开饼干管理，选择「导入饼干」，依次输入英文别名和饼干内容。饼干输入会隐藏；也可以选择「领取新饼干」并确认。
2. 按 `b` 选择板块，按 `c` 发串。编辑器中 `Tab` 切换标题和正文。
3. 阅读串时按 `r` 回复，按大写 `R` 引用当前楼层。也可直接在正文中写 `No.编号`；发送时自动提取所有引用编号。
4. `Ctrl+A` 打开文件选择器：`↑↓ / j k` 选择，`Enter` 进入目录或添加文件，`← / h` 返回上级，`~` 回主目录，`.` 显示／隐藏点文件。按 `:` 也可手动输入文件路径；`Esc` 返回编辑。再次按 `Ctrl+A` 会记住上次目录，方便继续添加。`Ctrl+X` 从草稿移除附件。
5. `Ctrl+P` 预览身份、板块／目标串、完整正文和附件；预览中用上下键滚动。只有按 `Enter` 确认后才上传附件并发布。
6. `Esc` 返回编辑。编辑时按 `Esc` 或 `Ctrl+S` 保存草稿并退出，之后按 `d` 找回当前饼干的草稿。

附件单个最多 20 MB，正文最多 8192 字节，标题最多 128 字节。上传后发布失败会保留草稿及上传结果。网络失败不自动重试；先在「我的内容」核对结果，再决定是否重新提交。

## 常用按键

顶部常驻 **`m 我的内容`** 按钮，可鼠标点击或按 `m` 打开当前饼干的发串与回复列表。访客会先进入饼干选择，选择／导入后继续打开。页面显示饼干别名；选中回复后按 `Enter` 定位原串，`Esc` 返回原来的列表位置。`[` / `]` 翻页，`i` 切换饼干。

阅读区用 `›`、标题底色和正文左侧竖线标记当前选中的楼层。用 `↑↓ / j k` 或 `n/p` 换楼；长正文用 `PgUp/PgDn` 或空格滚动。按 `Enter` 打开选中帖子的操作菜单，引用、SAGE、附件和删除等操作均针对该楼层，菜单和确认页会显示目标编号。

按 `v` 在帖子下方原位展开引用，多条引用会一起列出。用 `↓` 选中展开的引用，再按 `v` 可继续查看它的引用；选中父帖按 `v` 收起该分支，`Esc / ← / h` 从引用返回上层并恢复阅读位置。`↑↓ / j k` 遍历正文及展开的引用，`n/p` 只切换原串楼层。选中引用时，`R` 引用该帖并回复当前串，SAGE 和附件操作也对应选中的引用。循环引用不继续展开，最多展开 12 层。

| 按键 | 操作 |
|---|---|
| `↑↓` / `j k` | 列表选串；阅读区选择帖子及展开的引用 |
| `Enter` | 进入串详情；阅读时打开选中帖子的操作菜单 |
| `PgUp/PgDn` / 空格 | 滚动正文和原位引用；发布预览仍可用 `↑↓` 滚动 |
| `Esc` / `h` / `←` | 引用返回上层；原串返回列表，保留阅读位置 |
| `Tab` | 切换列表／正文焦点 |
| `n p` | 下一楼／上一楼 |
| `v` | 原位展开／收起选中帖子的引用，支持继续展开嵌套引用 |
| `:` | 按主楼或回复编号定位 |
| `[` / `]` | 前一页／后一页，页码从 1 开始 |
| `b` | 全部、各板块、SAGE、我的内容 |
| `m` / 点击顶部「我的内容」 | 直接打开当前饼干的发串与回复列表 |
| `1–9` | 顶部前九个板块入口 |
| `/` | 筛选当前页标题、摘要和编号 |
| `t` | 当前页按最新发布排序 |
| `Ctrl+R` | 刷新，恢复服务端顺序 |
| `c` / `r` / `R` | 发串／回复／引用回复 |
| `i` / `d` | 饼干／草稿管理，列表内 `x` 移除选中项 |
| `s` / `S` | SAGE／反对 SAGE，先确认再执行 |
| `x` / `X` | 删除／恢复自己的内容，先确认再执行 |
| `a` | 当前楼层附件列表 |
| `L` | 站务与友链 |
| `f` | 宽屏时切换单栏／分栏 |
| `?` / `q` | 帮助／退出 |

推荐 120 × 36、支持中文的终端字体；100 列起默认分栏，最低 44 × 16。Ghostty 背景已统一处理，文字、边框与空白不会混用默认底色。

为绕过半角浊音符的宽度兼容问题，TUI 显示时将 `ﾟ` / `ﾞ` 转为相近的独立占位字符 `°` / `˝`，例如 `(ﾟДﾟ)` 显示为 `(°Д°)`；草稿、发送内容和 CLI 输出保留原文。组合 emoji 仍按完整字形计算宽度。

## 附件

按 `a` 选附件，再选择：

- `Enter`：独立终端预览。默认检测 Kitty 图形协议；未确认支持时显示彩色字符缩略图。
- `o`：通过系统默认应用打开，适合视频或其他文件。
- `s`：下载到 `~/Downloads/islander/`，使用新文件名，不覆盖已有文件。

可用 `--images auto|kitty|blocks|off` 手动控制。原图预览支持 Go 解码的 PNG、JPEG 和 GIF 首帧，限制 20 MB／3200 万像素；其他格式可以外部打开。视频、音频不在终端中播放。图片预览运行于独立子进程，返回时恢复 TUI。

## 饼干和本地数据

默认使用系统凭证库保存饼干。没有可用的系统凭证库时，会提示错误；可显式选择本地明文文件方案：

```sh
./bin/islander tui --credential-store file
```

文件方案的目录权限为 `0700`、文件为 `0600`，不做静默降级。普通元数据和 JSON 输出不包含饼干。可通过 CLI 隐藏输入导入：

```sh
./bin/islander cookie import daily
./bin/islander cookie list
./bin/islander cookie use daily
```

`cookie use anonymous` 切回访客。`cookie import daily --stdin` 可从受控 stdin 导入，避免把饼干写进命令行参数。

默认数据位于系统用户配置目录的 `islander/<服务地址摘要>/`，可用 `--data-dir` 更改。正式／测试端点的饼干与草稿相互隔离。CLI 读取默认匿名，需要身份的操作必须显式带 `--cookie daily`；TUI 使用你当前选择的饼干。

## 给岛民娘的 CLI

岛民娘的人设和只读试用提示词见 [岛民娘 CLI 试读](docs/islander-poster-girl-cli.md)。本机 Codex CLI 登录后可运行 `python3 scripts/ai-read-trial.py`：让她自行浏览正式论坛、读取上下文并输出三份未发布的回复草稿。测试使用独立的岛民岛数据目录，不使用 TUI 中的饼干；草稿与实际命令日志位于 Git 忽略的 `.local/ai-cli-trial/`。这轮只验证匿名阅读与生成文本，不调用发帖／回复接口。

默认 stdout 是 JSON，结构含 `schemaVersion: 1`；错误写 stderr，不混入正文。`--output text` 可查看缩进后的结果。退出码：0 成功、2 参数／本地错误、3 鉴权、4 网络／响应错误、5 业务拒绝。

```sh
./bin/islander board list
./bin/islander thread list --board 1 --page 1
./bin/islander thread get 20459 --page 1
./bin/islander reply list --thread 20459 --page 1
./bin/islander post get 20459
./bin/islander mine list --cookie daily
./bin/islander sage list
./bin/islander draft list --cookie daily
./bin/islander draft save --cookie daily --board 1 --body-file reply.txt
```

列表带页码、总数及 `hasMore`。单页读取不代表完整串；`thread get` 包含指定帖子与该主串的一页内容。媒体同时提供原始 `mediaUrl` 与解析后的 `attachments`。

`thread get` 的 `replies.list` 及 `reply list` 沿用服务端分页，可能包含主楼（`followId=0`），`count` 也计入主楼。统计纯回复时按 `followId` 过滤，合并多次读取时按帖子编号去重。部分旧帖的 `replyArr` 为空，但正文仍含 `No.编号` 引用，需要结合正文识别。

写操作先返回预览和 `confirmation` 确认码。用户批准具体内容后，以相同参数附加 `--confirm <确认码>` 执行；没有通用的永久 `--yes` 写权限。

```sh
./bin/islander thread create --cookie daily --board 1 --title '标题' --body-file post.txt --dry-run
./bin/islander reply create --cookie daily --thread 20459 --quote 20460 --body-file reply.txt --dry-run
./bin/islander post sage 20459 --cookie daily --dry-run
./bin/islander post delete 20459 --cookie daily --dry-run
```

正文文件使用 UTF-8；`--body-file -` 读取 stdin。附件通过可重复的 `--attach` 添加。`--dry-run` 不上传。还提供 `post unsage/restore`、`cookie register/remove`、`draft get/remove/publish`，具体参数见 `--help`。

## 构建与检查

Go 版本见 `go.mod`，当前使用 Go 1.26.5。已在 Linux 上编译和验证，其他系统尚未验证。

```sh
go build -o bin/islander .
go test -race ./...
go vet ./...
```

测试覆盖 API 分页／引用／鉴权／上传契约、凭证隔离与权限、失败草稿恢复、确认前不发布、身份绑定、旧请求丢弃、终端布局及背景。正式服务验证限于匿名读取；写入通过本地模拟服务验证。

| 与 Flutter／Web 对照 | 状态 |
|---|---|
| 板块、时间线、串详情、回复分页、编号定位与引用 | 已接入 |
| 发串、回复、引用回复、媒体上传 | 已接入，发布前预览确认 |
| 多饼干导入／领取／切换／移除、我的内容、草稿 | 已接入 |
| SAGE、反对 SAGE、删除／恢复 | 已接入 |
| 当前页筛选／排序、站务／友链 | 已提供 |
| 图片、视频 | 图片终端预览与降级；视频外部打开 |
| 服务端全文搜索 | 现有客户端接口未提供，当前仅页内筛选 |
| 海浪之家与岛屿场景 | 本项目当前范围是论坛，不包含图形场景 |

`internal/forum` 是公共 API 层，`internal/local` 管理身份和草稿，`internal/cli` 与 `internal/tui` 提供两个入口，`internal/media` 管理附件预览／下载。
