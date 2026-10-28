package hls

import (
	"fmt"
	"strings"
)

// Rendition is a subtitle track a master playlist offers, as Jellyfin's
// DynamicHlsHelper.AddSubtitles lists the text subtitles of the media
// source.
type Rendition struct {
	// Name is the stream's DisplayTitle.
	Name string
	// Language is the stream's language, "Unknown" when it has none.
	Language string
	// Default marks the track the app chose, delivered in HLS; Forced, a
	// forced track.
	Default, Forced bool
	// URI is the track's subtitles.m3u8, relative to the master playlist.
	URI string
}

// writeRenditions writes the subtitle renditions of a master playlist in
// the "subs" group, as Jellyfin writes them.
func writeRenditions(b *strings.Builder, renditions []Rendition) {
	for _, r := range renditions {
		fmt.Fprintf(b, "#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subs\",NAME=\"%s\",DEFAULT=%s,FORCED=%s,AUTOSELECT=YES,URI=\"%s\",LANGUAGE=\"%s\"\n",
			r.Name, yes(r.Default), yes(r.Forced), r.URI, r.Language)
	}
}

func yes(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}
