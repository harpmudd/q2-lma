package main

import (
	"bufio"
	"bytes"
	"embed"
	"os"
	"path"
	"strings"

	"q2-lma/internal/q2"
)

// The Streaming icons for each Q2 Pod theme (tools/icons): a grey disc for Minimal, which greys a
// card app's icon by its luminance, and a black one for Classic, which shows it as it is. One file
// can't be both, so at each start the app writes the theme's into .lma/icon.png and icon-40.png.
// Q2 Pod reads them when it next loads its images: a theme change, Light Mode's Auto switching, or
// a restart. The theme is Q2 Pod's own setting, IPOD THEME in /mnt/data/config.ini (navigation.c
// setting_keys: 0 Minimal, its default, or 1 Classic); not a documented interface, so anything
// unexpected leaves the icons as they are.
//
//go:embed icons/*.png
var icons embed.FS

// theme is Q2 Pod's theme from config.ini: "classic", "minimal", or "" when it can't be read.
func theme(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToUpper(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || section != "IPOD" || strings.ToUpper(strings.TrimSpace(k)) != "THEME" {
			continue
		}
		switch strings.TrimSpace(v) {
		case "1":
			return "classic"
		case "0":
			return "minimal"
		}
		return ""
	}
	if section == "" {
		return "" // not the layout expected: leave the icons alone
	}
	return "minimal" // no THEME key: Q2 Pod's default
}

// themeIcons writes the icons the theme wants, only where they differ, and says what it did.
func (a *app) themeIcons(config string) {
	t := theme(config)
	if t == "" {
		a.log.Printf("theme: %s unreadable, icons left as they are", config)
		return
	}
	changed := 0
	for px, name := range map[string]string{"52": "icon.png", "40": "icon-40.png"} {
		want, err := icons.ReadFile("icons/" + t + "-" + px + ".png")
		if err != nil {
			continue
		}
		dst := path.Join(a.dir, name)
		if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, want) {
			continue
		}
		if err := q2.WriteFile(dst, want); err != nil {
			a.log.Printf("theme: %s: %v", name, err)
			continue
		}
		changed++
	}
	if changed > 0 {
		a.log.Printf("theme: %s icons written; Q2 Pod shows them once it reloads its images", t)
	}
}
