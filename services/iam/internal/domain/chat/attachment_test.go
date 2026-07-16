package chat

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAttachment(t *testing.T) {
	convID := uuid.New()
	uploaderID := uuid.New()

	tests := []struct {
		name         string
		fileName     string
		fileURL      string
		contentType  string
		fileSize     int64
		thumbnailURL string
	}{
		{
			name:         "image with thumbnail",
			fileName:     "photo.png",
			fileURL:      "https://cdn/iam/chat/attachments/x/y.png",
			contentType:  "image/png",
			fileSize:     2048,
			thumbnailURL: "https://cdn/iam/chat/attachments/x/y.png",
		},
		{
			name:         "document without thumbnail",
			fileName:     "report.pdf",
			fileURL:      "https://cdn/iam/chat/attachments/x/z.pdf",
			contentType:  "application/pdf",
			fileSize:     4096,
			thumbnailURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAttachment(convID, uploaderID, tt.fileName, tt.fileURL, tt.contentType, tt.fileSize, tt.thumbnailURL)
			require.NotNil(t, a)

			assert.NotEqual(t, uuid.Nil, a.AttachmentID())
			assert.Equal(t, convID, a.ConversationID())
			assert.Equal(t, uploaderID, a.UploaderUserID())
			assert.Equal(t, tt.fileName, a.FileName())
			assert.Equal(t, tt.fileURL, a.FileURL())
			assert.Equal(t, tt.contentType, a.ContentType())
			assert.Equal(t, tt.fileSize, a.FileSize())
			assert.Equal(t, tt.thumbnailURL, a.ThumbnailURL())
			assert.Nil(t, a.MessageID(), "new attachment must be unlinked")
			assert.False(t, a.CreatedAt().IsZero())
		})
	}
}

func TestAttachment_LinkToMessage(t *testing.T) {
	a := NewAttachment(uuid.New(), uuid.New(), "f.txt", "url", "text/plain", 10, "")
	require.Nil(t, a.MessageID())

	msgID := uuid.New()
	a.LinkToMessage(msgID)

	require.NotNil(t, a.MessageID())
	assert.Equal(t, msgID, *a.MessageID())
}

func TestReconstructAttachment(t *testing.T) {
	attID := uuid.New()
	convID := uuid.New()
	msgID := uuid.New()
	uploaderID := uuid.New()
	a := NewAttachment(convID, uploaderID, "f.txt", "url", "text/plain", 10, "")
	createdAt := a.CreatedAt()

	got := ReconstructAttachment(attID, convID, &msgID, uploaderID, "f.txt", "url", "text/plain", 10, "thumb", createdAt)

	assert.Equal(t, attID, got.AttachmentID())
	assert.Equal(t, convID, got.ConversationID())
	require.NotNil(t, got.MessageID())
	assert.Equal(t, msgID, *got.MessageID())
	assert.Equal(t, uploaderID, got.UploaderUserID())
	assert.Equal(t, "thumb", got.ThumbnailURL())
	assert.Equal(t, createdAt, got.CreatedAt())
}
