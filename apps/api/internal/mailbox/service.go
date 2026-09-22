package mailbox

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/realtime"
)

// AccountResolver supplies the set of mail accounts a request may read.
//
// The mailbox never takes account ids from the client: it asks for the ones the
// session user owns, and a single-account view is validated against that set.
type AccountResolver interface {
	AccountIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// Publisher notifies a user's other open browsers that the mailbox changed.
// Optional: the mailbox works without it, clients just refetch on their own.
type Publisher interface {
	PublishQuietly(ctx context.Context, event realtime.Event)
}

type Service struct {
	repo      *Repository
	accounts  AccountResolver
	publisher Publisher
	logger    *slog.Logger
}

func NewService(repo *Repository, accountResolver AccountResolver, logger *slog.Logger) *Service {
	return &Service{repo: repo, accounts: accountResolver, logger: logger}
}

// SetPublisher installs the realtime publisher.
func (s *Service) SetPublisher(publisher Publisher) { s.publisher = publisher }

// notifyThreads tells the user's other tabs that these threads changed, so a
// message archived on a laptop disappears from the phone too.
func (s *Service) notifyThreads(ctx context.Context, userID uuid.UUID, action Action, threadIDs []uuid.UUID) {
	if s.publisher == nil || len(threadIDs) == 0 {
		return
	}
	ids := make([]string, 0, len(threadIDs))
	for _, id := range threadIDs {
		ids = append(ids, id.String())
	}
	s.publisher.PublishQuietly(ctx, realtime.Event{
		Type:   realtime.EventThreadUpdated,
		UserID: userID,
		Data:   map[string]any{"action": string(action), "thread_ids": ids},
	})
}

// Scope resolves which accounts a request may touch.
//
// accountID nil means the unified view across every account the user owns.
// A supplied id must be one of them, otherwise the request reads as empty
// rather than as someone else's mail.
func (s *Service) Scope(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]uuid.UUID, error) {
	owned, err := s.accounts.AccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if accountID == nil {
		return owned, nil
	}
	for _, id := range owned {
		if id == *accountID {
			return []uuid.UUID{*accountID}, nil
		}
	}
	return nil, ErrNotFound
}

type ListRequest struct {
	AccountID  *uuid.UUID
	Folder     Folder
	LabelID    *uuid.UUID
	Limit      int
	Cursor     string
	UnreadOnly bool
}

func (s *Service) ListThreads(ctx context.Context, userID uuid.UUID, req ListRequest) (*Page, error) {
	scope, err := s.Scope(ctx, userID, req.AccountID)
	if err != nil {
		return nil, err
	}
	// With no accounts connected there is nothing to read, and ANY('{}') would
	// scan every thread in the table if the predicate were built differently.
	if len(scope) == 0 {
		return &Page{Threads: []Thread{}}, nil
	}

	return s.repo.ListThreads(ctx, ListParams{
		AccountIDs: scope,
		Folder:     req.Folder,
		LabelID:    req.LabelID,
		Limit:      req.Limit,
		Cursor:     req.Cursor,
		UnreadOnly: req.UnreadOnly,
	})
}

