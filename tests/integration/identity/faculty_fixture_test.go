package identity_test

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		p, e := pgxpool.New(ctx, url)
		if e == nil {
			_, e = p.Exec(ctx, `INSERT INTO hostelhive.faculties(name) VALUES('Science') ON CONFLICT DO NOTHING; INSERT INTO hostelhive.faculty_names(name_key,faculty_id) SELECT lower(name),faculty_id FROM hostelhive.faculties WHERE name='Science' ON CONFLICT DO NOTHING`)
			p.Close()
		}
		c()
		if e != nil {
			fmt.Fprintln(os.Stderr, "faculty fixture:", e)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
