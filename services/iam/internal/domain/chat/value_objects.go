package chat

// Type represents the conversation type.
type Type string

const (
	// TypeDirect is a 1:1 direct message conversation.
	TypeDirect Type = "DIRECT"
	// TypeGroup is a multi-user group conversation.
	TypeGroup Type = "GROUP"
)

// String returns the string representation.
func (t Type) String() string { return string(t) }

// ParseType parses a string into a Type.
func ParseType(s string) (Type, error) {
	switch Type(s) {
	case TypeDirect, TypeGroup:
		return Type(s), nil
	default:
		return "", ErrConversationNotFound
	}
}

// Role represents a participant's role in a conversation.
type Role string

const (
	// RoleOwner is the conversation creator.
	RoleOwner Role = "OWNER"
	// RoleAdmin can manage participants.
	RoleAdmin Role = "ADMIN"
	// RoleMember is a regular participant.
	RoleMember Role = "MEMBER"
)

// String returns the string representation.
func (r Role) String() string { return string(r) }

// ParseRole parses a string into a Role.
func ParseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleOwner, RoleAdmin, RoleMember:
		return Role(s), nil
	default:
		return "", ErrNotParticipant
	}
}

// IsAdminOrOwner returns true if the role allows admin actions.
func (r Role) IsAdminOrOwner() bool {
	return r == RoleOwner || r == RoleAdmin
}
