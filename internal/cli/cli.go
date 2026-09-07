package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/A-islander/islander-cli/internal/media"
	"github.com/A-islander/islander-cli/internal/tui"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

type app struct {
	f, u, dir, backend, cookie, output, images string
	demo                                       bool
	store                                      *local.Store
	client                                     *forum.Client
}

func (a *app) emit(v any) error {
	if a.output == "text" {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(forum.Clean(string(b)))
		return nil
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"schemaVersion": 1, "data": v})
}
func (a *app) auth() error {
	if a.cookie == "" || a.client.Token == "" {
		return errors.New("此命令需要 --cookie 明确指定饼干别名")
	}
	return nil
}
func (a *app) confirm(v any, provided string, dry bool) (bool, error) {
	b, e := json.Marshal(struct {
		Forum, User, Cookie string
		Payload             any
	}{a.f, a.u, a.cookie, v})
	if e != nil {
		return false, e
	}
	sum := sha256.Sum256(b)
	code := hex.EncodeToString(sum[:])
	if dry || provided == "" {
		return false, a.emit(map[string]any{"preview": v, "cookie": a.cookie, "confirmation": code, "submitted": false})
	}
	if provided != code {
		return false, errors.New("确认码与当前身份、目标或内容不符；请重新预览")
	}
	return true, nil
}
func Execute() int {
	a := &app{}
	root := a.root()
	if e := root.Execute(); e != nil {
		code := "invalid"
		exit := 2
		var apiErr *forum.Error
		if errors.As(e, &apiErr) {
			code = apiErr.Code
			exit = 4
			if code == "auth" {
				exit = 3
			} else if code == "business" {
				exit = 5
			}
		}
		msg := forum.Clean(e.Error())
		if a.client != nil && a.client.Token != "" {
			msg = strings.ReplaceAll(msg, a.client.Token, "[已隐藏]")
		}
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"schemaVersion": 1, "error": map[string]string{"code": code, "message": msg}})
		return exit
	}
	return 0
}
func (a *app) root() *cobra.Command {
	root := &cobra.Command{Use: "islander", Short: "岛民岛命令行客户端；islander tui 打开交互界面", SilenceUsage: true, SilenceErrors: true, Version: commandVersion(), Args: cobra.NoArgs,
		Example: "  islander board list\n  islander thread list --board 1 --page 1\n  islander thread get 20459\n  islander cookie import daily\n  islander mine list --cookie daily\n  islander tui"}
	pf := root.PersistentFlags()
	pf.StringVar(&a.f, "forum-url", forum.ForumURL, "论坛 API")
	pf.StringVar(&a.u, "user-url", forum.UserURL, "用户 API")
	pf.StringVar(&a.dir, "data-dir", "", "本地数据目录")
	pf.StringVar(&a.backend, "credential-store", "keyring", "keyring 或 file（显式选择 0600 明文文件）")
	pf.StringVar(&a.cookie, "cookie", "", "操作绑定的饼干别名；CLI 默认匿名")
	pf.StringVar(&a.output, "output", "json", "json 或 text")
	pf.StringVar(&a.images, "images", "auto", "auto、kitty、blocks 或 off")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd == root {
			return nil
		} // Help does not need a terminal, credentials or a data directory.
		if a.output != "json" && a.output != "text" {
			return errors.New("output 只能是 json 或 text")
		}
		if a.images != "auto" && a.images != "kitty" && a.images != "blocks" && a.images != "off" {
			return errors.New("images 只能是 auto、kitty、blocks 或 off")
		}
		var e error
		a.client, e = forum.New(a.f, a.u, "")
		if e != nil {
			return e
		}
		a.f, a.u = a.client.Forum, a.client.User
		a.store, e = local.New(a.dir, a.f, a.u, a.backend)
		if e != nil {
			return e
		}
		if a.cookie != "" {
			_, token, e := a.store.Identity(a.cookie)
			if e != nil {
				return e
			}
			a.client.Token = token
		}
		return nil
	}
	run := func(*cobra.Command, []string) error {
		return tui.Run(tui.Options{ForumURL: a.f, UserURL: a.u, Images: a.images, Cookie: a.cookie, Store: a.store, Demo: a.demo})
	}
	root.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	tc := &cobra.Command{Use: "tui", Short: "交互浏览论坛", Args: cobra.NoArgs, RunE: run}
	tc.Flags().BoolVar(&a.demo, "demo", false, "离线浏览体验")
	root.AddCommand(tc)
	hidden := &cobra.Command{Use: "_image URL MODE", Hidden: true, Args: cobra.ExactArgs(2), RunE: func(_ *cobra.Command, args []string) error { return media.Viewer(args[0], args[1]) }}
	root.AddCommand(hidden)
	board := &cobra.Command{Use: "board", Short: "查看论坛板块"}
	board.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		v, e := a.client.Boards(c.Context())
		if e != nil {
			return e
		}
		return a.emit(v)
	}})
	root.AddCommand(board)
	for _, kind := range []string{"thread", "reply", "mine", "sage"} {
		group := &cobra.Command{Use: kind, Short: map[string]string{"thread": "列串、读串、发新串", "reply": "读取回复、回复或引用回复", "mine": "查看指定饼干的内容", "sage": "查看 SAGE 内容"}[kind]}
		var page, id int
		list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			k := kind
			if k == "thread" {
				k = "timeline"
				if id > 0 {
					k = "board"
				}
			}
			if k == "reply" {
				k = "thread"
				if id <= 0 {
					return errors.New("请提供 --thread 编号")
				}
			}
			if k == "mine" {
				if e := a.auth(); e != nil {
					return e
				}
			}
			v, e := a.client.List(c.Context(), k, id, page)
			if e != nil {
				return e
			}
			return a.emit(v)
		}}
		list.Flags().IntVar(&page, "page", 1, "从 1 开始的页码")
		if kind == "thread" {
			list.Flags().IntVar(&id, "board", 0, "板块编号；默认时间线")
		}
		if kind == "reply" {
			list.Short = "读取主串的一页内容（服务端列表可能包含主楼）"
			list.Long = "读取主串的一页内容。list 可能包含主楼（followId=0），count 也包含主楼；统计纯回复时按 followId 过滤。页码从 1 开始，检查 hasMore 决定是否继续。旧帖的 replyArr 可能为空，识别引用时也应检查正文中的 No.编号。"
			list.Flags().IntVar(&id, "thread", 0, "主串编号")
		}
		group.AddCommand(list)
		if kind == "thread" {
			var detailPage int
			get := &cobra.Command{Use: "get ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
				id, e := positive(args[0])
				if e != nil {
					return e
				}
				p, e := a.client.Post(c.Context(), id)
				if e != nil {
					return e
				}
				page, e := a.client.List(c.Context(), "thread", p.ThreadID(), detailPage)
				if e != nil {
					return e
				}
				return a.emit(map[string]any{"post": p, "replies": page})
			}}
			get.Flags().IntVar(&detailPage, "page", 1, "回复页码")
			get.Short = "读取指定帖子及所在主串的一页内容"
			get.Long = "返回 post 和 replies 分页对象。replies.list 可能包含主楼（followId=0），与 post 重复；replies.count 也包含主楼。页码从 1 开始，hasMore=false 才表示本次读取到串尾。旧帖的 replyArr 可能为空，识别引用时也应检查正文中的 No.编号。附件元数据不代表已读取图片或视频内容。"
			group.AddCommand(get)
			group.AddCommand(a.composeCommand("create", false))
		}
		if kind == "reply" {
			group.AddCommand(a.composeCommand("create", true))
		}
		root.AddCommand(group)
	}
	post := &cobra.Command{Use: "post", Short: "按编号读取内容、SAGE、删除和恢复"}
	post.AddCommand(&cobra.Command{Use: "get ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		id, e := positive(args[0])
		if e != nil {
			return e
		}
		p, e := a.client.Post(c.Context(), id)
		if e != nil {
			return e
		}
		return a.emit(p)
	}})
	for _, action := range []string{"sage", "unsage", "delete", "restore"} {
		var confirm string
		var dry bool
		cmd := &cobra.Command{Use: action + " ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			if e := a.auth(); e != nil {
				return e
			}
			id, e := positive(args[0])
			if e != nil {
				return e
			}
			ok, e := a.confirm(map[string]any{"action": action, "postId": id}, confirm, dry)
			if e != nil || !ok {
				return e
			}
			if e = a.client.Action(c.Context(), action, id); e != nil {
				return e
			}
			return a.emit(map[string]any{"submitted": true})
		}}
		cmd.Flags().StringVar(&confirm, "confirm", "", "预览返回的内容确认码")
		cmd.Flags().BoolVar(&dry, "dry-run", false, "仅预览")
		post.AddCommand(cmd)
	}
	root.AddCommand(post)
	root.AddCommand(a.cookieCommands(), a.draftCommands())
	return root
}
func positive(s string) (int, error) {
	id, e := strconv.Atoi(strings.TrimPrefix(strings.ToLower(s), "no."))
	if e != nil || id <= 0 {
		return 0, errors.New("编号必须为正整数")
	}
	return id, nil
}
func readBody(path string) (string, error) {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, e := os.Open(path)
		if e != nil {
			return "", e
		}
		defer f.Close()
		r = f
	}
	b, e := io.ReadAll(io.LimitReader(r, 8193))
	if len(b) > 8192 {
		return "", errors.New("正文超过 8192 字节")
	}
	return string(b), e
}
func (a *app) composeCommand(use string, reply bool) *cobra.Command {
	var d forum.Draft
	var bodyFile, confirm string
	var dry bool
	var quote int
	cmd := &cobra.Command{Use: use, Short: "预览后使用 --confirm 发布", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if e := a.auth(); e != nil {
			return e
		}
		if bodyFile == "" {
			return errors.New("请用 --body-file 指定 UTF-8 正文文件，或 - 读取 stdin")
		}
		body, e := readBody(bodyFile)
		if e != nil {
			return e
		}
		d.Body = body
		d.Cookie = a.cookie
		if quote > 0 {
			d.Body = fmt.Sprintf("No.%d\n%s", quote, d.Body)
		}
		if e = d.Validate(); e != nil {
			return e
		}
		digests := []string{}
		for _, path := range d.Files {
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			h := sha256.New()
			n, e := io.Copy(h, io.LimitReader(f, forum.MaxFile+1))
			f.Close()
			if e != nil {
				return e
			}
			if n > forum.MaxFile {
				return errors.New("附件超过 20 MB")
			}
			digests = append(digests, hex.EncodeToString(h.Sum(nil)))
		}
		ok, e := a.confirm(map[string]any{"draft": d, "fileHashes": digests}, confirm, dry)
		if e != nil || !ok {
			return e
		}
		_, e = a.store.Publish(c.Context(), a.client, d)
		if e != nil {
			return e
		}
		return a.emit(map[string]bool{"submitted": true})
	}}
	f := cmd.Flags()
	f.StringVar(&bodyFile, "body-file", "", "正文文件，- 表示 stdin")
	f.StringVar(&d.Title, "title", "", "标题")
	f.StringSliceVar(&d.Files, "attach", nil, "本地附件，可重复")
	f.StringVar(&confirm, "confirm", "", "预览返回的确认码")
	f.BoolVar(&dry, "dry-run", false, "仅预览，不上传")
	if reply {
		f.IntVar(&d.ThreadID, "thread", 0, "回复目标串")
		f.IntVar(&quote, "quote", 0, "引用楼层编号")
	} else {
		f.IntVar(&d.BoardID, "board", 0, "目标板块")
	}
	return cmd
}
func (a *app) cookieCommands() *cobra.Command {
	group := &cobra.Command{Use: "cookie", Short: "导入、领取、切换和移除饼干"}
	group.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		v, e := a.store.Read()
		if e != nil {
			return e
		}
		return a.emit(v)
	}})
	group.AddCommand(&cobra.Command{Use: "use ALIAS", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		alias := args[0]
		if alias == "anonymous" {
			alias = ""
		}
		if e := a.store.Use(alias); e != nil {
			return e
		}
		return a.emit(map[string]string{"active": alias})
	}})
	var stdin bool
	imp := &cobra.Command{Use: "import ALIAS", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		var token []byte
		var e error
		if stdin {
			token, e = io.ReadAll(io.LimitReader(os.Stdin, 16385))
			if len(token) > 16384 {
				return errors.New("饼干过长")
			}
		} else {
			if !term.IsTerminal(os.Stdin.Fd()) {
				return errors.New("非交互导入请显式使用 --stdin")
			}
			fmt.Fprint(os.Stderr, "粘贴饼干（隐藏）：")
			token, e = term.ReadPassword(os.Stdin.Fd())
			fmt.Fprintln(os.Stderr)
		}
		if e != nil {
			return e
		}
		client, e := forum.New(a.f, a.u, strings.TrimSpace(string(token)))
		if e != nil {
			return e
		}
		user, e := client.Verify(c.Context())
		if e != nil {
			return e
		}
		if e = a.store.Import(args[0], client.Token, user); e != nil {
			return e
		}
		return a.emit(map[string]any{"alias": args[0], "user": user})
	}}
	imp.Flags().BoolVar(&stdin, "stdin", false, "从受控 stdin 读取饼干")
	group.AddCommand(imp)
	for _, action := range []string{"register", "remove"} {
		var confirm string
		cmd := &cobra.Command{Use: action + " ALIAS", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			alias := args[0]
			ok, e := a.confirm(map[string]string{"action": "cookie-" + action, "alias": alias}, confirm, false)
			if e != nil || !ok {
				return e
			}
			if action == "remove" {
				if e = a.store.Remove(alias); e != nil {
					return e
				}
			} else {
				token, e := a.client.Register(c.Context())
				if e != nil {
					return e
				}
				client, e := forum.New(a.f, a.u, token)
				if e != nil {
					return e
				}
				user, e := client.Verify(c.Context())
				if e != nil {
					return e
				}
				if e = a.store.Import(alias, token, user); e != nil {
					return e
				}
			}
			return a.emit(map[string]string{"alias": alias, "action": action})
		}}
		cmd.Flags().StringVar(&confirm, "confirm", "", "确认码")
		group.AddCommand(cmd)
	}
	return group
}
func (a *app) draftCommands() *cobra.Command {
	group := &cobra.Command{Use: "draft", Short: "保存、读取和发布本地草稿"}
	group.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		if e := a.auth(); e != nil {
			return e
		}
		ds, e := a.store.Drafts(a.cookie)
		if e != nil {
			return e
		}
		return a.emit(ds)
	}})
	var d forum.Draft
	var bodyFile string
	save := &cobra.Command{Use: "save", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		if e := a.auth(); e != nil {
			return e
		}
		body, e := readBody(bodyFile)
		if e != nil {
			return e
		}
		d.Body = body
		d.Cookie = a.cookie
		if e = a.store.SaveDraft(&d); e != nil {
			return e
		}
		return a.emit(d)
	}}
	save.Flags().StringVar(&bodyFile, "body-file", "", "正文文件")
	save.Flags().StringVar(&d.Title, "title", "", "标题")
	save.Flags().IntVar(&d.BoardID, "board", 0, "板块")
	save.Flags().IntVar(&d.ThreadID, "thread", 0, "回复目标")
	group.AddCommand(save)
	for _, action := range []string{"get", "remove", "publish"} {
		var confirm string
		cmd := &cobra.Command{Use: action + " ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			if e := a.auth(); e != nil {
				return e
			}
			drafts, e := a.store.Drafts(a.cookie)
			if e != nil {
				return e
			}
			for _, d := range drafts {
				if d.ID != args[0] {
					continue
				}
				if action == "get" {
					return a.emit(d)
				}
				if action == "publish" {
					if e = d.Validate(); e != nil {
						return e
					}
				}
				ok, e := a.confirm(map[string]any{"action": "draft-" + action, "draft": d}, confirm, false)
				if e != nil || !ok {
					return e
				}
				if action == "remove" {
					e = a.store.DeleteDraft(d.ID)
				} else {
					_, e = a.store.Publish(c.Context(), a.client, d)
				}
				if e != nil {
					return e
				}
				return a.emit(map[string]any{"action": action, "id": d.ID, "completed": true})
			}
			return errors.New("当前饼干没有这个草稿")
		}}
		cmd.Flags().StringVar(&confirm, "confirm", "", "预览返回的确认码")
		group.AddCommand(cmd)
	}
	return group
}
