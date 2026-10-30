package mysql

import (
	"context"
	"errors"
	"testing"
)

func TestCredentialsVersionChangesWithPasswordInSameTransaction(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_credentials_version_test", userTablesDDL)
	defer cleanup()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, status, created_at) VALUES (1, 'active', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_accounts
		(user_id, account_id, name, email, hashed_password, role, updated_at)
		VALUES (1, 'test', 'Test', 'test@example.com', 'old', 'user', 0)`); err != nil {
		t.Fatal(err)
	}
	repo := NewMySQLUserRepository(db)
	creds, err := repo.GetCredentialsByID(ctx, 1)
	if err != nil || creds == nil || creds.CredentialsVersion != 0 {
		t.Fatalf("initial credentials = %#v, err=%v", creds, err)
	}
	if err := repo.SaveCredentials(ctx, creds); err != nil {
		t.Fatal(err)
	}
	if version, err := repo.GetCredentialsVersionByID(ctx, 1); err != nil || version != 0 {
		t.Fatalf("unchanged password version = %d, err=%v", version, err)
	}
	creds.HashedPassword = "new"
	if err := repo.SaveCredentials(ctx, creds); err != nil {
		t.Fatal(err)
	}
	if version, err := repo.GetCredentialsVersionByID(ctx, 1); err != nil || version != 1 {
		t.Fatalf("changed password version = %d, err=%v", version, err)
	}
	rollback := errors.New("rollback")
	if err := NewMySQLTxManager(db).RunInTx(ctx, func(txCtx context.Context) error {
		creds.HashedPassword = "rolled-back"
		if err := repo.SaveCredentials(txCtx, creds); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	latest, err := repo.GetCredentialsByID(ctx, 1)
	if err != nil || latest.HashedPassword != "new" || latest.CredentialsVersion != 1 {
		t.Fatalf("rollback did not restore password and version: %#v, err=%v", latest, err)
	}
}
