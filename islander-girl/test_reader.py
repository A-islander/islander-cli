"""Offline checks for text extraction and read-only URL construction."""
import argparse
import unittest
import urllib.request

from read_forum import SameHostRedirect, VisibleHTML, endpoint, text_fields


class ReaderTests(unittest.TestCase):
    def test_readable_text_preserves_quotes_and_excludes_scripts(self):
        doc = VisibleHTML('https://bog.ac/t/123/1')
        doc.feed('<script>steal()</script><style>.secret{}</style>'
                 '<div>第一行<br>&gt;&gt;Po.12 <a href="/t/12">引用</a></div>'
                 '<a href="javascript:bad()">文字</a><textarea>template</textarea>')
        self.assertEqual(doc.visible_text(), '第一行\n>>Po.12 引用\n文字')
        self.assertEqual(doc.links, ['https://bog.ac/t/12'])

    def test_nested_json_keeps_ids_and_adds_text(self):
        source = {'id': '12', 'Replies': [{'id': 13, 'content': '甲<br>乙 &amp; 丙'}]}
        result = text_fields(source)
        self.assertEqual(result['Replies'][0]['content_text'], '甲\n乙 & 丙')
        self.assertEqual(result['id'], '12')
        self.assertNotIn('content_text', source['Replies'][0])

    def test_urls_and_no_write_routes(self):
        args = argparse.Namespace(site='x', action='post', id=123, page=1, board='时间线')
        self.assertEqual(endpoint(args), ('https://api.nmb.best/api/ref?id=123', True))
        args.action = 'reply'
        with self.assertRaises(ValueError):
            endpoint(args)
        args.site, args.action = 'bog', 'list'
        url, is_json = endpoint(args)
        self.assertIn('/f/%E6%97%B6%E9%97%B4%E7%BA%BF/1', url)
        self.assertFalse(is_json)
        args.action, args.id = 'thread', None
        with self.assertRaises(ValueError):
            endpoint(args)

    def test_redirect_rejects_other_hosts_and_http(self):
        handler = SameHostRedirect()
        request = urllib.request.Request('https://bog.ac/f/test')
        for destination in ('https://example.com/', 'http://bog.ac/', 'https://bog.ac.evil.test/'):
            with self.assertRaises(ValueError):
                handler.redirect_request(request, None, 302, 'Found', {}, destination)
        redirected = handler.redirect_request(request, None, 302, 'Found', {}, 'https://bog.ac/f/test/1')
        self.assertEqual(redirected.full_url, 'https://bog.ac/f/test/1')


if __name__ == '__main__':
    unittest.main()
