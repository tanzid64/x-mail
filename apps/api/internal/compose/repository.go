package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("compose: not found")
	// ErrDraftSending means a send is already under way for this draft, so it
	// must not be edited or sent again.
	ErrDraftSending = errors.New("compose: this draft is already being sent")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const draftColumns = `d.id, d.mail_account_id, d.thread_id, d.in_reply_to_id, d.from_address_id,
	d."to", d.cc, d.bcc, d.subject, d.html_body, d.text_body, d.status, d.created_at, d.updated_at`

func scanDraft(row pgx.Row) (*Draft, error) {
	var d Draft
	var to, cc, bcc []byte

	err := row.Scan(&d.ID, &d.MailAccountID, &d.ThreadID, &d.InReplyToID, &d.FromAddressID,
		&to, &cc, &bcc, &d.Subject, &d.HTMLBody, &d.TextBody, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	for target, raw := range map[*[]Address][]byte{&d.To: to, &d.Cc: cc, &d.Bcc: bcc} {
		*target = []Address{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, target); err != nil {
				return nil, fmt.Errorf("decode draft recipients: %w", err)
			}
		}
	}
	d.Attachments = []Attachment{}
	return &d, nil
}

// ownedDraft narrows to drafts under accounts the user owns, so every draft
// query carries its authorization in the SQL.
const draftOwnership = `
	JOIN mail_accounts a ON a.id = d.mail_account_id
	WHERE a.user_id = $1`

type CreateDraftParams struct {
	UserID        uuid.UUID
	MailAccountID uuid.UUID
	ThreadID      *uuid.UUID
	InReplyToID   *uuid.UUID
	FromAddressID *uuid.UUID
	To, Cc, Bcc   []Address
	Subject       string
	HTMLBody      string
	TextBody      string
}

func (r *Repository) CreateDraft(ctx context.Context, params CreateDraftParams) (*Draft, error) {
	to, cc, bcc, err := encodeRecipients(params.To, params.Cc, params.Bcc)
	if err != nil {
		return nil, err
	}

	var id uuid.UUID
	err = r.db.QueryRow(ctx, `
		INSERT INTO drafts (mail_account_id, thread_id, in_reply_to_id, from_address_id,
		                    "to", cc, bcc, subject, html_body, text_body)
		SELECT $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		 WHERE EXISTS (SELECT 1 FROM mail_accounts WHERE id = $2 AND user_id = $1)
		RETURNING id`,
		params.UserID, params.MailAccountID, params.ThreadID, params.InReplyToID, params.FromAddressID,
		to, cc, bcc, params.Subject, params.HTMLBody, params.TextBody).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The account is not the caller's, so it reads as missing.
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r.GetDraft(ctx, params.UserID, id)
}

func (r *Repository) GetDraft(ctx context.Context, userID, draftID uuid.UUID) (*Draft, error) {
	draft, err := scanDraft(r.db.QueryRow(ctx,
		`SELECT `+draftColumns+` FROM drafts d `+draftOwnership+` AND d.id = $2`, userID, draftID))
	if err != nil {
		return nil, err
	}
	attachments, err := r.ListDraftAttachments(ctx, userID, draftID)
	if err != nil {
		return nil, err
	}
	draft.Attachments = attachments
	return draft, nil
}

