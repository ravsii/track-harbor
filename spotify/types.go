package spotify

type PlaylistResponse struct {
	Data struct {
		PlaylistV2 PlaylistV2 `json:"playlistV2"`
	} `json:"data"`
}

type PlaylistV2 struct {
	Typename    string      `json:"__typename"`
	URI         string      `json:"uri"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Format      string      `json:"format"`
	Followers   int         `json:"followers"`
	Following   bool        `json:"following"`
	OwnerV2     OwnerV2     `json:"ownerV2"`
	Content     Content     `json:"content"`
	SharingInfo SharingInfo `json:"sharingInfo"`
}

type OwnerV2 struct {
	Data User `json:"data"`
}

type User struct {
	Typename string `json:"__typename"`
	Name     string `json:"name"`
	URI      string `json:"uri"`
	Username string `json:"username"`
	Avatar   Images `json:"avatar"`
}

type Images struct {
	Sources []ImageSource `json:"sources"`
}

type ImageSource struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type SharingInfo struct {
	ShareID  string `json:"shareId"`
	ShareURL string `json:"shareUrl"`
}

type Content struct {
	Typename   string     `json:"__typename"`
	Items      []Item     `json:"items"`
	PagingInfo PagingInfo `json:"pagingInfo"`
	TotalCount int        `json:"totalCount"`
}

type PagingInfo struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type Item struct {
	UID     string  `json:"uid"`
	AddedAt AddedAt `json:"addedAt"`
	AddedBy AddedBy `json:"addedBy"`
	ItemV2  ItemV2  `json:"itemV2"`
	ItemV3  ItemV3  `json:"itemV3"`
}

type AddedAt struct {
	ISOString string `json:"isoString"`
}

type AddedBy struct {
	Data User `json:"data"`
}

type ItemV2 struct {
	Typename string `json:"__typename"`
	Data     Track  `json:"data"`
}

type Track struct {
	Typename      string   `json:"__typename"`
	URI           string   `json:"uri"`
	Name          string   `json:"name"`
	DiscNumber    int      `json:"discNumber"`
	TrackNumber   int      `json:"trackNumber"`
	Playcount     string   `json:"playcount"`
	MediaType     string   `json:"mediaType"`
	AlbumOfTrack  Album    `json:"albumOfTrack"`
	Artists       Artists  `json:"artists"`
	ContentRating Rating   `json:"contentRating"`
	Playability   Playable `json:"playability"`
	TrackDuration Duration `json:"trackDuration"`
}

type Album struct {
	URI      string  `json:"uri"`
	Name     string  `json:"name"`
	Artists  Artists `json:"artists"`
	CoverArt Images  `json:"coverArt"`
}

type Artists struct {
	Items []ArtistRef `json:"items"`
}

type ArtistRef struct {
	URI     string  `json:"uri"`
	Profile Profile `json:"profile"`
}

type Profile struct {
	Name string `json:"name"`
}

type Rating struct {
	Label string `json:"label"`
}

type Playable struct {
	Playable bool   `json:"playable"`
	Reason   string `json:"reason"`
}

type Duration struct {
	TotalMilliseconds int `json:"totalMilliseconds"`
}

type ItemV3 struct {
	Data Entity `json:"data"`
}

type Entity struct {
	ConsumptionExperienceTrait ConsumptionExperienceTrait `json:"consumptionExperienceTrait"`
	IdentityTrait              IdentityTrait              `json:"identityTrait"`
	Playability                Playability                `json:"playability"`
	URI                        string                     `json:"uri"`
	VisualIdentityTrait        VisualIdentityTrait        `json:"visualIdentityTrait"`
}

type ConsumptionExperienceTrait struct {
	ContentRatings []any      `json:"contentRatings"`
	Duration       DurationV3 `json:"duration"`
	Formats        []string   `json:"formats"`
}

type DurationV3 struct {
	NanoSeconds int `json:"nanoSeconds"`
	Seconds     int `json:"seconds"`
}

type IdentityTrait struct {
	ContentHierarchyParent *ParentEntity `json:"contentHierarchyParent,omitempty"`
	Contributors           Contributors  `json:"contributors"`
	Description            string        `json:"description"`
	Name                   string        `json:"name"`
	Type                   string        `json:"type"`
}

type ParentEntity struct {
	IdentityTrait           ParentIdentityTrait     `json:"identityTrait"`
	PublishingMetadataTrait PublishingMetadataTrait `json:"publishingMetadataTrait"`
	URI                     string                  `json:"uri"`
}

type ParentIdentityTrait struct {
	Name string `json:"name"`
}

type PublishingMetadataTrait struct {
	FirstPublishedAt DatePrecision `json:"firstPublishedAt"`
}

type DatePrecision struct {
	IsoString string `json:"isoString"`
	Precision string `json:"precision"`
}

type Contributors struct {
	Items      []Contributor `json:"items"`
	TotalCount int           `json:"totalCount"`
}

type Contributor struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type Playability struct {
	Playable bool   `json:"playable"`
	Reason   string `json:"reason"`
}

type VisualIdentityTrait struct {
	SixteenByNineCoverImage any        `json:"sixteenByNineCoverImage"`
	SquareCoverImage        CoverImage `json:"squareCoverImage"`
}

type CoverImage struct {
	ExtractedColorSet ColorSet  `json:"extractedColorSet"`
	Image             ImageWrap `json:"image"`
}

type ImageWrap struct {
	Data ImageData `json:"data"`
}

type ImageData struct {
	Sources []ImageSourceV3 `json:"sources"`
}

type ImageSourceV3 struct {
	ImageFormat string `json:"imageFormat"`
	MaxHeight   int    `json:"maxHeight"`
	MaxWidth    int    `json:"maxWidth"`
	URL         string `json:"url"`
}

type ColorSet struct {
	EncoreBaseSetTextColor RGB         `json:"encoreBaseSetTextColor"`
	HighContrast           ContrastSet `json:"highContrast"`
	HigherContrast         ContrastSet `json:"higherContrast"`
	MinContrast            ContrastSet `json:"minContrast"`
}

type ContrastSet struct {
	BackgroundBase       RGB `json:"backgroundBase"`
	BackgroundTintedBase RGB `json:"backgroundTintedBase"`
	TextBase             RGB `json:"textBase"`
	TextBrightAccent     RGB `json:"textBrightAccent"`
	TextSubdued          RGB `json:"textSubdued"`
}

type RGB struct {
	Alpha int `json:"alpha"`
	Blue  int `json:"blue"`
	Green int `json:"green"`
	Red   int `json:"red"`
}
