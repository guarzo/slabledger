package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHScopedCollaboratorsUseOneBackend(t *testing.T) {
	for _, collaborator := range []string{"mapping-read", "config-read", "event-write"} {
		t.Run(collaborator, func(t *testing.T) {
			_, _, _, _, id := setupReturnCore(t, "scoped-collaborator", 42, "")
			runtime, e := Open(context.Background(), os.Getenv("POSTGRES_TEST_URL"), mocks.NewMockLogger())
			require.NoError(t, e)
			t.Cleanup(func() { _ = runtime.Close() })
			// Migrations use a separate pool; this is a genuinely single runtime backend.
			runtime.SetMaxOpenConns(1)
			runtime.SetMaxIdleConns(1)
			store := NewConfirmedReturnStore(runtime.DB)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			e = store.WithPurchaseMutation(ctx, id, func(c context.Context) error {
				switch collaborator {
				case "mapping-read":
					repo := NewCardIDMappingRepository(runtime.DB)
					if e := repo.SaveExternalID(c, "Scoped card", "Scoped set", "1", "dh", "123"); e != nil {
						return e
					}
					value, e := repo.GetExternalID(c, "Scoped card", "Scoped set", "1", "dh")
					if e != nil {
						return e
					}
					require.Equal(t, "123", value)
				case "config-read":
					_, e := NewDHStore(runtime.DB, mocks.NewMockLogger()).GetDHPushConfig(c)
					return e
				case "event-write":
					return NewDHEventStore(runtime.DB).Record(c, dhevents.Event{PurchaseID: id, CertNumber: "scoped-collaborator", Type: dhevents.TypeListed, Source: dhevents.SourceDHListing, DHInventoryID: 42})
				}
				return nil
			})
			require.NoError(t, e)
		})
	}
}
