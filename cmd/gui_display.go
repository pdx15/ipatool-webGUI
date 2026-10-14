package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

// errDisplayLookupFailed reports a non-OK response from the catalog lookup.
var errDisplayLookupFailed = errors.New("display lookup failed")

// appCard is an App Store search result plus the catalog fields the GUI shows
// on an app card. ipatool's public API exposes only the values the CLI needs
// (id, bundle ID, name, version, price, platforms), so the display-only fields
// are fetched from the same public iTunes catalog endpoint the search itself
// uses and merged in by track ID. Embedding appstore.App keeps ipatool's own
// JSON field names untouched.
type appCard struct {
	appstore.App
	ArtworkURL60      string  `json:"artworkUrl60,omitempty"`
	ArtworkURL100     string  `json:"artworkUrl100,omitempty"`
	ArtworkURL512     string  `json:"artworkUrl512,omitempty"`
	ArtistName        string  `json:"artistName,omitempty"`
	SellerName        string  `json:"sellerName,omitempty"`
	FormattedPrice    string  `json:"formattedPrice,omitempty"`
	FileSizeBytes     int64   `json:"fileSizeBytes,omitempty"`
	AverageUserRating float64 `json:"averageUserRating,omitempty"`
	UserRatingCount   int64   `json:"userRatingCount,omitempty"`
}

// displayLookupURL is the public catalog lookup used for display metadata. It is
// a variable so tests can point it at a local server.
var displayLookupURL = "https://itunes.apple.com/lookup"

// displayLookupClient bounds the extra request so a slow catalog never holds up
// the search results themselves.
var displayLookupClient = &http.Client{Timeout: 6 * time.Second}

// displayMetadata is the subset of a catalog lookup result the GUI renders.
type displayMetadata struct {
	TrackID           int64   `json:"trackId"`
	ArtworkURL60      string  `json:"artworkUrl60"`
	ArtworkURL100     string  `json:"artworkUrl100"`
	ArtworkURL512     string  `json:"artworkUrl512"`
	ArtistName        string  `json:"artistName"`
	SellerName        string  `json:"sellerName"`
	FormattedPrice    string  `json:"formattedPrice"`
	FileSizeBytes     string  `json:"fileSizeBytes"` // Apple quotes this number
	AverageUserRating float64 `json:"averageUserRating"`
	UserRatingCount   int64   `json:"userRatingCount"`
}

type displayLookupResponse struct {
	Results []displayMetadata `json:"results"`
}

// appCards attaches display metadata to search results. Enrichment is
// best-effort by design: when the lookup fails, or an app is not part of the
// default storefront catalog (delisted apps), the card keeps the icon and
// developer placeholders the frontend already renders.
func appCards(apps []appstore.App) []appCard {
	return appCardsForPlatform(apps, appstore.PlatformIPhone)
}

func appCardsForPlatform(apps []appstore.App, platform appstore.Platform) []appCard {
	cards := make([]appCard, len(apps))

	for i, app := range apps {
		cards[i] = appCard{App: app}
	}

	if len(apps) == 0 {
		return cards
	}

	metadata := fetchDisplayMetadata(apps, platform)

	for i := range cards {
		entry, ok := metadata[cards[i].ID]
		if !ok {
			continue
		}

		cards[i].ArtworkURL60 = entry.ArtworkURL60
		cards[i].ArtworkURL100 = entry.ArtworkURL100
		cards[i].ArtworkURL512 = entry.ArtworkURL512
		cards[i].ArtistName = entry.ArtistName
		cards[i].SellerName = entry.SellerName
		cards[i].FormattedPrice = entry.FormattedPrice
		cards[i].AverageUserRating = entry.AverageUserRating
		cards[i].UserRatingCount = entry.UserRatingCount

		if fileSize, err := strconv.ParseInt(entry.FileSizeBytes, 10, 64); err == nil {
			cards[i].FileSizeBytes = fileSize
		}
	}

	return cards
}

func displayLookupEntities(platform appstore.Platform) []string {
	switch platform {
	case appstore.PlatformIPad:
		return []string{"iPadSoftware", "software"}
	case appstore.PlatformAppleTV:
		return []string{"tvSoftware", "software"}
	case appstore.PlatformWatchOS:
		return []string{"software", "watchSoftware"}
	case appstore.PlatformVisionOS:
		return []string{"xrosSoftware", "software", "iPadSoftware"}
	case appstore.PlatformMacOS:
		return []string{"macSoftware"}
	default:
		return []string{"software"}
	}
}

// fetchDisplayMetadata looks the given apps up in the public catalog, in
// batches, and returns them keyed by track ID. Failures return whatever was
// collected so far: app cards without an icon still work.
func fetchDisplayMetadata(apps []appstore.App, platform appstore.Platform) map[int64]displayMetadata {
	ids := make([]int64, 0, len(apps))

	for _, app := range apps {
		if app.ID > 0 {
			ids = append(ids, app.ID)
		}
	}

	metadata := make(map[int64]displayMetadata, len(ids))
	entities := displayLookupEntities(platform)

	const batchSize = 100

	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}

		remaining := ids[start:end]
		for _, entity := range entities {
			if len(remaining) == 0 {
				break
			}

			strIDs := make([]string, len(remaining))
			for i, id := range remaining {
				strIDs[i] = strconv.FormatInt(id, 10)
			}

			batch, err := lookupDisplayMetadata(strIDs, entity)
			if err != nil {
				break
			}

			for _, entry := range batch {
				if entry.TrackID != 0 {
					metadata[entry.TrackID] = entry
				}
			}

			missing := make([]int64, 0, len(remaining))
			for _, id := range remaining {
				if _, ok := metadata[id]; !ok {
					missing = append(missing, id)
				}
			}
			remaining = missing
		}
	}

	return metadata
}

// lookupDisplayMetadata performs one catalog lookup for up to a batch of app IDs.
func lookupDisplayMetadata(ids []string, entity string) ([]displayMetadata, error) {
	query := url.Values{}
	query.Set("id", strings.Join(ids, ","))
	query.Set("entity", entity)

	res, err := displayLookupClient.Get(displayLookupURL + "?" + query.Encode())
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errDisplayLookupFailed
	}

	var payload displayLookupResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload.Results, nil
}
