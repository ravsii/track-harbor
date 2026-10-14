package store

// LibrarySummary contains catalog totals and only the current user's sources.
// It never exposes credentials or another user's playlist counts.
type LibrarySummary struct {
	Total            int  `json:"total"`
	Personal         int  `json:"personal"`
	Spotify          int  `json:"spotify"`
	VK               int  `json:"vk"`
	SpotifyConnected bool `json:"spotify_connected"`
	VKConnected      bool `json:"vk_connected"`
}

func (s *Store) LibrarySummary(userID int64) (LibrarySummary, error) {
	var summary LibrarySummary
	err := s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM tracks),
		(SELECT COUNT(*) FROM user_tracks WHERE user_id=?),
		(SELECT COUNT(*) FROM user_tracks p JOIN tracks t ON t.id=p.track_id WHERE p.user_id=? AND t.source='spotify'),
		(SELECT COUNT(*) FROM user_tracks p JOIN tracks t ON t.id=p.track_id WHERE p.user_id=? AND t.source='vk'),
		EXISTS(SELECT 1 FROM user_credentials WHERE user_id=? AND source='spotify'),
		EXISTS(SELECT 1 FROM user_credentials WHERE user_id=? AND source='vk')`,
		userID, userID, userID, userID, userID).
		Scan(&summary.Total, &summary.Personal, &summary.Spotify, &summary.VK, &summary.SpotifyConnected, &summary.VKConnected)
	return summary, err
}
