package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound    = errors.New("auth: user not found")
	ErrEmailTaken      = errors.New("auth: email already registered")
	ErrSessionNotFound = errors.New("auth: session not found or expired")
	ErrTokenNotFound   = errors.New("auth: token is invalid, expired or already used")
)

const uniqueViolation = "23505"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const userColumns = `id, email, password_hash, name, avatar_url, email_verified_at,
	is_admin, is_active, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL,
		&u.EmailVerifiedAt, &u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, name string) (*User, error) {
	user, err := scanUser(r.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING `+userColumns,
		email, passwordHash, name))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (r *Repository) UpdateProfile(ctx context.Context, id uuid.UUID, name string, avatarURL *string) (*User, error) {
	return scanUser(r.db.QueryRow(ctx,
		`UPDATE users SET name = $2, avatar_url = $3 WHERE id = $1 RETURNING `+userColumns,
		id, name, avatarURL))
}

func (r *Repository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	tag, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ---------------------------------------------------------------- sessions --

func (r *Repository) CreateSession(ctx context.Context, userID uuid.UUID, digest []byte,
	userAgent string, ip *netip.Addr, expiresAt time.Time) (*Session, error) {

	var s Session
	err := r.db.QueryRow(ctx,
		`INSERT INTO sessions (user_id, token_hash, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, user_agent, expires_at, last_seen_at, created_at`,
		userID, digest, userAgent, ip, expiresAt,
	).Scan(&s.ID, &s.UserID, &s.UserAgent, &s.ExpiresAt, &s.LastSeenAt, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &s, nil
}

// SessionUser looks up a live session and its user in one round trip. Expired
// sessions and deactivated users are treated as no session at all.
func (r *Repository) SessionUser(ctx context.Context, digest []byte) (*Session, *User, error) {
	var s Session
	var u User

	err := r.db.QueryRow(ctx,
		`SELECT s.id, s.user_id, s.user_agent, s.expires_at, s.last_seen_at, s.created_at,
		        u.id, u.email, u.password_hash, u.name, u.avatar_url, u.email_verified_at,
		        u.is_admin, u.is_active, u.created_at, u.updated_at
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = $1
		    AND s.expires_at > now()
		    AND u.is_active`,
		digest,
	).Scan(&s.ID, &s.UserID, &s.UserAgent, &s.ExpiresAt, &s.LastSeenAt, &s.CreatedAt,
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.EmailVerifiedAt,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrSessionNotFound
		}
		return nil, nil, err
	}
	return &s, &u, nil
}

// TouchSession records activity. It is best-effort: a failure here must not
// fail the request the user actually made.
func (r *Repository) TouchSession(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE sessions SET last_seen_at = now()
		  WHERE id = $1 AND last_seen_at < now() - interval '5 minutes'`, id)
	return err
}

func (r *Repository) DeleteSession(ctx context.Context, digest []byte) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, digest)
	return err
}

// DeleteUserSessions logs a user out everywhere, used after a password change.
func (r *Repository) DeleteUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

// DeleteUserSessionsExcept logs a user out of every browser but the one that
// made the current request.
func (r *Repository) DeleteUserSessionsExcept(ctx context.Context, userID uuid.UUID, keepDigest []byte) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, keepDigest)
	return err
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return tag.RowsAffected(), err
}

// -------------------------------------------------------- password resets --

func (r *Repository) CreatePasswordReset(ctx context.Context, userID uuid.UUID, digest []byte, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, digest, expiresAt)
	return err
}

// ConsumePasswordReset atomically marks a reset token used and returns its
// user. The UPDATE ... WHERE used_at IS NULL makes a replayed link a no-op even
// if two requests arrive at once.
func (r *Repository) ConsumePasswordReset(ctx context.Context, digest []byte) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.QueryRow(ctx,
		`UPDATE password_resets SET used_at = now()
		  WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		  RETURNING user_id`, digest).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrTokenNotFound
		}
		return uuid.Nil, err
	}
	return userID, nil
}

// InvalidatePasswordResets burns any outstanding reset links, so a stolen older
// email cannot be used after the password has already been changed.
func (r *Repository) InvalidatePasswordResets(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE password_resets SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID)
	return err
}

// ---------------------------------------------------- email verifications --

func (r *Repository) CreateEmailVerification(ctx context.Context, userID uuid.UUID, email string, digest []byte, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO email_verifications (user_id, email, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, email, digest, expiresAt)
	return err
}

// ConsumeEmailVerification marks the token used and flips the user's
// email_verified_at, both in one transaction.
func (r *Repository) ConsumeEmailVerification(ctx context.Context, digest []byte) (*User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var userID uuid.UUID
	var email string
	err = tx.QueryRow(ctx,
		`UPDATE email_verifications SET used_at = now()
		  WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		  RETURNING user_id, email`, digest).Scan(&userID, &email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	user, err := scanUser(tx.QueryRow(ctx,
		`UPDATE users SET email = $2, email_verified_at = now() WHERE id = $1 RETURNING `+userColumns,
		userID, email))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return user, nil
}
