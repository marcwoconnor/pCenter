package folders

import (
	"context"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_CreatesTables(t *testing.T) {
	db := openTestDB(t)

	// Verify both tables exist
	for _, table := range []string{"folders", "folder_members"} {
		var count int
		err := db.conn.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Fatalf("query %s table: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("expected 0 rows in fresh %s table, got %d", table, count)
		}
	}
}

func TestFolders_CreateRootFolder(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	folder, err := svc.CreateFolder(ctx, CreateFolderRequest{
		Name:     "Production",
		TreeView: TreeViewHosts,
	})
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	if folder.ID == "" {
		t.Fatal("folder ID should not be empty")
	}
	if folder.Name != "Production" {
		t.Errorf("expected name %q, got %q", "Production", folder.Name)
	}
	if folder.ParentID != nil {
		t.Error("root folder should have nil ParentID")
	}
	if folder.TreeView != TreeViewHosts {
		t.Errorf("expected tree_view %q, got %q", TreeViewHosts, folder.TreeView)
	}
}

func TestFolders_CreateNestedFolder(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	parent, err := svc.CreateFolder(ctx, CreateFolderRequest{
		Name:     "Production",
		TreeView: TreeViewHosts,
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	child, err := svc.CreateFolder(ctx, CreateFolderRequest{
		Name:     "Web Servers",
		ParentID: &parent.ID,
		TreeView: TreeViewHosts,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.ParentID == nil {
		t.Fatal("child should have a parent ID")
	}
	if *child.ParentID != parent.ID {
		t.Errorf("expected parent_id %q, got %q", parent.ID, *child.ParentID)
	}
}

func TestFolders_GetFolderTreeHierarchy(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	parent, err := svc.CreateFolder(ctx, CreateFolderRequest{
		Name:     "Production",
		TreeView: TreeViewHosts,
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	_, err = svc.CreateFolder(ctx, CreateFolderRequest{
		Name:     "Web Servers",
		ParentID: &parent.ID,
		TreeView: TreeViewHosts,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	// Also create a folder in a different tree to ensure filtering works
	_, err = svc.CreateFolder(ctx, CreateFolderRequest{
		Name:     "VM Group",
		TreeView: TreeViewVMs,
	})
	if err != nil {
		t.Fatalf("create vm folder: %v", err)
	}

	tree, err := svc.GetFolderTree(ctx, TreeViewHosts)
	if err != nil {
		t.Fatalf("GetFolderTree: %v", err)
	}

	// Should have 1 root folder in hosts tree
	if len(tree) != 1 {
		t.Fatalf("expected 1 root folder, got %d", len(tree))
	}
	if tree[0].Name != "Production" {
		t.Errorf("expected root name %q, got %q", "Production", tree[0].Name)
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree[0].Children))
	}
	if tree[0].Children[0].Name != "Web Servers" {
		t.Errorf("expected child name %q, got %q", "Web Servers", tree[0].Children[0].Name)
	}
}

func TestFolders_MoveFolderPreventsCircularReference(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create: grandparent -> parent -> child
	grandparent, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Grandparent", TreeView: TreeViewHosts,
	})
	parent, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Parent", ParentID: &grandparent.ID, TreeView: TreeViewHosts,
	})
	child, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Child", ParentID: &parent.ID, TreeView: TreeViewHosts,
	})

	// Try to move grandparent under child (circular)
	err := svc.MoveFolder(ctx, grandparent.ID, &child.ID)
	if err == nil {
		t.Fatal("expected error when creating circular reference, got nil")
	}
	expected := "cannot move folder into its own descendant"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}

	// Also verify moving to self is prevented
	err = svc.MoveFolder(ctx, parent.ID, &parent.ID)
	if err == nil {
		t.Fatal("expected error when moving folder into itself, got nil")
	}
}

func TestFolders_AddMember(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	folder, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Prod", TreeView: TreeViewHosts,
	})

	err := svc.AddMember(ctx, folder.ID, AddMemberRequest{
		ResourceType: "vm",
		ResourceID:   "100",
		Cluster:      "cluster1",
	})
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	members, err := db.GetMembersByFolder(ctx, folder.ID)
	if err != nil {
		t.Fatalf("GetMembersByFolder failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].ResourceType != "vm" || members[0].ResourceID != "100" {
		t.Errorf("unexpected member: %+v", members[0])
	}
}

func TestFolders_RemoveMember(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	folder, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Prod", TreeView: TreeViewHosts,
	})

	_ = svc.AddMember(ctx, folder.ID, AddMemberRequest{
		ResourceType: "vm", ResourceID: "100", Cluster: "cluster1",
	})

	err := svc.RemoveMember(ctx, folder.ID, RemoveMemberRequest{
		ResourceType: "vm", ResourceID: "100", Cluster: "cluster1",
	})
	if err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	members, err := db.GetMembersByFolder(ctx, folder.ID)
	if err != nil {
		t.Fatalf("GetMembersByFolder failed: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 members after remove, got %d", len(members))
	}
}

func TestFolders_DeleteFolderCascadesToChildren(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	parent, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Parent", TreeView: TreeViewHosts,
	})
	child, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Child", ParentID: &parent.ID, TreeView: TreeViewHosts,
	})

	// Add a member to child folder
	_ = svc.AddMember(ctx, child.ID, AddMemberRequest{
		ResourceType: "vm", ResourceID: "100", Cluster: "c1",
	})

	// Delete parent
	err := svc.DeleteFolder(ctx, parent.ID)
	if err != nil {
		t.Fatalf("DeleteFolder failed: %v", err)
	}

	// Child should be gone
	got, err := db.GetFolder(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetFolder for child: %v", err)
	}
	if got != nil {
		t.Error("child folder should have been cascade-deleted")
	}

	// Members of child should also be gone
	members, err := db.GetMembersByFolder(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetMembersByFolder: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members after cascade delete, got %d", len(members))
	}
}

func TestFolders_RenameFolder(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	folder, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "OldName", TreeView: TreeViewVMs,
	})

	err := svc.RenameFolder(ctx, folder.ID, "NewName")
	if err != nil {
		t.Fatalf("RenameFolder failed: %v", err)
	}

	got, err := db.GetFolder(ctx, folder.ID)
	if err != nil {
		t.Fatalf("GetFolder: %v", err)
	}
	if got.Name != "NewName" {
		t.Errorf("expected name %q, got %q", "NewName", got.Name)
	}
}

func TestFolders_RenameFolderEmptyNameFails(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	folder, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Something", TreeView: TreeViewHosts,
	})

	err := svc.RenameFolder(ctx, folder.ID, "")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestFolders_AddMemberInvalidResourceType(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	folder, _ := svc.CreateFolder(ctx, CreateFolderRequest{
		Name: "Test", TreeView: TreeViewHosts,
	})

	err := svc.AddMember(ctx, folder.ID, AddMemberRequest{
		ResourceType: "invalid", ResourceID: "100", Cluster: "c1",
	})
	if err == nil {
		t.Fatal("expected error for invalid resource_type, got nil")
	}
}
