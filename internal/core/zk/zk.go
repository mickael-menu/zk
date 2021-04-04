package zk

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mickael-menu/zk/internal/util/errors"
	"github.com/mickael-menu/zk/internal/util/paths"
)

// ErrNotebookNotFound is an error returned when a notebook cannot be found at the given path or its parents.
type ErrNotebookNotFound string

func (e ErrNotebookNotFound) Error() string {
	return fmt.Sprintf("no notebook found in %s or a parent directory", string(e))
}

const defaultConfig = `# zk configuration file
#
# Uncomment the properties you want to customize.

# NOTE SETTINGS
#
# Defines the default options used when generating new notes.
[note]

# Language used when writing notes.
# This is used to generate slugs or with date formats.
#language = "en"

# The default title used for new note, if no ` + "`" + `--title` + "`" + ` flag is provided.
#default-title = "Untitled"

# Template used to generate a note's filename, without extension.
#filename = "{{id}}"

# The file extension used for the notes.
#extension = "md"

# Template used to generate a note's content.
# If not an absolute path, it is relative to .zk/templates/
template = "default.md"

# Configure random ID generation.

# The charset used for random IDs. You can use:
#   * letters: only letters from a to z.
#   * numbers: 0 to 9
#   * alphanum: letters + numbers
#   * hex: hexadecimal, from a to f and 0 to 9
#   * custom string: will use any character from the provided value
#id-charset = "alphanum"

# Length of the generated IDs.
#id-length = 4

# Letter case for the random IDs, among lower, upper or mixed.
#id-case = "lower"


# EXTRA VARIABLES
#
# A dictionary of variables you can use for any custom values when generating
# new notes. They are accessible in templates with {{extra.<key>}}
[extra]

#key = "value"


# GROUP OVERRIDES
#
# You can override global settings from [note] and [extra] for a particular
# group of notes by declaring a [group."<name>"] section.
#
# Specify the list of directories which will automatically belong to the group
# with the optional ` + "`" + `paths` + "`" + ` property.
#
# Omitting ` + "`" + `paths` + "`" + ` is equivalent to providing a single path equal to the name of
# the group. This can be useful to quickly declare a group by the name of the
# directory it applies to.

#[dir."<NAME>"]
#paths = ["<DIR1>", "<DIR2>"]
#[dir."<NAME>".note]
#filename = "{{date now}}"
#[dir."<NAME>".extra]
#key = "value"


# MARKDOWN SETTINGS
[format.markdown]
# Enable support for #hashtags
hashtags = true
# Enable support for :colon:separated:tags:
#colon-tags = true
# Enable support for Bear's #multi-word tags#
# Hashtags must be enabled for multi-word tags to work.
#multiword-tags = true


# EXTERNAL TOOLS
[tool]

# Default editor used to open notes. When not set, the EDITOR or VISUAL
# environment variables are used.
#editor = "vim"

# Pager used to scroll through long output. If you want to disable paging
# altogether, set it to an empty string "".
#pager = "less -FIRX"

# Command used to preview a note during interactive fzf mode.
# Set it to an empty string "" to disable preview.

# bat is a great tool to render Markdown document with syntax highlighting.
#https://github.com/sharkdp/bat
#fzf-preview = "bat -p --color always {-1}"


# NAMED FILTERS
#
#    A named filter is a set of note filtering options used frequently together.
#
[filter]

# Matches the notes created the last two weeks. For example:
#    $ zk list recents --limit 15
#    $ zk edit recents --interactive
#recents = "--sort created- --created-after 'last two weeks'"


# COMMAND ALIASES
#
#   Aliases are user commands called with ` + "`" + `zk <alias> [<flags>] [<args>]` + "`" + `.
#
#   The alias will be executed with ` + "`" + `$SHELL -c` + "`" + `, please refer to your shell's
#   man page to see the available syntax. In most shells:
#     * $@ can be used to expand all the provided flags and arguments
#     * you can pipe commands together with the usual | character
#
[alias]
# Here are a few aliases to get you started.

# Shortcut to a command.
#ls = "zk list $@"

# Default flags for an existing command.
#list = "zk list --quiet $@"

# Edit the last modified note.
#editlast = "zk edit --limit 1 --sort modified- $@"

# Edit the notes selected interactively among the notes created the last two weeks.
# This alias doesn't take any argument, so we don't use $@.
#recent = "zk edit --sort created- --created-after 'last two weeks' --interactive"

# Print paths separated with colons for the notes found with the given
# arguments. This can be useful to expand a complex search query into a flag
# taking only paths. For example:
#   zk list --link-to "` + "`" + `zk path -m potatoe` + "`" + `"
#path = "zk list --quiet --format {{path}} --delimiter , $@"

# Show a random note.
#lucky = "zk list --quiet --format full --sort random --limit 1"

# Returns the Git history for the notes found with the given arguments.
# Note the use of a pipe and the location of $@.
#hist = "zk list --format path --delimiter0 --quiet $@ | xargs -t -0 git log --patch --"

# Edit this configuration file.
#conf = '$EDITOR "$ZK_NOTEBOOK_DIR/.zk/config.toml"'
`

const defaultTemplate = `# {{title}}

{{content}}
`

// Zk (Zettelkasten) represents an opened notebook.
type Zk struct {
	// Notebook root path.
	Path string
	// Global user configuration.
	Config Config
	// Working directory from which paths are relative.
	workingDir string
}

