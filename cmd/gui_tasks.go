package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/schollz/progressbar/v3"
)

// downloadTaskInput carries the values needed by the App Store download API
// together with the ipatool progress bar the GUI samples for its job status.
type downloadTaskInput struct {
	Context           context.Context
	Account           appstore.Account
	App               appstore.App
	OutputPath        string
	ExternalVersionID string
	Platform          appstore.Platform
	// Progress is the *progressbar.ProgressBar ipatool feeds the downloaded
	// bytes into. The GUI builds it with a discarding writer and reads its
	// state for percentage, size and speed.
	Progress       *progressbar.ProgressBar
	AcquireLicense bool
	OnPurchase     func()
	OnDownload     func()
}

// refreshAccount retries authentication with the stored password when Apple
// rejects an expired password token. If no password is stored, the user must
// sign in again to refresh the account.
func refreshAccount(account appstore.Account) (appstore.Account, error) {
	if account.Password == "" {
		return account, errors.New("password token is expired and no password is stored; log in again to refresh the session")
	}

	output, err := dependencies.AppStore.Login(appstore.LoginInput{
		Email:    account.Email,
		Password: account.Password,
	})
	if err != nil {
		return account, fmt.Errorf("failed to refresh Apple ID session: %w", err)
	}

	return output.Account, nil
}

// purchaseWithRetry attempts to acquire a license, refreshing the account once
// if Apple's cached password token has expired. The returned bool is true when
// Apple reported that the license already exists.
func purchaseWithRetry(account appstore.Account, app appstore.App) (appstore.Account, bool, error) {
	for attempt := 0; attempt < 2; attempt++ {
		err := dependencies.AppStore.Purchase(appstore.PurchaseInput{
			Account: account,
			App:     app,
		})
		if errors.Is(err, appstore.ErrLicenseAlreadyExists) {
			return account, true, nil
		}
		if !errors.Is(err, appstore.ErrPasswordTokenExpired) || attempt != 0 {
			return account, false, err
		}

		refreshed, refreshErr := refreshAccount(account)
		if refreshErr != nil {
			return account, false, refreshErr
		}
		account = refreshed
	}

	return account, false, errors.New("purchase failed after refreshing the account session")
}

// downloadWithRetry downloads a package, optionally acquiring its license when
// needed and refreshing an expired password token at most once.
func downloadWithRetry(input downloadTaskInput) (appstore.Account, appstore.DownloadOutput, bool, error) {
	account := input.Account
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	refreshed := false
	licenseAttempted := false
	licenseAcquired := false
	progress := input.Progress

	for attempt := 0; attempt < 4; attempt++ {
		output, err := dependencies.AppStore.Download(appstore.DownloadInput{
			Context:           ctx,
			Account:           account,
			App:               input.App,
			OutputPath:        input.OutputPath,
			ExternalVersionID: input.ExternalVersionID,
			Platform:          input.Platform,
			Progress:          progress,
		})
		if err == nil {
			return account, output, licenseAcquired, nil
		}

		if progress != nil && isProgressBarSetupError(err) {
			// The server did not report a content length, so ipatool cannot
			// size the progress bar and aborts before writing any package data.
			// Downloading without live progress beats not downloading at all.
			progress = nil
			continue
		}

		if errors.Is(err, appstore.ErrPasswordTokenExpired) && !refreshed {
			updated, refreshErr := refreshAccount(account)
			if refreshErr != nil {
				return account, appstore.DownloadOutput{}, licenseAcquired, refreshErr
			}
			account = updated
			refreshed = true
			continue
		}

		if errors.Is(err, appstore.ErrLicenseRequired) && input.AcquireLicense && !licenseAttempted {
			licenseAttempted = true
			if input.OnPurchase != nil {
				input.OnPurchase()
			}
			updated, alreadyOwned, purchaseErr := purchaseWithRetry(account, input.App)
			if purchaseErr != nil {
				return updated, appstore.DownloadOutput{}, licenseAcquired, purchaseErr
			}
			account = updated
			licenseAcquired = !alreadyOwned
			if input.OnDownload != nil {
				input.OnDownload()
			}
			continue
		}

		return account, appstore.DownloadOutput{}, licenseAcquired, err
	}

	return account, appstore.DownloadOutput{}, licenseAcquired, errors.New("download failed after retrying authentication and license acquisition")
}

// listVersionsWithRetry performs the license probe behind the GUI's batch
// check. ipatool's ListVersions sends the same App Store direct-download
// request as a real download and stops before any package is transferred, so
// it answers both questions the batch needs: which builds the account may
// download (external version identifiers) and whether a license is missing
// (ErrLicenseRequired). The session is refreshed once when Apple reports an
// expired password token.
func listVersionsWithRetry(account appstore.Account, app appstore.App, platform appstore.Platform) (appstore.Account, appstore.ListVersionsOutput, error) {
	for attempt := 0; attempt < 2; attempt++ {
		output, err := dependencies.AppStore.ListVersions(appstore.ListVersionsInput{
			Context:  context.Background(),
			Account:  account,
			App:      app,
			Platform: platform,
		})
		if !errors.Is(err, appstore.ErrPasswordTokenExpired) || attempt != 0 {
			return account, output, err
		}

		updated, refreshErr := refreshAccount(account)
		if refreshErr != nil {
			return account, appstore.ListVersionsOutput{}, refreshErr
		}
		account = updated
	}

	return account, appstore.ListVersionsOutput{}, errors.New("the license check failed after refreshing the account session")
}

// isProgressBarSetupError reports whether a download failed because ipatool
// could not hand the package size to the progress bar it was given. That
// happens when the App Store response carries no content length, which the bar
// cannot represent; the download itself is fine and is retried without one.
func isProgressBarSetupError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "can not set bar progress")
}

// catalogVersion resolves the version string shown next to a batch result.
// ListVersions reports build identifiers, not version numbers, so the value
// comes from the catalog lookup; delisted apps simply report no version.
func catalogVersion(account appstore.Account, app appstore.App, platform appstore.Platform) string {
	output, err := dependencies.AppStore.Lookup(appstore.LookupInput{
		Account:  account,
		AppID:    app.ID,
		Platform: platform,
	})
	if err != nil {
		return ""
	}

	return output.App.Version
}
