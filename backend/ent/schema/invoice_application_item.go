package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InvoiceApplicationItem snapshots one eligible recharge record selected for an invoice.
// The global source identity index permanently prevents a record from being invoiced twice.
type InvoiceApplicationItem struct {
	ent.Schema
}

func (InvoiceApplicationItem) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_application_items"}}
}

func (InvoiceApplicationItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("application_id"),
		field.String("source_type").MaxLen(32),
		field.Int64("source_id"),
		field.String("source_reference").MaxLen(128),
		field.Float("amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvoiceApplicationItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("application", InvoiceApplication.Type).Ref("items").Field("application_id").Unique().Required(),
	}
}

func (InvoiceApplicationItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("application_id"),
		index.Fields("source_type", "source_id").Unique(),
	}
}
