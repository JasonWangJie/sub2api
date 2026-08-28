package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// InvoiceProfile stores the most recently used invoice contact information for a user.
// Invoice applications retain their own immutable snapshots.
type InvoiceProfile struct {
	ent.Schema
}

func (InvoiceProfile) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_profiles"}}
}

func (InvoiceProfile) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").Unique(),
		field.String("email").MaxLen(255),
		field.String("tax_number").MaxLen(64),
		field.String("company_name").MaxLen(255),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvoiceProfile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("invoice_profile").Field("user_id").Unique().Required(),
	}
}
