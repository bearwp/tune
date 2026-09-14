package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	tracks := gatherTracks(os.Args[1:])
	if len(tracks) == 0 {
		fmt.Fprintln(os.Stderr, "tune: no .mp3 files found (pass paths or run in a directory with mp3s)")
		os.Exit(1)
	}

	player := NewPlayer()
	if err := InitSpeaker(); err != nil {
		fmt.Fprintf(os.Stderr, "tune: audio device unavailable: %v\n", err)
	}

	p := tea.NewProgram(NewModel(tracks, player), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tune: %v\n", err)
		os.Exit(1)
	}
}

func gatherTracks(args []string) []track {
	var files []string
	if len(args) == 0 {
		args = []string{musicDir()}
	}
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			continue
		}
		if info.IsDir() {
			entries, _ := os.ReadDir(a)
			for _, e := range entries {
				if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".mp3") {
					files = append(files, filepath.Join(a, e.Name()))
				}
			}
		} else if strings.EqualFold(filepath.Ext(a), ".mp3") {
			files = append(files, a)
		}
	}

	tracks := make([]track, 0, len(files))
	for _, f := range files {
		title, artist := parseName(filepath.Base(f))
		tracks = append(tracks, track{
			path:     f,
			title:    title,
			artist:   artist,
			duration: MP3Duration(f),
		})
	}
	return tracks
}

func parseName(name string) (title, artist string) {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if i := strings.Index(name, " - "); i > 0 {
		artist = strings.TrimSpace(name[:i])
		title = strings.TrimSpace(name[i+3:])
		return title, artist
	}
	return name, ""
}

func musicDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		candidates := []string{
			filepath.Join(home, "Music"),
			filepath.Join(home, "music"),
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && info.IsDir() {
				return c
			}
		}
		return home
	}
	return "."
}
