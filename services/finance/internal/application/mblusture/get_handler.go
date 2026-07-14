package mblusture

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// GetQuery represents the get MB lusture query.
type GetQuery struct {
	ID string
}

// GetHandler handles the GetMbLusture query.
type GetHandler struct {
	repo mblusture.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo mblusture.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get MB lusture query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*mblusture.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
