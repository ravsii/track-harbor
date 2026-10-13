package library

import (
	"github.com/ravsii/trackharbor/spotify"
	"testing"
)

func TestSpotifyTrackIDUsesSourceIdentityNotPlaylistEntry(t *testing.T) {
	var item spotify.Item
	item.UID = "playlist-entry"
	item.ItemV3.Data.URI = "spotify:track:0123456789ABCDEFGHIJKL"
	id, err := spotifyTrackID(item)
	if err != nil || id != "0123456789ABCDEFGHIJKL" {
		t.Fatalf("%q %v", id, err)
	}
	item.ItemV3.Data.URI = ""
	item.ItemV2.Data.URI = "spotify:track:abcdefghijklmnopqrstuv"
	id, err = spotifyTrackID(item)
	if err != nil || id != "abcdefghijklmnopqrstuv" {
		t.Fatalf("fallback: %q %v", id, err)
	}
	item.ItemV2.Data.URI = "spotify:album:abcdefghijklmnopqrstuv"
	if _, err = spotifyTrackID(item); err == nil {
		t.Fatal("album ID treated as track")
	}
	item.ItemV2.Data.URI = ""
	if _, err = spotifyTrackID(item); err == nil {
		t.Fatal("missing ID invented")
	}
}
