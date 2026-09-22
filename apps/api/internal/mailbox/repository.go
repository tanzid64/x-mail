package mailbox

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound    = errors.New("mailbox: not found")
	ErrLabelExists = errors.New("mailbox: a label with that name already exists")
	ErrSystemLabel = errors.New("mailbox: system labels cannot be renamed or deleted")
	ErrInvalidPage = errors.New("mailbox: invalid pagination cursor")
)

const uniqueViolation = "23505"

// defaultPageSize matches what the inbox renders before scrolling.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// ---------------------------------------------------------------- threads --

// ListParams describes one page of a mailbox view.
type ListParams struct {
	// AccountIDs is the set of accounts to read. A unified inbox passes every
	// account the user owns; a single-account view passes one. It is resolved
	// from the session user, never taken from the client.
	AccountIDs []uuid.UUID
	Folder     Folder
	// LabelID filters to a user label instead of a folder, when set.
	LabelID *uuid.UUID
	Limit   int
	Cursor  string
	// UnreadOnly narrows to threads with unread messages.
	UnreadOnly bool
}

// ListThreads returns one page of threads, newest first.
//
// Pagination is by cursor rather than offset: a mailbox changes under the
// reader, and an offset would silently skip or repeat rows as mail arrives.
func (r *Repository) ListThreads(ctx context.Context, params ListParams) (*Page, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	where := []string{"t.mail_account_id = ANY($1)"}
	args := []any{params.AccountIDs}

	// Placement filter.
	if params.LabelID != nil {
		args = append(args, *params.LabelID)
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM messages m
			  JOIN message_labels ml ON ml.message_id = m.id
			 WHERE m.thread_id = t.id AND ml.label_id = $%d)`, len(args)))
	} else if systemKey := params.Folder.SystemLabel(); systemKey != "" {
		args = append(args, systemKey)
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM messages m
			  JOIN message_labels ml ON ml.message_id = m.id
			  JOIN labels l ON l.id = ml.label_id
			 WHERE m.thread_id = t.id AND l.system_key = $%d)`, len(args)))
	} else {
		// Flag-defined folders. Trash and spam are excluded so a starred
		// message that was thrown away stops appearing under Starred.
		flag := "m.is_starred"
		if params.Folder == FolderImportant {
			flag = "m.is_important"
		}
		if params.Folder == FolderSnoozed {
			// Snoozing arrives in a later phase; until then the view is empty
			// rather than wrong.
			flag = "false"
		}
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM messages m
			 WHERE m.thread_id = t.id AND %s
			   AND NOT EXISTS (
			       SELECT 1 FROM message_labels ml
			         JOIN labels l ON l.id = ml.label_id
			        WHERE ml.message_id = m.id AND l.system_key IN ('TRASH', 'SPAM')))`, flag))
	}

	if params.UnreadOnly {
		where = append(where, "t.unread_count > 0")
	}

	if params.Cursor != "" {
		at, id, err := decodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		args = append(args, at, id)
		where = append(where, fmt.Sprintf("(t.last_message_at, t.id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	// One extra row tells us whether another page exists without a count query.
	args = append(args, limit+1)

	query := `
		SELECT t.id, t.mail_account_id, t.subject, t.snippet, t.message_count, t.unread_count,
		       t.has_attachments, t.is_starred, t.participants, t.last_message_at,
		       t.created_at, t.updated_at, a.name, a.color
		  FROM threads t
		  JOIN mail_accounts a ON a.id = t.mail_account_id
		 WHERE ` + strings.Join(where, "\n  AND ") + `
		 ORDER BY t.last_message_at DESC, t.id DESC
		 LIMIT $` + fmt.Sprint(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := make([]Thread, 0, limit)
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.MailAccountID, &t.Subject, &t.Snippet, &t.MessageCount,
			&t.UnreadCount, &t.HasAttachments, &t.IsStarred, &t.Participants, &t.LastMessageAt,
			&t.CreatedAt, &t.UpdatedAt, &t.AccountName, &t.AccountColor); err != nil {
			return nil, err
		}
		if t.Participants == nil {
			t.Participants = []Participant{}
		}
		threads = append(threads, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	page := &Page{Threads: threads}
	if len(threads) > limit {
		page.Threads = threads[:limit]
		last := page.Threads[limit-1]
		page.NextCursor = encodeCursor(last.LastMessageAt, last.ID)
	}

	if err := r.attachThreadLabels(ctx, page.Threads); err != nil {
		return nil, err
	}
	return page, nil
}

// attachThreadLabels loads the user labels for a page of threads in one query,
// rather than one per row.
func (r *Repository) attachThreadLabels(ctx context.Context, threads []Thread) error {
	if len(threads) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, len(threads))
	index := make(map[uuid.UUID]int, len(threads))
	for i, t := range threads {
		ids[i] = t.ID
		index[t.ID] = i
	}

	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT m.thread_id, l.id, l.mail_account_id, l.name, l.color,
		       l.system_key, l.created_at, l.updated_at
		  FROM messages m
		  JOIN message_labels ml ON ml.message_id = m.id
		  JOIN labels l ON l.id = ml.label_id
		 WHERE m.thread_id = ANY($1) AND l.system_key IS NULL
		 ORDER BY m.thread_id, l.name`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var threadID uuid.UUID
		var l Label
		if err := rows.Scan(&threadID, &l.ID, &l.MailAccountID, &l.Name, &l.Color,
			&l.SystemKey, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return err
		}
		if i, ok := index[threadID]; ok {
			threads[i].Labels = append(threads[i].Labels, l)
		}
	}
	return rows.Err()
}

