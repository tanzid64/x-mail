// Package sanitize cleans inbound email HTML before it is stored or rendered.
//
// Email HTML is attacker-controlled. The rendering frame already runs with
// scripts disabled, but defence here means a mistake in the frame's sandbox
// attributes is not immediately exploitable, and that what we store is safe to
// hand to anything else later — a search index, an export, a mobile client.
package sanitize

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// policy is built once: compiling it per message would be wasteful, and it is
// safe for concurrent use.
var policy = buildPolicy()

func buildPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// Structure and text.
	p.AllowElements(
		"p", "br", "div", "span", "hr",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"strong", "b", "em", "i", "u", "s", "strike", "del", "ins", "sub", "sup",
		"blockquote", "pre", "code", "small", "abbr", "cite", "q",
		"ul", "ol", "li", "dl", "dt", "dd",
		"table", "thead", "tbody", "tfoot", "tr", "td", "th", "caption", "colgroup", "col",
		"figure", "figcaption", "address", "center", "font",
	)

	// Links, forced through http(s) and mailto only — a javascript: or data:
	// href is a script that runs on click.
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	// Images. cid: is allowed so inline attachments can be rewritten to point
	// at our own authorized download endpoint.
	p.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	p.AllowURLSchemeWithCustomPolicy("cid", func(*url.URL) bool { return true })

	// Presentation. Style is allowed but filtered to a safe property list
	// below, because most real email is unreadable without it.
	p.AllowAttrs("align", "valign", "width", "height", "bgcolor", "color", "face", "size").Globally()
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowAttrs("border", "cellpadding", "cellspacing").OnElements("table")
	p.AllowAttrs("class", "dir", "lang", "title").Globally()

	p.AllowStyles(
		"color", "background-color", "background",
		"font-size", "font-family", "font-weight", "font-style", "text-align",
		"text-decoration", "line-height", "letter-spacing",
		"margin", "margin-top", "margin-bottom", "margin-left", "margin-right",
		"padding", "padding-top", "padding-bottom", "padding-left", "padding-right",
		"border", "border-top", "border-bottom", "border-left", "border-right",
		"border-radius", "border-color", "border-width", "border-style",
		"width", "max-width", "height", "vertical-align", "display",
	).Globally()

	// Everything not listed is dropped, including script, iframe, object,
	// embed, form, style, link, meta and every on* handler.
	return p
}

// expressionPattern catches the legacy IE `expression(...)` CSS vector and
// url(javascript:...) smuggling, which are not properties bluemonday inspects.
var expressionPattern = regexp.MustCompile(`(?i)(expression\s*\(|javascript\s*:|vbscript\s*:)`)

// HTML returns a sanitised copy of an email body, safe to store and render.
func HTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	cleaned := policy.Sanitize(input)
	// A second pass over the result, since a surviving style value could still
	// carry one of these.
	return expressionPattern.ReplaceAllString(cleaned, "")
}

// TrackingPixels strips 1x1 images, the usual read-receipt beacons.
//
// It is separate from HTML because whether to block them is a user preference,
// not a safety question.
var trackingPixel = regexp.MustCompile(
	`(?i)<img[^>]*(width\s*=\s*["']?1["']?[^>]*height\s*=\s*["']?1["']?|height\s*=\s*["']?1["']?[^>]*width\s*=\s*["']?1["']?)[^>]*>`)

func TrackingPixels(input string) string {
	return trackingPixel.ReplaceAllString(input, "")
}
