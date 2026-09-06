#!/usr/bin/env python3
"""Run an anonymous, draft-only Codex CLI reading trial of the live forum."""
import datetime
import os
import pathlib
import shutil
import subprocess
import tempfile


def main():
    repo = pathlib.Path(__file__).resolve().parents[1]
    executable = repo / "bin/islander"
    if not executable.is_file():
        raise SystemExit("先运行 make build 编译 CLI")
    if not shutil.which("codex"):
        raise SystemExit("需要本机已登录的 Codex CLI")
    base = repo / ".local/ai-cli-trial"
    base.mkdir(parents=True, exist_ok=True)
    run = pathlib.Path(tempfile.mkdtemp(
        prefix=datetime.datetime.now().strftime("%Y%m%d-%H%M%S-"), dir=base))
    shutil.copy2(executable, run / "islander")
    prompt = (repo / "docs/islander-poster-girl-cli.md").read_text()
    prompt += (
        "\n\n执行环境：当前目录的 ./islander 是真实 CLI。所有论坛命令使用 ./islander，"
        "匿名访问正式论坛。请在约 12 次 CLI 调用内完成；不要修改源码或新建工具，"
        "只用现有 CLI 和必要的 JSON 文本处理。最终用中文输出。\n")
    (run / "prompt.md").write_text(prompt)
    command = [
        "codex", "-a", "never", "exec", "--sandbox", "workspace-write",
        "-c", "sandbox_workspace_write.network_access=true",
        "--ephemeral", "--skip-git-repo-check", "--json", "--color", "never",
        "-C", str(run), "-o", str(run / "drafts.md"), "-",
    ]
    # Keep islander credentials and metadata separate from the user's TUI data.
    # Codex retains its existing login and configured model.
    env = dict(os.environ, XDG_CONFIG_HOME=str(run / "config"))
    print(f"试读记录与草稿：{run}", flush=True)
    with (run / "events.jsonl").open("w") as events, (run / "stderr.log").open("w") as err:
        result = subprocess.run(command, input=prompt, text=True, stdout=events,
                                stderr=err, env=env)
    if result.returncode:
        raise SystemExit(f"Codex 退出码 {result.returncode}；查看 {run / 'stderr.log'}")
    print(f"草稿已生成：{run / 'drafts.md'}")
    print("请核对 events.jsonl 中的实际命令；本轮提示词仅允许论坛读取和草稿输出。")


if __name__ == "__main__":
    main()
