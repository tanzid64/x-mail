package auth_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/mailer"
	"github.com/dark/x-mail/api/internal/testsupport"
)

// captureMailer records what would have been sent so tests can pull the
// one-time links out of the message body.
type captureMailer struct{ sent []mailer.Message }

func (m *captureMailer) Send(_ context.Context, msg mailer.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

func (m *captureMailer) last(t *testing.T) mailer.Message {
	t.Helper()
	if len(m.sent) == 0 {
		t.Fatal("expected an email to have been sent, got none")
	}
	return m.sent[len(m.sent)-1]
}

// tokenFrom pulls the `?token=` value out of a message body.
func tokenFrom(t *testing.T, msg mailer.Message) string {
	t.Helper()
	_, after, found := strings.Cut(msg.Text, "token=")
	if !found {
		t.Fatalf("no token in message: %q", msg.Text)
	}
	token, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(token)
}

func newService(t *testing.T) (*auth.Service, *captureMailer) {
	t.Helper()

	pool := testsupport.Pool(t)
	mail := &captureMailer{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := auth.NewService(
		auth.NewRepository(pool),
		mail,
		auth.Options{SessionTTL: time.Hour, AppBaseURL: "http://localhost:5180"},
		logger,
	)
	return svc, mail
}

func register(t *testing.T, svc *auth.Service, email, password string) (*auth.User, string) {
	t.Helper()
	user, token, err := svc.Register(context.Background(), email, password, "Test User", auth.RequestMeta{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return user, token
}

func TestRegisterCreatesUserAndSession(t *testing.T) {
	svc, mail := newService(t)
	ctx := context.Background()

	user, token := register(t, svc, "New@Example.com", "a-very-good-password")

	// Email is normalised so that casing cannot create a second account.
	if user.Email != "new@example.com" {
		t.Errorf("email = %q, want lowercased", user.Email)
	}
	if user.EmailVerifiedAt != nil {
		t.Error("a new account should start unverified")
	}
	if len(mail.sent) != 1 {
		t.Errorf("expected one verification email, got %d", len(mail.sent))
	}

	got, _, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate with the returned token: %v", err)
	}
	if got.ID != user.ID {
		t.Error("session resolved to a different user")
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "dup@example.com", "a-very-good-password")

	_, _, err := svc.Register(context.Background(), "DUP@example.com", "another-password", "", auth.RequestMeta{})
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	register(t, svc, "login@example.com", "a-very-good-password")

	if _, _, err := svc.Login(ctx, "login@example.com", "wrong-password", auth.RequestMeta{}); err == nil {
		t.Error("login with the wrong password should fail")
	}
	if _, _, err := svc.Login(ctx, "ghost@example.com", "a-very-good-password", auth.RequestMeta{}); err == nil {
		t.Error("login for an unknown account should fail")
	}

	if _, _, err := svc.Login(ctx, "LOGIN@example.com", "a-very-good-password", auth.RequestMeta{}); err != nil {
		t.Errorf("login with correct credentials failed: %v", err)
	}
}

func TestLogoutInvalidatesOnlyThatSession(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	register(t, svc, "multi@example.com", "a-very-good-password")

	_, first, err := svc.Login(ctx, "multi@example.com", "a-very-good-password", auth.RequestMeta{})
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	_, second, err := svc.Login(ctx, "multi@example.com", "a-very-good-password", auth.RequestMeta{})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if err := svc.Logout(ctx, first); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, first); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("logged-out session still resolves: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, second); err != nil {
		t.Errorf("the other session should still be valid: %v", err)
	}
}

func TestVerifyEmailConsumesTokenExactlyOnce(t *testing.T) {
	svc, mail := newService(t)
	ctx := context.Background()
	register(t, svc, "verify@example.com", "a-very-good-password")

	token := tokenFrom(t, mail.last(t))

	user, err := svc.VerifyEmail(ctx, token)
	if err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if user.EmailVerifiedAt == nil {
		t.Error("email_verified_at was not set")
	}

	if _, err := svc.VerifyEmail(ctx, token); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("replaying a verification token should fail, got %v", err)
	}
}

func TestPasswordResetRotatesCredentialsAndSessions(t *testing.T) {
	svc, mail := newService(t)
	ctx := context.Background()
	_, session := register(t, svc, "reset@example.com", "the-old-password")

	if err := svc.RequestPasswordReset(ctx, "reset@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	token := tokenFrom(t, mail.last(t))

	if err := svc.ResetPassword(ctx, token, "the-new-password"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Anyone holding a pre-reset session loses it.
	if _, _, err := svc.Authenticate(ctx, session); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("sessions should be revoked by a password reset, got %v", err)
	}
	if _, _, err := svc.Login(ctx, "reset@example.com", "the-old-password", auth.RequestMeta{}); err == nil {
		t.Error("the old password should no longer work")
	}
	if _, _, err := svc.Login(ctx, "reset@example.com", "the-new-password", auth.RequestMeta{}); err != nil {
		t.Errorf("the new password should work: %v", err)
	}
	if err := svc.ResetPassword(ctx, token, "yet-another-password"); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("replaying a reset token should fail, got %v", err)
	}
}

// An unknown address must not be distinguishable from a known one, otherwise
// the endpoint becomes an account-enumeration oracle.
func TestRequestPasswordResetHidesUnknownAccounts(t *testing.T) {
	svc, mail := newService(t)

	if err := svc.RequestPasswordReset(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset for an unknown address returned an error: %v", err)
	}
	if len(mail.sent) != 0 {
		t.Error("no email should be sent for an unregistered address")
	}
}

func TestChangePasswordKeepsCurrentSessionAndDropsOthers(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	user, current := register(t, svc, "change@example.com", "the-old-password")

	_, other, err := svc.Login(ctx, "change@example.com", "the-old-password", auth.RequestMeta{})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if err := svc.ChangePassword(ctx, user.ID, "wrong-current", "the-new-password", current); err == nil {
		t.Error("changing a password without the correct current one should fail")
	}
	if err := svc.ChangePassword(ctx, user.ID, "the-old-password", "the-new-password", current); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if _, _, err := svc.Authenticate(ctx, current); err != nil {
		t.Errorf("the session that changed the password should survive: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, other); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("other sessions should be revoked, got %v", err)
	}
}
