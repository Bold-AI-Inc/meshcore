package domain

import "time"

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type MeshAPIKey struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	KeyHash   string     `json:"key_hash"`
	KeyPrefix string     `json:"key_prefix"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

type UserWithKey struct {
	User      User    `json:"user"`
	KeyPrefix *string `json:"key_prefix"`
	KeyStatus *string `json:"key_status"`
}
