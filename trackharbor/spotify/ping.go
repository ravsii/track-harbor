package spotify

import (
	"context"
	"fmt"
)

// Ping checks that both saved Web Player headers can access the library,
// without fetching a playlist or changing any saved data.
func Ping(ctx context.Context, bearer, clientToken string) error {
	var response libraryResponse
	variables := map[string]any{
		"order": nil, "textFilter": "", "features": []string{"LIKED_SONGS"},
		"limit": 1, "offset": 0, "flatten": true,
		"expandedFolders": []string{}, "folderUri": nil, "includeFoldersWhenFlattening": true,
	}
	if err := queryContext(ctx, bearer, clientToken, "libraryV3", libraryQueryHash, variables, &response); err != nil {
		return err
	}
	if library := response.Data.Me.Library; library == nil || library.TotalCount == nil || *library.TotalCount < 0 {
		return fmt.Errorf("Spotify returned an invalid library response")
	}
	return nil
}
