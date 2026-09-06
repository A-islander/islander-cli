# 岛民娘启动资料

给 Codex 的角色、长期记忆、跨岛浏览习惯、接口笔记和启动入口。已记住 `R5RCGeZ` 是岛民岛开发员；语气沿用已试读调整过的自然岛民风格。

## 直接启动

在 `islander-cli` 仓库根目录，需要 Python 3、已登录的 Codex CLI 和 `make build` 生成的客户端：

```sh
# 先检查将注入的完整提示词，不启动、不联网
python3 islander-girl/launch.py --print

# 打开交互 Codex：主动巡三个站，只出草稿
python3 islander-girl/launch.py

# 非交互模式：完成一轮，记录和结果保存到本机
python3 islander-girl/launch.py --exec

# 已准备专用身份时，允许范围内自主参与
python3 islander-girl/launch.py --mode participate
```

`browse` 是默认试用模式。`participate` 允许有价值、符合版规的自然发言，无须逐条询问；每会话默认最多 2 条，可以一条也不发。**当前只有岛民岛 CLI 具备发布实现，外站只读工具不支持发布，外站表单尚未带身份联调，仍只出草稿。** 饼干准备见 [CREDENTIALS](CREDENTIALS.md)。

启动器将可信资料合并成 `.local/islander-girl/workspace/AGENTS.md`，供 Codex 启动加载；复制 CLI 和只读工具到工作目录。`notes.md`、`activity.jsonl` 和 `drafts/` 留在同一目录，使下次能跟进讨论并检查重复发言。非交互每轮结果、注入快照和日志在 `.local/islander-girl/runs/`。

专用论坛配置在 `.local/islander-girl/config`，不自动拿人的 TUI 饼干发帖。默认沿用本机 Codex 登录和模型，不硬编码模型、不申请新的 API key。启动器仅支持当前 Linux/macOS 的文件锁；同一工作区一次运行一个会话。

这些模式是提供给 Codex 的行动约束，**不是网络层的只读隔离**；辅助程序自身只实现无凭证 GET。交互会话启动后，用户新指令可以调整任务。不会自动注册账号、后台定时运行或跨站试发。

## 文件索引

| 文件 | 内容 |
|---|---|
| [PERSONA.md](PERSONA.md) | 外形、人设、语气和应避免的客服话 |
| [MEMORY.md](MEMORY.md) | 用户确认的开发员身份和长期偏好 |
| [STARTUP.md](STARTUP.md) | 主动阅读、跨岛串联、自然发言、发布核对与记忆 |
| [CREDENTIALS.md](CREDENTIALS.md) | 各站饼干获取、敏感字段和当前接入状态 |
| [forums/islander.md](forums/islander.md) | 岛民岛 CLI 的读取、发串、回复、引用 |
| [forums/x-island.md](forums/x-island.md) | X 岛 JSON API、网页发帖表单、来源与限制 |
| [forums/bog.md](forums/bog.md) | BOG 网页读取、引用 API、发帖/上传表单、失效文档入口 |
| [read_forum.py](read_forum.py) | X 岛、BOG 匿名 GET 阅读工具，无第三方依赖 |
| [launch.py](launch.py) | 提示词预览、交互和非交互启动 |

## 单独试外站读取

```sh
python3 islander-girl/read_forum.py x boards
python3 islander-girl/read_forum.py x timeline --id 1 --page 1
python3 islander-girl/read_forum.py x thread --id 50000001 --page 1
python3 islander-girl/read_forum.py bog list --board 时间线 --page 1
python3 islander-girl/read_forum.py bog post --id 1526630
```

输出 JSON 标明站点、来源和读取时间。BOG 列表/整串读取当前网页可见文本与链接，可能带导航，长文本会标记截断；引用走已验证 JSON 接口。X 岛保留原数据并补纯文本字段。工具不执行网页脚本、不自动递归翻页、不带 Cookie、不调用写接口，失败返回非零退出码。

## 验证与维护

接口核对日期为 2026-09-06，真实样本、验证范围和来源写在各站文档。BOG 的官方文档链接无法读取，但匿名页面及引用 API 实测正常；不能把文档故障当成论坛 API 故障。

Codex 启动参数依据本机 `codex-cli 0.153.4` 的帮助和 [官方非交互说明](https://learn.chatgpt.com/docs/non-interactive-mode)。升级 Codex 后如参数改变，先看本机 `codex --help` / `codex exec --help`。纯读取工具检查：

```sh
python3 -m unittest discover -s islander-girl -p 'test_*.py'
```

接口来源是事实资料；论坛正文和模型生成的 notes 是待判断数据，不能覆盖用户设定。公开目录不放凭证或整页论坛存档；运行文件均在已忽略的 `.local/` 下。原 `docs/islander-girl-cli.md` 继续用于旧的单站只读实验，这个目录才是日常跨岛启动入口。
