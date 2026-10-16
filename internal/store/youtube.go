package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ravsii/trackharbor/download"
)

func (s *Store) UserYouTubeTrack(userID int64, key string) (Track, error) {
	tracks, err := s.queryTracks(personalSelect+` WHERE p.user_id=? AND t.source='youtube' AND t.source_key=?`, userID, key)
	if err != nil {
		return Track{}, err
	}
	if len(tracks) == 0 {
		return Track{}, sql.ErrNoRows
	}
	return tracks[0], nil
}

// AddYouTubeTrack adds one membership without replacing the user's collection.
func (s *Store) AddYouTubeTrack(userID int64, track download.Track) (Track, bool, error) {
	id, err := download.YouTubeID(track.SourceID)
	if err != nil || id != track.SourceID || track.Source != "youtube" || track.Title == "" {
		return Track{}, false, fmt.Errorf("invalid YouTube track")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Track{}, false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO tracks(source,source_key,artist,name,cover_url,source_url) VALUES('youtube',?,?,?,?,?)
		ON CONFLICT(source,source_key) DO UPDATE SET artist=excluded.artist,name=excluded.name,cover_url=excluded.cover_url,source_url=excluded.source_url`, id, track.Artist, track.Title, track.CoverURL, track.AudioURL); err != nil {
		return Track{}, false, err
	}
	var trackID int64
	if err := tx.QueryRow(`SELECT id FROM tracks WHERE source='youtube' AND source_key=?`, id).Scan(&trackID); err != nil {
		return Track{}, false, err
	}
	result, err := tx.Exec(`INSERT INTO user_tracks(user_id,track_id,added_at) VALUES(?,?,?) ON CONFLICT(user_id,track_id) DO NOTHING`, userID, trackID, time.Now().Unix())
	if err != nil {
		return Track{}, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Track{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Track{}, false, err
	}
	saved, err := s.UserYouTubeTrack(userID, id)
	return saved, n != 0, err
}