// GetThread loads a thread and its messages, scoped to the accounts the caller
// owns.
func (r *Repository) GetThread(ctx context.Context, accountIDs []uuid.UUID, threadID uuid.UUID) (*ThreadDetail, error) {
	var t Thread
	err := r.db.QueryRow(ctx, `
		SELECT t.id, t.mail_account_id, t.subject, t.snippet, t.message_count, t.unread_count,
		       t.has_attachments, t.is_starred, t.participants, t.last_message_at,
		       t.created_at, t.updated_at, a.name, a.color
		  FROM threads t
		  JOIN mail_accounts a ON a.id = t.mail_account_id
		 WHERE t.id = $1 AND t.mail_account_id = ANY($2)`, threadID, accountIDs,
	).Scan(&t.ID, &t.MailAccountID, &t.Subject, &t.Snippet, &t.MessageCount, &t.UnreadCount,
		&t.HasAttachments, &t.IsStarred, &t.Participants, &t.LastMessageAt,
		&t.CreatedAt, &t.UpdatedAt, &t.AccountName, &t.AccountColor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t.Participants == nil {
		t.Participants = []Participant{}
	}

	messages, err := r.messagesForThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return &ThreadDetail{Thread: t, Messages: messages}, nil
}

const messageColumns = `m.id, m.mail_account_id, m.thread_id, m.message_id, m.in_reply_to,
	m."references", m.provider_message_id, m.sender_name, m.sender_email, m.subject, m.snippet,
	m.html_body, m.text_body, m.direction, m.status, m.is_read, m.is_starred, m.is_important,
	m.has_attachments, m.size_bytes, m.sent_at, m.received_at, m.created_at, m.updated_at`

func scanMessage(row pgx.Row) (*Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.MailAccountID, &m.ThreadID, &m.MessageID, &m.InReplyTo,
		&m.References, &m.ProviderMessageID, &m.SenderName, &m.SenderEmail, &m.Subject, &m.Snippet,
		&m.HTMLBody, &m.TextBody, &m.Direction, &m.Status, &m.IsRead, &m.IsStarred, &m.IsImportant,
		&m.HasAttachments, &m.SizeBytes, &m.SentAt, &m.ReceivedAt, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (r *Repository) messagesForThread(ctx context.Context, threadID uuid.UUID) ([]Message, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+messageColumns+` FROM messages m WHERE m.thread_id = $1 ORDER BY m.created_at`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]Message, 0)
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		message.Recipients = []Recipient{}
		message.Labels = []Label{}
		messages = append(messages, *message)
		ids = append(ids, message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return messages, nil
	}

	index := make(map[uuid.UUID]int, len(messages))
	for i, m := range messages {
		index[m.ID] = i
	}

	// Recipients and labels are loaded in one query each, not per message.
	recipientRows, err := r.db.Query(ctx,
		`SELECT message_id, id, email, name, type FROM message_recipients
		  WHERE message_id = ANY($1) ORDER BY type, email`, ids)
	if err != nil {
		return nil, err
	}
	defer recipientRows.Close()
	for recipientRows.Next() {
		var messageID uuid.UUID
		var rec Recipient
		if err := recipientRows.Scan(&messageID, &rec.ID, &rec.Email, &rec.Name, &rec.Type); err != nil {
			return nil, err
		}
		if i, ok := index[messageID]; ok {
			messages[i].Recipients = append(messages[i].Recipients, rec)
		}
	}
	if err := recipientRows.Err(); err != nil {
		return nil, err
	}

	labelRows, err := r.db.Query(ctx,
		`SELECT ml.message_id, l.id, l.mail_account_id, l.name, l.color, l.system_key,
		        l.created_at, l.updated_at
		   FROM message_labels ml JOIN labels l ON l.id = ml.label_id
		  WHERE ml.message_id = ANY($1) ORDER BY l.name`, ids)
	if err != nil {
		return nil, err
	}
	defer labelRows.Close()
	for labelRows.Next() {
		var messageID uuid.UUID
		var l Label
		if err := labelRows.Scan(&messageID, &l.ID, &l.MailAccountID, &l.Name, &l.Color,
			&l.SystemKey, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		if i, ok := index[messageID]; ok {
			messages[i].Labels = append(messages[i].Labels, l)
		}
	}
	return messages, labelRows.Err()
}

// ---------------------------------------------------------------- actions --

// ownedThreads narrows a caller-supplied thread list to the ones that actually
// belong to the given accounts. Every mutation runs through it, so an id the
// user does not own is silently dropped rather than acted on.
func (r *Repository) ownedThreads(ctx context.Context, tx pgx.Tx, accountIDs, threadIDs []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM threads WHERE id = ANY($1) AND mail_account_id = ANY($2)`,
		threadIDs, accountIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	owned := make([]uuid.UUID, 0, len(threadIDs))
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		owned = append(owned, id)
	}
	return owned, rows.Err()
}

// SetRead marks every message in the given threads read or unread.
func (r *Repository) SetRead(ctx context.Context, accountIDs, threadIDs []uuid.UUID, read bool) (int, error) {
	return r.mutateThreads(ctx, accountIDs, threadIDs, func(ctx context.Context, tx pgx.Tx, owned []uuid.UUID) error {
		_, err := tx.Exec(ctx,
			`UPDATE messages SET is_read = $2 WHERE thread_id = ANY($1)`, owned, read)
		return err
	})
}

// SetStarred stars or unstars every message in the given threads.
func (r *Repository) SetStarred(ctx context.Context, accountIDs, threadIDs []uuid.UUID, starred bool) (int, error) {
	return r.mutateThreads(ctx, accountIDs, threadIDs, func(ctx context.Context, tx pgx.Tx, owned []uuid.UUID) error {
		_, err := tx.Exec(ctx,
			`UPDATE messages SET is_starred = $2 WHERE thread_id = ANY($1)`, owned, starred)
		return err
	})
}

// SetImportant flags or unflags every message in the given threads.
func (r *Repository) SetImportant(ctx context.Context, accountIDs, threadIDs []uuid.UUID, important bool) (int, error) {
	return r.mutateThreads(ctx, accountIDs, threadIDs, func(ctx context.Context, tx pgx.Tx, owned []uuid.UUID) error {
		_, err := tx.Exec(ctx,
			`UPDATE messages SET is_important = $2 WHERE thread_id = ANY($1)`, owned, important)
		return err
	})
}

// Move replaces the placement labels on every message in the given threads.
//
// Placement is exclusive — a message is in exactly one of inbox, archive, spam
// or trash — so moving clears the others in the same statement. Drafts and Sent
// are left alone: they record how a message came to exist, not where it filed.
func (r *Repository) Move(ctx context.Context, accountIDs, threadIDs []uuid.UUID, target string) (int, error) {
	return r.mutateThreads(ctx, accountIDs, threadIDs, func(ctx context.Context, tx pgx.Tx, owned []uuid.UUID) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM message_labels ml
			 USING labels l, messages m
			 WHERE l.id = ml.label_id
			   AND m.id = ml.message_id
			   AND m.thread_id = ANY($1)
			   AND l.system_key IN ('INBOX', 'ARCHIVE', 'SPAM', 'TRASH')`, owned); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO message_labels (message_id, label_id)
			SELECT m.id, l.id
			  FROM messages m
			  JOIN labels l ON l.mail_account_id = m.mail_account_id AND l.system_key = $2
			 WHERE m.thread_id = ANY($1)
			ON CONFLICT DO NOTHING`, owned, target)
		return err
	})
}

// ApplyLabel adds or removes a user label across the given threads.
func (r *Repository) ApplyLabel(ctx context.Context, accountIDs, threadIDs []uuid.UUID, labelID uuid.UUID, apply bool) (int, error) {
	return r.mutateThreads(ctx, accountIDs, threadIDs, func(ctx context.Context, tx pgx.Tx, owned []uuid.UUID) error {
		// The label must belong to one of the caller's accounts.
		var labelAccount uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT mail_account_id FROM labels WHERE id = $1 AND mail_account_id = ANY($2)`,
			labelID, accountIDs).Scan(&labelAccount); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		if apply {
			_, err := tx.Exec(ctx, `
				INSERT INTO message_labels (message_id, label_id)
				SELECT m.id, $2 FROM messages m
				 WHERE m.thread_id = ANY($1) AND m.mail_account_id = $3
				ON CONFLICT DO NOTHING`, owned, labelID, labelAccount)
			return err
		}

		_, err := tx.Exec(ctx, `
			DELETE FROM message_labels ml USING messages m
			 WHERE m.id = ml.message_id AND m.thread_id = ANY($1) AND ml.label_id = $2`,
			owned, labelID)
		return err
	})
}

