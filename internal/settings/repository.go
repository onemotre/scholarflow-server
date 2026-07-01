package settings

import (
	"context"

	"scholarflow_server/internal/db"
)

// Repository persists setting overrides. Absence of a key means "use env/default".
type Repository interface {
	List(ctx context.Context) (map[string]string, error)
	Upsert(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

type SQLRepository struct {
	queries *db.Queries
}

func NewSQLRepository(queries *db.Queries) *SQLRepository {
	return &SQLRepository{queries: queries}
}

func (r *SQLRepository) List(ctx context.Context) (map[string]string, error) {
	rows, err := r.queries.ListAppSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func (r *SQLRepository) Upsert(ctx context.Context, key, value string) error {
	return r.queries.UpsertAppSetting(ctx, db.UpsertAppSettingParams{Key: key, Value: value})
}

func (r *SQLRepository) Delete(ctx context.Context, key string) error {
	return r.queries.DeleteAppSetting(ctx, key)
}
