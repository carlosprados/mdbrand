---
title: "Team \\*alpha\\* at \\$100 \\{\\{y\\}\\} \\@ops"
mdbrand:
  brand: none
  style: note
---

A tool writes names its user chose, and escapes what Markdown would read.
Escaped braces stopped the build: pandoc saw a placeholder that mdbrand,
reading the source, did not fill. A dollar in the title stopped Typst's
build in pandoc's template parser.

## Scenario \{\{y\}\} at \$100

- **Scenario \{\{y\}\} \| \*stars\* \@z \#0**: LITERAL-BULLET.

| Scenario | Cost |
|---|---|
| LITERAL-CELL \{\{y\}\} | \$100 |

{{words}} words.
