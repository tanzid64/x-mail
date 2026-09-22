package sanitize

import (
	"strings"
	"testing"
)

// Each case is a way email HTML has been used to run code in a mail client.
func TestHTMLRemovesScriptVectors(t *testing.T) {
	cases := map[string]string{
		"script tag":        `<p>Hi</p><script>alert(1)</script>`,
		"inline handler":    `<p onclick="alert(1)">Hi</p>`,
		"onerror on image":  `<img src="x" onerror="alert(1)">`,
		"javascript href":   `<a href="javascript:alert(1)">click</a>`,
		"vbscript href":     `<a href="vbscript:msgbox(1)">click</a>`,
		"data url":          `<a href="data:text/html,<script>alert(1)</script>">click</a>`,
		"iframe":            `<iframe src="https://evil.example"></iframe>`,
		"object":            `<object data="evil.swf"></object>`,
		"embed":             `<embed src="evil.swf">`,
		"form":              `<form action="https://evil.example"><input name="password"></form>`,
		"style block":       `<style>body{background:url(javascript:alert(1))}</style>`,
		"meta refresh":      `<meta http-equiv="refresh" content="0;url=https://evil.example">`,
		"svg script":        `<svg><script>alert(1)</script></svg>`,
		"css expression":    `<p style="width: expression(alert(1))">Hi</p>`,
		"base tag":          `<base href="https://evil.example/">`,
		"link stylesheet":   `<link rel="stylesheet" href="https://evil.example/x.css">`,
		"uppercase script":  `<SCRIPT>alert(1)</SCRIPT>`,
		"nested javascript": `<a href="JaVaScRiPt:alert(1)">click</a>`,
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			output := HTML(input)
			lower := strings.ToLower(output)

			for _, forbidden := range []string{
				"<script", "<iframe", "<object", "<embed", "<form", "<style", "<meta",
				"<link", "<base", "onclick", "onerror", "onload",
				"javascript:", "vbscript:", "expression(",
			} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("sanitised output still contains %q:\n%s", forbidden, output)
				}
			}
		})
	}
}

// Sanitising must not gut ordinary mail, or every message becomes unreadable.
func TestHTMLKeepsLegitimateFormatting(t *testing.T) {
	input := `<div style="color: #333; font-family: Arial">
		<h1>Invoice</h1>
		<p>Hello <strong>Dark</strong>, your <em>invoice</em> is ready.</p>
		<table border="1" cellpadding="4"><tr><td align="right">Total</td><td>$412.86</td></tr></table>
		<ul><li>One</li><li>Two</li></ul>
		<blockquote>Previously…</blockquote>
		<a href="https://example.com/invoice">View it</a>
		<img src="https://example.com/logo.png" alt="Logo" width="120">
	</div>`

	output := HTML(input)

	for _, expected := range []string{
		"<h1>", "<strong>", "<em>", "<table", "<td", "<ul>", "<li>", "<blockquote>",
		"https://example.com/invoice", "https://example.com/logo.png", "color", "Arial",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("sanitising removed %q from ordinary mail:\n%s", expected, output)
		}
	}
}

// Links in mail we did not write should not leak the reader's context or let
// the opened page reach back into ours.
func TestHTMLHardensLinks(t *testing.T) {
	output := HTML(`<a href="https://example.com">click</a>`)

	for _, expected := range []string{`rel=`, `nofollow`, `noreferrer`, `target="_blank"`} {
		if !strings.Contains(output, expected) {
			t.Errorf("link is missing %q:\n%s", expected, output)
		}
	}
}

// Inline images reference their attachment by cid:, which has to survive so it
// can be rewritten to an authorized download URL.
func TestHTMLKeepsInlineImageReferences(t *testing.T) {
	output := HTML(`<img src="cid:logo@example.com" alt="Logo">`)
	if !strings.Contains(output, "cid:logo@example.com") {
		t.Errorf("the cid: reference was dropped:\n%s", output)
	}
}

func TestHTMLHandlesEmptyInput(t *testing.T) {
	for _, input := range []string{"", "   ", "\n\t"} {
		if got := HTML(input); got != "" {
			t.Errorf("HTML(%q) = %q, want empty", input, got)
		}
	}
}

func TestTrackingPixelsRemovesBeacons(t *testing.T) {
	input := `<p>Hello</p><img src="https://track.example/o.gif" width="1" height="1"><img src="https://example.com/real.png" width="600" height="400">`

	output := TrackingPixels(input)

	if strings.Contains(output, "track.example") {
		t.Errorf("the tracking pixel survived:\n%s", output)
	}
	if !strings.Contains(output, "real.png") {
		t.Errorf("a real image was removed:\n%s", output)
	}
}
