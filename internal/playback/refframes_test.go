package playback

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// A remote H.264 video reports no RefFrames, as with Jellyfin, but the
// decision reads the count its analysis gave: an app requiring RefFrames
// <= 4 for 1920-wide video, as Moonfin does, plays four reference frames
// as they are and converts five.
func TestRemoteVideoRefFramesReachTheDecisionOnly(t *testing.T) {
	t.Parallel()
	profile := &DeviceProfile{
		DirectPlayProfiles:  []DirectPlayProfile{{Type: "Video", Container: "mkv", VideoCodec: "h264", AudioCodec: "aac"}},
		TranscodingProfiles: []TranscodingProfile{{Type: "Video", Container: "ts", Protocol: "hls", VideoCodec: "h264", AudioCodec: "aac"}},
		CodecProfiles: []CodecProfile{{Type: "Video", Codec: "h264",
			Conditions:      []ProfileCondition{{Condition: "LessThanEqual", Property: "RefFrames", Value: "4", IsRequired: true}},
			ApplyConditions: []ProfileCondition{{Condition: "GreaterThanEqual", Property: "Width", Value: "1900", IsRequired: true}}}},
	}
	for refs, reasons := range map[string][]string{"4": nil, "5": {"RefFramesNotSupported"}} {
		streams := MediaStreams(remoteProbe(t,
			`{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","width":1920,"height":1080,"refs":`+refs+`}`,
			`{"index":1,"codec_type":"audio","codec_name":"aac","channels":2,"disposition":{"default":1}}`), nil, "en")
		video := streams[0]
		if video.RefFrames != nil || video.KnownRefFrames == nil {
			t.Fatalf("%s reference frames: RefFrames %v, KnownRefFrames %v", refs, video.RefFrames, video.KnownRefFrames)
		}
		sent, err := json.Marshal(video)
		if err != nil || strings.Contains(string(sent), "RefFrames") {
			t.Errorf("%s reference frames sent: %s %v", refs, sent, err)
		}
		got := Decide(profile, MediaSource{Container: "mkv", Bitrate: 8_000_000, Streams: streams}, Options{EnableDirectPlay: true, EnableDirectStream: true})
		if got.DirectPlay != (reasons == nil) || !slices.Equal(got.Reasons, reasons) {
			t.Errorf("%s reference frames: DirectPlay %v, Reasons %v; want reasons %v", refs, got.DirectPlay, got.Reasons, reasons)
		}
	}
}
