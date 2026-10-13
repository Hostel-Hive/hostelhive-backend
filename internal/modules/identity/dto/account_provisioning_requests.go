package dto

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type Input struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

// Result reports current account state without exposing credential material.

type Result struct {
	Account  domain.Account `json:"account"`
	Replayed bool           `json:"replayed"`
}
