package state

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsAccountProtectionAndPathIdentity(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "router"), filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.AddAccount("Test")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{store.Root(), account.CodexHome} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatalf("unprotected ACL: %v", err)
		}
		acl, _, err := sd.DACL()
		if err != nil || acl == nil {
			t.Fatalf("missing ACL: %v", err)
		}
		if !samePath(path, strings.ToUpper(path)) {
			t.Fatal("Windows path case changed account identity")
		}
	}
	if err := store.SetProjectOwner(root, account.ID); err != nil {
		t.Fatal(err)
	}
	if owner, ok := store.ProjectOwner(strings.ToUpper(root)); !ok || owner != account.ID {
		t.Fatal("Windows path case lost project assignment")
	}
}
