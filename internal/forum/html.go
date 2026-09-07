package forum

import (
	"golang.org/x/net/html"
	"strings"
)

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func class(n *html.Node, c string) bool {
	for _, v := range strings.Fields(attr(n, "class")) {
		if v == c {
			return true
		}
	}
	return false
}
func walk(n *html.Node, fn func(*html.Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walk(child, fn)
	}
}
func firstClass(n *html.Node, c string) *html.Node {
	var found *html.Node
	walk(n, func(v *html.Node) bool {
		if found != nil {
			return false
		}
		if class(v, c) {
			found = v
			return false
		}
		return true
	})
	return found
}

func HTMLText(s string) string {
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return ""
	}
	return nodeText(n)
}
func nodeText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(v *html.Node) {
		if v == nil {
			return
		}
		if v.Type == html.ElementNode {
			switch v.Data {
			case "script", "style", "noscript", "template":
				return
			case "br":
				b.WriteByte('\n')
				return
			case "img":
				b.WriteString(attr(v, "alt"))
				return
			}
			if class(v, "item-content-shadow") {
				return
			}
		}
		if v.Type == html.TextNode {
			b.WriteString(v.Data)
		}
		for ch := v.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
		if v.Data == "p" || v.Data == "div" || v.Data == "li" {
			b.WriteByte('\n')
		}
	}
	visit(n)
	return Clean(strings.TrimSpace(b.String()))
}
func innerHTML(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		_ = html.Render(&b, ch)
	}
	return b.String()
}
