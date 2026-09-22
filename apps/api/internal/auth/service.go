package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/mailer"
)

const (
	passwordResetTTL     = time.Hour
	emailVerificationTTL = 24 * time.Hour
	minPasswordLength    = 10
)

// dummyHash lets Login spend the same CPU time on an unknown email as on a
// known one, so response timing does not reveal which addresses are registered.
var dummyHash string

func init() {
	h, err := HashPassword("timing-equalisation-placeholder")
	if err != nil {
		panic("auth: cannot hash placeholder password: " + err.Error())
	}
	dummyHash = h
}

type Options struct {
	SessionTTL time.Duration
	AppBaseURL string
}

type Service struct {
	repo   *Repository
	mail   mailer.Mailer
	opts   Options
	logger *slog.Logger
}

func NewService(repo *Repository, mail mailer.Mailer, opts Options, logger *slog.Logger) *Service {
	return &Service{repo: repo, mail: mail, opts: opts, logger: logger}
}

// RequestMeta describes where a request came from, recorded against sessions.
type RequestMeta struct {
	UserAgent string
	IP        *netip.Addr
}

// Register creates a user and emails a verification link. The returned token is
// the session cookie value: registration logs the new user straight in, while
// the unverified state is carried on the user record.
func (s *Service) Register(ctx context.Context, email, password, name string, meta RequestMeta) (*User, string, error) {
	email = normalizeEmail(email)

	hash, err := HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	user, err := s.repo.CreateUser(ctx, email, hash, strings.TrimSpace(name))
	if err != nil {
		return nil, "", err
	}

	// A failure to send verification email must not undo a valid signup — the
	// user can always request another link.
	if err := s.sendVerificationEmail(ctx, user); err != nil {
		s.logger.ErrorContext(ctx, "send verification email", "error", err, "user_id", user.ID)
	}

	token, err := s.issueSession(ctx, user.ID, meta)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// Login verifies credentials and issues a session. Every failure path returns
// the same error so the response cannot distinguish "no such user" from
// "wrong password".
func (s *Service) Login(ctx context.Context, email, password string, meta RequestMeta) (*User, string, error) {
	invalid := errors.New("auth: invalid credentials")

	user, err := s.repo.GetUserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Burn the same CPU a real verification would.
			_, _ = VerifyPassword(password, dummyHash)
			return nil, "", invalid
		}
		return nil, "", err
	}

	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		return nil, "", invalid
	}
	if !user.IsActive {
		return nil, "", errors.New("auth: account is disabled")
	}

	// Transparently upgrade hashes that predate the current cost parameters.
	if NeedsRehash(user.PasswordHash) {
		if newHash, err := HashPassword(password); err == nil {
			if err := s.repo.UpdatePassword(ctx, user.ID, newHash); err != nil {
				s.logger.WarnContext(ctx, "rehash password", "error", err, "user_id", user.ID)
			}
		}
	}

	token, err := s.issueSession(ctx, user.ID, meta)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, HashToken(token))
}

// Authenticate resolves a session cookie to its user.
func (s *Service) Authenticate(ctx context.Context, token string) (*User, *Session, error) {
	if token == "" {
		return nil, nil, ErrSessionNotFound
	}
	session, user, err := s.repo.SessionUser(ctx, HashToken(token))
	if err != nil {
		return nil, nil, err
	}
	if err := s.repo.TouchSession(ctx, session.ID); err != nil {
		s.logger.WarnContext(ctx, "touch session", "error", err, "session_id", session.ID)
	}
	return user, session, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, name string, avatarURL *string) (*User, error) {
	return s.repo.UpdateProfile(ctx, userID, strings.TrimSpace(name), avatarURL)
}

// ChangePassword requires the current password and then invalidates every other
// session, so a stolen cookie cannot outlive the password it was issued under.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword, keepToken string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	ok, err := VerifyPassword(currentPassword, user.PasswordHash)
	if err != nil || !ok {
		return errors.New("auth: current password is incorrect")
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.revokeOtherSessions(ctx, userID, keepToken)
}

