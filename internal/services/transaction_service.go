package services

import (
	"context"
	"time"

	"tierify/internal/errors"
	"tierify/internal/logger"
	"tierify/internal/models"
	"tierify/internal/repositories"
)

type TransactionService interface {
	Get(ctx context.Context, txID string) (*models.Transaction, error)
	Commit(ctx context.Context, txID string) error
	Rollback(ctx context.Context, txID string) error
	CleanExpired(ctx context.Context) (int, error)
}

type transactionService struct {
	repos    *repositories.Repositories
	usageSvc UsageService
	log      logger.Logger
}

func NewTransactionService(repos *repositories.Repositories, usageSvc UsageService, log logger.Logger) TransactionService {
	return &transactionService{repos: repos, usageSvc: usageSvc, log: log}
}

func (s *transactionService) Get(ctx context.Context, txID string) (*models.Transaction, error) {
	tx, err := s.repos.Transactions.GetByID(ctx, txID)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, errors.NotFound("transaction", txID)
	}

	// Auto-expire if TTL exceeded.
	if tx.Status == models.TransactionStatusPending && tx.ExpiresAt != nil && time.Now().UTC().After(*tx.ExpiresAt) {
		s.repos.Transactions.UpdateStatus(ctx, txID, models.TransactionStatusExpired)
		tx.Status = models.TransactionStatusExpired
	}

	return tx, nil
}

func (s *transactionService) Commit(ctx context.Context, txID string) error {
	tx, err := s.Get(ctx, txID)
	if err != nil {
		return err
	}

	if tx.Status != models.TransactionStatusPending {
		return errors.BadRequest("transaction is not pending (status: " + string(tx.Status) + ")")
	}

	// Convert transaction operations to usage operations and apply.
	ops := make([]UsageOperation, 0, len(tx.Operations))
	for _, op := range tx.Operations {
		amount := op.Amount
		if op.Operation == "decrement" {
			amount = -amount
		}
		ops = append(ops, UsageOperation{LimitKey: op.LimitKey, Amount: amount})
	}

	if len(ops) > 0 {
		if err := s.usageSvc.ApplyOperations(ctx, tx.TenantID, ops); err != nil {
			return err
		}
	}

	return s.repos.Transactions.UpdateStatus(ctx, txID, models.TransactionStatusCommitted)
}

func (s *transactionService) Rollback(ctx context.Context, txID string) error {
	tx, err := s.Get(ctx, txID)
	if err != nil {
		return err
	}

	if tx.Status != models.TransactionStatusPending {
		return errors.BadRequest("transaction is not pending (status: " + string(tx.Status) + ")")
	}

	return s.repos.Transactions.UpdateStatus(ctx, txID, models.TransactionStatusRolledBack)
}

func (s *transactionService) CleanExpired(ctx context.Context) (int, error) {
	return s.repos.Transactions.CleanExpired(ctx)
}