func (r *Repository) ListDrafts(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]Draft, error) {
	query := `SELECT ` + draftColumns + ` FROM drafts d ` + draftOwnership
	args := []any{userID}
	if accountID != nil {
		args = append(args, *accountID)
		query += ` AND d.mail_account_id = $2`
	}
	query += ` ORDER BY d.updated_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	drafts := make([]Draft, 0)
	for rows.Next() {
		draft, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, *draft)
	}
	return drafts, rows.Err()
}

type UpdateDraftParams struct {
	FromAddressID *uuid.UUID
	To, Cc, Bcc   *[]Address
	Subject       *string
	HTMLBody      *string
	TextBody      *string
}

// UpdateDraft applies an autosave.
//
// The status predicate is what stops autosave from overwriting a draft that a
// send has already picked up: once a send starts, further keystrokes cannot
// change what goes out.
func (r *Repository) UpdateDraft(ctx context.Context, userID, draftID uuid.UUID, params UpdateDraftParams) (*Draft, error) {
	var to, cc, bcc any
	var err error
	if params.To != nil {
		if to, err = json.Marshal(*params.To); err != nil {
			return nil, err
		}
	}
	if params.Cc != nil {
		if cc, err = json.Marshal(*params.Cc); err != nil {
			return nil, err
		}
	}
	if params.Bcc != nil {
		if bcc, err = json.Marshal(*params.Bcc); err != nil {
			return nil, err
		}
	}

	var id uuid.UUID
	err = r.db.QueryRow(ctx, `
		UPDATE drafts d SET
			from_address_id = COALESCE($3, d.from_address_id),
			"to"            = COALESCE($4, d."to"),
			cc              = COALESCE($5, d.cc),
			bcc             = COALESCE($6, d.bcc),
			subject         = COALESCE($7, d.subject),
			html_body       = COALESCE($8, d.html_body),
			text_body       = COALESCE($9, d.text_body)
		 WHERE d.id = $2
		   AND d.status = 'draft'
		   AND EXISTS (SELECT 1 FROM mail_accounts a WHERE a.id = d.mail_account_id AND a.user_id = $1)
		RETURNING d.id`,
		userID, draftID, params.FromAddressID, to, cc, bcc,
		params.Subject, params.HTMLBody, params.TextBody).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either it is not theirs, or a send already claimed it.
			if _, getErr := r.GetDraft(ctx, userID, draftID); getErr == nil {
				return nil, ErrDraftSending
			}
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r.GetDraft(ctx, userID, draftID)
}

func (r *Repository) DeleteDraft(ctx context.Context, userID, draftID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM drafts d
		 WHERE d.id = $2
		   AND EXISTS (SELECT 1 FROM mail_accounts a WHERE a.id = d.mail_account_id AND a.user_id = $1)`,
		userID, draftID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimDraftForSending flips a draft to "sending" and returns it, but only if
// it was still a draft. A second concurrent send finds nothing to claim, which
// is what stops a double click sending twice.
func (r *Repository) ClaimDraftForSending(ctx context.Context, userID, draftID uuid.UUID) (*Draft, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		UPDATE drafts d SET status = 'sending'
		 WHERE d.id = $2
		   AND d.status = 'draft'
		   AND EXISTS (SELECT 1 FROM mail_accounts a WHERE a.id = d.mail_account_id AND a.user_id = $1)
		RETURNING d.id`, userID, draftID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, getErr := r.GetDraft(ctx, userID, draftID); getErr == nil {
				return nil, ErrDraftSending
			}
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r.GetDraft(ctx, userID, draftID)
}

// ReleaseDraft puts a claimed draft back, used when the send could not be
// queued so the user is not left with a draft they cannot edit.
func (r *Repository) ReleaseDraft(ctx context.Context, draftID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE drafts SET status = 'draft' WHERE id = $1`, draftID)
	return err
}

// ------------------------------------------------------------ attachments --

const attachmentColumns = `at.id, at.message_id, at.draft_id, at.filename, at.mime_type,
	at.size_bytes, at.storage_key, at.content_id, at.is_inline, at.created_at`

