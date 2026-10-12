package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

// downloadTaskInput carries the GUI-specific progress hooks alongside the
// values needed by the App Store download API.
type downloadTaskInput struct {
	Context           context.Context
	Account           appstore.Account
	App               appstore.App
	OutputPath        string
	ExternalVersionID string
	Platform          appstore.Platform
	ProgressWriter    io.Writer
	OnTotalBytes      func(int64)
	AcquireLicense    bool
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

	for attempt := 0; attempt < 3; attempt++ {
		output, err := dependencies.AppStore.Download(appstore.DownloadInput{
			Context:           ctx,
			Account:           account,
			App:               input.App,
			OutputPath:        input.OutputPath,
			ExternalVersionID: input.ExternalVersionID,
			Platform:          input.Platform,
			ProgressWriter:    input.ProgressWriter,
			OnTotalBytes:      input.OnTotalBytes,
		})
		if err == nil {
			return account, output, licenseAcquired, nil
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
			updated, alreadyOwned, purchaseErr := purchaseWithRetry(account, input.App)
			if purchaseErr != nil {
				return updated, appstore.DownloadOutput{}, licenseAcquired, purchaseErr
			}
			account = updated
			licenseAcquired = !alreadyOwned
			continue
		}

		return account, appstore.DownloadOutput{}, licenseAcquired, err
	}

	return account, appstore.DownloadOutput{}, licenseAcquired, errors.New("download failed after retrying authentication and license acquisition")
}

// checkDownloadWithRetry performs the same license probe used by the App Store
// direct-download path without transferring the package. CheckDownload is an
// optional extension implemented by the concrete App Store backend used by the
// GUI; keeping it optional avoids widening ipatool's public AppStore interface.
func checkDownloadWithRetry(account appstore.Account, app appstore.App, platform appstore.Platform) (appstore.Account, appstore.CheckDownloadOutput, error) {
	checker, ok := dependencies.AppStore.(interface {
		CheckDownload(appstore.CheckDownloadInput) (appstore.CheckDownloadOutput, error)
	})
	if !ok {
		return account, appstore.CheckDownloadOutput{}, errors.New("the App Store backend does not support download checks")
	}

	for attempt := 0; attempt < 2; attempt++ {
		output, err := checker.CheckDownload(appstore.CheckDownloadInput{
			Account:  account,
			App:      app,
			Platform: platform,
		})
		if !errors.Is(err, appstore.ErrPasswordTokenExpired) || attempt != 0 {
			return account, output, err
		}

		updated, refreshErr := refreshAccount(account)
		if refreshErr != nil {
			return account, appstore.CheckDownloadOutput{}, refreshErr
		}
		account = updated
	}

	return account, appstore.CheckDownloadOutput{}, errors.New("download check failed after refreshing the account session")
}
