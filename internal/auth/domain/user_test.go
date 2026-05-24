package domain

import "testing"

func TestRole_IsValid(t *testing.T) {
	tests := []struct {
		role  Role
		valid bool
	}{
		{RoleAdmin, true},
		{RoleDeveloper, true},
		{RoleViewer, true},
		{Role("superadmin"), false},
		{Role(""), false},
		{Role("ADMIN"), false}, // case-sensitive
		{Role("Admin"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.valid {
				t.Errorf("Role(%q).IsValid() = %v, want %v", tt.role, got, tt.valid)
			}
		})
	}
}
