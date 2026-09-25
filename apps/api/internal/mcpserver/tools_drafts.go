package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dark/x-mail/api/internal/compose"
)

// writes marks a tool that creates or changes something but destroys nothing,
// so a client knows to confirm without warning about data loss.
func writes(title string) *mcp.ToolAnnotations {
	destructive := false
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &destructive}
}

// recipient is how a model supplies an addressee.
type recipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

func toAddresses(list []recipient) []compose.Address {
	out := make([]compose.Address, 0, len(list))
	for _, r := range list {
		out = append(out, compose.Address{
			Email: strings.TrimSpace(r.Email),
			Name:  strings.TrimSpace(r.Name),
		})
	}
	return out
}

func fromAddresses(list []compose.Address) []recipient {
	out := make([]recipient, 0, len(list))
	for _, a := range list {
		out = append(out, recipient{Email: a.Email, Name: a.Name})
	}
	return out
}

// draftOut is deliberately explicit that nothing has been sent, because that is
// the single most likely thing for a model to get wrong when reporting back.
type draftOut struct {
	ID            string      `json:"id"`
	MailAccountID string      `json:"mail_account_id"`
	FromAddressID string      `json:"from_address_id,omitempty"`
	To            []recipient `json:"to"`
	Cc            []recipient `json:"cc,omitempty"`
	Bcc           []recipient `json:"bcc,omitempty"`
	Subject       string      `json:"subject"`
	Body          string      `json:"body"`
	Status        string      `json:"status"`
	UpdatedAt     string      `json:"updated_at"`
	Note          string      `json:"note,omitempty"`
}

const notSentNote = "Saved as a draft. It has NOT been sent — the user sends it from x-mail."

func toDraftOut(d *compose.Draft, note string) draftOut {
	out := draftOut{
		ID:            d.ID.String(),
		MailAccountID: d.MailAccountID.String(),
		To:            fromAddresses(d.To),
		Cc:            fromAddresses(d.Cc),
		Bcc:           fromAddresses(d.Bcc),
		Subject:       d.Subject,
		Body:          d.TextBody,
		Status:        d.Status,
		UpdatedAt:     d.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Note:          note,
	}
	if d.FromAddressID != nil {
		out.FromAddressID = d.FromAddressID.String()
	}
	if out.Body == "" {
		out.Body = stripTags(d.HTMLBody)
	}
	return out
}

// ------------------------------------------------------------ list / get ----

type listDraftsIn struct {
	MailAccountID string `json:"mail_account_id,omitempty" jsonschema:"restrict to one mail account"`
}

type listDraftsOut struct {
	Drafts []draftOut `json:"drafts"`
}

func (s *Server) listDrafts(ctx context.Context, _ *mcp.CallToolRequest, in listDraftsIn) (*mcp.CallToolResult, listDraftsOut, error) {
	userID, err := userFrom(ctx)
	if err != nil {
		return nil, listDraftsOut{}, err
	}
	accountID, err := optionalUUID(in.MailAccountID)
	if err != nil {
		return nil, listDraftsOut{}, fmt.Errorf("mail_account_id is not a valid id")
	}

	list, err := s.deps.Compose.ListDrafts(ctx, userID, accountID)
	if err != nil {
		return nil, listDraftsOut{}, err
	}

	out := listDraftsOut{Drafts: make([]draftOut, 0, len(list))}
	for i := range list {
		out.Drafts = append(out.Drafts, toDraftOut(&list[i], ""))
	}
	return nil, out, nil
}

type getDraftIn struct {
	DraftID string `json:"draft_id"`
}

func (s *Server) getDraft(ctx context.Context, _ *mcp.CallToolRequest, in getDraftIn) (*mcp.CallToolResult, draftOut, error) {
	userID, err := userFrom(ctx)
	if err != nil {
		return nil, draftOut{}, err
	}
	draftID, err := uuid.Parse(in.DraftID)
	if err != nil {
		return nil, draftOut{}, fmt.Errorf("draft_id is not a valid id")
	}

	draft, err := s.deps.Compose.GetDraft(ctx, userID, draftID)
	if err != nil {
		return nil, draftOut{}, err
	}
	return nil, toDraftOut(draft, ""), nil
}

// ---------------------------------------------------------------- create ----

