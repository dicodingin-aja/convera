package server

import (
	"context"
	"strconv"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/gin-gonic/gin"
)

// query menjalankan SELECT dan mengembalikan baris sebagai map (kunci = nama kolom).
// Kolom uuid harus di-cast ke text dan numeric ke float8 di SQL agar JSON-nya rapi.
func query(ctx context.Context, db core.DBTX, sql string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fds := rows.FieldDescriptions()
	out := []map[string]any{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, len(fds))
		for i, fd := range fds {
			m[fd.Name] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// paging membaca ?page dan ?per_page (default 1 dan 20, maks. 100).
func paging(c *gin.Context) (page, per, offset int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	per, _ = strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 20
	}
	return page, per, (page - 1) * per
}
