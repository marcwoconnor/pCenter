package library

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/moconnor/pcenter/internal/state"
)

// Service provides content library operations
type Service struct {
	db    *DB
	store *state.Store
}

// NewService creates a new content library service
func NewService(db *DB, store *state.Store) *Service {
	return &Service{db: db, store: store}
}

// List returns library items matching the filter
func (s *Service) List(ctx context.Context, filter ListFilter) ([]*Item, error) {
	return s.db.List(ctx, filter)
}

// Get returns a single library item
func (s *Service) Get(ctx context.Context, id string) (*Item, error) {
	return s.db.Get(ctx, id)
}

// Create adds a new item to the library
func (s *Service) Create(ctx context.Context, req CreateItemRequest) (*Item, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	if req.Cluster == "" {
		return nil, fmt.Errorf("cluster is required")
	}
	if req.Storage == "" {
		return nil, fmt.Errorf("storage is required")
	}
	if req.Volume == "" {
		return nil, fmt.Errorf("volume is required")
	}

	// Validate type
	switch req.Type {
	case ItemTypeISO, ItemTypeVZTemplate, ItemTypeVMTemplate, ItemTypeOVA, ItemTypeSnippet:
		// ok
	default:
		return nil, fmt.Errorf("invalid type: %s", req.Type)
	}

	item := &Item{
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Category:    req.Category,
		Version:     req.Version,
		Tags:        req.Tags,
		Cluster:     req.Cluster,
		Node:        req.Node,
		Storage:     req.Storage,
		Volume:      req.Volume,
		Size:        req.Size,
		Format:      req.Format,
		VMID:        req.VMID,
		OSType:      req.OSType,
		Cores:       req.Cores,
		Memory:      req.Memory,
	}

	if item.Tags == nil {
		item.Tags = []string{}
	}

	if err := s.db.Create(ctx, item); err != nil {
		return nil, fmt.Errorf("create library item: %w", err)
	}

	slog.Info("library item created", "id", item.ID, "name", item.Name, "type", item.Type)
	return item, nil
}

// Update modifies an existing library item
func (s *Service) Update(ctx context.Context, id string, req UpdateItemRequest) error {
	if err := s.db.Update(ctx, id, req); err != nil {
		return fmt.Errorf("update library item: %w", err)
	}
	slog.Info("library item updated", "id", id)
	return nil
}

// Delete removes a library item (metadata only)
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.db.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete library item: %w", err)
	}
	slog.Info("library item deleted", "id", id)
	return nil
}

// GetCategories returns all distinct categories
func (s *Service) GetCategories(ctx context.Context) ([]string, error) {
	return s.db.GetCategories(ctx)
}
