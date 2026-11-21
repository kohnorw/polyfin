package jellyfin

import "testing"

// The movie runs 1h30, 54 000 000 000 ticks: 40% is 21 600 000 000.
const movieAt40 = int64(21_600_000_000)

func TestFailedStopLeavesTheUserDataAsItWas(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.movie, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.movie, "PositionTicks": movieAt40})
	// The player gave up near the end: nothing of it counts.
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "PositionTicks": int64(53_000_000_000), "Failed": true})
	if data := tr.userData(t, tr.movie); data.Played || data.PlaybackPositionTicks != movieAt40 || data.PlayCount != 1 {
		t.Errorf("after a failed stop: %+v", data)
	}
	// Nor does a failed stop without a position play the movie through.
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "Failed": true})
	if data := tr.userData(t, tr.movie); data.Played || data.PlaybackPositionTicks != movieAt40 || data.PlayCount != 1 {
		t.Errorf("after a failed stop without a position: %+v", data)
	}
}

func TestStopWithoutPositionKeepsTheLastProgress(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.movie, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.movie, "PositionTicks": movieAt40})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie})
	if data := tr.userData(t, tr.movie); data.Played || data.PlaybackPositionTicks != movieAt40 || data.PlayCount != 1 {
		t.Errorf("stopped without a position after progress at 40%%: %+v", data)
	}
	if resume := tr.list(t, "/UserItems/Resume?mediaTypes=Video"); len(resume) != 1 || resume[0] != tr.movie {
		t.Errorf("resume: %v", resume)
	}
	// An app that never reports its progress plays the item through on a
	// stop without a position, as with Jellyfin.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[0], "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[0]})
	if data := tr.userData(t, tr.episodes[0]); !data.Played || data.PlaybackPositionTicks != 0 || data.PlayCount != 2 {
		t.Errorf("stopped without a position nor progress: %+v", data)
	}
}