// Dir represents a directory inside a notebook.
type Dir struct {
	// Name of the directory, which is the path relative to the notebook's root.
	Name string
	// Absolute path to the directory.
	Path string
	// User configuration for this directory.
	Config GroupConfig
}

// Open locates a notebook at the given path and parses its configuration.
func Open(originalPath string, parentConfig Config) (*Zk, error) {
	wrap := errors.Wrapper("open failed")

	path, err := filepath.Abs(originalPath)
	if err != nil {
		return nil, wrap(err)
	}
	path, err = locateRoot(path)
	if err != nil {
		return nil, wrap(err)
	}

	config, err := OpenConfig(filepath.Join(path, ".zk/config.toml"), parentConfig)
	if err != nil {
		return nil, wrap(err)
	}

	return &Zk{
		Path:       path,
		Config:     config,
		workingDir: originalPath,
	}, nil
}

// Create initializes a new notebook at the given path.
func Create(path string) error {
	wrap := errors.Wrapper("init failed")

	path, err := filepath.Abs(path)
	if err != nil {
		return wrap(err)
	}

	if existingPath, err := locateRoot(path); err == nil {
		return wrap(fmt.Errorf("a notebook already exists in %v", existingPath))
	}

	// Write default config.toml.
	err = paths.WriteString(filepath.Join(path, ".zk/config.toml"), defaultConfig)
	if err != nil {
		return wrap(err)
	}

	// Write default template.
	err = paths.WriteString(filepath.Join(path, ".zk/templates/default.md"), defaultTemplate)
	if err != nil {
		return wrap(err)
	}

	return nil
}

// locate finds the root of the notebook containing the given path.
func locateRoot(path string) (string, error) {
	if !filepath.IsAbs(path) {
		panic("absolute path expected")
	}

	var locate func(string) (string, error)
	locate = func(currentPath string) (string, error) {
		if currentPath == "/" || currentPath == "." {
			return "", ErrNotebookNotFound(path)
		}
		exists, err := paths.DirExists(filepath.Join(currentPath, ".zk"))
		switch {
		case err != nil:
			return "", err
		case exists:
			return currentPath, nil
		default:
			return locate(filepath.Dir(currentPath))
		}
	}

	return locate(path)
}

// DBPath returns the path to the notebook database.
func (zk *Zk) DBPath() string {
	return filepath.Join(zk.Path, ".zk/notebook.db")
}

// RelPath returns the path relative to the notebook root to the given path.
func (zk *Zk) RelPath(originalPath string) (string, error) {
	wrap := errors.Wrapperf("%v: not a valid notebook path", originalPath)

	path, err := zk.absPath(originalPath)
	if err != nil {
		return path, wrap(err)
	}

	path, err = filepath.Rel(zk.Path, path)
	if err != nil {
		return path, wrap(err)
	}
	if strings.HasPrefix(path, "..") {
		return path, fmt.Errorf("%s: path is outside the notebook", originalPath)
	}
	if path == "." {
		path = ""
	}
	return path, nil
}

// AbsPath makes the given path absolute, using the current working directory
// as reference.
func (zk *Zk) absPath(originalPath string) (string, error) {
	var err error

	path := originalPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(zk.workingDir, path)
		path, err = filepath.Abs(path)
		if err != nil {
			return path, err
		}
	}

	return path, nil
}

// RootDir returns the root Dir for this notebook.
func (zk *Zk) RootDir() Dir {
	return Dir{
		Name:   "",
		Path:   zk.Path,
		Config: zk.Config.RootGroupConfig(),
	}
}

// DirAt returns a Dir representation of the notebook directory at the given path.
func (zk *Zk) DirAt(path string, overrides ...ConfigOverrides) (*Dir, error) {
	path, err := zk.absPath(path)
	if err != nil {
		return nil, errors.Wrapf(err, "%v: not a valid notebook directory", path)
	}

	name, err := zk.RelPath(path)
	if err != nil {
		return nil, err
	}

	config, err := zk.findConfigForDirNamed(name, overrides)
	if err != nil {
		return nil, err
	}

	config = config.Clone()
	for _, v := range overrides {
		config.Override(v)
	}

	return &Dir{
		Name:   name,
		Path:   path,
		Config: config,
	}, nil
}

func (zk *Zk) findConfigForDirNamed(name string, overrides []ConfigOverrides) (GroupConfig, error) {
	// If there's a Group overrides, attempt to find a matching group.
	overriddenGroup := ""
	for _, o := range overrides {
		if !o.Group.IsNull() {
			overriddenGroup = o.Group.Unwrap()
			if group, ok := zk.Config.Groups[overriddenGroup]; ok {
				return group, nil
			}
		}
	}

	if overriddenGroup != "" {
		return GroupConfig{}, fmt.Errorf("%s: group not find in the config file", overriddenGroup)
	}

	for groupName, group := range zk.Config.Groups {
		for _, path := range group.Paths {
			matches, err := filepath.Match(path, name)
			if err != nil {
				return GroupConfig{}, errors.Wrapf(err, "failed to match group %s to %s", groupName, name)
			} else if matches {
				return group, nil
			}
		}
	}
	// Fallback on root config.
	return zk.Config.RootGroupConfig(), nil
}

// RequiredDirAt is the same as DirAt, but checks that the directory exists
// before returning the Dir.
func (zk *Zk) RequireDirAt(path string, overrides ...ConfigOverrides) (*Dir, error) {
	dir, err := zk.DirAt(path, overrides...)
	if err != nil {
		return nil, err
	}
	exists, err := paths.Exists(dir.Path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%v: directory not found", path)
	}
	return dir, nil
}
