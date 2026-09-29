package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"moscow_hackathon_2026/api/internal/db"
)

func SeedDemos(ctx context.Context, q *db.Queries, userEmail, userPass, adminEmail, adminPass string, cost int) error {
	if err := ensureUser(ctx, q, userEmail, userPass, RoleUser, cost); err != nil {
		return err
	}
	return ensureUser(ctx, q, adminEmail, adminPass, RoleAdmin, cost)
}

func ensureUser(ctx context.Context, q *db.Queries, email, password, role string, cost int) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil
	}
	_, err := q.GetUserByEmail(ctx, email)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("auth.seed.get: %w", err)
	}
	hash, err := HashPassword(password, cost)
	if err != nil {
		return err
	}
	_, err = q.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		Role:         role,
	})
	if err != nil {
		return fmt.Errorf("auth.seed.create: %w", err)
	}
	return nil
}
