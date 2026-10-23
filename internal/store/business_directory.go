package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neko233-com/metrics233-server/internal/model"
)

type BusinessDirectoryFilter struct {
	Query, Environment, Role, Sort, Cursor string
	Limit                                  int
}

var ErrBusinessNoteConflict = errors.New("instance note changed; reload before saving")

type BusinessDirectoryPage struct {
	BusinessInstancePage
	Total        int            `json:"total"`
	Matched      int            `json:"matched"`
	Counts       map[string]int `json:"counts"`
	Environments []string       `json:"environments"`
}

// BusinessDirectory filters the bounded catalog before paging. The legacy
// template-scoped cursor API remains unchanged.
func (s *Store) BusinessDirectory(ctx context.Context, f BusinessDirectoryFilter) (BusinessDirectoryPage, error) {
	page := BusinessDirectoryPage{BusinessInstancePage: BusinessInstancePage{Items: []model.BusinessInstance{}, Capacity: BusinessInstanceLimit}, Counts: map[string]int{"all": 0, "game": 0, "gateway": 0, "sdk": 0}, Environments: []string{}}
	if f.Limit < 1 || f.Limit > 100 || len(f.Query) > 128 || len(f.Environment) > 128 || !utf8.ValidString(f.Query+f.Environment) || strings.ContainsAny(f.Query+f.Environment, "\x00\r\n") {
		return page, errors.New("invalid directory filter")
	}
	if f.Role == "" {
		f.Role = "all"
	}
	if f.Sort == "" {
		f.Sort = "server"
	}
	if f.Role != "all" && f.Role != "game" && f.Role != "gateway" && f.Role != "sdk" {
		return page, errors.New("invalid directory role")
	}
	if f.Sort != "server" && f.Sort != "recent" && f.Sort != "host" {
		return page, errors.New("invalid directory sort")
	}
	scope := sha256.Sum256([]byte(strings.Join([]string{f.Query, f.Environment, f.Role, f.Sort}, "\x00")))
	prefix := hex.EncodeToString(scope[:12]) + ":"
	after := ""
	if f.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil || len(raw) != len(prefix)+64 || !strings.HasPrefix(string(raw), prefix) {
			return page, errors.New("directory cursor does not match filters")
		}
		after = string(raw[len(prefix):])
		if value, err := hex.DecodeString(after); err != nil || len(value) != 32 {
			return page, errors.New("invalid directory cursor")
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+businessColumns+` FROM business_instances ORDER BY id LIMIT ?`, BusinessInstanceLimit)
	if err != nil {
		return page, err
	}
	items := []model.BusinessInstance{}
	environments := map[string]bool{}
	query := strings.ToLower(strings.TrimSpace(f.Query))
	for rows.Next() {
		item, e := scanBusiness(rows)
		if e != nil {
			rows.Close()
			return page, e
		}
		page.Total++
		environments[item.Identity.Environment] = true
		if f.Environment != "" && item.Identity.Environment != f.Environment {
			continue
		}
		search := strings.Join([]string{item.Identity.Service, item.Identity.Role, item.Identity.ServerID, item.Identity.Environment, item.Identity.Channel, item.Identity.Instance, item.Identity.Implementation, item.ReportedIP, item.ObservedIP, item.Note.Description}, "\n")
		if query != "" && !strings.Contains(strings.ToLower(search), query) {
			continue
		}
		role := item.Identity.Role
		if role == "logic" {
			role = "game"
		}
		page.Counts["all"]++
		page.Counts[role]++
		if f.Role == "all" || f.Role == role {
			items = append(items, item)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	page.AtCapacity = page.Total >= BusinessInstanceLimit
	page.Matched = len(items)
	for env := range environments {
		page.Environments = append(page.Environments, env)
	}
	sort.Strings(page.Environments)
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if f.Sort == "recent" && a.LastSeen != b.LastSeen {
			return a.LastSeen > b.LastSeen
		}
		if f.Sort == "host" {
			ah, bh := a.ReportedIP, b.ReportedIP
			if ah == "" {
				ah = a.ObservedIP
			}
			if bh == "" {
				bh = b.ObservedIP
			}
			if n := naturalBusinessCompare(ah, bh); n != 0 {
				return n < 0
			}
		}
		for _, pair := range [][2]string{{a.Identity.ServerID, b.Identity.ServerID}, {a.Identity.Service, b.Identity.Service}, {a.Identity.Instance, b.Identity.Instance}, {a.ID, b.ID}} {
			if n := naturalBusinessCompare(pair[0], pair[1]); n != 0 {
				return n < 0
			}
		}
		return false
	})
	start := 0
	if after != "" {
		found := false
		for i, item := range items {
			if item.ID == after {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return page, errors.New("directory cursor expired; reload the first page")
		}
	}
	end := min(start+f.Limit, len(items))
	page.Items = items[start:end]
	if end < len(items) {
		page.Next = base64.RawURLEncoding.EncodeToString([]byte(prefix + items[end-1].ID))
	}
	return page, nil
}

func naturalBusinessCompare(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		if a[0] >= '0' && a[0] <= '9' && b[0] >= '0' && b[0] <= '9' {
			i, j := 0, 0
			for i < len(a) && a[i] >= '0' && a[i] <= '9' {
				i++
			}
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			x, y := strings.TrimLeft(a[:i], "0"), strings.TrimLeft(b[:j], "0")
			if len(x) != len(y) {
				return len(x) - len(y)
			}
			if n := strings.Compare(x, y); n != 0 {
				return n
			}
			a, b = a[i:], b[j:]
		} else {
			if a[0] != b[0] {
				return int(a[0]) - int(b[0])
			}
			a, b = a[1:], b[1:]
		}
	}
	return len(a) - len(b)
}

// SaveBusinessNote uses a separate revision so ingestion and template/host
// changes cannot overwrite user text or accidentally satisfy its CAS.
func (s *Store) SaveBusinessNote(ctx context.Context, id string, note model.BusinessNote) (model.BusinessNote, error) {
	if note.Version < 0 || len(note.Description) > 4096 || !utf8.ValidString(note.Description) {
		return note, errors.New("description must be valid UTF-8 and at most 4096 bytes")
	}
	for _, r := range note.Description {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return note, errors.New("invalid description control character")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return note, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM business_instances WHERE id=?`, id).Scan(&exists); err != nil {
		return note, err
	}
	if exists != 1 {
		return note, sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO business_instance_notes(instance_id,version,description,updated_at) VALUES(?,0,'',0) ON CONFLICT(instance_id) DO NOTHING`, id); err != nil {
		return note, err
	}
	note.UpdatedAt = time.Now().UnixMilli()
	result, err := tx.ExecContext(ctx, `UPDATE business_instance_notes SET version=version+1,description=?,updated_at=? WHERE instance_id=? AND version=?`, note.Description, note.UpdatedAt, id, note.Version)
	if err != nil {
		return note, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return note, err
	}
	if n != 1 {
		return note, ErrBusinessNoteConflict
	}
	if err = tx.Commit(); err == nil {
		note.Version++
	}
	return note, err
}
