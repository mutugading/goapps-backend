package chat_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

func TestConversation_NewDirect(t *testing.T) {
	ownerID := uuid.New()
	peerID := uuid.New()
	convKey := make([]byte, 32)
	encKey := []byte("encrypted-key-bytes")

	conv, err := chat.NewDirectConversation(ownerID, peerID, convKey, encKey)
	require.NoError(t, err)
	assert.Equal(t, chat.TypeDirect, conv.Type())
	assert.Len(t, conv.Participants(), 2)
	assert.NotEqual(t, uuid.Nil, conv.ID())
}

func TestConversation_DirectParticipantLimit(t *testing.T) {
	ownerID := uuid.New()
	peerID := uuid.New()
	conv, err := chat.NewDirectConversation(ownerID, peerID, make([]byte, 32), []byte("enc"))
	require.NoError(t, err)

	err = conv.AddParticipant(uuid.New(), chat.RoleMember)
	assert.ErrorIs(t, err, chat.ErrDirectConversationFull)
}

func TestMessage_SoftDelete(t *testing.T) {
	msg := chat.ReconstructMessage(
		uuid.New(), uuid.New(), uuid.New(),
		[]byte("enc"), []byte("plain-enc"),
		false, false, uuid.Nil,
		nil, time.Now(), time.Now(),
	)
	assert.False(t, msg.IsDeleted())
	msg.SoftDelete()
	assert.True(t, msg.IsDeleted())
}

func TestMessage_Edit(t *testing.T) {
	msg := chat.ReconstructMessage(
		uuid.New(), uuid.New(), uuid.New(),
		[]byte("enc"), []byte("plain-enc"),
		false, false, uuid.Nil,
		nil, time.Now(), time.Now(),
	)
	newEnc := []byte("new-encrypted-body")
	newPlain := []byte("new-plain-enc")
	old := msg.Edit(newEnc, newPlain)
	assert.Equal(t, []byte("enc"), old)
	assert.True(t, msg.IsEdited())
	assert.Equal(t, newEnc, msg.BodyEncrypted())
}