// DeleteThreads permanently removes threads and everything hanging off them.
func (r *Repository) DeleteThreads(ctx context.Context, accountIDs, threadIDs []uuid.UUID) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	owned, err := r.ownedThreads(ctx, tx, accountIDs, threadIDs)
	if err != nil {
		return 0, err
	}
	if len(owned) == 0 {
		return 0, tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM threads WHERE id = ANY($1)`, owned); err != nil {
		return 0, err
	}
	return len(owned), tx.Commit(ctx)
}

// mutateThreads runs a change over the caller's own threads and refreshes the
// affected thread summaries in the same transaction, so a list rendered
// immediately afterwards cannot show stale counters.
func (r *Repository) mutateThreads(
	ctx context.Context,
	accountIDs, threadIDs []uuid.UUID,
	apply func(context.Context, pgx.Tx, []uuid.UUID) error,
) (int, error) {
	if len(threadIDs) == 0 {
		return 0, nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	owned, err := r.ownedThreads(ctx, tx, accountIDs, threadIDs)
	if err != nil {
		return 0, err
	}
	if len(owned) == 0 {
		return 0, tx.Commit(ctx)
	}

	if err := apply(ctx, tx, owned); err != nil {
		return 0, err
	}
	for _, id := range owned {
		if _, err := tx.Exec(ctx, `SELECT refresh_thread($1)`, id); err != nil {
			return 0, err
		}
	}
	return len(owned), tx.Commit(ctx)
}

// ----------------------------------------------------------------- labels --

const labelColumns = `id, mail_account_id, name, color, system_key, created_at, updated_at`

func scanLabel(row pgx.Row) (*Label, error) {
	var l Label
	err := row.Scan(&l.ID, &l.MailAccountID, &l.Name, &l.Color, &l.SystemKey, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

func (r *Repository) ListLabels(ctx context.Context, accountIDs []uuid.UUID) ([]Label, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+labelColumns+` FROM labels WHERE mail_account_id = ANY($1)
		  ORDER BY system_key NULLS LAST, lower(name)`, accountIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := make([]Label, 0)
	for rows.Next() {
		label, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		labels = append(labels, *label)
	}
	return labels, rows.Err()
}

func (r *Repository) CreateLabel(ctx context.Context, accountID uuid.UUID, name, color string) (*Label, error) {
	label, err := scanLabel(r.db.QueryRow(ctx,
		`INSERT INTO labels (mail_account_id, name, color) VALUES ($1, $2, $3)
		 RETURNING `+labelColumns, accountID, name, color))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrLabelExists
		}
		return nil, err
	}
	return label, nil
}

func (r *Repository) GetLabel(ctx context.Context, accountIDs []uuid.UUID, labelID uuid.UUID) (*Label, error) {
	return scanLabel(r.db.QueryRow(ctx,
		`SELECT `+labelColumns+` FROM labels WHERE id = $1 AND mail_account_id = ANY($2)`,
		labelID, accountIDs))
}

func (r *Repository) UpdateLabel(ctx context.Context, accountIDs []uuid.UUID, labelID uuid.UUID, name, color *string) (*Label, error) {
	existing, err := r.GetLabel(ctx, accountIDs, labelID)
	if err != nil {
		return nil, err
	}
	if existing.IsSystem() {
		return nil, ErrSystemLabel
	}

	label, err := scanLabel(r.db.QueryRow(ctx,
		`UPDATE labels SET name = COALESCE($2, name), color = COALESCE($3, color)
		  WHERE id = $1 RETURNING `+labelColumns, labelID, name, color))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrLabelExists
		}
		return nil, err
	}
	return label, nil
}

func (r *Repository) DeleteLabel(ctx context.Context, accountIDs []uuid.UUID, labelID uuid.UUID) error {
	existing, err := r.GetLabel(ctx, accountIDs, labelID)
	if err != nil {
		return err
	}
	if existing.IsSystem() {
		return ErrSystemLabel
	}
	_, err = r.db.Exec(ctx, `DELETE FROM labels WHERE id = $1`, labelID)
	return err
}

// FolderCounts returns the unread thread count per folder, for the sidebar.
func (r *Repository) FolderCounts(ctx context.Context, accountIDs []uuid.UUID) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT l.system_key, count(DISTINCT t.id)
		  FROM threads t
		  JOIN messages m ON m.thread_id = t.id
		  JOIN message_labels ml ON ml.message_id = m.id
		  JOIN labels l ON l.id = ml.label_id
		 WHERE t.mail_account_id = ANY($1)
		   AND l.system_key IS NOT NULL
		   AND t.unread_count > 0
		 GROUP BY l.system_key`, accountIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return counts, rows.Err()
}

// ---------------------------------------------------------------- cursors --

// encodeCursor packs the sort key of the last row on a page. It is opaque to
// the client, which must only pass it back untouched.
func encodeCursor(at time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s|%s", at.UTC().Format(time.RFC3339Nano), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidPage
	}
	at, id, found := strings.Cut(string(raw), "|")
	if !found {
		return time.Time{}, uuid.Nil, ErrInvalidPage
	}
	parsedAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidPage
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidPage
	}
	return parsedAt, parsedID, nil
}
