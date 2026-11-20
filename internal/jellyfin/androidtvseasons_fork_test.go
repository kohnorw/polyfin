package jellyfin

import (
	"testing"
)

// Jellyfin for Android TV opens a season only when its listing counts the
// season's episodes (ChildCount), and lists the series' other seasons
// leaving the one shown out (excludeItemIds).
func TestAndroidTvSeasonCountsItsEpisodes(t *testing.T) {
	tr := newTracking(t)
	var seasons QueryResult
	tr.get(t, "/Shows/"+tr.series+"/Seasons?userId="+tr.user+"&fields=ChildCount,DisplayPreferencesId", tr.token, &seasons)
	if len(seasons.Items) == 0 {
		t.Fatal("no seasons")
	}
	for _, season := range seasons.Items {
		var episodes QueryResult
		tr.get(t, "/Items?userId="+tr.user+"&parentId="+season.Id+"&fields=ChildCount", tr.token, &episodes)
		if season.ChildCount == nil || *season.ChildCount != len(episodes.Items) {
			t.Errorf("season %s: ChildCount %v, %d episodes listed", season.Name, season.ChildCount, len(episodes.Items))
		}
	}

	var series QueryResult
	tr.get(t, "/Items?userId="+tr.user+"&ids="+tr.series+"&fields=ChildCount", tr.token, &series)
	if len(series.Items) != 1 || series.Items[0].ChildCount == nil || *series.Items[0].ChildCount != len(seasons.Items) {
		t.Errorf("series ChildCount: %+v, want %d", series.Items, len(seasons.Items))
	}

	var others QueryResult
	tr.get(t, "/Items?userId="+tr.user+"&parentId="+tr.series+"&includeItemTypes=Season&excludeItemIds="+seasons.Items[0].Id, tr.token, &others)
	for _, season := range others.Items {
		if season.Id == seasons.Items[0].Id {
			t.Errorf("excluded season %s listed", season.Name)
		}
	}
	if len(others.Items) != len(seasons.Items)-1 {
		t.Errorf("%d other seasons, want %d", len(others.Items), len(seasons.Items)-1)
	}
}
