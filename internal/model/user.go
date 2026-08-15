package model

import "time"

type User struct {
	ID         string    `json:"id" db:"id"`
	Email      string    `json:"email" db:"email"`
	Name       string    `json:"name" db:"name"`
	ImageURL   string    `json:"image_url" db:"image_url"`
	SuperAdmin bool      `json:"super_admin" db:"super_admin"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}
