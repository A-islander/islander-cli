# X 岛访问笔记

核对日期：2026-09-06。网页入口：https://www.nmbxd1.com/Forum 。以下 GET 均通过本机匿名请求验证 HTTP 200 和响应结构；所有写入仅核对页面表单，**未提交**。

## 匿名 JSON 接口

基址 `https://api.nmb.best/api`，页码从 1 开始。

| 方法与路径 | 参数 / 用途 | 本次样本 |
|---|---|---|
| GET `/getForumList` | 分组和板块，含 `notice`、访问限制等信息 | 返回分组数组 |
| GET `/getTimelineList` | 时间线及 `max_page` | 综合线 ID 1 |
| GET `/getCDNPath` | CDN 地址和权重 `rate` | `https://image.nmb.best/` |
| GET `/showf` | `id=板块ID&page=1`，串列表与预览回复 | 板块 4 |
| GET `/timeline` | `id=时间线ID&page=1` | 时间线 1 |
| GET `/thread` | `id=主串ID&page=1`，主楼和当页回复 | 50000001 |
| GET `/po` | `id=主串ID&page=1`，只看 PO | 50000001 |
| GET `/ref` | `id=任意楼ID`，单楼引用查询 | 50019613 |

```sh
python3 read_forum.py x boards
python3 read_forum.py x timelines
python3 read_forum.py x timeline --id 1 --page 1
python3 read_forum.py x list --id 4 --page 1
python3 read_forum.py x thread --id 50000001 --page 1
python3 read_forum.py x post --id 50019613
```

列表 / 串字段有 `id`、`fid`、`ReplyCount`、`content`（HTML）、`now`、`user_hash`、`img`、`ext`、`Replies`；列表还可含 `RemainReplies`。部分数字以字符串返回，不能强依赖类型。辅助工具保留 JSON 结构并添加 `content_text` 等纯文本字段。

- `Replies` 的列表预览不是整串。详情需要分页，不能照搬岛民岛的 `hasMore`。参考总回复数、当页实际 ID、时间线 `max_page`；空页、重复页或达到上限就停止，不假定所有端点固定每页数量。
- API 可插入 `user_hash=Tips`、特殊 ID 的系统提示对象；不要把它当真实岛民来回话，也不要误把提示 ID 当发言目标。
- `/ref` 样本没有主串 ID。保持最初读取时的主串对应关系，不能拿回复 ID 充当 `resto`。
- 正文是 HTML，有换行、引用、隐藏内容等；不执行 HTML。引用可见 `>>No.编号`，回复时按网页支持的 `>>` 引用写法；跨站编号必须用完整 URL。
- 原图：CDN + `image/` + `img` + `ext`；缩略图：CDN + `thumb/` + `img` + `ext`。先取 CDN 表，图片无须带饼干。只有实际打开图片才算看过。

## 饼干与网页写入

敏感 Cookie 名是 `userhash`，页面的七位 `user_hash` 是公开显示身份。领取步骤见 CREDENTIALS。部分板块要求身份，匿名失败不表示 API 全站失效。

从当前 [综合版 1 页面](https://www.nmbxd1.com/f/%E7%BB%BC%E5%90%88%E7%89%881) 和 [主串页面](https://www.nmbxd1.com/t/50000001) 核对到：

| 操作 | 当前表单 action | 主要字段 |
|---|---|---|
| 发串 | POST `https://www.nmbxd1.com/Home/Forum/doPostThread.html` | `fid`、`content`；可选 `name`、`email`、`title`、`image` 文件、`water` |
| 回串 | POST `https://www.nmbxd1.com/Home/Forum/doReplyThread.html` | `resto` 主串 ID、`content`；其余同上 |

表单使用 `multipart/form-data`，含动态隐藏字段 `__hash__`。写入实现必须先以同一身份、同一 Cookie jar 打开目标页面，读取当前 action 和隐藏字段，不能复制本次研究页面中的 hash。网页还存在管理员控件，普通客户端不要提交 `isManager`。`email` 的特殊含义没有在本轮验证，不擅自设置。

这是网页表单，不承诺返回 JSON；HTTP 200 或重定向不等于发布成功。应检查业务错误并回读目标串确认，结果不明不重试。没有饼干，本轮没有验证鉴权、验证码、上传或实际发布。

## 当前版规优先

启动时查看 [总版规](https://www.nmbxd1.com/t/50000001)、主页及目标板块 `notice`。应进入相关板块，避免非必要身份介绍、刷屏、攻击和推广链接；不同板块发文间隔可能不同。不要借看板娘身份给岛民岛做广告，用户允许偶尔提身份不覆盖站点规则。

## 来源与历史资料

- 站点 [主页](https://www.nmbxd1.com/Forum)、[总版规与系统说明](https://www.nmbxd1.com/t/50000001)、上表各 API 的实际响应、当前发帖表单是这份笔记的依据。
- [原 API 作者的旧文档](https://www.zybuluo.com/ovear/note/151481) 属于历史接口资料，不能直接套用旧域名、旧 appid、领饼干接口。
- [xdcmd 作者整理的接口笔记](https://github.com/TransparentLC/xdcmd/wiki/自己整理的-X-岛匿名版-API-文档) 是客户端作者的一手研究，不是站方正式契约；本次可用性结论以上面的真实请求为准。
