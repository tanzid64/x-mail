package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/httpx"
)

// CreatedAPIToken carries the one and only sight of the secret.
type CreatedAPIToken struct {
	Token     *APIToken
	Plaintext string
}

type CreateTokenRequest struct {
	Name          string
	Lifetime      Lifetime
	MailAccountID *uuid.UUID
}

// CreateAPIToken issues a token for the user.
//
// The plaintext is returned exactly once and never stored, so a lost token is
// replaced rather than recovered.
func (s *Service) CreateAPIToken(ctx context.Context, userID uuid.UUID, req CreateTokenRequest) (*CreatedAPIToken, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, httpx.UnprocessableEntity("Give the token a name.").WithFields(map[string]string{
			"name": "Give the token a name so you can recognise it later.",
		})
	}
	if len(name) > 100 {
		return nil, httpx.UnprocessableEntity("That name is too long.").WithFields(map[string]string{
			"name": "Use 100 characters or fewer.",
		})
	}

	expiresAt, err := req.Lifetime.ExpiresAt(time.Now())
	if err != nil {
		return nil, httpx.UnprocessableEntity("Choose how long the token should last.").WithFields(map[string]string{
			"expires_in": "Choose 15 days, 30 days, 1 year or never.",
		})
	}

	plaintext, digest, err := NewAPIToken()
	if err != nil {
		return nil, err
	}

	token, err := s.repo.CreateAPIToken(ctx, CreateAPITokenParams{
		UserID:        userID,
		MailAccountID: req.MailAccountID,
		Name:          name,
		Digest:        digest,
		Hint:          TokenHint(plaintext),
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		// The insert only produces a row when the mail account, if one was
		// named, belongs to this user — so no rows means it did not.
		if errors.Is(err, ErrAPITokenNotFound) {
			return nil, httpx.UnprocessableEntity("That mail account does not exist.").WithFields(map[string]string{
				"mail_account_id": "Pick one of your own mail accounts.",
			})
		}
		return nil, err
	}
	return &CreatedAPIToken{Token: token, Plaintext: plaintext}, nil
}

func (s *Service) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]APIToken, error) {
	return s.repo.ListAPITokens(ctx, userID)
}

func (s *Service) RevokeAPIToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	return s.repo.RevokeAPIToken(ctx, userID, tokenID)
}

// AuthenticateAPIToken resolves a presented secret to its owner.
//
// A token that is revoked or past its expiry is refused here rather than at any
// call site, which is what makes the expiry a real boundary instead of a label.
func (s *Service) AuthenticateAPIToken(ctx context.Context, plaintext string) (*User, *APIToken, error) {
	if plaintext == "" || !strings.HasPrefix(plaintext, TokenPrefix) {
		return nil, nil, ErrAPITokenNotFound
	}

	token, user, err := s.repo.APITokenUser(ctx, HashToken(plaintext))
	if err != nil {
		return nil, nil, err
	}
	if token.RevokedAt != nil {
		return nil, nil, ErrAPITokenRevoked
	}
	if token.IsExpired(time.Now()) {
		return nil, nil, ErrAPITokenExpired
	}

	if err := s.repo.TouchAPIToken(ctx, token.ID); err != nil {
		s.logger.WarnContext(ctx, "touch api token", "error", err, "token_id", token.ID)
	}
	return user, token, nil
}
