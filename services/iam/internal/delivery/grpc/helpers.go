// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// toAuditProto converts domain AuditInfo to proto AuditInfo.
// Proto AuditInfo uses string fields in ISO 8601 format.
func toAuditProto(a shared.AuditInfo) *commonv1.AuditInfo {
	info := &commonv1.AuditInfo{
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
		CreatedBy: a.CreatedBy,
	}

	if a.UpdatedAt != nil {
		info.UpdatedAt = a.UpdatedAt.Format(time.RFC3339)
	}
	if a.UpdatedBy != nil {
		info.UpdatedBy = *a.UpdatedBy
	}

	return info
}
