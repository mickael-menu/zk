package sqlite

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mickael-menu/zk/core/note"
	"github.com/mickael-menu/zk/util/errors"
)

// DB holds the connections to a SQLite database.
type DB struct {
	db *sql.DB
}

// Open creates a new DB instance for the SQLite database at the given path.
func Open(path string) (*DB, error) {
	return open("file:" + path)
}

// OpenInMemory creates a new in-memory DB instance.
func OpenInMemory() (*DB, error) {
	return open(":memory:")
}

func open(uri string) (*DB, error) {
	wrap := errors.Wrapper("failed to open the database")

	db, err := sql.Open("sqlite3", uri)
	if err != nil {
		return nil, wrap(err)
	}

	// Make sure that CASCADE statements are properly applied by enabling
	// foreign keys.
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		return nil, wrap(err)
	}

	return &DB{db}, nil
}

// Close terminates the connections to the SQLite database.
func (db *DB) Close() error {
	err := db.db.Close()
	return errors.Wrap(err, "failed to close the database")
}

// Migrate upgrades the SQL schema of the database.
func (db *DB) Migrate() (needsReindexing bool, err error) {
	err = db.WithTransaction(func(tx Transaction) error {
		var version int
		err := tx.QueryRow("PRAGMA user_version").Scan(&version)
		if err != nil {
			return err
		}

		if version <= 0 {
			err = tx.ExecStmts([]string{
				// Notes
				`CREATE TABLE IF NOT EXISTS notes (
					id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
					path TEXT NOT NULL,
					sortable_path TEXT NOT NULL,
					title TEXT DEFAULT('') NOT NULL,
					lead TEXT DEFAULT('') NOT NULL,
					body TEXT DEFAULT('') NOT NULL,
					raw_content TEXT DEFAULT('') NOT NULL,
					word_count INTEGER DEFAULT(0) NOT NULL,
					checksum TEXT NOT NULL,
					created DATETIME DEFAULT(CURRENT_TIMESTAMP) NOT NULL,
					modified DATETIME DEFAULT(CURRENT_TIMESTAMP) NOT NULL,
					UNIQUE(path)
				)`,
				`CREATE INDEX IF NOT EXISTS index_notes_checksum ON notes (checksum)`,
				`CREATE INDEX IF NOT EXISTS index_notes_path ON notes (path)`,

				// Links
				`CREATE TABLE IF NOT EXISTS links (
					id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
					source_id INTEGER NOT NULL REFERENCES notes(id)
						ON DELETE CASCADE,
					target_id INTEGER REFERENCES notes(id)
						ON DELETE SET NULL,
					title TEXT DEFAULT('') NOT NULL,
					href TEXT NOT NULL,
					external INT DEFAULT(0) NOT NULL,
					rels TEXT DEFAULT('') NOT NULL,
					snippet TEXT DEFAULT('') NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS index_links_source_id_target_id ON links (source_id, target_id)`,

				// FTS index
				`CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
					path, title, body,
					content = notes,
					content_rowid = id,
					tokenize = "porter unicode61 remove_diacritics 1 tokenchars '''&/'"
				)`,
				// Triggers to keep the FTS index up to date.
				`CREATE TRIGGER IF NOT EXISTS trigger_notes_ai AFTER INSERT ON notes BEGIN
					INSERT INTO notes_fts(rowid, path, title, body) VALUES (new.id, new.path, new.title, new.body);
				END`,
				`CREATE TRIGGER IF NOT EXISTS trigger_notes_ad AFTER DELETE ON notes BEGIN
					INSERT INTO notes_fts(notes_fts, rowid, path, title, body) VALUES('delete', old.id, old.path, old.title, old.body);
				END`,
				`CREATE TRIGGER IF NOT EXISTS trigger_notes_au AFTER UPDATE ON notes BEGIN
					INSERT INTO notes_fts(notes_fts, rowid, path, title, body) VALUES('delete', old.id, old.path, old.title, old.body);
					INSERT INTO notes_fts(rowid, path, title, body) VALUES (new.id, new.path, new.title, new.body);
				END`,
				`PRAGMA user_version = 1`,
			})

			if err != nil {
				return err
			}
		}

		if version <= 1 {
			err = tx.ExecStmts([]string{
				// Collections
				`CREATE TABLE IF NOT EXISTS collections (
					id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
					kind TEXT NO NULL,
					name TEXT NOT NULL,
					UNIQUE(kind, name)
				)`,
				`CREATE INDEX IF NOT EXISTS index_collections ON collections (kind, name)`,

				// Note-Collection association
				`CREATE TABLE IF NOT EXISTS notes_collections (
					id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
					note_id INTEGER NOT NULL REFERENCES notes(id)
						ON DELETE CASCADE,
					collection_id INTEGER NOT NULL REFERENCES collections(id)
						ON DELETE CASCADE
				)`,
				`CREATE INDEX IF NOT EXISTS index_notes_collections ON notes_collections (note_id, collection_id)`,

				// View of notes with their associated metadata (e.g. tags), for simpler queries.
				`CREATE VIEW notes_with_metadata AS
				 SELECT n.*, GROUP_CONCAT(c.name, '` + "\x01" + `') AS tags
				   FROM notes n
				   LEFT JOIN notes_collections nc ON nc.note_id = n.id
				   LEFT JOIN collections c ON nc.collection_id = c.id AND c.kind = '` + string(note.CollectionKindTag) + `'
				  GROUP BY n.id`,

				`PRAGMA user_version = 2`,
			})

			if err != nil {
				return err
			}
		}

		if version <= 2 {
			err = tx.ExecStmts([]string{
				// Add a `metadata` column to `notes`
				`ALTER TABLE notes ADD COLUMN metadata TEXT DEFAULT('{}') NOT NULL`,

				// Add snippet's start and end offsets to `links`
				`ALTER TABLE links ADD COLUMN snippet_start INTEGER DEFAULT(0) NOT NULL`,
				`ALTER TABLE links ADD COLUMN snippet_end INTEGER DEFAULT(0) NOT NULL`,

				`PRAGMA user_version = 3`,
			})

			if err != nil {
				return err
			}

			needsReindexing = true
		}

		return nil
	})

	err = errors.Wrap(err, "database migration failed")
	return
}
