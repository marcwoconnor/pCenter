package tags

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var validColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var validObjectTypes = map[string]bool{"vm": true, "ct": true, "node": true, "storage": true}

// Service provides tag business logic
type Service struct {
	db *DB
}

// NewService creates a new tag service
func NewService(db *DB) *Service {
	return &Service{db: db}
}

// CreateTag validates and creates a tag
func (s *Service) CreateTag(ctx context.Context, req CreateTagRequest) (*Tag, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.Color = strings.TrimSpace(req.Color)

	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Color == "" {
		req.Color = DefaultColors[0]
	}
	if !validColor.MatchString(req.Color) {
		return nil, fmt.Errorf("color must be hex format (#rrggbb)")
	}
	return s.db.CreateTag(ctx, req.Category, req.Name, req.Color)
}

// UpdateTag validates and updates a tag
func (s *Service) UpdateTag(ctx context.Context, id string, req UpdateTagRequest) error {
	if req.Color != "" && !validColor.MatchString(req.Color) {
		return fmt.Errorf("color must be hex format (#rrggbb)")
	}
	return s.db.UpdateTag(ctx, id, req.Category, req.Name, req.Color)
}

// DeleteTag removes a tag and all its assignments
func (s *Service) DeleteTag(ctx context.Context, id string) error {
	return s.db.DeleteTag(ctx, id)
}

// ListTags returns all tags
func (s *Service) ListTags(ctx context.Context) ([]Tag, error) {
	return s.db.ListTags(ctx)
}

// AssignTag assigns a tag to an object
func (s *Service) AssignTag(ctx context.Context, req AssignTagRequest) error {
	if !validObjectTypes[req.ObjectType] {
		return fmt.Errorf("invalid object_type: %s (must be vm, ct, node, or storage)", req.ObjectType)
	}
	if req.ObjectID == "" {
		return fmt.Errorf("object_id is required")
	}
	return s.db.AssignTag(ctx, req.TagID, req.ObjectType, req.ObjectID, req.Cluster)
}

// UnassignTag removes a tag from an object
func (s *Service) UnassignTag(ctx context.Context, req UnassignTagRequest) error {
	return s.db.UnassignTag(ctx, req.TagID, req.ObjectType, req.ObjectID, req.Cluster)
}

// GetObjectTags returns all tags for an object
func (s *Service) GetObjectTags(ctx context.Context, objectType, objectID, cluster string) ([]Tag, error) {
	return s.db.GetObjectTags(ctx, objectType, objectID, cluster)
}

// GetAllAssignments returns all tag assignments (for frontend cache)
func (s *Service) GetAllAssignments(ctx context.Context) ([]TagAssignment, error) {
	return s.db.GetAllAssignments(ctx)
}

// BulkAssign assigns multiple tags to multiple objects
func (s *Service) BulkAssign(ctx context.Context, req BulkAssignRequest) error {
	if len(req.TagIDs) == 0 {
		return fmt.Errorf("tag_ids is required")
	}
	if len(req.Objects) == 0 {
		return fmt.Errorf("objects is required")
	}
	for _, obj := range req.Objects {
		if !validObjectTypes[obj.ObjectType] {
			return fmt.Errorf("invalid object_type: %s", obj.ObjectType)
		}
	}
	return s.db.BulkAssign(ctx, req.TagIDs, req.Objects)
}

// GetCategories returns default categories plus any custom ones in use
func (s *Service) GetCategories(ctx context.Context) ([]string, error) {
	tags, err := s.db.ListTags(ctx)
	if err != nil {
		return nil, err
	}

	catSet := make(map[string]bool)
	for _, c := range DefaultCategories {
		catSet[c] = true
	}
	for _, t := range tags {
		if t.Category != "" {
			catSet[t.Category] = true
		}
	}

	cats := make([]string, 0, len(catSet))
	for c := range catSet {
		cats = append(cats, c)
	}
	return cats, nil
}
