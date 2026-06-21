package mp3party

import "meshvault/internal/download"

type trackInternal struct {
	SongID     string
	Artist     string
	Title      string
	URL        string
	Duration   string
	Size       string
	Bitrate    string
	PlayURL    string
	PageURL    string
	IsPlaylist bool
	Favorite   string
}

func (t trackInternal) toCommon() download.Track {
	return download.Track{
		ID:       t.SongID,
		Artist:   t.Artist,
		Title:    t.Title,
		Duration: t.Duration,
		Size:     t.Size,
		Bitrate:  t.Bitrate,
	}
}

func tracksToCommon(ts []trackInternal) []download.Track {
	out := make([]download.Track, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.toCommon())
	}
	return out
}
