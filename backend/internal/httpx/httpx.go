// Package httpx berisi helper HTTP bersama untuk API admin: format respons, query ke map, paging, CORS, rate limit.
package httpx

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/time/rate"
)

// DBTX dipenuhi oleh *pgxpool.Pool dan pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// OK menjawab 200 {data}.
func OK(c *gin.Context, data any) { c.JSON(http.StatusOK, gin.H{"data": data}) }

// Status menjawab dengan kode tertentu {data}.
func Status(c *gin.Context, code int, data any) { c.JSON(code, gin.H{"data": data}) }

// Page menjawab {data, meta:{page, per_page, total}}.
func Page(c *gin.Context, data any, page, per, total int) {
	c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "per_page": per, "total": total}})
}

// Fail menjawab {message, code} dan menghentikan rantai handler.
func Fail(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"message": msg, "code": code})
}

// ServerError mencatat error ke log server dan menjawab 500 tanpa detail.
func ServerError(c *gin.Context, err error) {
	_ = c.Error(err)
	Fail(c, http.StatusInternalServerError, "internal_error", "Terjadi kesalahan di server.")
}

// Query menjalankan SELECT dan mengembalikan baris sebagai map. Kolom uuid harus di-cast ke text
// dan numeric ke float8 di SQL.
func Query(ctx context.Context, db DBTX, sql string, args ...any) ([]map[string]any, error) {
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

// Paging membaca ?page dan ?per_page (default 1 dan 20, maks. 100).
func Paging(c *gin.Context) (page, per, offset int) {
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

// Where membangun klausa WHERE dengan placeholder bernomor ($1, $2, ...).
type Where struct {
	parts []string
	Args  []any
}

// Add menambah kondisi; "?" diganti dengan placeholder bernomor untuk val.
func (w *Where) Add(cond string, val any) {
	w.Args = append(w.Args, val)
	w.parts = append(w.parts, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(w.Args))))
}

// Raw menambah kondisi tanpa argumen.
func (w *Where) Raw(cond string) { w.parts = append(w.parts, cond) }

// SQL menggabungkan kondisi dengan AND ("TRUE" bila kosong).
func (w *Where) SQL() string {
	if len(w.parts) == 0 {
		return "TRUE"
	}
	return strings.Join(w.parts, " AND ")
}

// EscapeLike meloloskan %, _ dan \ pada input pencarian untuk dipakai di ILIKE.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// CORS mengizinkan origin tertentu dengan kredensial (cookie).
func CORS(origins []string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[o] = true
	}
	return func(c *gin.Context) {
		if o := c.GetHeader("Origin"); o != "" && allowed[o] {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Add("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// LimitBody membatasi ukuran body request.
func LimitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// Limiters adalah kumpulan token-bucket per kunci.
type Limiters struct {
	mu sync.Mutex
	m  map[string]*rate.Limiter
}

// Get mengembalikan limiter untuk kunci (dibuat bila belum ada).
func (l *Limiters) Get(key string, perSec float64, burst int) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil || len(l.m) > 50000 {
		l.m = map[string]*rate.Limiter{}
	}
	lim, ok := l.m[key]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(perSec), burst)
		l.m[key] = lim
	}
	return lim
}
