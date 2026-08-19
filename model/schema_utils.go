package model

// Helper queries for schema introspection live here so migrations can share them across files.

// mysqlTableExists returns whether the given table is present in the current MySQL schema.
func mysqlTableExists(table string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var res result
	query := "SELECT COUNT(*) AS count FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?"
	if err := DB.Raw(query, table).Scan(&res).Error; err != nil {
		return false, err
	}
	return res.Count > 0, nil
}

// mysqlColumnExists reports whether the provided column exists for the table in the current MySQL schema.
func mysqlColumnExists(table, column string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var res result
	query := "SELECT COUNT(*) AS count FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	if err := DB.Raw(query, table, column).Scan(&res).Error; err != nil {
		return false, err
	}
	return res.Count > 0, nil
}

// mysqlIndexExists reports whether the provided index exists for the table in the current MySQL schema.
func mysqlIndexExists(table, index string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var res result
	query := "SELECT COUNT(*) AS count FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?"
	if err := DB.Raw(query, table, index).Scan(&res).Error; err != nil {
		return false, err
	}
	return res.Count > 0, nil
}