type createDraftIn struct {
	MailAccountID string      `json:"mail_account_id" jsonschema:"from list_accounts"`
	FromAddressID string      `json:"from_address_id,omitempty" jsonschema:"from list_addresses; must belong to this mail account and is required before a human can send"`
	To            []recipient `json:"to,omitempty"`
	Cc            []recipient `json:"cc,omitempty"`
	Bcc           []recipient `json:"bcc,omitempty"`
	Subject       string      `json:"subject,omitempty"`
	Body          string      `json:"body,omitempty" jsonschema:"plain text body"`
	InReplyToID   string      `json:"in_reply_to_id,omitempty" jsonschema:"the message id being replied to; sets the headers that thread the reply correctly"`
}

func (s *Server) createDraft(ctx context.Context, _ *mcp.CallToolRequest, in createDraftIn) (*mcp.CallToolResult, draftOut, error) {
	userID, err := userFrom(ctx)
	if err != nil {
		return nil, draftOut{}, err
	}
	accountID, err := uuid.Parse(strings.TrimSpace(in.MailAccountID))
	if err != nil {
		return nil, draftOut{}, fmt.Errorf("mail_account_id is required; call list_accounts to find one")
	}
	fromID, err := optionalUUID(strings.TrimSpace(in.FromAddressID))
	if err != nil {
		return nil, draftOut{}, fmt.Errorf("from_address_id is not a valid id")
	}
	replyTo, err := optionalUUID(strings.TrimSpace(in.InReplyToID))
	if err != nil {
		return nil, draftOut{}, fmt.Errorf("in_reply_to_id is not a valid id")
	}

	draft, err := s.deps.Compose.CreateDraft(ctx, compose.CreateDraftParams{
		UserID:        userID,
		MailAccountID: accountID,
		FromAddressID: fromID,
		InReplyToID:   replyTo,
		To:            toAddresses(in.To),
		Cc:            toAddresses(in.Cc),
		Bcc:           toAddresses(in.Bcc),
		Subject:       in.Subject,
		TextBody:      in.Body,
		HTMLBody:      textToHTML(in.Body),
	})
	if err != nil {
		return nil, draftOut{}, err
	}
	return nil, toDraftOut(draft, notSentNote), nil
}

// ---------------------------------------------------------------- update ----

type updateDraftIn struct {
	DraftID       string      `json:"draft_id"`
	FromAddressID string      `json:"from_address_id,omitempty"`
	To            []recipient `json:"to,omitempty" jsonschema:"replaces the current recipients"`
	Cc            []recipient `json:"cc,omitempty"`
	Bcc           []recipient `json:"bcc,omitempty"`
	Subject       *string     `json:"subject,omitempty"`
	Body          *string     `json:"body,omitempty" jsonschema:"plain text; replaces the current body"`
}

func (s *Server) updateDraft(ctx context.Context, _ *mcp.CallToolRequest, in updateDraftIn) (*mcp.CallToolResult, draftOut, error) {
	userID, err := userFrom(ctx)
	if err != nil {
		return nil, draftOut{}, err
	}
	draftID, err := uuid.Parse(strings.TrimSpace(in.DraftID))
	if err != nil {
		return nil, draftOut{}, fmt.Errorf("draft_id is not a valid id")
	}

	params := compose.UpdateDraftParams{Subject: in.Subject}

	if in.FromAddressID != "" {
		fromID, parseErr := uuid.Parse(strings.TrimSpace(in.FromAddressID))
		if parseErr != nil {
			return nil, draftOut{}, fmt.Errorf("from_address_id is not a valid id")
		}
		params.FromAddressID = &fromID
	}
	// A supplied list replaces what is there; an omitted one leaves it alone.
	if in.To != nil {
		to := toAddresses(in.To)
		params.To = &to
	}
	if in.Cc != nil {
		cc := toAddresses(in.Cc)
		params.Cc = &cc
	}
	if in.Bcc != nil {
		bcc := toAddresses(in.Bcc)
		params.Bcc = &bcc
	}
	if in.Body != nil {
		html := textToHTML(*in.Body)
		params.TextBody = in.Body
		params.HTMLBody = &html
	}

	draft, err := s.deps.Compose.UpdateDraft(ctx, userID, draftID, params)
	if err != nil {
		return nil, draftOut{}, err
	}
	return nil, toDraftOut(draft, notSentNote), nil
}

// textToHTML turns a plain-text body into the HTML part.
//
// Models write prose, not markup. Escaping first means a model that emits a
// literal "<" produces a message that reads correctly rather than one with a
// broken tag in it.
func textToHTML(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	var out strings.Builder
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		out.WriteString("<p>")
		for i, line := range strings.Split(paragraph, "\n") {
			if i > 0 {
				out.WriteString("<br>")
			}
			out.WriteString(escapeHTML(line))
		}
		out.WriteString("</p>")
	}
	return out.String()
}

func escapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;",
	).Replace(s)
}