func scanAttachment(row pgx.Row) (*Attachment, error) {
	var a Attachment
	err := row.Scan(&a.ID, &a.MessageID, &a.DraftID, &a.Filename, &a.MimeType,
		&a.SizeBytes, &a.StorageKey, &a.ContentID, &a.IsInline, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

type CreateAttachmentParams struct {
	DraftID    uuid.UUID
	Filename   string
	MimeType   string
	SizeBytes  int64
	StorageKey string
	ContentID  *string
	IsInline   bool
}

func (r *Repository) CreateAttachment(ctx context.Context, userID uuid.UUID, params CreateAttachmentParams) (*Attachment, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO attachments (draft_id, filename, mime_type, size_bytes, storage_key, content_id, is_inline)
		SELECT $2, $3, $4, $5, $6, $7, $8
		 WHERE EXISTS (
		     SELECT 1 FROM drafts d
		       JOIN mail_accounts a ON a.id = d.mail_account_id
		      WHERE d.id = $2 AND a.user_id = $1)
		RETURNING id`,
		userID, params.DraftID, params.Filename, params.MimeType, params.SizeBytes,
		params.StorageKey, params.ContentID, params.IsInline).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return scanAttachment(r.db.QueryRow(ctx,
		`SELECT `+attachmentColumns+` FROM attachments at WHERE at.id = $1`, id))
}

func (r *Repository) ListDraftAttachments(ctx context.Context, userID, draftID uuid.UUID) ([]Attachment, error) {
	return r.queryAttachments(ctx, `
		SELECT `+attachmentColumns+` FROM attachments at
		  JOIN drafts d ON d.id = at.draft_id
		  JOIN mail_accounts a ON a.id = d.mail_account_id
		 WHERE at.draft_id = $2 AND a.user_id = $1
		 ORDER BY at.created_at`, userID, draftID)
}

func (r *Repository) ListMessageAttachments(ctx context.Context, userID, messageID uuid.UUID) ([]Attachment, error) {
	return r.queryAttachments(ctx, `
		SELECT `+attachmentColumns+` FROM attachments at
		  JOIN messages m ON m.id = at.message_id
		  JOIN mail_accounts a ON a.id = m.mail_account_id
		 WHERE at.message_id = $2 AND a.user_id = $1
		 ORDER BY at.created_at`, userID, messageID)
}

func (r *Repository) queryAttachments(ctx context.Context, query string, args ...any) ([]Attachment, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]Attachment, 0)
	for rows.Next() {
		attachment, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, *attachment)
	}
	return attachments, rows.Err()
}

// GetAttachment resolves an attachment the user owns, through either its draft
// or its message. This is the authorization check performed before a download.
func (r *Repository) GetAttachment(ctx context.Context, userID, attachmentID uuid.UUID) (*Attachment, error) {
	return scanAttachment(r.db.QueryRow(ctx, `
		SELECT `+attachmentColumns+` FROM attachments at
		 WHERE at.id = $2
		   AND (
		     EXISTS (SELECT 1 FROM drafts d JOIN mail_accounts a ON a.id = d.mail_account_id
		              WHERE d.id = at.draft_id AND a.user_id = $1)
		     OR
		     EXISTS (SELECT 1 FROM messages m JOIN mail_accounts a ON a.id = m.mail_account_id
		              WHERE m.id = at.message_id AND a.user_id = $1)
		   )`, userID, attachmentID))
}

func (r *Repository) DeleteAttachment(ctx context.Context, userID, attachmentID uuid.UUID) (*Attachment, error) {
	attachment, err := r.GetAttachment(ctx, userID, attachmentID)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.Exec(ctx, `DELETE FROM attachments WHERE id = $1`, attachmentID); err != nil {
		return nil, err
	}
	return attachment, nil
}

func encodeRecipients(to, cc, bcc []Address) ([]byte, []byte, []byte, error) {
	encode := func(list []Address) ([]byte, error) {
		if list == nil {
			list = []Address{}
		}
		return json.Marshal(list)
	}
	toJSON, err := encode(to)
	if err != nil {
		return nil, nil, nil, err
	}
	ccJSON, err := encode(cc)
	if err != nil {
		return nil, nil, nil, err
	}
	bccJSON, err := encode(bcc)
	if err != nil {
		return nil, nil, nil, err
	}
	return toJSON, ccJSON, bccJSON, nil
}
