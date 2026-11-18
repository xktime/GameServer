package mongodb

import (
	"context"
	"testing"
)

type testDocument struct{ ID int64 }

func (d testDocument) GetPersistId() interface{} { return d.ID }

func TestWritesRejectUninitializedMongo(t *testing.T) {
	ctx := context.Background()
	doc := testDocument{ID: 1}
	for _, test := range []struct {
		name  string
		write func() error
	}{
		{name: "Save", write: func() error {
			_, err := SaveContext(ctx, doc)
			return err
		}},
		{name: "Delete", write: func() error {
			_, err := DeleteByIDContext[testDocument](ctx, doc.ID)
			return err
		}},
		{name: "SaveIfAbsent", write: func() error {
			_, err := SaveIfAbsentContext(ctx, doc)
			return err
		}},
		{name: "BulkSave", write: func() error {
			_, err := BulkSave([]PersistData{doc})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.write(); err == nil {
				t.Fatal("write reported success without an initialized MongoDB client")
			}
		})
	}
}
