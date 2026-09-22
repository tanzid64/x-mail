package mailbox

import "fmt"

// Folder is a mailbox view in the sidebar. Most map onto a system label;
// starred and important are message flags instead, because they are per-message
// booleans with no need for the label machinery.
type Folder string

const (
	FolderInbox     Folder = "inbox"
	FolderStarred   Folder = "starred"
	FolderSnoozed   Folder = "snoozed"
	FolderSent      Folder = "sent"
	FolderDrafts    Folder = "drafts"
	FolderArchive   Folder = "archive"
	FolderSpam      Folder = "spam"
	FolderTrash     Folder = "trash"
	FolderImportant Folder = "important"
)

// SystemLabel returns the label a folder maps to, or "" when the folder is
// defined by a message flag instead.
func (f Folder) SystemLabel() string {
	switch f {
	case FolderInbox:
		return LabelInbox
	case FolderSent:
		return LabelSent
	case FolderDrafts:
		return LabelDraft
	case FolderArchive:
		return LabelArchive
	case FolderSpam:
		return LabelSpam
	case FolderTrash:
		return LabelTrash
	default:
		return ""
	}
}

// Valid reports whether f is a folder the API serves.
func (f Folder) Valid() bool {
	switch f {
	case FolderInbox, FolderStarred, FolderSnoozed, FolderSent, FolderDrafts,
		FolderArchive, FolderSpam, FolderTrash, FolderImportant:
		return true
	default:
		return false
	}
}

func ParseFolder(value string) (Folder, error) {
	folder := Folder(value)
	if !folder.Valid() {
		return "", fmt.Errorf("mailbox: unknown folder %q", value)
	}
	return folder, nil
}

// AllFolders is the sidebar order.
var AllFolders = []Folder{
	FolderInbox, FolderStarred, FolderSnoozed, FolderSent,
	FolderDrafts, FolderArchive, FolderSpam, FolderTrash,
}
