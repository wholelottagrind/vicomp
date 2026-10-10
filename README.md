# vicomp

Compress macOS screen recordings for Telegram (or anywhere) with ffmpeg,
with every setting explained.

## Install

Needs Go and ffmpeg (`brew install ffmpeg`).

```bash
go build -o ~/.local/bin/vicomp .
```

## Use

```
vicomp                 pick a recording from your folders and compress it
vicomp <file>          open the settings for a specific video
vicomp last            compress the newest recording with the default preset
vicomp last -dry-run   print the ffmpeg command and what each part does
vicomp presets         list presets
vicomp config          open the config file in your editor
```

In the app: `↑↓` pick a setting, `←→` change it, `enter` compress.
`?` explains every part of the ffmpeg command, `s` saves your settings as a
preset, `d` makes the current output folder the default, and `y` copies the command.

When it's done, the compressed file is on your clipboard. Press ⌘V in a Telegram chat.

## Config

`~/.config/vicomp/config.toml` is created on first run with comments for every
option: source folders, default output folder, filename template, default
preset, after-compress actions (clipboard / reveal in Finder / move original
to Trash), and your presets.

## Presets

| name     | what it's for |
|----------|---------------|
| telegram | default. 1080p, 30 fps, quality 26. Small with crisp text |
| tiny     | 720p, 24 fps, quality 30. For long recordings |
| quality  | original size and fps, quality 20 |
| fast     | hardware encoder: seconds instead of minutes, bigger file |
| classic  | x264 defaults, same as `ffmpeg -i in.mov -c:v libx264 -c:a aac out.mp4` |
