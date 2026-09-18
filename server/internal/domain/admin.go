package domain

import "time"

type Admin struct {
	ID                  string     `json:"id"`
	Email               string     `json:"email"`
	PasswordHash        string     `json:"-"`
	Status              string     `json:"status"`
	CreatedBy           string     `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
	FailedLoginAttempts int        `json:"-"`
	LockedUntil         *time.Time `json:"-"`
}
