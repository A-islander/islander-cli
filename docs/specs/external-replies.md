# X 岛 / BOG 回复接入

日期：2026-09-07；首发版本 v0.0.3，开发分支 `feat/persistent-forum-state`。

后续 v0.0.4 已补充新主串，见 [外站发串规格](external-threads.md)；以下范围描述对应 v0.0.3。

## 范围与交互

新增文字回复、引用回复、回复图片，复用站点隔离的饼干、草稿、编辑历史与显式发布预览。`r` 回复当前主串，`R` 按 `>>No.ID`（X）或 `>>Po.ID`（BOG）引用选中楼层；选中嵌套引用时，回复目标仍为正在阅读的主串。`Ctrl+A` 选择图片，`Ctrl+P` 预览，Enter 发送；失败回到编辑器并保留草稿。

CLI `reply create` 和 `draft publish` 同样支持外站回复；没有确认码或使用 `--dry-run` 只生成预览，不获取提交表单或上传。确认码绑定站点、endpoint、饼干别名、目标、正文与附件摘要（`reply create`）。新主串、外站“我的内容”、SAGE 和删除恢复不在本次范围。

`Capabilities.reply` 独立于 `publish`（新主串）；`CanPublish(reply)` 兼容既有岛民岛能力。可选的适配器 `ValidateDraft` 执行外站校验；X 的 `InlineFiles` 表明图片随最终提交发送，通用 Store 跳过独立上传步骤。BOG 继续使用逐张上传与检查点保存，成功提交才清除草稿。

## X 协议

1. 默认 `https://api.nmb.best/api/` 的写入网页为 `https://www.nmbxd1.com/`；自定义 endpoint 始终使用本实例。只使用当前站点的 `userhash`，不发送岛民岛 Authorization。
2. 每次确认后的提交创建独立 Cookie jar，GET `/t/主串ID`，保留网页返回的会话 Cookie。解析唯一 POST 回复表单，要求隐藏字段 `resto` 等于目标 ID、存在当前 `__hash__`、正文 textarea 可用。
3. 校验 action 为同源 `/Home/Forum/doReplyThread.html`。正文字段 `content`，可选 `title`，图片字段 `image`，最多一张；保留网页选中的水印设置，不提交管理员、email/SAGE 或名称字段。
4. 正文和图片在一次 multipart 请求中发送。独立上传链接无法复用于 X 回复。
5. 解析响应 `.error` / `.success`，仅明确的成功消息视为成功。认证、验证码、频率及其他拒绝映射为本地错误；不回显可能含 Cookie 的任意响应文本。

## BOG 协议

1. 当前身份为 `bog_master`、`bog_sel`，可附 `bog_list`。导入仍仅校验格式，不宣称真实账户已经验证。
2. GET `/t/主串ID/1`，要求唯一 `/post` 回复表单、`res` 等于目标主串、存在 `comment`。根据站点 JS，最终 URL 为 `/post/post`。
3. 图片以 multipart 的 `image` 字段先 POST `/post/upload`；只有 `code=200` 且有效 `pic` 才产生上传记录。记录绑定当前 endpoint 的图片预览 URL 和 `pic`；拒绝其他站点图片引用与带路径的非法 `pic`。
4. 回复使用 `application/x-www-form-urlencoded`：`res`、`comment`、可选标题和 `img[]`。不把本地板块导航哈希传成服务端 `forum` ID。
5. 回复成功码仅为 `1`；`101` 是验证码，`4` 是反垃圾暂停，`1000` 等为身份问题，`1101` 为目标不存在/锁定，`1102` 为重复内容。上传的 `200` 和读取的 `6001` 不能当成回复成功。

## 限制与失败恢复

- 沿用客户端正文 8192 字节、标题 128 字节、图片单张 20 MB 的保护上限，并进一步校验网页 maxlength；BOG 标题最多 50 个字符。这里的客户端上限不代表外站服务端配额。
- 图片只接受 JPEG、PNG、GIF、BMP，通过文件头识别。X 最多一张本地图片；BOG 客户端最多九张，最终受站点权限、数量和格式限制约束。
- 上传未成功不会提交回复。BOG 上传成功而回复失败时保留 `Media`，用户手动重试不会再次上传已保存的图片。X 原始文件路径始终保留，失败不会伪装成独立上传成功。
- 禁止跟随 GET/POST 重定向，目标 origin 不匹配、主串不匹配、表单缺失/重复、缺少校验字段或出现验证码输入都会中止提交。
- HTTP 200 本身不代表成功。POST 超时、连接失败、无法解析的响应、重定向等返回 `unknown_result`，保留草稿并要求先核对原串；不自动重试、不通过遍历全串猜测成功。
- 验证码需要在原站网页完成，本次没有内置验证码解题或挑战绕过。锁串、权限及发言间隔由服务端执行。
- 默认 X 写入域名是明确映射；自定义实例禁止回落到官方域名。凭证只存于原有凭证系统，表单会话 jar 仅在内存中存活，日志与 spec 不包含真实 Cookie。

## 验证与来源

本轮重新 GET 两站真实主串网页及 BOG 脚本，并将下载页面交给实际表单解析器验证。未向真实论坛发帖，也未验证真实账户的发布权限。

本地 HTTP fixture 覆盖 X 动态 hash/会话 Cookie/multipart 图片、BOG 上传与数组表单、目标及 action 检查、验证码与锁串错误、成功码区分、重定向不泄露、错误不回显凭证、不自动重试。Store 测试验证两站失败草稿恢复、BOG 上传结果复用及成功清理；TUI 测试验证外站引用目标、操作菜单、预览与身份选择；Cobra 测试验证预览和错误确认码零网络写入。

协议依据：[X 当前回复表单](https://www.nmbxd1.com/t/50000001)、[BOG 当前回复表单](https://bog.ac/t/1526630/1)、[BOG 官方前端脚本](https://bog.ac/static/js/script.js?20230911:2)。X `.success` 回复结果结构亦可参照[客户端增强脚本作者的实现](https://greasyfork.org/zh-CN/scripts/513156-增强x岛匿名版/code)；它不是站方接口保证。
