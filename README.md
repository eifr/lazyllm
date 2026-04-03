# LazyLLM

A Terminal User Interface (TUI) for managing local large language models using [Ollama](https://ollama.com/). Built with Go and the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework, featuring a clean Gruvbox Dark aesthetic.

## Features

- **Model Management**: View your installed models, including size, format, quantization, and last modified date.
- **Resource Control**: Easily load (`l`) models into memory or unload (`u`) them to free up resources.
- **Model Discovery**: Browse the Ollama registry (`r`) to discover and pull new models.
- **Pull Models**: Pull models (`p`) directly from the UI with real-time progress bars. Supports pulling with the `--insecure` flag.
- **Quick Chat**: Start a chat session with a model instantly by pressing `Enter`.
- **Open With...**: Quickly launch models with your favorite external CLI tools like Claude Code, OpenCode, and OpenClaw (`o`).
- **Ollama Detection**: Automatically detects if Ollama is installed or running, and offers to install it via the official script if missing.

## Installation

Ensure you have Go installed, then clone and build the project:

```bash
git clone https://github.com/yourusername/lazyllm.git
cd lazyllm
go build -o lazyllm .
```

Move the binary to somewhere in your `$PATH` to use it from anywhere.

```bash
sudo mv lazyllm /usr/local/bin/
```

## Usage

Simply run `lazyllm` in your terminal:

```bash
lazyllm
```

### Keybindings

- **`Up`/`Down`** or **`j`/`k`**: Navigate lists
- **`/`**: Filter list items
- **`p`**: Pull a new model
- **`r`**: Browse the Ollama Registry
- **`d`**: Delete the selected model
- **`l`**: Load the selected model into memory
- **`u`**: Unload the selected model from memory
- **`Enter`**: Launch the default chat command for the selected model
- **`o`**: Open the "Open With..." menu to launch the model with an external tool
- **`q`** / **`ctrl+c`**: Quit the application

### Configuration

You can customize the default chat command that runs when you press `Enter` on a model by setting the `LAZYLLM_CHAT_CMD` environment variable. Use `{model}` as a placeholder for the selected model name.

```bash
export LAZYLLM_CHAT_CMD="ollama run {model}" # This is the default
```

## Built With

*   [Ollama](https://github.com/ollama/ollama) - Local LLM runner.
*   [Bubble Tea](https://github.com/charmbracelet/bubbletea) - The fun, functional and stateful way to build terminal apps.
*   [Bubbles](https://github.com/charmbracelet/bubbles) - UI components for Bubble Tea.
*   [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions for nice terminal layouts.
*   [goquery](https://github.com/PuerkitoBio/goquery) - Used for scraping the Ollama registry.
