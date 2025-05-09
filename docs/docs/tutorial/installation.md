---
description: Learn how to install Huma and create your first API.
---

# Installation

## Prerequisites

Huma requires [Go 1.23 or newer](https://go.dev/dl/), so install that first. You'll also want some kind of [text editor or IDE](https://code.visualstudio.com/) to write code and a terminal to run commands.

## Project Setup

Next, open a terminal and create a new Go project, then go get the Huma dependency to it's ready to be imported:

{{ asciinema("../../terminal/install.cast", rows="12") }}

You should now have a directory structure like this:

```title="Directory Structure"
my-api/
  |-- go.mod
  |-- go.sum
```

That's it! Now you are ready to build your first Huma API!
