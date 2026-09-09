package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestBeginAPIKeyMutationReusesTransactionalClient(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	root := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	mock.ExpectBegin()
	tx, err := root.Tx(context.Background())
	require.NoError(t, err)

	client := tx.Client()
	got, nestedTx, err := beginAPIKeyMutation(context.Background(), client)
	require.NoError(t, err)
	require.Same(t, client, got)
	require.Nil(t, nestedTx)

	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