// RequestPasswordReset always reports success to the caller: telling an
// anonymous visitor whether an address is registered is an enumeration oracle.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := s.repo.GetUserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		return err
	}

	token, digest, err := NewToken()
	if err != nil {
		return err
	}
	if err := s.repo.CreatePasswordReset(ctx, user.ID, digest, time.Now().Add(passwordResetTTL)); err != nil {
		return err
	}

	link := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(s.opts.AppBaseURL, "/"), token)
	return s.mail.Send(ctx, mailer.Message{
		To:      user.Email,
		Subject: "Reset your x-mail password",
		HTML: fmt.Sprintf(
			`<p>Hi%s,</p><p>Use the link below to choose a new password. It expires in one hour.</p>`+
				`<p><a href="%s">Reset your password</a></p>`+
				`<p>If you did not request this, you can ignore this email.</p>`,
			greetingName(user), link),
		Text: fmt.Sprintf("Reset your x-mail password (link expires in one hour):\n%s\n\n"+
			"If you did not request this, you can ignore this email.", link),
	})
}

// ResetPassword consumes a reset token and signs every existing session out.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	userID, err := s.repo.ConsumePasswordReset(ctx, HashToken(token))
	if err != nil {
		return err
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	if err := s.repo.InvalidatePasswordResets(ctx, userID); err != nil {
		s.logger.WarnContext(ctx, "invalidate password resets", "error", err, "user_id", userID)
	}
	// Whoever held a session before the reset may be the attacker.
	return s.repo.DeleteUserSessions(ctx, userID)
}

func (s *Service) VerifyEmail(ctx context.Context, token string) (*User, error) {
	return s.repo.ConsumeEmailVerification(ctx, HashToken(token))
}

// ResendVerification re-sends the link. It is a no-op for already-verified
// users rather than an error, so the endpoint is safe to retry.
func (s *Service) ResendVerification(ctx context.Context, user *User) error {
	if user.EmailVerifiedAt != nil {
		return nil
	}
	return s.sendVerificationEmail(ctx, user)
}

func (s *Service) sendVerificationEmail(ctx context.Context, user *User) error {
	token, digest, err := NewToken()
	if err != nil {
		return err
	}
	if err := s.repo.CreateEmailVerification(ctx, user.ID, user.Email, digest, time.Now().Add(emailVerificationTTL)); err != nil {
		return err
	}

	link := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(s.opts.AppBaseURL, "/"), token)
	return s.mail.Send(ctx, mailer.Message{
		To:      user.Email,
		Subject: "Confirm your x-mail address",
		HTML: fmt.Sprintf(
			`<p>Welcome%s,</p><p>Confirm this address to finish setting up your account. `+
				`The link expires in 24 hours.</p><p><a href="%s">Confirm my email</a></p>`,
			greetingName(user), link),
		Text: fmt.Sprintf("Confirm your x-mail address (link expires in 24 hours):\n%s", link),
	})
}

func (s *Service) issueSession(ctx context.Context, userID uuid.UUID, meta RequestMeta) (string, error) {
	token, digest, err := NewToken()
	if err != nil {
		return "", err
	}
	_, err = s.repo.CreateSession(ctx, userID, digest, truncate(meta.UserAgent, 512), meta.IP,
		time.Now().Add(s.opts.SessionTTL))
	if err != nil {
		return "", err
	}
	return token, nil
}

// revokeOtherSessions drops every session except the one making the request, so
// the caller is not logged out of the tab they are using.
func (s *Service) revokeOtherSessions(ctx context.Context, userID uuid.UUID, keepToken string) error {
	if keepToken == "" {
		return s.repo.DeleteUserSessions(ctx, userID)
	}
	return s.repo.DeleteUserSessionsExcept(ctx, userID, HashToken(keepToken))
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func greetingName(u *User) string {
	if name := strings.TrimSpace(u.Name); name != "" {
		return " " + name
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
