package library

import "testing"

func TestReleaseTitles(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ file, title string }{
		{"Friends.S01E01.1080p.BluRay.x265.mkv", "friends"},
		{"Smiling.Friends.S01E01.1080p.WEB.mkv", "smiling friends"},
		{"[Group] Friends - S01E01 [1080p].mkv", "friends"},
		{"Spider-Man.No.Way.Home.2021.2160p.mkv", "spider man no way home"},
		{"The Office (US) S02E01.mkv", "the office us"},
		{"Friends (1994) S01E01.mkv", "friends"},
		{"movie.mkv", ""},
		{"Movie.English.1080p.mkv", "movie"},
		{"Friends.FRENCH.S01E01.mkv", "friends"},
		{"", ""},
	} {
		if got := releaseTitle(c.file); got != c.title {
			t.Errorf("%q: %q, want %q", c.file, got, c.title)
		}
	}
	if !sameReleaseTitle("friends", matchKey("Friends")) || sameReleaseTitle("smiling friends", matchKey("Friends")) {
		t.Error("Friends and Smiling Friends are told apart wrong")
	}
	if !otherTitleWith("smiling friends", "friends") || otherTitleWith("friends", "friends") || otherTitleWith("movie", "test movie") {
		t.Error("only longer titles holding the title are other titles")
	}
	if !sameReleaseTitle("spider man no way home", matchKey("Spider-Man: No Way Home")) || !sameReleaseTitle("office", matchKey("The Office")) {
		t.Error("punctuation or the leading the counts")
	}
}

func TestMismatchedFiles(t *testing.T) {
	t.Parallel()
	movie := func(name string, year int) target { return target{kind: KindMovie, name: name, year: year} }
	episode := func(name string, season, number int) target {
		return target{kind: KindEpisode, name: name, season: season, episode: number}
	}
	for _, c := range []struct {
		t    target
		file string
		drop bool
	}{
		{movie("Spider-Man: Brand New Day", 2026), "Spider-Man.Brand.New.Day.2026.2160p.WEB-DL.mkv", false},
		{movie("Spider-Man: Brand New Day", 2026), "Spider-Man.No.Way.Home.2021.2160p.mkv", true},
		{movie("Spider-Man: Brand New Day", 2026), "Spiderman.Brand.New.Day.2026.1080p.mkv", false},
		{movie("Dune", 2021), "Dune.1984.1080p.BluRay.mkv", true},
		{movie("Dune", 2021), "Dune.2021.2160p.mkv", false},
		{movie("Dune", 2021), "Dune.Part.Two.2024.mkv", true},
		{movie("Blade Runner 2049", 2017), "Blade.Runner.2049.2017.2160p.mkv", false},
		{movie("Blade Runner 2049", 2017), "Blade.Runner.1982.Final.Cut.mkv", true},
		{movie("The Matrix", 1999), "Matrix.1999.1080p.mkv", false},
		{movie("Theory of Everything", 2014), "The.Theory.of.Everything.2014.mkv", false},
		{movie("Some Movie", 2020), "movie.mkv", false},
		{movie("Some Movie", 2020), "", false},
		{episode("Friends", 1, 1), "Friends.S01E01.1080p.mkv", false},
		{episode("Friends", 1, 1), "Smiling.Friends.S01E01.1080p.mkv", true},
		{episode("Friends", 1, 1), "Friends.S01E02.1080p.mkv", true},
		{episode("Friends", 1, 1), "Friends.S02E01.1080p.mkv", true},
		{episode("Friends", 1, 3), "Friends.S01E02-E03.mkv", false},
		{episode("Friends", 1, 3), "Friends.S01E02E03.mkv", false},
		{episode("Friends", 1, 4), "Friends.S01E02E03.mkv", true},
		{episode("Friends", 2, 5), "Friends.2x05.mkv", false},
		{episode("The Office", 2, 1), "The Office (US) S02E01.mkv", false},
		{episode("Marvel's Agents of S.H.I.E.L.D.", 1, 1), "Marvels.Agents.of.S.H.I.E.L.D.S01E01.mkv", false},
		{episode("Doctor Who (2005)", 1, 1), "Doctor.Who.2005.S01E01.mkv", false},
		{episode("Doctor Who (2005)", 1, 1), "Doctor.Who.S01E01.mkv", false},
		{episode("VisionQuest", 1, 1), "VisionQuest.S01.1080p.mkv", false},
		{episode("Friends", 1, 1), "[Group] Friends - 01 [1080p].mkv", false},
	} {
		if got := mismatched(c.t, matchKey(c.t.name), c.file); got != c.drop {
			t.Errorf("%q for %q: left out %v, want %v", c.file, c.t.name, got, c.drop)
		}
	}
}
