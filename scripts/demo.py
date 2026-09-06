#!/usr/bin/env python3
"""Local-only forum for trying posting, quoting, identities and attachments."""
import struct
import zlib
import json
import pathlib
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

def sample_png():
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xffffffff)
    rows = bytearray()
    for y in range(80):
        rows.append(0)
        for x in range(160):
            color = (120, 200, 195) if y < 40 else (24, 94 + y, 132)
            if (x - 118) ** 2 + (y - 22) ** 2 < 100:
                color = (245, 208, 130)
            rows.extend(color)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 160, 80, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b""))


PNG = sample_png()


def make_post(id, body, follow=0, **extra):
    return dict(id=id, value=body, followId=follow, plateId=1, status=0,
                userId=1, name="试用饼干", time=int(time.time()), title="",
                mediaUrl="[]", replyArr=[], replyCount=0, **extra)


class DemoHandler(BaseHTTPRequestHandler):
    posts = []
    lock = threading.Lock()
    counters = {"publish": 0, "upload": 0, "action": 0}

    def log_message(self, *args):
        pass

    def respond(self, data=None, code=200, message=""):
        body = json.dumps(dict(code=code, msg=message, data=data), ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        with self.lock:
            self.handle_get()

    def handle_get(self):
        u = urlparse(self.path)
        q = parse_qs(u.query)
        number = lambda key, default=0: int(q.get(key, [default])[0])
        if u.path == "/image.png":
            self.send_response(200)
            self.send_header("Content-Type", "image/png")
            self.end_headers()
            self.wfile.write(PNG)
            return
        if u.path == "/user/get":
            return self.respond(dict(id=1, name="试用饼干"))
        if u.path == "/user/register":
            return self.respond(dict(token="demo-token"))
        if u.path == "/plate/get":
            return self.respond([dict(id=1, name="综合版", status=0, value="本地试用"),
                                 dict(id=2, name="测试版", status=0, value="随便试")])
        if u.path == "/forum/get":
            p = next((p for p in self.posts if p["id"] == number("postId")), None)
            return self.respond(p, 200 if p else 404, "内容不存在" if not p else "")
        if u.path == "/forum/postPage":
            posts = [p for p in self.posts if p["id"] == number("postId") or p["followId"] == number("postId")]
            index = next((i for i, p in enumerate(posts) if p["id"] == number("replyId")), 0)
            return self.respond(dict(page=index // 20 + 1, floor=index))
        if u.path in ("/forum/sage/add", "/forum/sage/sub", "/forum/delete/ownPost", "/forum/recover/ownPost"):
            p = next((p for p in self.posts if p["id"] == number("postId")), None)
            if not p:
                return self.respond(code=404, message="内容不存在")
            self.counters["action"] += 1
            p["status"] = 2 if "delete" in u.path else 1 if u.path.endswith("add") else 0
            return self.respond(dict(status=True) if "ownPost" in u.path else True)
        if u.path in ("/forum/index", "/forum/indexLast", "/forum/list", "/forum/userList", "/forum/sage/list"):
            posts = list(self.posts)
            if u.path == "/forum/list":
                posts = [p for p in posts if p["id"] == number("postId") or p["followId"] == number("postId")]
            elif u.path == "/forum/sage/list":
                posts = [p for p in posts if p["status"] == 1]
            elif u.path != "/forum/userList":
                posts = [p for p in posts if p["followId"] == 0 and p["status"] != 2]
                if u.path == "/forum/index":
                    posts = [p for p in posts if p["plateId"] == number("plateId")]
            page = number("page")
            return self.respond(dict(list=posts[page * 20:(page + 1) * 20], count=len(posts)))
        return self.respond(code=404, message="未实现的模拟接口")

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        with self.lock:
            if self.path == "/img/upload":
                self.counters["upload"] += 1
                return self.respond(dict(success=True, RequestId="demo-image", data=dict(url=self.server.base_url + "/image.png")))
            if self.path in ("/forum/post", "/forum/reply"):
                data = json.loads(body)
                id = max(p["id"] for p in self.posts) + 1
                p = make_post(id, data["value"], data.get("followId", 0))
                p.update(title=data.get("title", ""), plateId=data.get("plateId", 1),
                         mediaUrl=data.get("mediaUrl", "[]"), replyArr=data.get("replyArr", []))
                self.posts.append(p)
                if p["followId"]:
                    for parent in self.posts:
                        if parent["id"] == p["followId"]:
                            parent["replyCount"] += 1
                self.counters["publish"] += 1
                return self.respond()
            return self.respond(code=404)


def start_server():
    server = ThreadingHTTPServer(("127.0.0.1", 0), DemoHandler)
    server.base_url = f"http://127.0.0.1:{server.server_port}"
    DemoHandler.posts = [make_post(100, "这里是本地试用岛。可以放心发串、回复、引用回复、删除和恢复。\n\n试试 r 回复，R 引用当前楼层，i 管理饼干。所有内容退出后消失。")]
    DemoHandler.posts[0].update(title="欢迎来试用终端版岛民岛", replyCount=24)
    for i in range(1, 25):
        p = make_post(100 + i, f"第 {i} 条模拟回复。\nNo.100\n可以翻到第二页，再跳转回来。", 100)
        p["replyArr"] = [100]
        DemoHandler.posts.append(p)
    DemoHandler.posts[1]["mediaUrl"] = json.dumps([dict(url=server.base_url + "/image.png", type="image")])
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


def main():
    server = start_server()
    executable = pathlib.Path(__file__).resolve().parents[1] / "bin/islander"
    try:
        with tempfile.TemporaryDirectory(prefix="islander-demo-") as data:
            flags = ["--forum-url", server.base_url, "--user-url", server.base_url,
                     "--data-dir", data, "--credential-store", "file"]
            subprocess.run([str(executable), *flags, "cookie", "import", "demo", "--stdin"],
                           input=b"demo-token", stdout=subprocess.DEVNULL, check=True)
            subprocess.run([str(executable), *flags, "tui"], check=True)
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    main()
