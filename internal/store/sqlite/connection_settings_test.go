package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"git.f4mily.net/goloom/internal/security"
)

func TestEveryPooledConnectionHasRequiredSettings(t *testing.T) {
	for _, query := range []string{"", "?_txlock=deferred&_fk=off&_timeout=0&_pragma=busy_timeout(0)&_pragma=cache_size(-500)"} {
		t.Run(query, func(t *testing.T) {
			ctx := context.Background()
			enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
			if err != nil {
				t.Fatal(err)
			}
			s, err := New(ctx, "file:"+filepath.Join(t.TempDir(), "settings.db")+query, enc)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			held := make([]*sql.Conn, 0, 3)
			defer func() {
				for _, conn := range held {
					_ = conn.Close()
				}
			}()
			for i := range 3 {
				conn, err := s.db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				held = append(held, conn)
				var foreignKeys, timeout int
				if err := conn.QueryRowContext(ctx, "pragma foreign_keys").Scan(&foreignKeys); err != nil {
					t.Fatal(err)
				}
				if err := conn.QueryRowContext(ctx, "pragma busy_timeout").Scan(&timeout); err != nil {
					t.Fatal(err)
				}
				if foreignKeys != 1 || timeout != 5000 {
					t.Errorf("connection %d: foreign_keys=%d busy_timeout=%d", i, foreignKeys, timeout)
				}
				if _, err := conn.ExecContext(ctx, `insert into team_memberships(team_id,user_id,role,created_at) values ('missing-team','missing-user','owner','now')`); err == nil {
					t.Errorf("connection %d accepted broken foreign keys", i)
				}
				if query != "" {
					var cache int
					if err := conn.QueryRowContext(ctx, "pragma cache_size").Scan(&cache); err != nil || cache != -500 {
						t.Errorf("unrelated pragma changed: %d %v", cache, err)
					}
				}
			}
		})
	}
}
