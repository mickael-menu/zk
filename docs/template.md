# Template syntax

`zk` uses the [Handlebars template syntax](https://handlebarsjs.com/guide) for its templates. The list of variables available depends of the running command:

* [Template context when creating notes](template-creation.md) (i.e. `zk new`)
* [Template context when formatting a note](template-format.md) (i.e. `zk list --format <template>`)

## Additional helpers

Besides the default Handlebars helpers, `zk` ships with additional helpers which you might find useful. They are available to all templates.

### Date helper

The `{{date}}` helper formats the given date for display.

Template contexts usually provide a `now` variable which can be used to print the current date.

The default format output by `{{date <variable>}}` looks like `2009-11-17`, but you can choose a different format by providing a second argument, e.g. `{{date now "medium"}}`.

| Format           | Output                     | Notes                                            |
|------------------|----------------------------|--------------------------------------------------|
| `short`          | 11/17/2009                 |                                                  |
| `medium`         | Nov 17, 2009               |                                                  |
| `long`           | November 17, 2009          |                                                  |
| `full`           | Tuesday, November 17, 2009 |                                                  |
| `year`           | 2009                       |                                                  |
| `time`           | 20:34                      |                                                  |
| `timestamp`      | 200911172034               | Useful for sortable filenames                    |
| `timestamp-unix` | 1258490098                 | Number of seconds since January 1, 1970          |
| `elapsed`        | 12 years ago               | Time elapsed since then in human-friendly format |

If none of the provided formats suit you, you can use a custom format using `strftime`-style placeholders, e.g. `{{date now "%m-%d-%Y"}}`. See `man strftime` for a list of placeholders.

### Slug helper

The `{{slug}}` helper generates a URL friendly version of a text. For example, `{{slug "This will be slugified!"}}` becomes `this-will-be-slugified`.

This is mostly useful to generate a safe filename containing the title passed to `zk new --title "An interesting note"`. With the [`filename`](config-note.md) template `{{slug title}}`, it becomes `an-interesting-note.md`.

### Prepend helper

The `{{prepend}}` helper adds a prefix to every line of the given text or block. You can use it to generate a Markdown quote, for example:

```
{{prepend "> " "A quote"}}

{{#prepend "> "}}
A multiline
quote.
{{/prepend}}
```

### Shell helper

The `{{sh}}` helper will call the given shell command and insert its output in the template. Your imagination is the limit!

```
Get today's events from your calendar:
{{sh "icalBuddy -b '* ' -nc eventsToday"}}

Insert a random quote:
{{prepend '> ' (sh 'fortune')}}

Download today's weather:
{{sh 'curl http://wttr.in/?0'}}
```

When used as a block helper, the block content will be passed to the command through a standard input pipe.

```
Will output "HELLO, WORLD!":
{{#sh "tr '[a-z]' '[A-Z]'"}}
Hello, world!
{{/sh}}
```

### Style helper

The `{{style}}` helper is mostly useful when formatting content for the command-line. See the [styling rules](style.md) for more information.

```
{{style 'red bold' 'A text'}}

{{#style 'underline'}}Another text{{/style}}
```
