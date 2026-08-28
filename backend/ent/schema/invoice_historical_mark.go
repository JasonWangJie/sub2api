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

// InvoiceHistoricalMark permanently records a recharge that was invoiced before
// the enterprise invoicing workflow was introduced.
type InvoiceHistoricalMark struct {
	ent.Schema
}

func (InvoiceHistoricalMark) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_historical_marks"}}
}

func (InvoiceHistoricalMark) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("source_type").MaxLen(32),
		field.Int64("source_id"),
		field.String("source_reference").MaxLen(128),
		field.Float("amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Int64("marked_by"),
		field.Time("marked_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvoiceHistoricalMark) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("invoice_historical_marks").Field("user_id").Unique().Required(),
	}
}

func (InvoiceHistoricalMark) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_type", "source_id").Unique(),
		index.Fields("user_id", "marked_at"),
		index.Fields("marked_by"),
	}
}
