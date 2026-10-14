package firebase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
	"os"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
)

type RealtimeDatabase struct{ client *db.Client }

func NewRealtimeDatabase(ctx context.Context, projectID, databaseURL string) (*RealtimeDatabase, error) {
	if os.Getenv("FIREBASE_DATABASE_EMULATOR_HOST") != "" {
		return nil, errors.New("RTDB emulator routing is unsupported in the application; use isolated rules tests")
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID, DatabaseURL: databaseURL})
	if err != nil {
		return nil, errors.New("initialize Firebase RTDB: check configuration and Application Default Credentials")
	}
	client, err := app.Database(ctx)
	if err != nil {
		return nil, errors.New("initialize Firebase RTDB: check configuration and Application Default Credentials")
	}
	return &RealtimeDatabase{client}, nil
}

const maxSafeVersion int64 = 9007199254740991

func (p *RealtimeDatabase) Publish(ctx context.Context, next domain.Projection) error {
	if next.Version < 1 || next.Version > maxSafeVersion || next.ChangedAt < 1 || next.PublishedAt < next.ChangedAt ||
		next.Summary.Total < 0 || next.Summary.In < 0 || next.Summary.Out < 0 || next.Summary.Unknown < 0 ||
		next.Summary.Total != next.Summary.In+next.Summary.Out+next.Summary.Unknown {
		return errors.New("invalid attendance projection")
	}
	return p.client.NewRef("hostelhive/attendance").Transaction(ctx, func(node db.TransactionNode) (interface{}, error) {
		var prior *domain.Projection
		if err := node.Unmarshal(&prior); err != nil {
			return nil, err
		}
		if prior != nil && prior.Version >= next.Version {
			return prior, nil
		}
		return next, nil
	})
}

func (p *RealtimeDatabase) SetGrant(ctx context.Context, uid string, next domain.Grant) error {
	if uid == "" || len(uid) > 128 || !utf8.ValidString(uid) || strings.ContainsAny(uid, ".#$[]/\x00\r\n") || next.Version < 1 || next.Version > maxSafeVersion || next.ExpiresAt < 0 || next.ExpiresAt > maxSafeVersion {
		return errors.New("invalid realtime grant")
	}
	return p.client.NewRef("hostelhive/staffAccess/"+uid).Transaction(ctx, func(node db.TransactionNode) (interface{}, error) {
		var prior *domain.Grant
		if err := node.Unmarshal(&prior); err != nil {
			return nil, err
		}
		if prior != nil && prior.Version >= next.Version {
			return prior, nil
		}
		return next, nil
	})
}
