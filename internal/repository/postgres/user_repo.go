package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type userRepo struct {
	db *sqlx.DB
}

// NewUserRepo creates a new PostgreSQL-backed UserRepository.
func NewUserRepo(db *sqlx.DB) port.UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) Create(ctx context.Context, user *domain.User) error {
	user.ID = uuid.New()
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now
	if user.CurrentPeriodStart.IsZero() {
		user.CurrentPeriodStart = now
	}

	query := `INSERT INTO users (id, tenant_id, email, password_hash, full_name, role, is_active,
		monthly_document_limit, documents_used_this_period, current_period_start,
		email_verified, email_verified_at, auth_provider, provider_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.TenantID, user.Email, user.PasswordHash, user.FullName,
		user.Role, user.IsActive, user.MonthlyDocumentLimit, user.DocumentsUsedThisPeriod,
		user.CurrentPeriodStart, user.EmailVerified, user.EmailVerifiedAt,
		user.AuthProvider, user.ProviderUserID,
		user.CreatedAt, user.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return domain.ErrDuplicateEmail
		}
		return fmt.Errorf("userRepo.Create: %w", err)
	}
	return nil
}

func (r *userRepo) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		"SELECT * FROM users WHERE id = $1 AND tenant_id = $2", userID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("userRepo.GetByID: %w", err)
	}
	return &user, nil
}

func (r *userRepo) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		"SELECT * FROM users WHERE tenant_id = $1 AND email = $2", tenantID, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("userRepo.GetByEmail: %w", err)
	}
	return &user, nil
}

func (r *userRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.User, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM users WHERE tenant_id = $1", tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("userRepo.ListByTenant count: %w", err)
	}

	var users []domain.User
	err = r.db.SelectContext(ctx, &users,
		"SELECT * FROM users WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("userRepo.ListByTenant: %w", err)
	}
	return users, total, nil
}

func (r *userRepo) Update(ctx context.Context, user *domain.User) error {
	user.UpdatedAt = time.Now().UTC()
	query := `UPDATE users SET email = $1, full_name = $2, role = $3, is_active = $4, updated_at = $5
		WHERE id = $6 AND tenant_id = $7`
	result, err := r.db.ExecContext(ctx, query,
		user.Email, user.FullName, user.Role, user.IsActive, user.UpdatedAt, user.ID, user.TenantID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return domain.ErrDuplicateEmail
		}
		return fmt.Errorf("userRepo.Update: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *userRepo) Delete(ctx context.Context, tenantID, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM users WHERE id = $1 AND tenant_id = $2", userID, tenantID)
	if err != nil {
		return fmt.Errorf("userRepo.Delete: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *userRepo) CheckAndIncrementQuota(ctx context.Context, tenantID, userID uuid.UUID) error {
	// 0 means unlimited — skip check entirely
	var limit int
	err := r.db.GetContext(ctx, &limit,
		"SELECT monthly_document_limit FROM users WHERE id = $1 AND tenant_id = $2", userID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("userRepo.CheckAndIncrementQuota lookup: %w", err)
	}
	if limit == 0 {
		return nil
	}

	// Reset period if >30 days have elapsed, then increment if under limit.
	// Single atomic UPDATE with conditional logic.
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET
			current_period_start = CASE
				WHEN NOW() - current_period_start > INTERVAL '30 days' THEN NOW()
				ELSE current_period_start
			END,
			documents_used_this_period = CASE
				WHEN NOW() - current_period_start > INTERVAL '30 days' THEN 1
				WHEN documents_used_this_period < monthly_document_limit THEN documents_used_this_period + 1
				ELSE documents_used_this_period
			END,
			updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
		  AND (
				NOW() - current_period_start > INTERVAL '30 days'
				OR documents_used_this_period < monthly_document_limit
		  )`,
		userID, tenantID)
	if err != nil {
		return fmt.Errorf("userRepo.CheckAndIncrementQuota: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrQuotaExceeded
	}
	return nil
}

func (r *userRepo) SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET email_verified = true, email_verified_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2`,
		userID, tenantID)
	if err != nil {
		return fmt.Errorf("userRepo.SetEmailVerified: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *userRepo) SetPasswordResetToken(ctx context.Context, tenantID, userID uuid.UUID, tokenID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_reset_token_id = $1, updated_at = NOW()
		 WHERE id = $2 AND tenant_id = $3`,
		tokenID, userID, tenantID)
	if err != nil {
		return fmt.Errorf("userRepo.SetPasswordResetToken: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *userRepo) ResetPassword(ctx context.Context, tenantID, userID uuid.UUID, passwordHash, expectedTokenID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, password_reset_token_id = NULL, updated_at = NOW()
		 WHERE id = $2 AND tenant_id = $3 AND password_reset_token_id = $4`,
		passwordHash, userID, tenantID, expectedTokenID)
	if err != nil {
		return fmt.Errorf("userRepo.ResetPassword: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrPasswordResetTokenInvalid
	}
	return nil
}

func (r *userRepo) AcceptInvitation(ctx context.Context, tenantID, userID uuid.UUID, passwordHash, expectedTokenID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users
		 SET password_hash = $1, email_verified = true, email_verified_at = NOW(),
		     password_reset_token_id = NULL, updated_at = NOW()
		 WHERE id = $2 AND tenant_id = $3 AND password_reset_token_id = $4`,
		passwordHash, userID, tenantID, expectedTokenID)
	if err != nil {
		return fmt.Errorf("userRepo.AcceptInvitation: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrInvitationTokenInvalid
	}
	return nil
}

func (r *userRepo) GetByProviderID(ctx context.Context, tenantID uuid.UUID, provider domain.AuthProvider, providerUserID string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		"SELECT * FROM users WHERE tenant_id = $1 AND auth_provider = $2 AND provider_user_id = $3",
		tenantID, provider, providerUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("userRepo.GetByProviderID: %w", err)
	}
	return &user, nil
}

func (r *userRepo) LinkProvider(ctx context.Context, tenantID, userID uuid.UUID, provider domain.AuthProvider, providerUserID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET auth_provider = $1, provider_user_id = $2, updated_at = NOW()
		 WHERE id = $3 AND tenant_id = $4`,
		provider, providerUserID, userID, tenantID)
	if err != nil {
		return fmt.Errorf("userRepo.LinkProvider: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
