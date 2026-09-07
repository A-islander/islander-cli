# 岛民岛终端客户端

用于交互浏览的 **Bubble Tea TUI**，用于命令行与自动化的 **Cobra CLI**。共用论坛 API、饼干和草稿逻辑。TUI 延续原型的深海色分栏界面，窄窗口自动切为单栏。

当前为试用版本，源码仓库：[A-islander/islander-cli](https://github.com/A-islander/islander-cli)。

## 安装

当前版本为 **v0.0.2**。TUI 和 CLI 共用 `islander` 程序，`islander --version` 查看实际版本。

### 使用 Go 安装

需要 Go 1.26.5 或更新的工具链：

```sh
go install github.com/A-islander/islander-cli/cmd/islander@latest
islander --version
islander tui
```

安装指定版本：

```sh
go install github.com/A-islander/islander-cli/cmd/islander@v0.0.2
```

可执行文件安装到 `GOBIN`；未设置时为 `GOPATH/bin`，通常是 `~/go/bin`。如果提示找不到命令，请将实际安装目录加入 `PATH`，例如：

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

旧的仓库根目录安装入口仍兼容，但生成的文件名是 `islander-cli`；推荐使用上面的 `cmd/islander` 入口。

### 下载发布包

[GitHub Releases](https://github.com/A-islander/islander-cli/releases) 提供 Linux、macOS 和 Windows 的 x86_64 / aarch64 发布包，无需安装 Go：

| 系统 / 格式 | x86_64 文件名 |
|---|---|
| Linux 压缩包 | `islander-v0.0.2-linux-x86_64.tar.gz` |
| Linux AppImage | `islander-v0.0.2-linux-x86_64.AppImage` |
| macOS 压缩包 | `islander-v0.0.2-macos-x86_64.tar.gz` |
| macOS DMG | `islander-v0.0.2-macos-x86_64.dmg` |
| Windows 压缩包 | `islander-v0.0.2-windows-x86_64.zip` |
| Windows 可执行文件 | `islander-v0.0.2-windows-x86_64.exe` |

ARM64（包括 Apple Silicon）对应文件名中的架构为 `aarch64`。Release 同时提供 `SHA256SUMS`，可在下载目录用 `sha256sum --ignore-missing -c SHA256SUMS` 校验。

Linux 压缩包解压后即可运行：

```sh
tar -xzf islander-v0.0.2-linux-x86_64.tar.gz
./islander --version
./islander tui
```

AppImage 在终端中启动；不带参数默认进入 TUI，带参数时转交 CLI：

```sh
chmod +x islander-v0.0.2-linux-x86_64.AppImage
./islander-v0.0.2-linux-x86_64.AppImage
./islander-v0.0.2-linux-x86_64.AppImage --version
./islander-v0.0.2-linux-x86_64.AppImage board list
```

没有可用 FUSE 时，可加 `--appimage-extract-and-run` 启动。桌面入口使用系统配置的终端；密钥环和外部浏览器仍使用宿主系统服务。

macOS 可下载对应架构的 DMG，将 `Islander.app` 拖入 Applications，双击后在 Terminal 中启动 TUI；首次启动可能需要允许控制 Terminal。也可下载 `macos` 压缩包，解压后在 Ghostty 或 Terminal 中运行 `./islander tui`。AppImage 仅适用于 Linux。macOS 包未做 Apple Developer ID 签名或公证。

Windows 推荐下载 ZIP，解压后在 Windows Terminal / PowerShell 中运行：

```powershell
.\islander.exe --version
.\islander.exe tui
```

也可下载独立 `.exe`，使用下载文件的完整名称运行并附加 `tui` 参数。

### 从源码构建

```sh
git clone https://github.com/A-islander/islander-cli.git
cd islander-cli
make build
./bin/islander tui
```

直接运行 `islander` 显示 CLI 帮助；`islander tui` 打开交互界面，默认连接正式论坛，以访客身份浏览；选择饼干并确认后才能发布。`islander <命令> --help` 查看该命令的参数，例如 `islander reply create --help`。

**想先练习发帖、回复、引用、附件和删除，可以启动独立的本地试用岛：**

```sh
python3 scripts/demo.py
```

这个入口自动启动本机模拟服务并导入虚构饼干。发帖等操作只影响内存里的测试内容，退出后清除测试数据。需要 Python 3，无第三方 Python 依赖。

`./bin/islander tui --demo` 则保留最初的八条虚构串，仅供离线浏览原型。

## 多站点浏览

**v0.0.2** 接入岛民岛、X 岛和 BOG。设计、接口依据和后续范围见 [多论坛 spec](docs/specs/multi-forum.md)。安装后直接运行：

```sh
islander --site x tui
islander --site bog tui
```

宽屏列表右侧自动显示主楼及第一页最多五条回复摘要；选串停留片刻后加载，快速移动时取消旧请求。长内容和更多回复按 `Enter` 阅读完整串，预览不改变已保存的阅读位置。

TUI 按 **`g` 切换站点**，按 `b` 选板块；`[` / `]` 翻页，`v` 原位展开引用，`a` 查看附件。每次切站从时间线第一页加载，各站点的饼干、草稿和阅读缓存分开。默认仍为岛民岛。

CLI 使用同一个 `--site` 参数，输出保留原有 `schemaVersion` / `data`，另附 `site` 来源字段：

```sh
islander site list
islander --site x site info
islander --site x board list
islander --site x thread list --board 30
islander --site x thread get 50000002
islander --site bog thread list --board 综合版
islander --site bog thread get 1423209 --page 2
islander --site bog post get 1526630
```

外站目前支持浏览，发帖、我的内容、SAGE、删除恢复和领取饼干尚未接入。`site info` 返回已实现的能力；不支持的 CLI 操作返回 `unsupported`，TUI 隐藏相应菜单。下文发串与管理说明适用于岛民岛。

X 岛使用 JSON API；BOG 的板块和串分页解析公开网页，引用读取 JSON。总数未知时 `count=-1`，楼层偏移未知时 `offset=-1`；按 `hasMore` 判断能否继续。BOG 板块 `key` 为原始名称，`id` 是稳定的本地导航编号，不能用于 BOG 的发帖 API。`--board` 可直接使用名称。X 岛的 `post get` 返回 `parentUnknown=true` 表示引用接口没有父串信息；`thread get` 和 TUI 的编号跳转应提供主串编号。

导入外站饼干同样使用隐藏输入，不把凭证放进参数：

```sh
islander --site x cookie import daily
islander --site x --cookie daily thread list --board 27
islander --site bog cookie import daily
```

X 输入 `userhash` 的值，保留百分号编码；导入时只读检查受限板块访问权限。BOG 输入 `bog_master=值; bog_sel=值`（可选追加 `bog_list=值`），目前仅检查格式并保存，明确显示「未验证」，不代表账户或权限已通过验证。CLI 默认匿名；TUI 自动使用当前站点的活动饼干。自定义外站用 `--forum-url` 覆盖 endpoint，`--user-url` 仅适用于岛民岛。

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
| `g` | 切换岛民岛、X 岛、BOG；当前功能以站点能力为准 |
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

按 `a` 打开当前帖子或引用的附件列表，选中附件后按 `Enter`，直接在 TUI 内显示图片。窗口内可用：

- `←` / `→`（或 `h` / `l`）：前后切换附件。
- `Esc`：返回附件列表，再按一次返回原帖，保留阅读位置。
- `r` / `Enter`：重新加载；`b`：切换为字符预览。
- `o`：通过系统默认应用打开，适合视频或其他文件。
- `s`：下载到 `~/Downloads/islander/`，使用新文件名，不覆盖已有文件。

可用 `--images auto|kitty|blocks|off` 手动控制。原图预览支持 Go 解码的 PNG、JPEG 和 GIF 首帧，限制 20 MB／3200 万像素；其他格式可以外部打开。视频、音频不在终端中播放。图片预览在当前 TUI 内异步加载。默认查询终端能力，确认支持后使用 Kitty 图形协议的 Unicode 占位方式显示；未确认支持时自动显示彩色字符预览。切换或关闭图片会取消旧请求并清理对应图片，窗口缩放自动适配。

## 饼干和本地数据

默认使用系统凭证库保存饼干。没有可用的系统凭证库时，会提示错误；可显式选择本地明文文件方案：

```sh
./bin/islander tui --credential-store file
```

文件方案在 Linux/macOS 的目录权限为 `0700`、文件为 `0600`；Windows 使用所在用户目录继承的 ACL。凭证库失败不会静默降级。普通元数据和 JSON 输出不包含饼干。可通过 CLI 隐藏输入导入：

```sh
./bin/islander cookie import daily
./bin/islander cookie list
./bin/islander cookie use daily
```

`cookie use anonymous` 切回访客。`cookie import daily --stdin` 可从受控 stdin 导入，避免把饼干写进命令行参数。

默认数据位于系统用户配置目录的 `islander/<服务地址摘要>/`，可用 `--data-dir` 更改。正式／测试端点的饼干与草稿相互隔离。CLI 读取默认匿名，需要身份的操作必须显式带 `--cookie daily`；TUI 使用你当前选择的饼干。

## CLI 用法

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

Go 版本见 `go.mod`，当前使用 Go 1.26.5。发布构建面向 Linux、macOS 和 Windows 的 x86_64 与 aarch64；CI 在对应系统运行测试并验证发布程序的版本和帮助入口。

```sh
go build -o bin/islander ./cmd/islander
go test -race ./...
go vet ./...
```

测试覆盖 API 分页／引用／鉴权／上传契约、凭证隔离与权限、失败草稿恢复、确认前不发布、身份绑定、旧请求丢弃、终端布局及背景。正式服务验证限于只读请求（含 X 岛饼干访问）；写入通过本地模拟服务验证。

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

`internal/forum` 定义共享操作接口与数据模型，并分别适配岛民岛 JSON、X 岛 JSON 和 BOG 网页／JSON；`internal/local` 管理身份和草稿，`internal/cli` 与 `internal/tui` 提供两个入口，`internal/media` 管理附件预览／下载。

## 发布构建

本机构建当前架构的压缩包与 AppImage：

```sh
make release VERSION=v0.0.2
```

需要 Linux、Go、Python 3 和 `desktop-file-validate`。打包脚本下载并校验固定版本的 AppImage 工具与运行时；产物在 `dist/`。仅生成压缩包或交叉编译 ARM64 时：

```sh
python3 scripts/package.py --version v0.0.2 --arch aarch64
```

macOS / Windows 也可交叉编译：

```sh
python3 scripts/package.py --version v0.0.2 --os macos --arch aarch64
python3 scripts/package.py --version v0.0.2 --os windows --arch x86_64
```

macOS 本机打包时追加 `--dmg`，生成含 `Islander.app` 的磁盘映像；脚本会校验应用、挂载 DMG，并测试其中的启动器。`macOS DMG` 工作流可为已有 Release 补打 DMG，下载并校验原有 tar.gz 后封装，产物保存在 Actions artifacts 中。

推送 `v*` 标签会触发 GitHub Actions：在三个系统的两个架构上测试、构建并验证版本及启动入口，全部成功后发布四个 tar.gz、两个 AppImage、两个 DMG、两个 ZIP、两个 EXE 和校验文件。发布说明取自 `docs/releases/<版本>.md`。发布包只包含可执行文件、说明及必要的桌面资源。
