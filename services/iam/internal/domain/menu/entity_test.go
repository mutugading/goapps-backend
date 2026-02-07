// Package menu provides domain logic for dynamic menu management.
package menu

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewMenu(t *testing.T) {
	tests := []struct {
		name        string
		parentID    *uuid.UUID
		code        string
		title       string
		url         string
		iconName    string
		serviceName string
		level       int
		sortOrder   int
		isVisible   bool
		createdBy   string
		wantErr     bool
	}{
		{
			name:        "valid root menu",
			parentID:    nil,
			code:        "DASHBOARD",
			title:       "Dashboard",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       MenuLevelRoot,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     false,
		},
		{
			name:        "empty code",
			parentID:    nil,
			code:        "",
			title:       "Dashboard",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       MenuLevelRoot,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     true,
		},
		{
			name:        "empty title",
			parentID:    nil,
			code:        "DASH",
			title:       "",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       MenuLevelRoot,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     true,
		},
		{
			name:        "invalid code format - lowercase",
			parentID:    nil,
			code:        "dashboard",
			title:       "Dashboard",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       MenuLevelRoot,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     true,
		},
		{
			name:        "invalid level",
			parentID:    nil,
			code:        "DASH",
			title:       "Dashboard",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       0,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     true,
		},
		{
			name:        "root menu with parent should fail",
			parentID:    ptrUUID(uuid.New()),
			code:        "DASH",
			title:       "Dashboard",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       MenuLevelRoot,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     true,
		},
		{
			name:        "child menu without parent should fail",
			parentID:    nil,
			code:        "DASH",
			title:       "Dashboard",
			url:         "/dashboard",
			iconName:    "home",
			serviceName: "goapps",
			level:       MenuLevelParent,
			sortOrder:   1,
			isVisible:   true,
			createdBy:   "admin",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewMenu(
				tt.parentID,
				tt.code,
				tt.title,
				tt.url,
				tt.iconName,
				tt.serviceName,
				tt.level,
				tt.sortOrder,
				tt.isVisible,
				tt.createdBy,
			)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewMenu() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewMenu() unexpected error: %v", err)
				}
				if m == nil {
					t.Errorf("NewMenu() returned nil")
					return
				}
				if m.Code() != tt.code {
					t.Errorf("Menu.Code() = %v, want %v", m.Code(), tt.code)
				}
				if m.Title() != tt.title {
					t.Errorf("Menu.Title() = %v, want %v", m.Title(), tt.title)
				}
				if m.Level() != tt.level {
					t.Errorf("Menu.Level() = %v, want %v", m.Level(), tt.level)
				}
				if !m.IsActive() {
					t.Errorf("Menu.IsActive() = false, want true")
				}
			}
		})
	}
}

func TestMenu_Update(t *testing.T) {
	m, _ := NewMenu(nil, "DASH", "Dashboard", "/dash", "home", "goapps", 1, 1, true, "admin")

	newTitle := "Updated Dashboard"
	newURL := "/updated"
	newIcon := "star"
	newOrder := 5
	isVisible := false

	err := m.Update(&newTitle, &newURL, &newIcon, &newOrder, &isVisible, nil, "updater")
	if err != nil {
		t.Errorf("Menu.Update() unexpected error: %v", err)
	}

	if m.Title() != newTitle {
		t.Errorf("Menu.Title() = %v, want %v", m.Title(), newTitle)
	}
	if m.URL() != newURL {
		t.Errorf("Menu.URL() = %v, want %v", m.URL(), newURL)
	}
	if m.IconName() != newIcon {
		t.Errorf("Menu.IconName() = %v, want %v", m.IconName(), newIcon)
	}
	if m.SortOrder() != newOrder {
		t.Errorf("Menu.SortOrder() = %v, want %v", m.SortOrder(), newOrder)
	}
	if m.IsVisible() != isVisible {
		t.Errorf("Menu.IsVisible() = %v, want %v", m.IsVisible(), isVisible)
	}
}

func TestMenu_SoftDelete(t *testing.T) {
	m, _ := NewMenu(nil, "DASH", "Dashboard", "/dash", "home", "goapps", 1, 1, true, "admin")

	err := m.SoftDelete("deleter")
	if err != nil {
		t.Errorf("Menu.SoftDelete() unexpected error: %v", err)
	}

	if !m.IsDeleted() {
		t.Errorf("Menu.IsDeleted() = false, want true")
	}
	if m.IsActive() {
		t.Errorf("Menu.IsActive() = true, want false")
	}

	// Double delete should fail
	err = m.SoftDelete("deleter")
	if err == nil {
		t.Errorf("Menu.SoftDelete() on deleted menu should return error")
	}
}

func TestValidChildMenu(t *testing.T) {
	// Create parent
	parent, err := NewMenu(nil, "PARENT", "Parent Menu", "/parent", "folder", "goapps", MenuLevelRoot, 1, true, "admin")
	if err != nil {
		t.Fatalf("Failed to create parent menu: %v", err)
	}

	parentID := parent.ID()

	// Create child
	child, err := NewMenu(&parentID, "CHILD", "Child Menu", "/child", "file", "goapps", MenuLevelParent, 1, true, "admin")
	if err != nil {
		t.Fatalf("Failed to create child menu: %v", err)
	}

	if child.ParentID() == nil || *child.ParentID() != parentID {
		t.Errorf("Child menu parent ID mismatch")
	}
	if child.Level() != MenuLevelParent {
		t.Errorf("Child menu level = %v, want %v", child.Level(), MenuLevelParent)
	}
}

// Helper function
func ptrUUID(u uuid.UUID) *uuid.UUID {
	return &u
}
