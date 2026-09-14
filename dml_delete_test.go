// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestDeleteTargetAliasCapabilities(t *testing.T) {
	query := func(d Dialect) *DeleteBuilder {
		return Delete().FromAlias("users", "u").Where(Eq("u.id", 1)).SetDialect(d)
	}

	for _, d := range []Dialect{
		dialect.MySQL,
		dialect.WithVersion(dialect.MySQL, 8, 0, 15),
		dialect.WithFeatures(dialect.WithVersion(dialect.MySQL, 8, 0, 16), nil, []dialect.Feature{dialect.DeleteTargetAlias}),
	} {
		checkBuildError(t, query(d))
	}

	for _, d := range []Dialect{
		dialect.WithVersion(dialect.MySQL, 8, 0, 16),
		dialect.WithVersion(dialect.MySQL, 8, 4, 0),
		dialect.WithFeatures(dialect.MySQL, []dialect.Feature{dialect.DeleteTargetAlias}, nil),
	} {
		checkSQL(t, query(d), "DELETE FROM `users` AS `u` WHERE (`u`.`id` = ?)", 1)
		checkSQL(t, query(d).OrderByAsc("u.id").Limit(1),
			"DELETE FROM `users` AS `u` WHERE (`u`.`id` = ?) ORDER BY `u`.`id` ASC LIMIT 1", 1)
	}

	checkSQL(t, query(dialect.Postgres), `DELETE FROM "users" AS "u" WHERE ("u"."id" = $1)`, 1)
	checkSQL(t, query(dialect.SQLite), `DELETE FROM "users" AS "u" WHERE ("u"."id" = ?)`, 1)
	checkSQL(t, Delete().From("users").Where(Eq("id", 1)).SetDialect(dialect.MySQL),
		"DELETE FROM `users` WHERE (`id` = ?)", 1)
	checkSQL(t, query(dialect.MySQL).Join("roles", "r", On("u.role_id", "r.id")),
		"DELETE `u` FROM `users` AS `u` INNER JOIN `roles` AS `r` ON `u`.`role_id`=`r`.`id` WHERE (`u`.`id` = ?)", 1)
}
