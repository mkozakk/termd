package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"termd/internal/root"
)

func main() {
	var startPath string
	var openFile string

	if len(os.Args) > 1 {
		abs, err := filepath.Abs(os.Args[1])
		if err == nil {
			if info, err := os.Stat(abs); err == nil {
				if info.IsDir() {
					startPath = abs
				} else {
					startPath = filepath.Dir(abs)
					openFile = abs
				}
			}
		}
	}

	if startPath == "" {
		startPath, _ = os.Getwd()
	}

	m := root.New(startPath, openFile)
	p := tea.NewProgram(m, tea.WithMouseCellMotion(), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