// GetThread loads a conversation and marks it read, which is what opening a
// thread means in a mail client.
func (s *Service) GetThread(ctx context.Context, userID, threadID uuid.UUID, markRead bool) (*ThreadDetail, error) {
	scope, err := s.Scope(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	if len(scope) == 0 {
		return nil, ErrNotFound
	}

	detail, err := s.repo.GetThread(ctx, scope, threadID)
	if err != nil {
		return nil, err
	}

	if markRead && detail.Thread.UnreadCount > 0 {
		if _, err := s.repo.SetRead(ctx, scope, []uuid.UUID{threadID}, true); err != nil {
			// Failing to clear the unread flag should not hide the mail.
			s.logger.WarnContext(ctx, "mark thread read", "error", err, "thread_id", threadID)
		} else {
			return s.repo.GetThread(ctx, scope, threadID)
		}
	}
	return detail, nil
}

// Action is a bulk operation applied to a set of threads.
type Action string

const (
	ActionRead        Action = "read"
	ActionUnread      Action = "unread"
	ActionStar        Action = "star"
	ActionUnstar      Action = "unstar"
	ActionImportant   Action = "important"
	ActionUnimportant Action = "unimportant"
	ActionArchive     Action = "archive"
	ActionInbox       Action = "inbox"
	ActionSpam        Action = "spam"
	ActionNotSpam     Action = "not_spam"
	ActionTrash       Action = "trash"
	ActionRestore     Action = "restore"
	ActionDelete      Action = "delete"
)

func ParseAction(value string) (Action, bool) {
	action := Action(strings.TrimSpace(value))
	switch action {
	case ActionRead, ActionUnread, ActionStar, ActionUnstar, ActionImportant, ActionUnimportant,
		ActionArchive, ActionInbox, ActionSpam, ActionNotSpam, ActionTrash, ActionRestore, ActionDelete:
		return action, true
	default:
		return "", false
	}
}

// Apply performs a bulk action. It returns how many threads were affected;
// ids the user does not own are silently skipped rather than reported, so the
// response cannot be used to probe for other people's threads.
func (s *Service) Apply(ctx context.Context, userID uuid.UUID, action Action, threadIDs []uuid.UUID, labelID *uuid.UUID) (int, error) {
	scope, err := s.Scope(ctx, userID, nil)
	if err != nil {
		return 0, err
	}
	if len(scope) == 0 || len(threadIDs) == 0 {
		return 0, nil
	}

	// Whatever the outcome, the user's other tabs should refetch.
	defer s.notifyThreads(ctx, userID, action, threadIDs)

	switch action {
	case ActionRead:
		return s.repo.SetRead(ctx, scope, threadIDs, true)
	case ActionUnread:
		return s.repo.SetRead(ctx, scope, threadIDs, false)
	case ActionStar:
		return s.repo.SetStarred(ctx, scope, threadIDs, true)
	case ActionUnstar:
		return s.repo.SetStarred(ctx, scope, threadIDs, false)
	case ActionImportant:
		return s.repo.SetImportant(ctx, scope, threadIDs, true)
	case ActionUnimportant:
		return s.repo.SetImportant(ctx, scope, threadIDs, false)
	case ActionArchive:
		return s.repo.Move(ctx, scope, threadIDs, LabelArchive)
	case ActionInbox, ActionNotSpam, ActionRestore:
		// Restoring from trash or spam puts a thread back in the inbox.
		return s.repo.Move(ctx, scope, threadIDs, LabelInbox)
	case ActionSpam:
		return s.repo.Move(ctx, scope, threadIDs, LabelSpam)
	case ActionTrash:
		return s.repo.Move(ctx, scope, threadIDs, LabelTrash)
	case ActionDelete:
		return s.repo.DeleteThreads(ctx, scope, threadIDs)
	default:
		return 0, ErrNotFound
	}
}

// ApplyLabel adds or removes a user label across threads.
func (s *Service) ApplyLabel(ctx context.Context, userID uuid.UUID, threadIDs []uuid.UUID, labelID uuid.UUID, apply bool) (int, error) {
	scope, err := s.Scope(ctx, userID, nil)
	if err != nil {
		return 0, err
	}
	if len(scope) == 0 {
		return 0, nil
	}
	return s.repo.ApplyLabel(ctx, scope, threadIDs, labelID, apply)
}

// ----------------------------------------------------------------- labels --

func (s *Service) ListLabels(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]Label, error) {
	scope, err := s.Scope(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if len(scope) == 0 {
		return []Label{}, nil
	}
	return s.repo.ListLabels(ctx, scope)
}

func (s *Service) CreateLabel(ctx context.Context, userID, accountID uuid.UUID, name, color string) (*Label, error) {
	if _, err := s.Scope(ctx, userID, &accountID); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if color == "" {
		color = "slate"
	}
	return s.repo.CreateLabel(ctx, accountID, name, color)
}

func (s *Service) UpdateLabel(ctx context.Context, userID, labelID uuid.UUID, name, color *string) (*Label, error) {
	scope, err := s.Scope(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		name = &trimmed
	}
	return s.repo.UpdateLabel(ctx, scope, labelID, name, color)
}

func (s *Service) DeleteLabel(ctx context.Context, userID, labelID uuid.UUID) error {
	scope, err := s.Scope(ctx, userID, nil)
	if err != nil {
		return err
	}
	return s.repo.DeleteLabel(ctx, scope, labelID)
}

// FolderCounts returns the unread count per folder for the sidebar.
func (s *Service) FolderCounts(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) (map[string]int, error) {
	scope, err := s.Scope(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if len(scope) == 0 {
		return map[string]int{}, nil
	}
	return s.repo.FolderCounts(ctx, scope)
}
