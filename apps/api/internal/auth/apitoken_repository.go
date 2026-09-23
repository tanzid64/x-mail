package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const apiTokenColumns = `id, user_id, mail_account_id, name, hint, expires_at,
	last_used_at, revoked_at, created_at`

func scanAPIToken(row pgx.Row) (*APIToken, error) {
	var t APIToken
	err := row.Scan(&t.ID, &t.UserID, &t.MailAccountID, &t.Name, &t.Hint,
		&t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPITokenNotFound
		}
		return nil, err
	}
	return &t, nil
}

type CreateAPITokenParams struct {
	UserID        uuid.UUID
	MailAccountID *uuid.UUID
	Name          string
	Digest        []byte
	Hint          string
	ExpiresAt     *time.Time
}

// CreateAPIToken stores a new token. The mail account, when given, is verified
// against the same user in SQL, so a token cannot be pointed at someone else's
// mailbox by passing its id.
func (r *Repository) CreateAPIToken(ctx context.Context, p CreateAPITokenParams) (*APIToken, error) {
	return scanAPIToken(r.db.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, mail_account_id, name, token_hash, hint, expires_at)
		 SELECT $1, $2, $3, $4, $5, $6
		  WHERE $2::uuid IS NULL
		     OR EXISTS (SELECT 1 FROM mail_accounts WHERE id = $2 AND user_id = $1)
		 RETURNING `+apiTokenColumns,
		p.UserID, p.MailAccountID, p.Name, p.Digest, p.Hint, p.ExpiresAt))
}

// ListAPITokens returns a user's live tokens, newest first. Revoked ones stay
// in the table but out of the list: keeping the row means the digest can never
// be reissued, and it is the record an audit log will want later.
func (r *Repository) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]APIToken, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens
		  WHERE user_id = $1 AND revoked_at IS NULL
		  ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]APIToken, 0)
	for rows.Next() {
		token, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, *token)
	}
	return tokens, rows.Err()
}

// APITokenUser resolves a presented secret to its token and owner in one query.
//
// The digest is the lookup key, so an invalid secret never even names a row.
// Expiry and revocation are reported separately from "no such token" because
// the owner of a real token deserves to know which of the three it is.
func (r *Repository) APITokenUser(ctx context.Context, digest []byte) (*APIToken, *User, error) {
	var t APIToken
	var u User
	err := r.db.QueryRow(ctx,
		`SELECT t.id, t.user_id, t.mail_account_id, t.name, t.hint, t.expires_at,
		        t.last_used_at, t.revoked_at, t.created_at,
		        u.id, u.email, u.name, u.avatar_url, u.email_verified_at,
		        u.is_admin, u.is_active, u.created_at, u.updated_at
		   FROM api_tokens t
		   JOIN users u ON u.id = t.user_id
		  WHERE t.token_hash = $1 AND u.is_active`, digest,
	).Scan(&t.ID, &t.UserID, &t.MailAccountID, &t.Name, &t.Hint, &t.ExpiresAt,
		&t.LastUsedAt, &t.RevokedAt, &t.CreatedAt,
		&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.EmailVerifiedAt,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrAPITokenNotFound
		}
		return nil, nil, err
	}
	return &t, &u, nil
}

// TouchAPIToken records that a token was used, at most once a minute.
//
// "Last used" is what tells an owner whether a token is still wanted; writing
// it on every request would mean a row update per API call for no extra
// information.
func (r *Repository) TouchAPIToken(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE api_tokens SET last_used_at = now()
		  WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`,
		id)
	return err
}

// RevokeAPIToken retires one of the user's tokens, immediately and for good.
// A token belonging to anyone else reports ErrAPITokenNotFound rather than a
// refusal, so the row's existence is not disclosed.
func (r *Repository) RevokeAPIToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE api_tokens SET revoked_at = now()
		  WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, tokenID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAPITokenNotFound
	}
	return nil
}
