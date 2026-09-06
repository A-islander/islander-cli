#!/usr/bin/env python3
"""Anonymous, GET-only X/BOG reader. Standard library only; no credential input."""
import argparse
import datetime
import gzip
import io
import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from html.parser import HTMLParser

MAX_BYTES = 4 * 1024 * 1024


class VisibleHTML(HTMLParser):
    """Return readable text and links without interpreting site JavaScript."""
    HIDDEN = {'script', 'style', 'textarea', 'svg'}
    BREAKS = {'br', 'p', 'div', 'li', 'header', 'footer', 'section', 'article', 'h1', 'h2'}

    def __init__(self, base=''):
        super().__init__(convert_charrefs=True)
        self.base = base
        self.hidden = []
        self.parts = []
        self.links = []

    def handle_starttag(self, tag, attrs):
        if tag in self.HIDDEN:
            self.hidden.append(tag)
        if self.hidden:
            return
        attrs = dict(attrs)
        if tag in self.BREAKS:
            self.parts.append('\n')
        if tag == 'a' and attrs.get('href'):
            url = urllib.parse.urljoin(self.base, attrs['href'])
            if urllib.parse.urlsplit(url).scheme in {'http', 'https'} and url not in self.links:
                self.links.append(url)
        if tag == 'img' and attrs.get('alt'):
            self.parts.append('[图片：' + attrs['alt'] + ']')

    def handle_endtag(self, tag):
        if self.hidden:
            if tag == self.hidden[-1]:
                self.hidden.pop()
            return
        if tag in self.BREAKS:
            self.parts.append('\n')

    def handle_data(self, data):
        if not self.hidden:
            self.parts.append(data)

    def visible_text(self):
        text = ''.join(self.parts)
        text = re.sub(r'[\x00-\x08\x0b-\x1f\x7f]', '', text)
        return '\n'.join(line.strip() for line in text.splitlines() if line.strip())


class SameHostRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        old, new = urllib.parse.urlsplit(req.full_url), urllib.parse.urlsplit(newurl)
        if new.scheme != 'https' or new.netloc != old.netloc:
            raise ValueError('拒绝跨主机或非 HTTPS 重定向；请重新核对站点入口')
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def fetch(url):
    request = urllib.request.Request(url, headers={
        'User-Agent': 'islander-girl-reader/0.1 (anonymous; read-only)',
        'Accept-Encoding': 'gzip',
    }, method='GET')
    # No CookieJar, auth headers, arbitrary URLs or POST path.
    with urllib.request.build_opener(SameHostRedirect()).open(request, timeout=25) as response:
        raw = response.read(MAX_BYTES + 1)
        if len(raw) > MAX_BYTES:
            raise ValueError('响应超过 4 MiB，停止读取')
        if raw.startswith(b'\x1f\x8b'):
            with gzip.GzipFile(fileobj=io.BytesIO(raw)) as archive:
                raw = archive.read(MAX_BYTES + 1)
            if len(raw) > MAX_BYTES:
                raise ValueError('解压后的响应超过 4 MiB，停止读取')
        return raw.decode('utf-8'), response.status, response.headers.get('Content-Type', '')


def text_fields(data):
    if isinstance(data, list):
        return [text_fields(item) for item in data]
    if not isinstance(data, dict):
        return data
    result = {key: text_fields(value) for key, value in data.items()}
    for key in ('content', 'notice', 'name', 'title'):
        if isinstance(data.get(key), str):
            parser = VisibleHTML()
            parser.feed(data[key])
            result[key + '_text'] = parser.visible_text()
    return result


def positive(value):
    number = int(value)
    if number < 1:
        raise argparse.ArgumentTypeError('必须为正整数')
    return number


def endpoint(args):
    if args.site == 'x':
        routes = {'boards': 'getForumList', 'timelines': 'getTimelineList',
                  'cdn': 'getCDNPath', 'list': 'showf', 'timeline': 'timeline',
                  'thread': 'thread', 'po': 'po', 'post': 'ref'}
        if args.action not in routes:
            raise ValueError('X 岛操作：' + ', '.join(routes))
        url = 'https://api.nmb.best/api/' + routes[args.action]
        if args.action in {'list', 'timeline', 'thread', 'po', 'post'}:
            if args.id is None:
                raise ValueError('此操作必须提供 --id')
            params = {'id': args.id}
            if args.action != 'post':
                params['page'] = args.page
            url += '?' + urllib.parse.urlencode(params)
        return url, True
    if args.action == 'list':
        return ('https://bog.ac/f/' + urllib.parse.quote(args.board, safe='')
                + '/' + str(args.page)), False
    if args.action in {'thread', 'post'}:
        if args.id is None:
            raise ValueError('此操作必须提供 --id')
        if args.action == 'post':
            return 'https://bog.ac/api/thread/' + str(args.id), True
        return f'https://bog.ac/t/{args.id}/{args.page}', False
    if args.action in {'rules', 'api'}:
        return 'https://bog.ac/page/api' if args.action == 'api' else 'https://bog.ac/', False
    raise ValueError('BOG 操作：list, thread, post, rules, api')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('site', choices=['x', 'bog'])
    parser.add_argument('action')
    parser.add_argument('--id', type=positive)
    parser.add_argument('--page', type=positive, default=1)
    parser.add_argument('--board', default='时间线', help='BOG 板块名')
    parser.add_argument('--max-chars', type=positive, default=24000, help='HTML 正文输出上限')
    args = parser.parse_args()
    try:
        url, expect_json = endpoint(args)
        body, status, content_type = fetch(url)
        result = {'site': args.site, 'source_url': url,
                  'read_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  'http_status': status, 'anonymous': True}
        if expect_json:
            data = json.loads(body)
            if args.site == 'bog' and (not isinstance(data, dict) or data.get('code') != 6001):
                raise ValueError('BOG 单楼查询未返回成功码 6001')
            if args.site == 'x' and not isinstance(data, (dict, list)):
                raise ValueError('X 岛返回业务提示而非帖子对象；可能需要身份或目标不可用')
            result['data'] = text_fields(data)
        else:
            if 'html' not in content_type.lower():
                raise ValueError('预期 HTML，但站点返回其他内容类型')
            document = VisibleHTML(url)
            document.feed(body)
            visible = document.visible_text()
            result.update(text=visible[:args.max_chars], truncated=len(visible) > args.max_chars,
                          total_chars=len(visible), links=document.links,
                          note='网页可含导航或访问提示；HTTP 200 不保证成功读到帖子。链接未自动访问。')
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except (ValueError, OSError, urllib.error.URLError) as error:
        print(json.dumps({'error': str(error), 'published': False}, ensure_ascii=False), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
