# BOG 岛访问笔记

核对日期：2026-09-06。入口：[时间线](https://bog.ac/f/%E6%97%B6%E9%97%B4%E7%BA%BF)。匿名 GET 已实测，写接口仅核对站点 HTML 和公开 JS，**未提交**。

## 文档入口与当前可用路径

[官方 API 页](https://bog.ac/page/api) 指向 `https://easydoc.net/s/74385062`，本次请求握手超时，未取得接口文档；[站内反馈 No.1526630](https://bog.ac/t/1526630/1) 也有用户报告文档托管停止服务。这是站内反馈，不作为全部 API 不可用的证据。

因此不猜测完整 `/api2` 列表接口。以下据 [站点公开脚本](https://bog.ac/static/js/script.js?20230911:2)、当前网页表单和真实 GET 响应整理。

| 方法与路径 | 用途 | 验证情况 |
|---|---|---|
| GET `/f/时间线` 或 `/f/板块名/页码` | HTML 列表，板块中文需 URL 编码 | 时间线、综合版首页成功 |
| GET `/t/主串ID/页码` | HTML 主串和回复 | `/t/1526630/1` 成功 |
| GET `/api/thread/楼ID` | JSON 单楼查询 / 展开引用，**不是分页串详情** | `/api/thread/1526630` 返回 `code:6001` |
| GET `/page/set` | 领取 / 导入饼干页面 | 成功，未提交表单 |
| GET `/page/api` | 官方文档入口 | 成功，外部文档无法读取 |

```sh
python3 read_forum.py bog list --board 时间线
python3 read_forum.py bog list --board 综合版 --page 1
python3 read_forum.py bog thread --id 1526630 --page 1
python3 read_forum.py bog post --id 1526630
```

HTML 读取工具输出可见文本、页面链接和 `truncated` 标记。正文太长时增加 `--max-chars` 或按楼查引用；不要声称截断内容已读。下一页以页面实际链接为准；不能因为参数能构造就认定任意页存在。

单楼 JSON 形如 `{type, code, info}`，成功码 `6001`；本次 `info` 字段：`id,res,time,forum,name,emoji,cookie,admin,title,content,lock,images`。`cookie` 是显示身份，`content` 是 HTML，`res` 保存回复关联信息，主楼样本值需与回复样本区分；引用展开不能被误算成读取完整原串。

引用写法以页面的 `>>Po.编号` 为准，界面也显示 `#编号`。保留主串 URL 和具体楼 ID，不把其他岛的编号写成本岛引用。

附件由 `images[]` 的 `url` 和 `ext` 构造：

- 原图 `https://bog.ac/image/large/{url}{ext}`。
- 缩略图 `https://bog.ac/image/thumb/{url}.jpg`。
- 缺扩展名可能是缺失附件；不要臆造原图。图片读取不需要主饼干。

## 网页发串、回串、附件

当前页面 `<form action="/post">` 由 `formpost('post')` 拦截，实际 JS 请求 **POST `/post/post`**；它序列化表单字段，再附加 `img` 数组。不能只看 HTML action 就断定请求地址。

| 操作 | 路径 / 数据 | 依据 |
|---|---|---|
| 发串 | POST `/post/post`，`comment` 正文、`forum` 板块数值 ID | 综合版隐藏字段 `forum=1` |
| 回串 | POST `/post/post`，`comment` 正文、`res` 主串 ID | No.1526630 表单 |
| 可选署名/标题 | `name`、`title` | 页面默认 disabled，不能自行假定可用 |
| 上传 | POST `/post/upload`，multipart 文件字段 `image` | 站点 JS，返回 `code=200` 时取 `pic` |
| 附件关联 | 正文表单追加 `img` 数组，元素为上传返回的 `pic` | jQuery 表单序列化，需保留数组语义 |

网页正文提交是 URL 编码表单，不是臆测的 JSON body。写入须保留浏览器实际 Cookie jar、选中身份、当前表单隐藏字段与当前校验要求。时间线只供浏览，不能发主串；新串应先选择具体板块。

`/post/*` 的前端成功码通常是 `1`（重载页面）；上传成功码是 `200`；单楼读取是 `6001`，不要混用。脚本还处理 `4` 反垃圾暂停、`101` 验证码、`1000/1001` 无饼干、`1002` 无效、`1004` 影武者不存在、`1005` 图片过多、`1008` 图片权限不足、`1101` 目标不存在/锁定、`1102` 近期重复、`1103` 板块禁止发主题。这里只列核对到的常见值，不承诺覆盖所有响应。

任何实际写入都需要后续带专用身份联调。目前不提供 BOG 自动投稿器；不能只因查到路由就称发帖已经可用。

## 饼干与其他 API

设置页声明饼干格式为 `8位ID#32位密钥`。脚本中 `bog_master` 为主饼干，`bog_sel` 为选中显示 ID，`bog_list` 为影武者列表；格式与导入建议见 CREDENTIALS。

脚本还使用 POST `/api2/userinfo`，提交 `{cookie:主ID, code:密钥}` 来读取账户信息；本轮未带凭证测试。POST `/api2/cookieShadow` 会生成影武者，其他 `/post/cookie*` 可领取、导入或删除身份；它们不是匿名浏览接口，不要为研究效果调用。表情接口可能消耗积分，也不属于普通回串。

版规以当前首页、板块说明、置顶和站务公告为准；此次未找到可用的完整官方 API 文档，不把旧客户端经验写成站方保证。
