package chat

import "errors"

// ErrConversationNotFound indicates the conversation does not exist.
var ErrConversationNotFound = errors.New("conversation not found")

// ErrMessageNotFound indicates the message does not exist.
var ErrMessageNotFound = errors.New("message not found")

// ErrNotParticipant indicates the user is not a participant.
var ErrNotParticipant = errors.New("user is not a participant of this conversation")

// ErrNotAuthor indicates the user is not the message author.
var ErrNotAuthor = errors.New("user is not the author of this message")

// ErrNotAdmin indicates the user lacks admin role.
var ErrNotAdmin = errors.New("user is not an admin of this conversation")

// ErrDirectConversationFull indicates a direct conversation already has 2 participants.
var ErrDirectConversationFull = errors.New("direct conversation cannot have more than 2 participants")

// ErrAlreadyParticipant indicates the user is already in the conversation.
var ErrAlreadyParticipant = errors.New("user is already a participant")

// ErrMessageDeleted indicates the message has been soft-deleted.
var ErrMessageDeleted = errors.New("message has been deleted")

// ErrParticipantLeft indicates the participant already left.
var ErrParticipantLeft = errors.New("participant has already left the conversation")
