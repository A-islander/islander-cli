# 岛民岛终端客户端

用于交互浏览的 **Bubble Tea TUI**，用于命令行与自动化的 **Cobra CLI**。共用论坛 API、饼干和草稿逻辑。TUI 延续原型的深海色分栏界面，窄窗口自动切为单栏。

当前为试用版本，源码仓库：[A-islander/islander-cli](https://github.com/A-islander/islander-cli)。

## 安装

当前版本为 **v0.0.9**。TUI 和 CLI 共用 `islander` 程序，`islander --version` 查看实际版本。

### 使用 Go 安装

需要 Go 1.26.5 或更新的工具链：

```sh
go install github.com/A-islander/islander-cli/cmd/islander@latest
islander --version
islander tui
```

安装指定版本：

```sh
go install github.com/A-islander/islander-cli/cmd/islander@v0.0.9
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
| Linux 压缩包 | `islander-v0.0.9-linux-x86_64.tar.gz` |
| Linux AppImage | `islander-v0.0.9-linux-x86_64.AppImage` |
| macOS 压缩包 | `islander-v0.0.9-macos-x86_64.tar.gz` |
| macOS DMG | `islander-v0.0.9-macos-x86_64.dmg` |
| Windows 压缩包 | `islander-v0.0.9-windows-x86_64.zip` |
| Windows 可执行文件 | `islander-v0.0.9-windows-x86_64.exe` |

ARM64（包括 Apple Silicon）对应文件名中的架构为 `aarch64`。Release 同时提供 `SHA256SUMS`，可在下载目录用 `sha256sum --ignore-missing -c SHA256SUMS` 校验。

Linux 压缩包解压后即可运行：

```sh
tar -xzf islander-v0.0.9-linux-x86_64.tar.gz
./islander --version
./islander tui
```

AppImage 在终端中启动；不带参数默认进入 TUI，带参数时转交 CLI：

```sh
chmod +x islander-v0.0.9-linux-x86_64.AppImage
./islander-v0.0.9-linux-x86_64.AppImage
./islander-v0.0.9-linux-x86_64.AppImage --version
./islander-v0.0.9-linux-x86_64.AppImage board list
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

直接运行 `islander` 显示 CLI 帮助；`islander tui` 打开交互界面，首次默认加载 X 岛时间线，之后沿用上次访问的岛，以访客身份浏览；选择饼干并确认后才能发布。`islander <命令> --help` 查看该命令的参数，例如 `islander reply create --help`。

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

进入首页时固定加载当前岛时间线第 1 页，选中第一条并从串第 1 页展示，不再自动返回上次的板块或串。之后手动选串时，右栏直接请求串的第一页（有阅读记录则读取上次页码），显示完整正文与本页全部回复，不再裁成摘要或限制五条。快速移动时取消旧请求；`Enter` / `Tab` 切到右栏阅读，已加载的页面直接复用。已有阅读记录会在右栏加载时恢复；`l` / `→` / `Enter` 进入、`h` / `←` 返回仅切换焦点，保留已加载页面和滚动位置。长串按页加载，可自动翻页或按 `P` 跳页。

TUI 按 **`g` 切换站点**，按 `b` 选板块；`[` / `]` 翻页，`P` 按页码跳转，边界处继续浏览可自动加载相邻页，`v` 原位展开引用，`a` 查看附件。各站点的饼干、草稿、收藏和阅读位置分开保存。TUI 记住上次访问的岛，启动时回到该岛时间线首页；历史和收藏仍可手动续读。首次启动默认 X 岛；已有站点偏好继续沿用。普通 CLI 默认仍为岛民岛。

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

v0.0.2 的外站能力为浏览。v0.0.3 已接入 X 岛 / BOG 的回复、引用回复和回复图片；v0.0.4 另外接入了两站的新主串。外站我的内容、SAGE、删除恢复和领取饼干尚未接入。`site info` 返回已实现的能力；不支持的 CLI 操作返回 `unsupported`，TUI 隐藏相应菜单。发串支持三个站点；管理操作仍仅适用于岛民岛。

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

## X 岛 / BOG 回复

从 v0.0.3 起，先按 `g` 切岛、`i` 导入或选择该岛饼干，然后打开主串：`r` 回复，`R` 引用选中帖子；`Ctrl+A` 选择图片，`Ctrl+P` 预览站点、身份、目标和正文，Enter 确认提交。v0.0.3 的外站能力为 `reply: true / publish: false`。v0.0.4 两项均为 `true`：按 `b` 选择具体板块，再按 `c` 发串；没有饼干时，饼干弹窗顶部会说明无法发串/回复，并提示导入或选择。CLI 使用 `board list` 中的编号运行 `thread create --board 编号 --cookie 别名 --body-file 正文文件`，先预览，再带确认码提交。详见 [外站发串规格](docs/specs/external-threads.md)。

CLI 示例（替换为你要回复的真实主串和引用编号）：

```sh
islander --site x --cookie daily reply create --thread 12345678 --quote 12345679 --body-file reply.txt --dry-run
islander --site bog --cookie daily reply create --thread 1234567 --body-file reply.txt --attach picture.png --dry-run
# 检查输出后，使用完全相同的参数，把 --dry-run 换为 --confirm <返回的确认码>
```

X 回复先读取目标网页的最新表单与校验字段，再提交正文和最多一张本地图片；默认 JSON API 对应的提交网页为 `https://www.nmbxd1.com/`。自定义 endpoint 的提交只访问该实例。BOG 使用表单回复，图片先上传，成功上传记录随草稿保留，回复失败后手动重试可复用。

客户端目前把正文限制为 8192 字节、图片单张 20 MB，支持 JPEG/PNG/GIF/BMP；这不是外站允许的全部额度。X 最多一张，BOG 客户端最多九张，实际还受站点图片权限与数量限制；标题同时受客户端和网页限制（BOG 最多 50 个字符），X 保留网页默认水印选项。

验证码需在原站网页完成，TUI/CLI 会保留草稿。遇到频率限制、锁串或饼干失效会报错；响应不明、超时或重定向不当作成功，不自动重发，先在原串核对。此功能通过真实网页 GET 和本地模拟提交验证，尚未用真实饼干向外站提交回复。实现边界见 [外站回复 spec](docs/specs/external-replies.md)。

## 常用按键

v0.0.4 修复长回复的 `j/k` 阅读，并新增编辑器 **`F3` 颜文字选择器**。选择器每行四个，用方向键 / `hjkl` 选择，Tab / Shift+Tab 选择下一个 / 上一个，Enter 在正文或标题的当前光标处插入，Esc 取消返回；按 Web / Flutter 原顺序收录 99 个颜文字，不附分类或说明；BOG 岛直接使用 Web 的方括号版本列表；插入后自动保存。见 [交互规格](docs/specs/reader-scroll-kaomoji.md)。

v0.0.3 新增状态保存：无参数站点的 `islander tui` 记住上次访问的岛，阅读位置可从历史和收藏恢复，`--site` 可显式覆盖；普通 CLI 的默认站点仍是岛民岛。

按 **`H` 查看当前岛 / 当前身份的浏览历史**，`/` 筛选，Enter 继续阅读，`x` 删除。菜单可清空或暂停记录；每份历史最多 500 条，保留 90 天。编辑时自动保存，**`F2` 查看和恢复最近 20 个编辑快照**。历史和草稿分别保存，清空历史不会删草稿。附件仍是原文件路径或已上传链接，不是文件备份。

**`*` 收藏 / 取消收藏当前主串，`F` 打开当前岛 / 当前身份的收藏列表**，`/` 筛选，Enter 续读，`x` 确认取消收藏。收藏在列表和正文标题中标为 `★`，永久保留，不随浏览历史过期。除启动时自动展示的首页第一条外，从普通列表、收藏、历史或主串编号重新打开都会恢复上次页码、选中楼层和楼内滚动位置。普通串的进度随历史保留 90 天 / 500 条；已收藏串的进度随收藏保留。清空历史会保留收藏项、清除其阅读进度；暂停历史也暂停自动更新收藏进度。收藏只存在本地，不改变论坛服务端状态。

数据根目录为 Linux 的 `~/.config/islander/`（支持 `XDG_CONFIG_HOME`）、macOS 的 `~/Library/Application Support/islander/`、Windows 的 `%AppData%/islander/`，也可用 `--data-dir` 指定。根目录的 `preferences.json` 保存当前岛和 `agentSimulation` 界面模式，各站哈希目录的 `browsing.json` 保存导航、历史和收藏，`draft-*.json` 保存草稿与快照。完整规则见 [状态持久化 spec](docs/specs/persistent-forum-state.md)。

`Ctrl+[` / `Ctrl+]` 切换当前岛的上一个 / 下一个板块（包含时间线，到边界停止），两种布局通用；终端需支持区分 `Ctrl+[` 与 Esc，不支持时可按 `b` 选择板块。图片浏览页内滚轮向上放大、向下缩小，串内滚轮仍滚动正文；点击串内小图进入原图浏览。

鼠标支持左侧单击标题行直接进入阅读（无标题时点击首行正文），其他卡片行单击选串、双击阅读；右侧单击选楼层、右键打开操作菜单，菜单打开后再次右键或点击外部关闭；滚轮跟随鼠标所在栏，并支持边界自动翻页。板块标签、我的内容、菜单项、引用提示、附件和页码均可点击。编辑器可用 `Shift+方向键` 选中文本、`Ctrl+G` 全选、`Ctrl+Shift+C` 复制选区、`Ctrl+V` 粘贴到光标处；复制粘贴依赖终端/系统剪贴板支持，若快捷键被终端截获，按终端配置执行。论坛正文尚无应用内鼠标选区复制功能。

顶部 **`F6 Agent模拟切换`** 按钮（或按 `F6`）切换为 Codex 风格的独立对话页面：深灰底、白灰文字，隐藏论坛顶栏、板块标签、标语和常驻快捷键提示，隐藏作者 ID、帖子编号、楼层和时间，以通用文件读取记录及随机穿插的 `Ran …` 工具输出模拟对话过程。命令块对命令、参数、路径、字符串和输出分别配色。输入区上方显示 `Working (4m 31s • esc to interrupt)`：进入帖子开始计时，Working 高光从左向右循环扫过，退出阅读停止。工具输出仅为预设文字，同次运行内位置稳定，不执行其中的命令；正文引用编号在浏览时显示为 `[reference]`，原始内容和回复目标保留。底部保留 `›` 输入区与模拟状态行 `gpt-6-astra high · ~/Develope/islander · Main [default]`。点击输入区或按 `r` 回复，`R` 引用选中楼层；Enter 换行，`Ctrl+P` 预览、确认页 Enter 发送，Esc 保存草稿，颜文字和附件选择沿用。按 **F6** 返回论坛，阅读位置与编辑内容保留；`b` 板块、`g` 切岛等按键在模拟页仍有效。模型、目录和分支文字是模拟装饰，不调用 AI，也不读取本机 Git 状态。模式切换立即保存到 `preferences.json`，下次启动沿用；再次切回论坛也会记住。Agent 模式启动或从列表切入时保持在帖子列表，后台加载第一条的串首页；单击卡片任意内容行进入阅读，复用已加载内容；点击顶部最右侧的 **`exit`** 返回串列表，保留列表和阅读位置。见 [鼠标和 Agent 模拟 spec](docs/specs/mouse-agent-style.md)。

顶部常驻 **`m 我的内容`** 按钮，可鼠标点击或按 `m` 打开当前饼干的发串与回复列表。访客会先进入饼干选择，选择／导入后继续打开。页面显示饼干别名；选中回复后按 `Enter` 定位原串，`Esc` 返回原来的列表位置。`[` / `]` 翻页，`i` 切换饼干。

v0.0.4 支持连续浏览：读到已加载内容底部继续按 `j/↓`、滚轮或 PgDn，自动接上下一页；回到顶部继续向上可补载上一页，保留已有内容并去重。`n/p` 直接换楼也可跨页。`P` 打开页码输入，显示当前页与已知总页数；总数未知时明确标注。窗口内 `Ctrl+Home` 首页，已知总数时 `Ctrl+End` 末页、串内 `Ctrl+L` 最新回复。加载失败保留原位置，用 `[ ]` 或 `P` 手动重试；本地筛选或页内最新排序时暂停列表自动加载。详见 [翻页规格](docs/specs/continuous-pagination.md)。

左侧选中的串卡片和右侧当前楼层都使用整块背景高亮，右侧正文、附件和引用提示一并高亮；回复不再预留左侧光标和竖线的位置，正文使用完整可用宽度。右栏顶部显示板块名和串号，保留帖子间距，帖子内部的编号、标题、正文、附件提示直接换行，不额外插入空行；正文中的连续换行及只含空格的空行在浏览时压为一次换行，原始正文和编辑内容保持不变；帖子末尾及底部状态栏上方也不额外留空行；左侧有标题的串占 4 行（标题、信息、摘要、间隔）；无标题的串用正文代替标题，占 3 行（正文、信息、间隔），列表按实际行数滚动和翻屏。用滚轮或 `↑↓ / j k` 浏览：每次切换一层，普通回复以约 150 ms 的平滑滚动移到视口中间；串头和串尾按实际内容边界停靠，不为居中额外留白。超过一屏的长楼层从开头显示，再逐行滚动，尾部完整显示后才进入下一条；向上返回长楼层时从尾部继续往上读。`n/p` 直接跳原串楼层；`PgUp/PgDn` 或空格可快速滚动。按 `Enter` 打开选中帖子的操作菜单，引用、SAGE、附件和删除等操作均针对该楼层，菜单和确认页会显示目标编号。

按 `v` 在帖子下方原位展开引用，多条引用会一起列出。用 `↓` 选中展开的引用，再按 `v` 可继续查看它的引用；选中父帖按 `v` 收起该分支，`Esc / ← / h` 从引用返回上层并恢复阅读位置。`↑↓ / j k` 遍历正文及展开的引用，`n/p` 只切换原串楼层。选中引用时，`R` 引用该帖并回复当前串，SAGE 和附件操作也对应选中的引用。循环引用不继续展开，最多展开 12 层。

| 按键 | 操作 |
|---|---|
| `g` | 切换岛民岛、X 岛、BOG；当前功能以站点能力为准 |
| `↑↓` / `j k` | 列表选串；阅读区先滚动当前长帖或引用，读完再换条 |
| `Enter` | 进入串详情；阅读时打开选中帖子的操作菜单 |
| `PgUp/PgDn` / 空格 | 滚动正文和原位引用；发布预览仍可用 `↑↓` 滚动 |
| `Esc` / `h` / `←` | 引用返回上层；原串返回列表，保留阅读位置 |
| `Tab` | 切换列表／正文焦点 |
| `n p` | 下一楼／上一楼 |
| `v` | 原位展开／收起选中帖子的引用，支持继续展开嵌套引用 |
| `:` | 按主楼或回复编号定位 |
| `[` / `]` | 前一页／后一页，页码从 1 开始 |
| `P` | 输入页码跳转，支持列表与串内 |
| `b` | 全部、各板块、SAGE、我的内容 |
| `F3`（编辑时） | 打开颜文字选择器，在当前光标处插入 |
| `H` / `F` | 当前岛与身份的历史 / 收藏（v0.0.3 起） |
| `*` | 收藏 / 取消收藏当前主串（v0.0.3 起） |
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

终端支持图片时，三个岛的右栏正文及展开的引用均显示小图，多图依次排列。`+` / `-` 调整小图大小（`=` 也可放大），已加载的图片复用缓存。小图靠近阅读位置时加载，优先使用站点缩略图；加载前后预留相同高度，避免正文跳动。

终端未确认支持图片时，右栏正文只显示 `a 加载附件`，不会自动下载图片或显示字符图。按 `a` 后才加载附件，必要时在附件窗口降级字符预览。列表焦点下的 `a` 包含主楼和已加载这一页回复的附件，左右键切换；进入串后仍只查看选中楼层或引用的附件。`--images blocks` 也只在手动打开附件时显示字符图。

从 v0.0.3 起，按 `a` 直接在 TUI 内打开选中帖子或引用的首张附件，无须先进入附件列表。窗口内可用：

- `+` / `=` 放大、`-` 缩小原图；`0` 恢复适应窗口（100%）。缩放复用已加载图片。
- `Shift + 方向键`：移动放大后的画面；`↑` / `↓` 也可上下移动。
- `←` / `→`（或 `h` / `l`）：前后切换附件，新图片从适应窗口开始。
- `Esc`：直接返回原帖，保留选中楼层和阅读位置。
- `r` / `Enter`：重新加载；`b`：切换为字符预览。
- `o`：通过系统默认应用打开，适合视频或其他文件。
- `s`：下载到 `~/Downloads/islander/`，使用新文件名，不覆盖已有文件。

可用 `--images auto|kitty|blocks|off` 手动控制。原图预览支持 Go 解码的 PNG、JPEG 和 GIF 首帧，限制 20 MB／3200 万像素；其他格式可以外部打开。视频、音频不在终端中播放。图片预览在当前 TUI 内异步加载。默认查询终端能力，确认支持后使用 Kitty 图形协议的 Unicode 占位方式显示；未确认支持时，仅在按 `a` 打开的附件窗口显示彩色字符预览。切换或关闭图片会取消旧请求并清理对应图片，窗口缩放自动适配。

TUI 的串内小图和原图默认使用磁盘缓存，换串、切岛和重启后可复用。按完整 URL 区分缓存，保存成功解码的源文件；小图与原图 URL 相同时只下载一次，不会用缩略图替代原图。默认上限 **256 MiB / 1024 张**，写入时清理 30 天未使用及最久未使用的图片。原图页按 `r` 强制重新下载；刷新失败保留已有缓存，普通重开仍可使用。缓存目录不可写时继续正常加载图片。

缓存位置：Linux 为 `$XDG_CACHE_HOME/islander/images`（通常 `~/.cache/islander/images`），macOS 为 `~/Library/Caches/islander/images`，Windows 为 `%LOCALAPPDATA%\islander\images`。指定 `--data-dir` 时使用该目录下的 `cache/images`。缓存是按 URL 哈希命名的图片文件，不写入浏览历史 JSON；删除缓存目录即可清空，下次查看会重新下载。详见 [图片缓存规格](docs/specs/image-cache.md)。

Ghostty / Kitty 配合 tmux 时，程序会识别外层终端，为当前窗格临时开启图片透传，退出时恢复原设置，不修改 `~/.tmux.conf`。tmux 内采用静默上传，避免等待无法回传的图形确认；其他兼容终端可用 `--images kitty` 指定。未知终端或透传不可用时保持附件文字提示，按 `a` 后再加载。若终端关闭了图片能力或使用嵌套 tmux，可使用 `islander --images blocks tui`。详见 [串内图片与 tmux 规格](docs/specs/inline-images-tmux.md)。

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
make release VERSION=v0.0.9
```

需要 Linux、Go、Python 3 和 `desktop-file-validate`。打包脚本下载并校验固定版本的 AppImage 工具与运行时；产物在 `dist/`。仅生成压缩包或交叉编译 ARM64 时：

```sh
python3 scripts/package.py --version v0.0.9 --arch aarch64
```

macOS / Windows 也可交叉编译：

```sh
python3 scripts/package.py --version v0.0.9 --os macos --arch aarch64
python3 scripts/package.py --version v0.0.9 --os windows --arch x86_64
```

macOS 本机打包时追加 `--dmg`，生成含 `Islander.app` 的磁盘映像；脚本会校验应用、挂载 DMG，并测试其中的启动器。`macOS DMG` 工作流可为已有 Release 补打 DMG，下载并校验原有 tar.gz 后封装，产物保存在 Actions artifacts 中。

推送 `v*` 标签会触发 GitHub Actions：在三个系统的两个架构上测试、构建并验证版本及启动入口，全部成功后发布四个 tar.gz、两个 AppImage、两个 DMG、两个 ZIP、两个 EXE 和校验文件。发布说明取自 `docs/releases/<版本>.md`。发布包只包含可执行文件、说明及必要的桌面资源。
