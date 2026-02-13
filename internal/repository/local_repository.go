package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"subscriptions/internal/model"

	"github.com/google/uuid"
)

type LocalSubscriptionRepository struct {
	mu    sync.Mutex
	path  string
	items []model.Subscription
}

func NewLocalSubscriptionRepository(path string) (*LocalSubscriptionRepository, error) {
	repo := &LocalSubscriptionRepository{path: path, items: []model.Subscription{}}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *LocalSubscriptionRepository) load() error {
	if _, err := os.Stat(r.path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
			return fmt.Errorf("create localdb dir: %w", err)
		}
		if err := os.WriteFile(r.path, []byte("[]"), 0o644); err != nil {
			return fmt.Errorf("create localdb file: %w", err)
		}
		return nil
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		return fmt.Errorf("read localdb file: %w", err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		r.items = []model.Subscription{}
		return nil
	}
	if err := json.Unmarshal(b, &r.items); err != nil {
		return fmt.Errorf("parse localdb file: %w", err)
	}
	return nil
}

func (r *LocalSubscriptionRepository) save() error {
	b, err := json.MarshalIndent(r.items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal localdb: %w", err)
	}
	if err := os.WriteFile(r.path, b, 0o644); err != nil {
		return fmt.Errorf("write localdb file: %w", err)
	}
	return nil
}

func (r *LocalSubscriptionRepository) Create(_ context.Context, s model.Subscription) (model.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	s.ID = uuid.New()
	s.CreatedAt = now
	s.UpdatedAt = now
	r.items = append(r.items, s)
	if err := r.save(); err != nil {
		return model.Subscription{}, err
	}
	return s, nil
}

func (r *LocalSubscriptionRepository) GetByID(_ context.Context, id uuid.UUID) (model.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.items {
		if s.ID == id {
			return s, nil
		}
	}
	return model.Subscription{}, ErrNotFound
}

func (r *LocalSubscriptionRepository) Update(_ context.Context, id uuid.UUID, in model.Subscription) (model.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.items {
		if s.ID == id {
			in.ID = s.ID
			in.CreatedAt = s.CreatedAt
			in.UpdatedAt = time.Now().UTC()
			r.items[i] = in
			if err := r.save(); err != nil {
				return model.Subscription{}, err
			}
			return in, nil
		}
	}
	return model.Subscription{}, ErrNotFound
}

func (r *LocalSubscriptionRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.items {
		if s.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return r.save()
		}
	}
	return ErrNotFound
}

func (r *LocalSubscriptionRepository) List(_ context.Context, userID *uuid.UUID, serviceName string, limit, offset int) ([]model.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]model.Subscription, 0)
	serviceName = strings.ToLower(serviceName)
	for _, s := range r.items {
		if userID != nil && s.UserID != *userID {
			continue
		}
		if serviceName != "" && !strings.Contains(strings.ToLower(s.ServiceName), serviceName) {
			continue
		}
		filtered = append(filtered, s)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt.After(filtered[j].CreatedAt) })

	if offset >= len(filtered) {
		return []model.Subscription{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], nil
}

func (r *LocalSubscriptionRepository) TotalCost(_ context.Context, periodStart, periodEnd time.Time, userID *uuid.UUID, serviceName string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	var total int64
	for month := periodStart; !month.After(periodEnd); month = month.AddDate(0, 1, 0) {
		for _, s := range r.items {
			if userID != nil && s.UserID != *userID {
				continue
			}
			if serviceName != "" && !strings.Contains(strings.ToLower(s.ServiceName), serviceName) {
				continue
			}
			if s.StartDate.After(month) {
				continue
			}
			if s.EndDate != nil && s.EndDate.Before(month) {
				continue
			}
			total += int64(s.Price)
		}
	}
	return total, nil
}
