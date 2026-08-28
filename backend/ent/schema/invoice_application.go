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

// InvoiceApplication is an immutable user invoice request after submission.
type InvoiceApplication struct {
	ent.Schema
}

func (InvoiceApplication) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_applications"}}
}

func (InvoiceApplication) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("application_no").MaxLen(64).Unique(),
		field.String("email").MaxLen(255),
		field.String("tax_number").MaxLen(64),
		field.String("company_name").MaxLen(255),
		field.Float("total_amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.String("status").MaxLen(20).Default("PENDING"),
		field.String("rejection_reason").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("completed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("completed_by").Optional().Nillable(),
		field.Time("rejected_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("rejected_by").Optional().Nillable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvoiceApplication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("invoice_applications").Field("user_id").Unique().Required(),
		edge.To("items", InvoiceApplicationItem.Type),
	}
}

func (InvoiceApplication) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("created_at"),
	}
}
