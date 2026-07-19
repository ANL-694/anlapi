package schema

import (
	"fmt"

	"ikik-api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserPlatformQuota defines independent daily, weekly and monthly USD quotas
// for one user on one gateway platform.
type UserPlatformQuota struct {
	ent.Schema
}

func (UserPlatformQuota) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "user_platform_quotas"}}
}

func (UserPlatformQuota) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (UserPlatformQuota) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("platform").
			MaxLen(32).
			NotEmpty().
			Validate(func(value string) error {
				switch value {
				case "anthropic", "openai", "gemini", "antigravity", "grok", "kiro":
					return nil
				default:
					return fmt.Errorf("platform %q is not allowed", value)
				}
			}),
		field.Float("daily_limit_usd").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("weekly_limit_usd").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("monthly_limit_usd").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("daily_usage_usd").Default(0).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("weekly_usage_usd").Default(0).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("monthly_usage_usd").Default(0).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Time("daily_window_start").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("weekly_window_start").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("monthly_window_start").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserPlatformQuota) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("platform_quotas").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (UserPlatformQuota) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "platform").Unique().Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("user_id"),
	}
}
