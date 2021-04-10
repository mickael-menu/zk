package adapter

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mickael-menu/zk/internal/adapter/fs"
	"github.com/mickael-menu/zk/internal/adapter/fzf"
	"github.com/mickael-menu/zk/internal/adapter/handlebars"
	"github.com/mickael-menu/zk/internal/adapter/markdown"
	"github.com/mickael-menu/zk/internal/adapter/sqlite"
	"github.com/mickael-menu/zk/internal/adapter/term"
	"github.com/mickael-menu/zk/internal/core"
	"github.com/mickael-menu/zk/internal/core/note"
	"github.com/mickael-menu/zk/internal/core/zk"
	"github.com/mickael-menu/zk/internal/util"
	"github.com/mickael-menu/zk/internal/util/date"
	"github.com/mickael-menu/zk/internal/util/errors"
	osutil "github.com/mickael-menu/zk/internal/util/os"
	"github.com/mickael-menu/zk/internal/util/pager"
	"github.com/mickael-menu/zk/internal/util/paths"
	"github.com/mickael-menu/zk/internal/util/rand"
	"github.com/schollz/progressbar/v3"
)

type Container struct {
	NotebookStore  *core.NotebookStore
	Version        string
	Config         zk.Config
	Date           date.Provider
	Logger         util.Logger
	Terminal       *term.Terminal
	WorkingDir     string
	templateLoader *handlebars.Loader
	newConfig      core.Config
	notebook       *core.Notebook
	notebookErr    error
	zk             *zk.Zk
	zkErr          error
	fs             core.FileStorage
}

func NewContainer(version string) (*Container, error) {
	wrap := errors.Wrapper("initialization")

	term := term.New()
	logger := util.NewStdLogger("zk: ", 0)
	fs, err := fs.NewFileStorage("")
	config := zk.NewDefaultConfig()
	newConfig := core.NewDefaultConfig()

	// Load global user config
	configPath, err := locateGlobalConfig()
	if err != nil {
		return nil, wrap(err)
	}
	if configPath != "" {
		config, err = zk.OpenConfig(configPath, config)
		if err != nil {
			return nil, wrap(err)
		}
		newConfig, err = core.OpenConfig(configPath, newConfig, fs)
		if err != nil {
			return nil, wrap(err)
		}
	}

	date := date.NewFrozenNow()

	return &Container{
		NotebookStore: core.NewNotebookStore(newConfig, core.Ports{
			FS: fs,
			TemplateLoaderFactory: func(language string, lookupPaths []string) (core.TemplateLoader, error) {
				handlebars.Init(language, term.SupportsUTF8(), logger, term)
				return handlebars.NewLoader(lookupPaths), nil
			},
			IDGeneratorFactory: func(opts core.IDOptions) func() string {
				return rand.NewIDGenerator(opts)
			},
			OSEnv: func() map[string]string {
				return osutil.Env()
			},
		}),
		Version: version,
		Config:  config,
		// zk is short-lived, so we freeze the current date to use the same
		// date for any template rendering during the execution.
		Date:      &date,
		Logger:    logger,
		Terminal:  term,
		newConfig: newConfig,
		fs:        fs,
	}, nil
}

// locateGlobalConfig looks for the global zk config file following the
// XDG Base Directory specification
// https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
func locateGlobalConfig() (string, error) {
	configHome, ok := os.LookupEnv("XDG_CONFIG_HOME")
	if !ok {
		home, ok := os.LookupEnv("HOME")
		if !ok {
			home = "~/"
		}
		configHome = filepath.Join(home, ".config")
	}

	configPath := filepath.Join(configHome, "zk/config.toml")
	exists, err := paths.Exists(configPath)
	switch {
	case err != nil:
		return "", err
	case exists:
		return configPath, nil
	default:
		return "", nil
	}
}

// OpenNotebook resolves and loads the first notebook found in the given
// searchPaths.
func (c *Container) OpenNotebook(searchPaths []string) {
	if len(searchPaths) == 0 {
		panic("no notebook search paths provided")
	}

	for _, path := range searchPaths {
		c.notebook, c.notebookErr = c.NotebookStore.OpenNotebook(path)
		c.zk, c.zkErr = zk.Open(path, c.Config)
		if c.notebookErr == nil && c.zkErr == nil {
			c.WorkingDir = path
			c.Config = c.zk.Config
			os.Setenv("ZK_NOTEBOOK_DIR", c.zk.Path)
			return
		}
	}
}

func (c *Container) Notebook() (*core.Notebook, error) {
	return c.notebook, c.notebookErr
}

func (c *Container) Zk() (*zk.Zk, error) {
	return c.zk, c.zkErr
}

func (c *Container) TemplateLoader(lang string) *handlebars.Loader {
	if c.templateLoader == nil {
		handlebars.Init(lang, c.Terminal.SupportsUTF8(), c.Logger, c.Terminal)
		c.templateLoader = handlebars.NewLoader([]string{})
	}
	return c.templateLoader
}

func (c *Container) Parser() *markdown.Parser {
	return markdown.NewParser(markdown.ParserOpts{
		HashtagEnabled:      c.Config.Format.Markdown.Hashtags,
		MultiWordTagEnabled: c.Config.Format.Markdown.MultiwordTags,
		ColontagEnabled:     c.Config.Format.Markdown.ColonTags,
	})
}

func (c *Container) NoteFinder(tx sqlite.Transaction, opts fzf.NoteFinderOpts) *fzf.NoteFinder {
	return nil
	// notes := sqlite.NewNoteDAO(tx, c.Logger)
	// return fzf.NewNoteFinder(opts, notes, c.Terminal)
}

// Database returns the DB instance for the given notebook, after executing any
// pending migration and indexing the notes if needed.
func (c *Container) Database(forceIndexing bool) (*sqlite.DB, note.IndexingStats, error) {
	var stats note.IndexingStats

	if c.zkErr != nil {
		return nil, stats, c.zkErr
	}

	db, err := sqlite.Open(c.zk.DBPath())
	if err != nil {
		return nil, stats, err
	}
	needsReindexing, err := db.Migrate()
	if err != nil {
		return nil, stats, errors.Wrap(err, "failed to migrate the database")
	}

	stats, err = c.index(db, forceIndexing || needsReindexing)
	if err != nil {
		return nil, stats, err
	}

	return db, stats, err
}

func (c *Container) index(db *sqlite.DB, force bool) (note.IndexingStats, error) {
	var bar = progressbar.NewOptions(-1,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSpinnerType(14),
	)

	var err error
	var stats note.IndexingStats

	if c.zkErr != nil {
		return stats, c.zkErr
	}

	err = db.WithTransaction(func(tx sqlite.Transaction) error {
		stats, err = note.Index(
			c.zk,
			force,
			c.Parser(),
			nil,
			c.Logger,
			func(change paths.DiffChange) {
				bar.Add(1)
				bar.Describe(change.String())
			},
		)
		return err
	})
	bar.Clear()

	return stats, err
}

// Paginate creates an auto-closing io.Writer which will be automatically
// paginated if noPager is false, using the user's pager.
//
// You can write to the pager only in the run callback.
func (c *Container) Paginate(noPager bool, run func(out io.Writer) error) error {
	pager, err := c.pager(noPager || c.Config.Tool.Pager.IsEmpty())
	if err != nil {
		return err
	}
	err = run(pager)
	pager.Close()
	return err
}

func (c *Container) pager(noPager bool) (*pager.Pager, error) {
	if noPager || !c.Terminal.IsInteractive() {
		return pager.PassthroughPager, nil
	} else {
		return pager.New(c.Config.Tool.Pager, c.Logger)
	}
}
